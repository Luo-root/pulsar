// Command pulsar 是 pulsar 的命令行入口。
//
// v0.1 起手式只有 run：装配本地 runtime 并跑一个回合。serve（本地 HTTP API）
// 与其余子命令随 v0.1 的后续步骤补上。
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/Luo-root/pulse/host"
	"github.com/Luo-root/pulse/llm"

	"github.com/Luo-root/pulsar/internal/app"
	"github.com/Luo-root/pulsar/internal/config"
)

const usage = `pulsar —— 本地优先的 AI 总管

用法：
  pulsar run [-c 配置文件] [-C 工作区] [-session 会话ID] [-model 模型名] [-y] <prompt>

子命令：
  run    执行一个回合；-session 为空时新建会话并打印其 ID

说明：
  配置文件默认 ./pulsar.yaml，其中的相对目录相对配置文件所在目录解析；
  凭据只以环境变量名出现在配置里（api_key_env），密钥不进文件、不进日志。

  工作区是**会话属性**（不是 runtime 属性）：新建会话时 -C 显式指定、缺省用当前
  目录，它被写进会话头随会话持久化；续跑时不传 -C 就以会话记录的为准（从别的
  目录接着聊也可以），显式传了则必须与会话一致。工具的相对路径与读写边界都以
  生效的工作区为准。

  工具调用默认逐次在终端询问；-y 跳过审批门（所有调用直接执行）。
  Ctrl+C 取消当前回合（日志闭合为 interrupted，下次可续跑），不会留下半开的会话。
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "pulsar: "+err.Error())
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return errors.New("缺少子命令")
	}
	switch args[0] {
	case "run":
		return runTurn(args[1:])
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	default:
		fmt.Fprint(os.Stderr, usage)
		return fmt.Errorf("未知子命令 %q", args[0])
	}
}

func runTurn(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	cfgPath := fs.String("c", config.DefaultPath, "配置文件路径")
	workspace := fs.String("C", "", "工作区目录（空 = 当前目录）")
	sessionID := fs.String("session", "", "会话 ID（空 = 新建）")
	modelName := fs.String("model", "", "模型名（空 = 配置里的第一条）")
	assumeYes := fs.Bool("y", false, "跳过审批门（所有工具调用直接执行）")
	if err := fs.Parse(args); err != nil {
		return err
	}
	prompt := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if prompt == "" {
		return errors.New("run: 缺少 prompt")
	}

	// 工作区的兜底只对**新建会话**生效（缺省 = 当前目录，与所有 coding agent 的
	// 习惯一致）。续跑时没给就是没给——以会话头记录的为准，这样从别的目录接着
	// 聊不会被拒；显式给了 -C 就必须与会话一致（不一致由 app 层拒绝）。
	ws := *workspace
	if ws == "" && *sessionID == "" {
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("run: 取当前目录失败：%w", err)
		}
		ws = wd
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}
	a, err := app.New(app.Options{Config: cfg, Gate: gateFor(*assumeYes)})
	if err != nil {
		return err
	}
	defer a.Close()

	// Ctrl+C / SIGTERM 走 ctx 取消，而不是让进程被杀：回合日志在 turn_end 处
	// 闭合为 interrupted，落盘停在真实现场，下次 Open 冷恢复接着跑。
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	res, err := a.RunTurn(ctx, app.Turn{
		Workspace: ws,
		SessionID: *sessionID,
		Model:     *modelName,
		Prompt:    prompt,
	})
	if res != nil {
		if res.Text != "" {
			fmt.Println(res.Text)
		}
		fmt.Fprintf(os.Stderr, "\n---\nsession %s\nworkspace %s\nsteps %d  stopped_by %s  tokens in=%d out=%d\n",
			res.SessionID, res.Workspace, res.Steps, res.StoppedBy,
			res.Usage.InputTokens, res.Usage.OutputTokens)
	}
	if err != nil && res != nil {
		// 部分产出是有效产出（canceled / 中途基础设施失败）：先消费再报错。
		fmt.Fprintf(os.Stderr, "pulsar: 回合未正常结束（stopped_by %s）：%v\n", res.StoppedBy, err)
	}
	return err
}

// gateFor 返回工具审批门：-y 时返回 nil（不设防，显式选择）；否则逐次在终端
// 询问，读不到输入即视为拒绝（fail closed）。
//
// 门持有同一个 stdin reader（闭包捕获）。当前 CLI 的 prompt 走命令行参数、不
// 读 stdin，所以不存在「reader 预读吃掉 prompt」的问题；将来若加交互式 REPL，
// 需要把 stdin 收口到一处统一管理。
func gateFor(assumeYes bool) host.ToolGate {
	if assumeYes {
		return nil
	}
	in := bufio.NewReader(os.Stdin)
	return func(call llm.ToolCall) (approved bool, reason string) {
		fmt.Fprintf(os.Stderr, "\n[审批] 工具 %s\n  参数 %s\n  执行？[y/N] ", call.Name, string(call.Arguments))
		line, err := in.ReadString('\n')
		if err != nil && line == "" {
			fmt.Fprintln(os.Stderr)
			return false, "审批门：无输入（视为拒绝）"
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes":
			return true, ""
		default:
			return false, "用户拒绝执行"
		}
	}
}
