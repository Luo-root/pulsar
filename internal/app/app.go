// Package app 是装配层：把 kernel + host 与各能力（模型、工具、会话栈、观测）
// 接成一个可运行的 runtime。
//
// 两条纪律：
//
//  1. 所有外部副作用（模型网络、工具执行、落盘）在这里显式 opt-in 一次；上层
//     （CLI / 本地 API / 前端）一律只经 App 的面调用——API 优先。
//  2. **工作区是会话属性，不是 runtime 属性**：建会话时由调用方给出、写进
//     `SessionHeader.Workspace`、随会话持久化；续跑以会话头记录的为准（调用方
//     给了不一样的直接拒绝，避免把「接着上次聊」接到另一个项目上）。工具面按
//     该工作区**每回合装配**——不是装配期定死一个全局根。
package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/Luo-root/pulse/host"
	"github.com/Luo-root/pulse/kernel"
	"github.com/Luo-root/pulse/llm"
	"github.com/Luo-root/pulse/llm/anthropic"
	"github.com/Luo-root/pulse/llm/openai"
	"github.com/Luo-root/pulse/loop"
	"github.com/Luo-root/pulse/memory"
	"github.com/Luo-root/pulse/memory/session"
	"github.com/Luo-root/pulse/observability"
	"github.com/Luo-root/pulse/toolset"
	"github.com/Luo-root/pulse/toolset/builtins"

	"github.com/Luo-root/pulsar/internal/config"
)

// HostID 是观测记录里的宿主标识（trace 归属）。
const HostID = "pulsar"

// ObservabilityFile 是观测落盘的默认文件名（在 cfg.LogsDir 下）。
const ObservabilityFile = "observability.log"

var (
	// ErrClosed 表示 runtime 已关闭（Close 之后不再接受回合）。
	ErrClosed = errors.New("app: runtime is closed")
	// ErrWorkspaceRequired 表示新建会话时没给工作区——不猜，直接拒绝。
	ErrWorkspaceRequired = errors.New("app: workspace is required for a new session")
	// ErrWorkspaceInvalid 表示工作区路径不可用（不存在 / 不是目录）。
	ErrWorkspaceInvalid = errors.New("app: workspace is invalid")
	// ErrWorkspaceMismatch 表示续跑时给的工作区与会话记录的不一致（拒绝把会话
	// 接到另一个项目上）。
	ErrWorkspaceMismatch = errors.New("app: workspace does not match the session")
)

// Options 是装配参数。
type Options struct {
	// Config 是单一配置面。必填。
	Config *config.Config
	// Sink 覆盖观测出口；nil = 落 cfg.LogsDir/ObservabilityFile。
	Sink observability.Sink
	// Providers 追加在默认 provider（openai / anthropic）之后注册——测试与
	// 自定义模型来源（stub / scripted）经此注入。
	Providers []host.Provider
	// Gate 是工具执行闸门（HITL 最小挂点）；nil = 不设防（所有调用直接执行）。
	// 审批面（UI / 交互式终端）经此接入，闸门本身不是审批策略。
	Gate host.ToolGate
}

// App 是装配好的 pulsar runtime。零值不可用，用 New 构造。
type App struct {
	cfg    *config.Config
	kernel *kernel.Context
	host   *host.Host
	gate   host.ToolGate
	logs   *os.File

	sessMu sync.Mutex
	// opened 是本 runtime 打开过的会话句柄。Close 时统一释放写者——JSONL 会话
	// 持有文件锁与日志句柄，官方契约要求「正常路径用完即 Close」（会话的 Close
	// 在 Session 接口之外，是文件实现特有的释放，按契约做类型断言调用）。
	opened map[string]session.Session
	// closed 标记 runtime 已回收；此后 RunTurn 显式报错，而不是在 nil 内核上
	// 炸掉（Read 语义：Close 之后本实例只读，不再发回合）。
	closed bool
}

