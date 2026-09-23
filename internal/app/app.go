// Package app 是装配层：把 kernel + host 与各能力（模型、工具、会话栈、观测）
// 接成一个可运行的 runtime。
//
// 纪律：所有外部副作用（模型网络、工具执行、落盘）在这里显式 opt-in 一次；
// 上层（CLI / 本地 API / 前端）一律只经 App 的面调用——API 优先。
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
}

// New 装配 runtime。失败时不留下半开的资源（已建的文件与内核一并回收）。
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

	// 工具面：builtins 的路径边界直接来自配置（Root 必填由 config 保证）。
	tools := host.ToolSource(func(c *kernel.Context, reg *toolset.Registry) error {
		_, err := builtins.Register(c, reg, builtins.Options{
			Root:       cfg.Workspace.Root,
			WriteRoots: cfg.Workspace.WriteRoots,
			ForbidRead: cfg.Workspace.ForbidRead,
			Enabled:    cfg.Tools.Enabled,
		})
		return err
	})

	providers := make([]host.Provider, 0, 2+len(opt.Providers))
	providers = append(providers, openai.Register, anthropic.Register)
	providers = append(providers, opt.Providers...)

	h, err := host.New(host.Options{
		Kernel:    a.kernel,
		Providers: providers,
		Models:    decls,
		Tools:     []host.ToolSource{tools},
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
// 逆序还原全部已装载插件的效应，最后关闭观测日志。幂等。
func (a *App) Close() error {
	if a == nil {
		return nil
	}
	a.sessMu.Lock()
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

// Host 暴露宿主装配（进阶用法：自定义 agent 构造、工具注册面）。
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
	// Text 是最终 assistant 消息的文本。
	Text string
	// Steps 是实际执行的推理-行动步数。
	Steps int
	// StoppedBy 解释终止原因（completed / max_steps / canceled / error）。
	StoppedBy loop.StopReason
	// Usage 是本回合全部模型调用的用量累计。
	Usage llm.TokenUsage
}

// RunTurn 执行一个回合：装配 agent（模型按名解析、工具取宿主装配、会话按
// SessionID 新建或打开）→ 跑一轮 ReAct → 回传结论与用量。
//
// 返回的 error 仅在基础设施失败（模型调用失败、ctx 取消、落盘失败）时非 nil；
// 此时 TurnResult 仍可能携带已发生的部分（StoppedBy=canceled/error），调用方
// 应当先消费结果再处理错误。
func (a *App) RunTurn(ctx context.Context, t Turn) (*TurnResult, error) {
	if t.Prompt == "" {
		return nil, errors.New("app: prompt is required")
	}
	decl, err := a.cfg.Model(t.Model)
	if err != nil {
		return nil, err
	}
	agent, err := a.host.DefaultAgent(ctx, host.DefaultAgentOptions{
		Name:      a.cfg.Agent.Name,
		Model:     decl.Name,
		System:    a.cfg.Agent.System,
		SessionID: t.SessionID,
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
		SessionID: t.SessionID,
		Steps:     res.Steps,
		StoppedBy: res.StoppedBy,
		Usage:     res.Usage,
	}
	if sess := agent.Session(); sess != nil {
		out.SessionID = sess.Header().SessionID
		a.track(sess)
	}
	if res.Final != nil {
		out.Text = res.Final.Text()
	}
	return out, err
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
