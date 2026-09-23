package app_test

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/Luo-root/pulse/host"
	"github.com/Luo-root/pulse/llm"
	"github.com/Luo-root/pulse/observability"

	"github.com/Luo-root/pulsar/internal/app"
)

// TestRunTurn_AfterClose 盯住回收后的误用：Close 之后 RunTurn 必须返回明确的
// ErrClosed，而不是在已回收的内核上炸掉；Close 本身幂等。
func TestRunTurn_AfterClose(t *testing.T) {
	cfg := testConfig(t)
	a, err := app.New(app.Options{
		Config:    cfg,
		Sink:      observability.NewLineSink(io.Discard),
		Providers: []host.Provider{scripted(map[string]llm.ChatModel{"script-1": llm.NewScripted(llm.Resp("x"))})},
	})
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	if err := a.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := a.RunTurn(context.Background(), app.Turn{Prompt: "hi"}); !errors.Is(err, app.ErrClosed) {
		t.Fatalf("RunTurn after Close: err = %v, want ErrClosed", err)
	}
	if err := a.Close(); err != nil {
		t.Fatalf("第二次 Close 应当幂等返回 nil，得到 %v", err)
	}
}