// New 装配 runtime。失败时不留下半开的资源（已建的文件与内核一并回收）。
//
// 注意这里**不注册**内置工具：工具面依赖工作区，而工作区是会话属性——每回合按
// 该会话的工作区装配（见 buildToolSet）。
func New(opt Options) (*App, error) {
	cfg := opt.Config
	if cfg == nil {
		return nil, errors.New("app: config is required")
	}
	for _, dir := range []string{cfg.SessionsDir, cfg.LogsDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("app: mkdir %s: %w", dir, err)
		}
	}

	a := &App{cfg: cfg, kernel: kernel.New(), gate: opt.Gate}
	ok := false
	defer func() {
		if !ok {
			_ = a.Close()
		}
	}()

	sink := opt.Sink
	if sink == nil {
		f, err := os.OpenFile(filepath.Join(cfg.LogsDir, ObservabilityFile),
			os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, fmt.Errorf("app: open observability log: %w", err)
		}
		a.logs = f
		sink = observability.NewLineSink(f, observability.WithImmediate())
	}

	sessions, err := memory.NewJSONLSessionStack(cfg.SessionsDir)
	if err != nil {
		return nil, fmt.Errorf("app: session stack: %w", err)
	}

	decls, err := modelDecls(cfg)
	if err != nil {
		return nil, err
	}

	providers := make([]host.Provider, 0, 2+len(opt.Providers))
	providers = append(providers, openai.Register, anthropic.Register)
	providers = append(providers, opt.Providers...)

	h, err := host.New(host.Options{
		Kernel:    a.kernel,
		Providers: providers,
		Models:    decls,
		Session:   sessions,
		Observe:   host.ObserveConfig{HostID: HostID, Sink: sink},
	})
	if err != nil {
		return nil, err
	}
	a.host = h
	ok = true
	return a, nil
}

// Close 回收 runtime：先释放会话写者（文件锁与日志句柄），再 kernel.Dispose
// 逆序还原全部已装载插件的效应，最后关闭观测日志。幂等——第二次调用返回 nil。
func (a *App) Close() error {
	if a == nil {
		return nil
	}
	a.sessMu.Lock()
	if a.closed {
		a.sessMu.Unlock()
		return nil
	}
	a.closed = true
	opened := a.opened
	a.opened = nil
	a.sessMu.Unlock()

	var errs []error
	for id, s := range opened {
		// 会话的 Close 不在 Session 接口里——文件实现特有的资源释放，
		// memory/session 的文档明确要求调用方做类型断言调用它。
		if c, ok := s.(interface{ Close() error }); ok {
			if err := c.Close(); err != nil {
				errs = append(errs, fmt.Errorf("app: close session %s: %w", id, err))
			}
		}
	}
	if a.kernel != nil {
		a.kernel.Dispose()
		a.kernel = nil
	}
	if a.logs != nil {
		if err := a.logs.Close(); err != nil {
			errs = append(errs, fmt.Errorf("app: close observability log: %w", err))
		}
		a.logs = nil
	}
	return errors.Join(errs...)
}

// Config 返回生效配置（只读用途）。
func (a *App) Config() *config.Config { return a.cfg }

// Host 暴露宿主装配（进阶用法：自定义 agent 构造、模型注册面）。Close 之后返回
// 的是内核已回收的宿主，只可读、不可再发回合。
func (a *App) Host() *host.Host { return a.host }

// Sessions 暴露会话栈（列表、打开、Fork、导出导入面）。
func (a *App) Sessions() *memory.SessionStack { return a.host.SessionStack() }

// track 记下本 runtime 打开过的会话，供 Close 统一释放写者。
func (a *App) track(s session.Session) {
	a.sessMu.Lock()
	defer a.sessMu.Unlock()
	if a.opened == nil {
		a.opened = make(map[string]session.Session)
	}
	a.opened[s.Header().SessionID] = s
}

