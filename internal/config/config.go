// Package config 收敛 pulsar 的单一配置面：模型声明、工作区边界、工具子集与
// 落盘目录。
//
// 凭据纪律：配置里只出现**环境变量名**（api_key_env）；密钥本体永不进配置
// 文件、不进日志、不进仓库。
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// DefaultPath 是未显式指定时的配置文件路径（相对当前工作目录）。
const DefaultPath = "pulsar.yaml"

// DefaultAgentName 是未配置 agent.name 时的 agent 标识。
const DefaultAgentName = "pulsar"

// 默认落盘目录（相对配置文件所在目录解析）。
const (
	DefaultSessionsDir = "data/sessions"
	DefaultLogsDir     = "data/logs"
)

// Model 是一条模型声明：pulsar 侧的名字 + provider + provider 侧模型 id。
type Model struct {
	// Name 是 pulsar 侧的模型名（会话审计、观测、路由都用它）。
	Name string `yaml:"name"`
	// Provider 是 provider 名：openai / openai-responses / anthropic。
	Provider string `yaml:"provider"`
	// Model 是 provider 侧的模型 id（如 gpt-5、claude-sonnet-4-5）。
	Model string `yaml:"model"`
	// BaseURL 覆盖 provider 默认端点（网关 / 兼容服务）；空 = 用默认。
	BaseURL string `yaml:"base_url,omitempty"`
	// APIKeyEnv 是**存放密钥的环境变量名**；空则交给 adapter 的默认来源。
	APIKeyEnv string `yaml:"api_key_env,omitempty"`
	// Options 是 provider 特有参数（超时、组织、自定义头…），原样交给 adapter。
	Options map[string]any `yaml:"options,omitempty"`
}

// Workspace 是工具面的路径边界（落到 builtins 的 Root / WriteRoots / ForbidRead）。
type Workspace struct {
	// Root 是默认工作区根（相对路径的解析基准）。必填。
	Root string `yaml:"root"`
	// WriteRoots 限制写操作（edit / write / apply_patch）的目标；空 = 只允许 Root。
	WriteRoots []string `yaml:"write_roots,omitempty"`
	// ForbidRead 是读操作（read / ls / glob / grep）不得进入的路径前缀。
	ForbidRead []string `yaml:"forbid_read,omitempty"`
}

// Tools 控制内置工具的装配子集。
type Tools struct {
	// Enabled 非空时只装配列出的内置工具；空 = 全部。
	Enabled []string `yaml:"enabled,omitempty"`
}

// Agent 是 agent 标识与系统提示词。
type Agent struct {
	// Name 是 agent 标识；空则用 DefaultAgentName。
	Name string `yaml:"name,omitempty"`
	// System 是系统提示词；空 = 无。
	System string `yaml:"system,omitempty"`
}

// Config 是 pulsar 的单一配置。
type Config struct {
	// Models 是模型声明表；至少一条。
	Models []Model `yaml:"models"`
	// Agent 是 agent 标识与系统提示词。
	Agent Agent `yaml:"agent,omitempty"`
	// Workspace 是工具面的路径边界。
	Workspace Workspace `yaml:"workspace"`
	// Tools 控制内置工具子集。
	Tools Tools `yaml:"tools,omitempty"`
	// SessionsDir 是会话日志目录（JSONL 明文，文件即密钥面）。
	SessionsDir string `yaml:"sessions_dir,omitempty"`
	// LogsDir 是观测日志目录。
	LogsDir string `yaml:"logs_dir,omitempty"`
}

// Load 读配置、填默认值、校验。path 为空时用 DefaultPath；相对路径一律相对
// **配置文件所在目录**解析（可预期，不受调用方 CWD 影响）。
func Load(path string) (*Config, error) {
	if path == "" {
		path = DefaultPath
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("config: resolve %s: %w", path, err)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", abs, err)
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", abs, err)
	}
	if err := c.finalize(filepath.Dir(abs)); err != nil {
		return nil, err
	}
	return &c, nil
}

// finalize 填默认值并把相对路径落到 base 上。失败即配置错误——宁可启动期
// 明确报错，也不要跑到回合中段才失败。
func (c *Config) finalize(base string) error {
	if c.Agent.Name == "" {
		c.Agent.Name = DefaultAgentName
	}
	if len(c.Models) == 0 {
		return errors.New("config: models is empty (declare at least one model)")
	}
	seen := make(map[string]bool, len(c.Models))
	for i := range c.Models {
		m := &c.Models[i]
		if m.Name == "" {
			return fmt.Errorf("config: models[%d]: name is required", i)
		}
		if seen[m.Name] {
			return fmt.Errorf("config: models: duplicate name %q", m.Name)
		}
		seen[m.Name] = true
		if m.Provider == "" {
			return fmt.Errorf("config: model %q: provider is required", m.Name)
		}
		if m.Model == "" {
			return fmt.Errorf("config: model %q: model is required", m.Name)
		}
	}
	if c.Workspace.Root == "" {
		return errors.New("config: workspace.root is required (tools resolve relative paths against it)")
	}
	c.Workspace.Root = resolve(base, c.Workspace.Root)
	c.Workspace.WriteRoots = resolveAll(base, c.Workspace.WriteRoots)
	c.Workspace.ForbidRead = resolveAll(base, c.Workspace.ForbidRead)
	if c.SessionsDir == "" {
		c.SessionsDir = DefaultSessionsDir
	}
	if c.LogsDir == "" {
		c.LogsDir = DefaultLogsDir
	}
	c.SessionsDir = resolve(base, c.SessionsDir)
	c.LogsDir = resolve(base, c.LogsDir)
	return nil
}

// Model 按名取模型声明；name 为空时取第一条。
func (c *Config) Model(name string) (Model, error) {
	if name == "" {
		return c.Models[0], nil
	}
	for _, m := range c.Models {
		if m.Name == name {
			return m, nil
		}
	}
	return Model{}, fmt.Errorf("config: unknown model %q", name)
}

// APIKey 解析该模型的凭据。api_key_env 为空 → 返回空串（交给 adapter 的默认
// 来源）；设了却取不到 → 报错，不静默降级成无凭据请求。
func (m Model) APIKey() (string, error) {
	if m.APIKeyEnv == "" {
		return "", nil
	}
	v := os.Getenv(m.APIKeyEnv)
	if v == "" {
		return "", fmt.Errorf("config: model %q: env %s is unset or empty", m.Name, m.APIKeyEnv)
	}
	return v, nil
}

func resolve(base, p string) string {
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Clean(filepath.Join(base, p))
}

func resolveAll(base string, ps []string) []string {
	if len(ps) == 0 {
		return nil
	}
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, resolve(base, p))
	}
	return out
}
