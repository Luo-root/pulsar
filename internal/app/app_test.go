package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Luo-root/pulse/host"
	"github.com/Luo-root/pulse/kernel"
	"github.com/Luo-root/pulse/llm"
	"github.com/Luo-root/pulse/loop"
	"github.com/Luo-root/pulse/observability"

	"github.com/Luo-root/pulsar/internal/app"
	"github.com/Luo-root/pulsar/internal/config"
)

// scripted 把预置的脚本模型登记为 provider "scripted"——测试与宿主 stub 走的
// 是同一条官方路径（llm.ProviderScripted），不打桩 host 的任何内部。
func scripted(models map[string]llm.ChatModel) host.Provider {
	return func(c *kernel.Context, reg *llm.Registry) error {
		_, err := reg.RegisterProvider(c, llm.ProviderScripted, func(cfg llm.Config) (llm.ChatModel, error) {
			m, ok := models[cfg.Model]
			if !ok {
				return nil, fmt.Errorf("test: no scripted model %q", cfg.Model)
			}
			return m, nil
		})
		return err
	}
}

// testConfig 造一份自带临时工作区与落盘目录的配置。
func testConfig(t *testing.T) *config.Config {
	t.Helper()
	dir := t.TempDir()
	return &config.Config{
		Agent:       config.Agent{Name: "test-agent", System: "你是测试 agent。"},
		Workspace:   config.Workspace{Root: dir},
		SessionsDir: filepath.Join(dir, "sessions"),
		LogsDir:     filepath.Join(dir, "logs"),
		Models: []config.Model{{
			Name:     "main",
			Provider: llm.ProviderScripted,
			Model:    "script-1",
		}},
	}
}

// TestRunTurn_PersistsAndResumes 覆盖最小闭环：新建会话 → 跑一个回合 →
// 同一会话续跑第二个回合 → 会话可按事件日志回读，观测已落盘。
func TestRunTurn_PersistsAndResumes(t *testing.T) {
	cfg := testConfig(t)
	model := llm.NewScripted(llm.Resp("第一回合的答复。"), llm.Resp("第二回合的答复。"))
	a, err := app.New(app.Options{
		Config:    cfg,
		Providers: []host.Provider{scripted(map[string]llm.ChatModel{"script-1": model})},
	})
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	defer a.Close()

	ctx := context.Background()
	first, err := a.RunTurn(ctx, app.Turn{Prompt: "你好"})
	if err != nil {
		t.Fatalf("turn 1: %v", err)
	}
	if first.Text != "第一回合的答复。" {
		t.Fatalf("turn 1 text = %q", first.Text)
	}
	if first.SessionID == "" {
		t.Fatal("turn 1: 空 session id")
	}
	if first.StoppedBy != loop.StopCompleted {
		t.Fatalf("turn 1 stopped_by = %s, want completed", first.StoppedBy)
	}

	second, err := a.RunTurn(ctx, app.Turn{SessionID: first.SessionID, Prompt: "再来一句"})
	if err != nil {
		t.Fatalf("turn 2: %v", err)
	}
	if second.SessionID != first.SessionID {
		t.Fatalf("turn 2 session = %q, want %q", second.SessionID, first.SessionID)
	}
	if second.Text != "第二回合的答复。" {
		t.Fatalf("turn 2 text = %q", second.Text)
	}

	// 会话回读：两个回合 = user/assistant × 2，顺序与内容都要对得上。
	sess, err := a.Sessions().Open(ctx, first.SessionID)
	if err != nil {
		t.Fatalf("open session: %v", err)
	}
	msgs, err := sess.Surface(ctx)
	if err != nil {
		t.Fatalf("surface: %v", err)
	}
	if len(msgs) != 4 {
		t.Fatalf("surface 有 %d 条消息，want 4", len(msgs))
	}
	if got := msgs[0].Text(); got != "你好" {
		t.Fatalf("surface[0] = %q", got)
	}
	if got := msgs[3].Text(); got != "第二回合的答复。" {
		t.Fatalf("surface[3] = %q", got)
	}

	// 观测落盘：装配与回合事件必须留下记录。
	info, err := os.Stat(filepath.Join(cfg.LogsDir, app.ObservabilityFile))
	if err != nil {
		t.Fatalf("observability log: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("observability log 是空的")
	}
}

// TestRunTurn_ToolGateRejectionReachesModel 覆盖 HITL 缺口：闸门拒绝后工具
// 不执行，模型收到的是错误结果（而不是静默跳过）。
func TestRunTurn_ToolGateRejectionReachesModel(t *testing.T) {
	cfg := testConfig(t)
	model := llm.NewScripted(
		llm.RespToolCalls(llm.ToolCall{
			ID:        "call-1",
			Name:      "ls",
			Arguments: json.RawMessage(`{"path":"."}`),
		}),
		llm.Resp("已知悉：读取被拒绝。"),
	)
	gateCalls := 0
	gate := func(call llm.ToolCall) (bool, string) {
		gateCalls++
		if call.Name != "ls" {
			t.Errorf("闸门收到 %q，want ls", call.Name)
		}
		return false, "测试拒绝：不允许读取"
	}

	a, err := app.New(app.Options{
		Config:    cfg,
		Sink:      observability.NewLineSink(io.Discard),
		Providers: []host.Provider{scripted(map[string]llm.ChatModel{"script-1": model})},
		Gate:      gate,
	})
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	defer a.Close()

	ctx := context.Background()
	res, err := a.RunTurn(ctx, app.Turn{Prompt: "列出目录"})
	if err != nil {
		t.Fatalf("turn: %v", err)
	}
	if gateCalls != 1 {
		t.Fatalf("闸门被调用 %d 次，want 1", gateCalls)
	}
	if res.Steps < 2 {
		t.Fatalf("steps = %d，want >= 2（模型调用 + 工具回合）", res.Steps)
	}
	if res.Text != "已知悉：读取被拒绝。" {
		t.Fatalf("text = %q", res.Text)
	}

	sess, err := a.Sessions().Open(ctx, res.SessionID)
	if err != nil {
		t.Fatalf("open session: %v", err)
	}
	msgs, err := sess.Surface(ctx)
	if err != nil {
		t.Fatalf("surface: %v", err)
	}
	var toolText string
	for _, m := range msgs {
		if m.Role == llm.RoleTool {
			toolText = toolResultText(m)
		}
	}
	if toolText == "" {
		t.Fatalf("surface 里没有工具结果消息：%d 条消息", len(msgs))
	}
	t.Logf("工具结果原文：%s", toolText)
	if !strings.Contains(toolText, "测试拒绝") {
		t.Fatalf("拒绝原因没进模型可见的结果：%q", toolText)
	}
}

// toolResultText 取工具结果消息里的文本——工具结果是独立的 part 类型
// （PartToolResult），Message.Text() 只拼文本块，取不到它。
func toolResultText(m *llm.Message) string {
	var out []string
	for _, p := range m.Parts {
		if p.Kind != llm.PartToolResult || p.ToolResultValue == nil {
			continue
		}
		for _, c := range p.ToolResultValue.Content {
			if c.Text != "" {
				out = append(out, c.Text)
			}
		}
	}
	return strings.Join(out, "\n")
}
