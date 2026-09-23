package app_test

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/Luo-root/pulse/host"
	"github.com/Luo-root/pulse/kernel"
	"github.com/Luo-root/pulse/llm"
	"github.com/Luo-root/pulse/loop"
	"github.com/Luo-root/pulse/memory/session"
	"github.com/Luo-root/pulse/observability"

	"github.com/Luo-root/pulsar/internal/app"
)

// cancelOnceModel 第一次调用阻塞到 ctx 取消（以 EventError 收尾，符合 llm 的流
// 契约），此后正常回一条文本——用来确定性地验证「取消 → 回合闭合 → 同一会话续跑」，
// 而不是靠 sleep 抢时序。
type cancelOnceModel struct {
	entered chan struct{}
	mu      sync.Mutex
	calls   int
}

func (m *cancelOnceModel) Generate(ctx context.Context, req *llm.GenerateRequest) (*llm.Response, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (m *cancelOnceModel) Stream(ctx context.Context, req *llm.GenerateRequest) (<-chan llm.StreamEvent, error) {
	m.mu.Lock()
	m.calls++
	n := m.calls
	m.mu.Unlock()

	ch := make(chan llm.StreamEvent, 1)
	go func() {
		defer close(ch)
		if n == 1 {
			close(m.entered)
			<-ctx.Done()
			ch <- llm.StreamEvent{Kind: llm.EventError, Err: ctx.Err()}
			return
		}
		ch <- llm.StreamEvent{Kind: llm.EventDone, Response: llm.Resp("续跑成功。")}
	}()
	return ch, nil
}

// TestRunTurn_CancelClosesTurnAndResumes 盯住取消语义（Ctrl+C / 上游取消同一条路径）：
// 取消后回合必须**闭合**（turn.ended 记 interrupted），且同一会话下一回合能正常续跑。
func TestRunTurn_CancelClosesTurnAndResumes(t *testing.T) {
	cfg := testConfig(t)
	model := &cancelOnceModel{entered: make(chan struct{})}
	a, err := app.New(app.Options{
		Config: cfg,
		Sink:   observability.NewLineSink(io.Discard),
		Providers: []host.Provider{
			func(c *kernel.Context, reg *llm.Registry) error {
				_, err := reg.RegisterProvider(c, llm.ProviderScripted, func(llm.Config) (llm.ChatModel, error) {
					return model, nil
				})
				return err
			},
		},
	})
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	defer a.Close()

	ctx, cancel := context.WithCancel(context.Background())
	type outcome struct {
		res *app.TurnResult
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		res, err := a.RunTurn(ctx, app.Turn{Prompt: "这一轮会被取消"})
		done <- outcome{res: res, err: err}
	}()

	select {
	case <-model.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("模型没被调用到")
	}
	cancel()

	var first outcome
	select {
	case first = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("取消后回合没在 5s 内返回（ctx 没被透传？）")
	}
	if first.err == nil {
		t.Fatal("取消应当返回错误")
	}
	if first.res == nil {
		t.Fatal("取消应当返回部分结果（StoppedBy 说明原因），而不是 nil")
	}
	if first.res.StoppedBy != loop.StopError {
		t.Fatalf("StoppedBy = %s, want %s", first.res.StoppedBy, loop.StopError)
	}
	if first.res.SessionID == "" {
		t.Fatal("取消前会话已建立，SessionID 不应为空")
	}

	// 日志必须闭合：最末的 turn.ended 记 interrupted。否则下次 Open 会看到半开的
	// 回合，续跑时 surface 会拒投影。
	readCtx := context.Background()
	sess, err := a.Sessions().Open(readCtx, first.res.SessionID)
	if err != nil {
		t.Fatalf("open session: %v", err)
	}
	envs, err := sess.Events(readCtx, 0)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	var lastTurnEnd *session.LifecyclePayload
	for _, e := range envs {
		if e.Type != session.EventTurnEnded {
			continue
		}
		var p session.LifecyclePayload
		if err := json.Unmarshal(e.Data, &p); err != nil {
			t.Fatalf("unmarshal turn.ended: %v", err)
		}
		lastTurnEnd = &p
	}
	if lastTurnEnd == nil {
		t.Fatal("日志里没有 turn.ended——回合没闭合")
	}
	if lastTurnEnd.Reason != session.ReasonInterrupted {
		t.Fatalf("turn.ended reason = %q, want %q", lastTurnEnd.Reason, session.ReasonInterrupted)
	}

	// 同一会话续跑：取消过的会话应当能接着用。
	second, err := a.RunTurn(context.Background(), app.Turn{
		SessionID: first.res.SessionID,
		Prompt:    "接着来",
	})
	if err != nil {
		t.Fatalf("取消后同一会话续跑失败：%v", err)
	}
	if second.Text != "续跑成功。" {
		t.Fatalf("续跑 text = %q", second.Text)
	}
}