// Turn 是一个回合的请求。
type Turn struct {
	// Workspace 是本回合的工作区目录（工具面的根与边界基准）。
	//
	// 新建会话（SessionID 为空）时**必填**——不猜、不用默认；它会被写进会话头。
	// 续跑时可省略（以会话头记录的为准）；给了但与会话记录不一致则拒绝。
	Workspace string
	// SessionID 非空 = 打开既有会话续跑（冷恢复语义由会话栈的 store 决定）；
	// 空 = 新建会话。
	SessionID string
	// Model 是模型名；空 = 配置里的第一条。
	Model string
	// Prompt 是本回合的用户输入。必填。
	Prompt string
}

// TurnResult 是一个回合的产出。
type TurnResult struct {
	// SessionID 是本回合所属会话（新建时即新会话 ID）。
	SessionID string
	// Workspace 是本回合生效的工作区（绝对路径；续跑时为会话头记录的那个）。
	Workspace string
	// Text 是最终 assistant 消息的文本。
	Text string
	// Steps 是实际执行的推理-行动步数。
	Steps int
	// StoppedBy 解释终止原因（completed / max_steps / canceled / error）。
	StoppedBy loop.StopReason
	// Usage 是本回合全部模型调用的用量累计。
	Usage llm.TokenUsage
}

// RunTurn 执行一个回合：解析工作区 → 开/建会话（工作区随会话头持久化）→ 按该
// 工作区装配本回合工具面 → 跑一轮 ReAct → 回传结论与用量。
//
// 返回的 error 仅在基础设施失败（模型调用失败、ctx 取消、落盘失败）时非 nil；
// 此时 TurnResult 仍可能携带已发生的部分（StoppedBy=canceled/error）。**调用方
// 应当先消费结果、再处理错误**——部分产出是有效产出（取消发生在模型已经给出
// 结论之后时，那个结论不该被丢掉）。
func (a *App) RunTurn(ctx context.Context, t Turn) (*TurnResult, error) {
	if a.isClosed() {
		return nil, ErrClosed
	}
	if t.Prompt == "" {
		return nil, errors.New("app: prompt is required")
	}
	decl, err := a.cfg.Model(t.Model)
	if err != nil {
		return nil, err
	}

	sess, ws, err := a.openSession(ctx, t)
	if err != nil {
		return nil, err
	}
	a.track(sess)

	// 本回合的工具面：注册挂在派生作用域上，回合结束即撤销（卸载即不可见）。
	scope, err := a.kernel.Derive()
	if err != nil {
		return nil, fmt.Errorf("app: derive turn scope: %w", err)
	}
	defer scope.Dispose()
	tools, err := a.buildToolSet(scope, ws)
	if err != nil {
		return nil, err
	}

	model, err := a.host.Models().Open(decl.Name)
	if err != nil {
		return nil, fmt.Errorf("app: open model %q: %w", decl.Name, err)
	}
	agent, err := a.host.NewAgent(host.AgentOptions{
		Name:      a.cfg.Agent.Name,
		Model:     model,
		ModelName: decl.Name,
		ToolSet:   tools,
		Session:   sess,
		System:    a.cfg.Agent.System,
		ToolGate:  a.gate,
	})
	if err != nil {
		return nil, err
	}

	res, err := agent.Run(ctx, llm.UserText(t.Prompt))
	if res == nil {
		if err == nil {
			err = errors.New("app: turn produced no result")
		}
		return nil, err
	}
	out := &TurnResult{
		SessionID: sess.Header().SessionID,
		Workspace: ws,
		Steps:     res.Steps,
		StoppedBy: res.StoppedBy,
		Usage:     res.Usage,
	}
	if res.Final != nil {
		out.Text = res.Final.Text()
	}
	return out, err
}

