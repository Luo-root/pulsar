package bootloader

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// 定义错误类型
var (
	// ErrConfigNotFound 表示配置文件不存在（首次运行）
	ErrConfigNotFound = errors.New("config file not found")

	// ErrConfigInvalid 表示配置文件存在但格式错误（需要重新配置）
	ErrConfigInvalid = errors.New("config file is invalid")
)

// Config 表示完整的配置结构
type Config struct {
	Version   string          `yaml:"version"`
	CreatedAt time.Time       `yaml:"created_at"`
	API       APIConfig       `yaml:"api"`
	Model     ModelConfig     `yaml:"model"`
	Embedding EmbeddingConfig `yaml:"embedding"`
	Mcp       Mcp             `yaml:"mcp"`
	Skill     Skill           `yaml:"skill"`
	Memory    Memory          `yaml:"memory"`
	Worker    Worker          `yaml:"worker"`
}

// APIConfig 存储 API 相关配置
type APIConfig struct {
	Style   string `yaml:"style"` // openai 或 anthropic
	BaseURL string `yaml:"base_url"`
	APIKey  string `yaml:"api_key"`
}

// ModelConfig 存储模型相关配置
type ModelConfig struct {
	Vendor   string `yaml:"vendor"` // Kimi, DeepSeek, XiaomiMimo
	ModelID  string `yaml:"model_id"`
	Thinking bool   `yaml:"thinking"`
}

type EmbeddingConfig struct {
	BaseURL            string `yaml:"base_url"`
	ModelID            string `yaml:"model_id"`
	EmbeddingDimension int    `yaml:"embedding_dimension"`
	ChunkSize          int    `yaml:"chunk_size"`
}

type Mcp struct {
	Path string `yaml:"path"`
}

type Skill struct {
	Path string `yaml:"path"`
}

type Memory struct {
	Path               string `yaml:"path"`
	PromptPath         string `yaml:"prompt_path"`
	MaxHistoryMessages int    `yaml:"max_history_messages"`
	ReserveTokens      int    `yaml:"reserve_tokens"`
}

type Worker struct {
	SessionID     string `yaml:"session_id"`
	MaxToolRounds int    `yaml:"max_tool_rounds"`
	Mode          string `yaml:"mode"` // "safe" | "auto"
}

// SaveConfig 将配置保存到文件
func SaveConfig(config *Config, filePath string) error {
	// 确保目录存在
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// 获取工作目录
	getwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	basePath := filepath.Join(getwd, ".pulse-tui")

	if err := os.MkdirAll(basePath, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// 设置默认值与路径
	config.Model.Thinking = false

	// Embedding 配置
	config.Embedding.BaseURL = "http://localhost:11434"
	config.Embedding.ModelID = "nomic-embed-text"
	config.Embedding.EmbeddingDimension = 768
	config.Embedding.ChunkSize = 512

	// Memory 配置
	config.Memory.MaxHistoryMessages = 200
	config.Memory.ReserveTokens = 8000
	config.Memory.Path = filepath.Join(basePath, "memory.db")
	config.Memory.PromptPath = filepath.Join(basePath, "system_prompt")

	// Mcp & Skill 配置
	config.Mcp.Path = filepath.Join(basePath, "pulse-mcp.json")
	config.Skill.Path = filepath.Join(basePath, "skills")

	// Worker 配置
	config.Worker.SessionID = "default"
	config.Worker.MaxToolRounds = 50
	config.Worker.Mode = "safe"

	// 创建 memory.db（空文件）
	if _, err := os.OpenFile(config.Memory.Path, os.O_CREATE|os.O_WRONLY, 0644); err != nil {
		return fmt.Errorf("failed to create memory.db: %w", err)
	}

	// 创建 pulse-mcp.json（不存在时写入空 JSON 数组）
	if _, err := os.Stat(config.Mcp.Path); os.IsNotExist(err) {
		if err := os.WriteFile(config.Mcp.Path, []byte("{}"), 0644); err != nil {
			return fmt.Errorf("failed to create pulse-mcp.json: %w", err)
		}
	}

	// 创建 skills 目录
	if err := os.MkdirAll(config.Skill.Path, 0755); err != nil {
		return fmt.Errorf("failed to create skills directory: %w", err)
	}

	// 创建 system_prompt 目录并写入默认文件
	if err := initSystemPrompts(config.Memory.PromptPath); err != nil {
		return fmt.Errorf("failed to initialize system prompts: %w", err)
	}

	// 序列化为 YAML
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// 写入文件
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// initSystemPrompts 初始化系统提示词文件
func initSystemPrompts(promptDir string) error {
	// 创建目录
	if err := os.MkdirAll(promptDir, 0755); err != nil {
		return fmt.Errorf("failed to create prompt directory: %w", err)
	}

	// 定义默认文件内容
	defaultFiles := map[string]string{
		"rules.md":         rules,
		"safety.md":        safety,
		"system_prompt.md": systemPrompt,
	}

	// 写入每个文件（仅在文件不存在时创建）
	for filename, content := range defaultFiles {
		filePath := filepath.Join(promptDir, filename)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
				return fmt.Errorf("failed to create %s: %w", filename, err)
			}
		}
	}

	return nil
}

// LoadConfig 从文件加载配置
func LoadConfig(filePath string) (*Config, error) {
	// 检查文件是否存在
	if _, err := os.Stat(filePath); err != nil {
		if os.IsNotExist(err) {
			return nil, ErrConfigNotFound
		}
		return nil, fmt.Errorf("failed to access config file: %w", err)
	}

	// 读取文件
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// 解析 YAML
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConfigInvalid, err)
	}

	// 验证配置完整性
	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConfigInvalid, err)
	}

	return &config, nil
}

