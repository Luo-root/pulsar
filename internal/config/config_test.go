package config_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Luo-root/pulsar/internal/config"
)

const minimal = `
models:
  - name: main
    provider: openai
    model: gpt-5
workspace:
  root: .
`

// writeConfig 把配置正文落进 dir/pulsar.yaml，返回文件路径。
func writeConfig(t *testing.T, dir, body string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "pulsar.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// realDir 返回目录解析符号链接后的路径——Load 以真实目录为相对基准，比对期望
// 值时也要先解析，免得把平台细节（TempDir 可能带 junction 成分）当成被测行为。
func realDir(t *testing.T, dir string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("EvalSymlinks(%s): %v", dir, err)
	}
	return r
}

func TestLoad_DefaultsAndRelativePaths(t *testing.T) {
	base := t.TempDir()
	cfg, err := config.Load(writeConfig(t, base, minimal))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Agent.Name != config.DefaultAgentName {
		t.Fatalf("agent.name = %q, want %q", cfg.Agent.Name, config.DefaultAgentName)
	}
	if want := realDir(t, base); cfg.Workspace.Root != want {
		t.Fatalf("workspace.root = %q, want %q", cfg.Workspace.Root, want)
	}
	if want := filepath.Join(realDir(t, base), config.DefaultSessionsDir); cfg.SessionsDir != want {
		t.Fatalf("sessions_dir = %q, want %q", cfg.SessionsDir, want)
	}
	if want := filepath.Join(realDir(t, base), config.DefaultLogsDir); cfg.LogsDir != want {
		t.Fatalf("logs_dir = %q, want %q", cfg.LogsDir, want)
	}
	if len(cfg.Models) != 1 || cfg.Models[0].Name != "main" {
		t.Fatalf("models = %+v", cfg.Models)
	}
}

func TestLoad_RelativeAndAbsolutePaths(t *testing.T) {
	base := t.TempDir()
	absDir := t.TempDir()
	body := fmt.Sprintf(`
models:
  - name: main
    provider: openai
    model: gpt-5
workspace:
  root: ./ws
  write_roots: ["./ws/out", %q]
  forbid_read: ["./.git"]
sessions_dir: ./state/sessions
logs_dir: ./state/logs
`, absDir)
	cfg, err := config.Load(writeConfig(t, base, body))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	real := realDir(t, base)
	if want := filepath.Join(real, "ws"); cfg.Workspace.Root != want {
		t.Fatalf("root = %q, want %q", cfg.Workspace.Root, want)
	}
	if want := filepath.Join(real, "ws", "out"); cfg.Workspace.WriteRoots[0] != want {
		t.Fatalf("write_roots[0] = %q, want %q", cfg.Workspace.WriteRoots[0], want)
	}
	if want := realDir(t, absDir); cfg.Workspace.WriteRoots[1] != want {
		t.Fatalf("write_roots[1] = %q, want 原样的绝对路径 %q", cfg.Workspace.WriteRoots[1], want)
	}
	if want := filepath.Join(real, ".git"); cfg.Workspace.ForbidRead[0] != want {
		t.Fatalf("forbid_read[0] = %q, want %q", cfg.Workspace.ForbidRead[0], want)
	}
	if want := filepath.Join(real, "state", "sessions"); cfg.SessionsDir != want {
		t.Fatalf("sessions_dir = %q, want %q", cfg.SessionsDir, want)
	}
	if want := filepath.Join(real, "state", "logs"); cfg.LogsDir != want {
		t.Fatalf("logs_dir = %q, want %q", cfg.LogsDir, want)
	}
}

func TestLoad_Errors(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"没有模型", `workspace: {root: .}`, "models is empty"},
		{"模型重名", `
models:
  - {name: a, provider: p, model: m}
  - {name: a, provider: p, model: m}
workspace: {root: .}
`, "duplicate name"},
		{"模型缺名字", `
models:
  - {provider: p, model: m}
workspace: {root: .}
`, "name is required"},
		{"模型缺 provider", `
models:
  - {name: a, model: m}
workspace: {root: .}
`, "provider is required"},
		{"模型缺 model", `
models:
  - {name: a, provider: p}
workspace: {root: .}
`, "model is required"},
		{"缺 workspace.root", `
models:
  - {name: a, provider: p, model: m}
`, "workspace.root is required"},
		{"YAML 语法坏", "models: [}", "parse"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := config.Load(writeConfig(t, t.TempDir(), c.body))
			if err == nil {
				t.Fatalf("want error containing %q, got nil", c.want)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want containing %q", err, c.want)
			}
		})
	}
}

func TestLoad_MissingFile(t *testing.T) {
	if _, err := config.Load(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Fatal("文件不存在时应当报错")
	}
}

func TestModel_APIKey(t *testing.T) {
	const envName = "PULSAR_TEST_API_KEY"

	withEnv := config.Model{Name: "main", Provider: "openai", Model: "gpt-5", APIKeyEnv: envName}
	t.Setenv(envName, "")
	if _, err := withEnv.APIKey(); err == nil {
		t.Fatal("api_key_env 设了但环境变量为空时应当报错（不静默降级成无凭据请求）")
	}
	t.Setenv(envName, "sk-test-value")
	got, err := withEnv.APIKey()
	if err != nil {
		t.Fatalf("APIKey: %v", err)
	}
	if got != "sk-test-value" {
		t.Fatalf("APIKey = %q", got)
	}

	// 没配 api_key_env：返回空串，交给 adapter 的默认来源。
	plain := config.Model{Name: "x", Provider: "openai", Model: "m"}
	if v, err := plain.APIKey(); err != nil || v != "" {
		t.Fatalf("未配 api_key_env 时 = (%q, %v)，want (\"\", nil)", v, err)
	}
}

func TestConfig_ModelLookup(t *testing.T) {
	base := t.TempDir()
	cfg, err := config.Load(writeConfig(t, base, `
models:
  - {name: first, provider: openai, model: m1}
  - {name: second, provider: anthropic, model: m2}
workspace: {root: .}
`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if m, err := cfg.Model(""); err != nil || m.Name != "first" {
		t.Fatalf("Model(\"\") = (%+v, %v)，want first", m, err)
	}
	if m, err := cfg.Model("second"); err != nil || m.Provider != "anthropic" {
		t.Fatalf("Model(\"second\") = (%+v, %v)", m, err)
	}
	if _, err := cfg.Model("missing"); err == nil {
		t.Fatal("未知模型名应当报错")
	}
}

// TestLoad_SymlinkedConfigResolvesToTargetDir 盯住「相对基准 = 配置真实所在目录」：
// 把配置软链到别处，相对路径仍应落在**目标**目录下。
func TestLoad_SymlinkedConfigResolvesToTargetDir(t *testing.T) {
	real := t.TempDir()
	realCfg := writeConfig(t, real, minimal)
	linkDir := t.TempDir()
	link := filepath.Join(linkDir, "pulsar.yaml")
	if err := os.Symlink(realCfg, link); err != nil {
		t.Skipf("本机不支持创建符号链接：%v", err)
	}
	cfg, err := config.Load(link)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if want := realDir(t, real); cfg.Workspace.Root != want {
		t.Fatalf("root = %q, want 链接目标 %q", cfg.Workspace.Root, want)
	}
	if cfg.Workspace.Root == realDir(t, linkDir) {
		t.Fatal("相对基准落到了链接所在目录——EvalSymlinks 没生效")
	}
}