// openSession 解析本回合的工作区并取到会话句柄：新建会话时把工作区写进会话头；
// 续跑时以会话头的记录为准，调用方给了不同的工作区直接拒绝。
func (a *App) openSession(ctx context.Context, t Turn) (session.Session, string, error) {
	stack := a.host.SessionStack()
	if stack == nil {
		return nil, "", errors.New("app: session stack is not configured")
	}
	if t.SessionID == "" {
		ws, err := resolveWorkspace(t.Workspace)
		if err != nil {
			return nil, "", err
		}
		sess, err := stack.Create(ctx, session.SessionHeader{Workspace: ws})
		if err != nil {
			return nil, "", fmt.Errorf("app: create session: %w", err)
		}
		return sess, ws, nil
	}

	sess, err := stack.Open(ctx, t.SessionID)
	if err != nil {
		return nil, "", fmt.Errorf("app: open session: %w", err)
	}
	recorded := sess.Header().Workspace
	if recorded == "" {
		// 会话头里没有工作区（例如更早版本建的会话）：必须由调用方补齐，
		// 否则无从知道工具该在哪个目录下跑。
		ws, err := resolveWorkspace(t.Workspace)
		if err != nil {
			return nil, "", err
		}
		return sess, ws, nil
	}
	if t.Workspace == "" {
		return sess, recorded, nil
	}
	ws, err := resolveWorkspace(t.Workspace)
	if err != nil {
		return nil, "", err
	}
	if ws != recorded {
		return nil, "", fmt.Errorf("%w: 会话 %s 在 %s，调用方给了 %s",
			ErrWorkspaceMismatch, t.SessionID, recorded, ws)
	}
	return sess, recorded, nil
}

// buildToolSet 按工作区装配本回合的工具面：builtins 的 Root 就是该工作区，策略
// 里的相对路径相对它解析。注册是 scope 上的效应，scope 销毁即整批撤销。
func (a *App) buildToolSet(scope *kernel.Context, ws string) (loop.ToolSet, error) {
	reg := toolset.NewRegistry()
	if _, err := builtins.Register(scope, reg, builtins.Options{
		Root:       ws,
		WriteRoots: resolvePatterns(ws, a.cfg.Workspace.WriteRoots),
		ForbidRead: resolvePatterns(ws, a.cfg.Workspace.ForbidRead),
		Enabled:    a.cfg.Tools.Enabled,
	}); err != nil {
		return nil, fmt.Errorf("app: register builtins for workspace %s: %w", ws, err)
	}
	return reg.AsToolSet(), nil
}

func (a *App) isClosed() bool {
	a.sessMu.Lock()
	defer a.sessMu.Unlock()
	return a.closed
}

// resolveWorkspace 归一化工作区：必须是存在的目录，返回解析符号链接后的绝对路径
// （绝对化让「不同写法指向同一目录」的比对可靠）。
func resolveWorkspace(p string) (string, error) {
	if p == "" {
		return "", ErrWorkspaceRequired
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("%w: %s: %v", ErrWorkspaceInvalid, p, err)
	}
	if resolved, rerr := filepath.EvalSymlinks(abs); rerr == nil {
		abs = resolved
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("%w: %s: %v", ErrWorkspaceInvalid, p, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%w: %s 不是目录", ErrWorkspaceInvalid, p)
	}
	return abs, nil
}

// resolvePatterns 把策略里的相对路径按工作区解析成绝对路径（builtins 的
// WriteRoots / ForbidRead 要绝对前缀）。
func resolvePatterns(ws string, ps []string) []string {
	if len(ps) == 0 {
		return nil
	}
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, resolve(ws, p))
	}
	return out
}

func resolve(base, p string) string {
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Clean(filepath.Join(base, p))
}

// modelDecls 把配置里的模型声明翻译成 host 的声明，顺带把凭据从环境变量取出来
// ——密钥只在内存里传递，不落配置文件。
func modelDecls(cfg *config.Config) ([]host.ModelDecl, error) {
	out := make([]host.ModelDecl, 0, len(cfg.Models))
	for _, m := range cfg.Models {
		key, err := m.APIKey()
		if err != nil {
			return nil, err
		}
		out = append(out, host.ModelDecl{
			Name: m.Name,
			Config: llm.Config{
				Provider: m.Provider,
				Model:    m.Model,
				BaseURL:  m.BaseURL,
				APIKey:   key,
				Options:  m.Options,
			},
		})
	}
	return out, nil
}