// validateConfig 验证配置的完整性
func validateConfig(config *Config) error {
	// 1. API 配置校验
	if config.API.BaseURL == "" {
		return errors.New("missing api.base_url")
	}
	// 校验 URL 格式
	if _, err := url.ParseRequestURI(config.API.BaseURL); err != nil {
		return fmt.Errorf("invalid api.base_url format: %w", err)
	}

	if config.API.APIKey == "" {
		return errors.New("missing api.api_key")
	}

	// 校验 API 风格
	validStyles := map[string]bool{"openai": true, "anthropic": true}
	if !validStyles[config.API.Style] {
		return fmt.Errorf("invalid api.style: '%s' (must be 'openai' or 'anthropic')", config.API.Style)
	}

	// 2. Model 配置校验
	if config.Model.Vendor == "" {
		return errors.New("missing model.vendor")
	}
	if config.Model.ModelID == "" {
		return errors.New("missing model.model_id")
	}

	// 3. Embedding 配置校验
	if config.Embedding.BaseURL == "" {
		return errors.New("missing embedding.base_url")
	}
	if config.Embedding.ModelID == "" {
		return errors.New("missing embedding.model_id")
	}
	if config.Embedding.EmbeddingDimension <= 0 {
		return errors.New("embedding.embedding_dimension must be greater than 0")
	}
	if config.Embedding.ChunkSize <= 0 {
		return errors.New("embedding.chunk_size must be greater than 0")
	}

	// 4. Worker 配置校验
	if config.Worker.Mode == "" {
		return errors.New("missing worker.mode")
	}
	// 校验 Worker 模式（假设支持 safe, aggressive, auto 等）
	validModes := map[string]bool{"safe": true, "auto": true}
	if !validModes[config.Worker.Mode] {
		return fmt.Errorf("invalid worker.mode: '%s'", config.Worker.Mode)
	}
	if config.Worker.MaxToolRounds <= 0 {
		return errors.New("worker.max_tool_rounds must be greater than 0")
	}

	// 5. Memory 配置校验
	if config.Memory.MaxHistoryMessages < 0 {
		return errors.New("memory.max_history_messages cannot be negative")
	}
	if config.Memory.ReserveTokens < 0 {
		return errors.New("memory.reserve_tokens cannot be negative")
	}

	return nil
}

// GetDefaultConfigPath 获取默认配置文件路径
func GetDefaultConfigPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(configDir, ".pulse-tui", "config.yaml"), nil
}

const (
	rules = `# 铁律一：工作目录约束（绝对禁止违反）
当前固定工作目录：{{work_dir}}
1. 所有文件/文件夹操作 必须基于此目录执行，禁止使用绝对路径、禁止跳出目录、禁止擅自修改路径
2. 每次执行工具前，必须再次确认工作目录正确无误
3. 若涉及路径拼接，必须以当前工作目录为根路径，拼接后需调用工具验证路径合法性
4. 任何场景下均不得绕过工作目录约束执行文件操作

# 工具调用规则（强制高频调用）
1. 【不确定 → 必须调用工具】：信息不明确、数据未验证、路径/内容存疑 → 立即调用工具查询确认
2. 【主动确认】：用户需求模糊、参数缺失、结果需要校验 → 主动调用工具获取真实信息
3. 【多轮验证】：工具返回结果后，若仍不完整/不准确 → 继续调用工具补充查询，直至信息完整无误
4. 【禁止凭空回答】：无工具验证的信息，绝对不输出给用户；工具能完成的操作，绝不依赖记忆/常识猜测
5. 【优先工具】：所有关键操作（路径验证、内容读取、命令执行等）优先通过工具完成，禁止主观判断

## 记忆管理
你在以下情况必须调用 user_config 工具：
1. 用户表达了明确偏好 → set_preference
   例："我喜欢简洁的回答"、"代码用 TypeScript"
2. 用户设定了行为规则 → set_rules
   例："回复不要用 emoji"、"代码注释用中文"
3. 对话开始时 → get_preference + get_rules 获取已存储的配置
`
	safety = `# 安全策略（与工具调用/工作目录约束强绑定）
1. 禁止执行危险命令：rm -rf、mkfs、dd、rd /s、del /s 等；任何命令执行前，必须先调用工具完成安全检查，再询问用户确认
2. 禁止访问系统敏感目录：/etc/passwd、C:\Windows\System32\config 等；目录访问前需调用工具验证路径，确认非敏感目录后方可操作
3. 禁止执行破坏性操作：格式化磁盘、删除系统文件、修改系统配置等；涉及文件/目录修改操作时，必须先验证路径是否在工作目录内，再确认操作安全性
4. 如遇不确定操作（包括但不限于命令安全性、路径合法性、操作影响范围），先调用工具验证信息，再询问用户确认，禁止擅自执行
5. 工作目录内的文件操作也需遵守安全策略，禁止在工作目录内执行危险命令/破坏性操作
`
	systemPrompt = `# 核心身份
你是专业的自动化执行助手，严格遵守所有指令与约束规则，绝不臆测、绝不编造任何信息，输出内容完全基于工具返回的真实数据。

# 行为准则
1. 不确定 → 必须调用工具：信息不明确、数据未验证、路径/内容存疑时，立即调用工具查询确认
2. 主动确认：用户需求模糊、参数缺失、结果需要校验时，主动调用工具获取真实信息
3. 禁止凭空回答：无工具验证的信息，绝对不输出给用户；绝不依赖记忆/常识猜测
4. 所有操作基于工具返回的真实数据：路径、文件名、内容等关键信息必须经过工具验证
5. 严格执行工具调用循环：直到信息完整、确认无误后，方可终止工具调用并输出结果
`
)
