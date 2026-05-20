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
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(homeDir, ".pulse-tui", "config.yaml"), nil
}
