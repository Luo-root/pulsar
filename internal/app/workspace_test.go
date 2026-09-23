package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Luo-root/pulse/llm"

	"github.com/Luo-root/pulsar/internal/app"
)

// 这一组盯住「工作区是会话属性」的三条边界 + 一条端到端的隔离验证：
//  1. 新建会话不给工作区 → 报错（不猜默认）；
//  2. 续跑不带工作区 → 用会话头记录的那个；
//  3. 续跑带了不一致的工作区 → 拒绝（不把会话接到另一个项目上）；
//  4. 两个会话挂不同工作区 → 工具面各看各的目录，互不串。
func TestRunTurn_WorkspaceIsSessionScoped(t *testing.T) {
	cfg := testConfig(t)
	wsA, wsB := t.TempDir(), t.TempDir()
	writeFile(t, wsA, "only-in-a.txt")
	writeFile(t, wsB, "only-in-b.txt")

	lsCall := func(id string) *llm.Response {
		return llm.RespToolCalls(llm.ToolCall{
			ID:        id,
			Name:      "ls",
			Arguments: json.RawMessage(`{"path":"."}`),
		})
	}
	// 每个回合两步：模型发 ls → 工具结果回来 → 模型收尾。
	model := llm.NewScripted(
		lsCall("c1"), llm.Resp("A 看完"),
		lsCall("c2"), llm.Resp("B 看完"),
		llm.Resp("续跑完成"),
	)
	a := newApp(t, cfg, map[string]llm.ChatModel{"script-1": model})
	defer a.Close()
	ctx := context.Background()

	// 1) 新建会话必须给工作区。
	if _, err := a.RunTurn(ctx, app.Turn{Prompt: "没有工作区"}); !errors.Is(err, app.ErrWorkspaceRequired) {
		t.Fatalf("新建会话不给工作区：err = %v, want ErrWorkspaceRequired", err)
	}

	// 2) 两个会话各自的工作区。
	first, err := a.RunTurn(ctx, app.Turn{Workspace: wsA, Prompt: "列目录"})
	if err != nil {
		t.Fatalf("turn A: %v", err)
	}
	if want := realPath(t, wsA); first.Workspace != want {
		t.Fatalf("turn A workspace = %q, want %q", first.Workspace, want)
	}
	second, err := a.RunTurn(ctx, app.Turn{Workspace: wsB, Prompt: "列目录"})
	if err != nil {
		t.Fatalf("turn B: %v", err)
	}
	if want := realPath(t, wsB); second.Workspace != want {
		t.Fatalf("turn B workspace = %q, want %q", second.Workspace, want)
	}

	// 4) 工具面按各自工作区装配：互不串。
	textA := toolResultOf(t, a, first.SessionID)
	textB := toolResultOf(t, a, second.SessionID)
	t.Logf("A 的 ls 结果：%s", textA)
	t.Logf("B 的 ls 结果：%s", textB)
	if !strings.Contains(textA, "only-in-a.txt") || strings.Contains(textA, "only-in-b.txt") {
		t.Fatalf("会话 A 的工具面看到了不属于它的内容：%q", textA)
	}
	if !strings.Contains(textB, "only-in-b.txt") || strings.Contains(textB, "only-in-a.txt") {
		t.Fatalf("会话 B 的工具面看到了不属于它的内容：%q", textB)
	}

	// 3) 续跑不带工作区 → 用会话记录的；带不一致的 → 拒绝。
	resume, err := a.RunTurn(ctx, app.Turn{SessionID: first.SessionID, Prompt: "接着来"})
	if err != nil {
		t.Fatalf("续跑（不带工作区）：%v", err)
	}
	if resume.Workspace != first.Workspace {
		t.Fatalf("续跑 workspace = %q, want %q（会话记录的那个）", resume.Workspace, first.Workspace)
	}
	if _, err := a.RunTurn(ctx, app.Turn{
		SessionID: first.SessionID,
		Workspace: wsB,
		Prompt:    "换个项目接着聊",
	}); !errors.Is(err, app.ErrWorkspaceMismatch) {
		t.Fatalf("续跑带不一致工作区：err = %v, want ErrWorkspaceMismatch", err)
	}
}

// TestRunTurn_WorkspaceMustBeADirectory 盯住归一化：不存在的路径与文件都要在
// 开工前拒绝（而不是让工具在后面报一堆错）。
func TestRunTurn_WorkspaceMustBeADirectory(t *testing.T) {
	cfg := testConfig(t)
	a := newApp(t, cfg, map[string]llm.ChatModel{"script-1": llm.NewScripted(llm.Resp("x"))})
	defer a.Close()
	ctx := context.Background()

	missing := filepath.Join(t.TempDir(), "nope")
	if _, err := a.RunTurn(ctx, app.Turn{Workspace: missing, Prompt: "x"}); !errors.Is(err, app.ErrWorkspaceInvalid) {
		t.Fatalf("不存在的工作区：err = %v, want ErrWorkspaceInvalid", err)
	}

	file := filepath.Join(t.TempDir(), "a.txt")
	writeFile(t, filepath.Dir(file), "a.txt")
	if _, err := a.RunTurn(ctx, app.Turn{Workspace: file, Prompt: "x"}); !errors.Is(err, app.ErrWorkspaceInvalid) {
		t.Fatalf("工作区是文件：err = %v, want ErrWorkspaceInvalid", err)
	}
}

func writeFile(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
}
