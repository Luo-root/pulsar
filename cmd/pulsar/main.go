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
	"strings"

	"github.com/Luo-root/pulse/host"
	"github.com/Luo-root/pulse/llm"

	"github.com/Luo-root/pulsar/internal/app"
	"github.com/Luo-root/pulsar/internal/config"
)

const usage = `pulsar —— 本地优先的 AI 总管

用法：
  pulsar run [-c 配置文件] [-session 会话ID] [-model 模型名] [-y] <prompt>

子命令：
  run    执行一个回合；-session 为空时新建会话并打印其 ID

说明：
  配置文件默认 ./pulsar.yaml，相对路径相对配置文件所在目录解析；
  凭据只以环境变量名出现在配置里（api_key_env），密钥不进文件、不进日志。
  工具调用默认逐次在终端询问；-y 跳过审批门（所有调用直接执行）。
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

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}
	a, err := app.New(app.Options{Config: cfg, Gate: gateFor(*assumeYes)})
	if err != nil {
		return err
	}
	defer a.Close()

	res, err := a.RunTurn(context.Background(), app.Turn{
		SessionID: *sessionID,
		Model:     *modelName,
		Prompt:    prompt,
	})
	if res != nil {
		if res.Text != "" {
			fmt.Println(res.Text)
		}
		fmt.Fprintf(os.Stderr, "\n---\nsession %s\nsteps %d  stopped_by %s  tokens in=%d out=%d\n",
			res.SessionID, res.Steps, res.StoppedBy, res.Usage.InputTokens, res.Usage.OutputTokens)
	}
	return err
}

// gateFor 返回工具审批门：-y 时返回 nil（不设防，显式选择）；否则逐次在终端
// 询问，读不到输入即视为拒绝（fail closed）。
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
