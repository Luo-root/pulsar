# Pulse-TUI

一个基于 Go 和 Charm Bubble Tea v2 构建的终端 AI 聊天界面，支持多厂商大语言模型、工具调用、MCP 协议和对话记忆。

## ✨ 特性

- **多厂商支持**：支持 OpenAI 和 Anthropic 两种 API 风格，内置 Kimi、DeepSeek、Xiaomi MiMo 等厂商配置
- **交互式初始化**：首次启动时通过精美的 TUI 引导完成 API 配置和模型选择
- **流式响应**：实时显示 AI 的思考和回复过程，带有动态动画效果
- **工具调用**：支持工具注册、执行和权限管理（safe/auto 两种模式）
- **MCP 协议**：支持 Model Context Protocol，可连接外部工具服务
- **对话记忆**：基于 SQLite 的长期记忆存储，支持向量嵌入检索
- **技能系统**：可加载自定义技能（Skill）扩展 AI 能力
- **精美界面**：采用 Catppuccin Mocha 配色方案，气泡式聊天界面

## 📁 项目结构

```
pulse-tui/
├── cmd/pulse-tui/
│   └── main.go              # 程序入口
├── internal/
│   ├── bootloader/          # 启动引导与配置
│   │   ├── bootloader.go    # TUI 初始化向导
│   │   ├── config.go        # 配置管理（YAML）
│   │   └── model_list.go    # 模型列表获取
│   ├── view/
│   │   └── main_view.go     # 主聊天界面
│   └── worker/
│       ├── manager.go       # 组件管理器
│       ├── worker.go        # Agent 初始化
│       ├── chatmodel.go     # 聊天模型配置
│       ├── memory.go        # 记忆控制器
│       ├── mcp.go           # MCP 管理器
│       ├── skill.go         # 技能加载器
│       ├── tool_registry.go # 工具注册
│       └── embedder.go      # 嵌入模型
├── system_prompt/           # 系统提示词
│   ├── system_prompt.md     # 核心身份与行为准则
│   ├── rules.md             # 工作目录与工具调用规则
│   └── safety.md            # 安全策略
├── go.mod                   # Go 模块定义
├── go.sum                   # 依赖校验
└── README.md                # 本文件
```

## 🚀 快速开始

### 前置要求

- Go 1.26.3+
- 有效的 AI API Key（支持 OpenAI 风格或 Anthropic 风格）
- （可选）Ollama 本地服务用于嵌入模型

### 安装与运行

```bash
# 克隆项目
git clone <repository-url>
cd pulse-tui

# 下载依赖
go mod download

# 构建
go build -o pulse-tui ./cmd/pulse-tui

# 运行
./pulse-tui
```

### 首次配置

首次运行时会启动交互式配置向导：

1. **选择 API 风格**：OpenAI 或 Anthropic
2. **选择模型厂商**：Kimi / DeepSeek / Xiaomi MiMo
3. **配置 API 信息**：
   - Base URL（如 `https://api.moonshot.cn/v1`）
   - API Key
4. **选择模型**：自动从 API 获取可用模型列表

配置将保存至 `~/.pulse-tui/config.yaml`。

## ⚙️ 配置文件

配置文件路径：`~/.pulse-tui/config.yaml`

```yaml
version: "1.0.0"
created_at: 2026-05-20T10:00:00Z
api:
  style: openai          # openai | anthropic
  base_url: "https://api.example.com/v1"
  api_key: "sk-..."
model:
  vendor: "Kimi"
  model_id: "moonshot-v1-8k"
  thinking: false
embedding:
  base_url: "http://localhost:11434"
  model_id: "nomic-embed-text"
  embedding_dimension: 768
  chunk_size: 512
mcp:
  path: "/path/to/.pulse-tui/pulse-mcp.json"
skill:
  path: "/path/to/.pulse-tui/skills"
memory:
  path: "/path/to/.pulse-tui/memory.db"
  max_history_messages: 200
  reserve_tokens: 8000
worker:
  session_id: "default"
  max_tool_rounds: 50
  mode: "safe"           # safe | auto
```

## 🎮 使用指南

### 快捷键

| 快捷键 | 功能 |
|--------|------|
| `Ctrl+S` | 发送消息 |
| `PageUp` | 向上滚动 |
| `PageDown` | 向下滚动 |
| `Ctrl+C` | 退出程序 |

### 工具调用确认（Safe 模式）

当 AI 需要执行工具时，会显示确认提示：

```
⚠ dangerous_tool(args)  [y]es [n]kip [a]llow
```

- **y**：允许本次执行
- **n**：跳过本次执行
- **a**：始终允许该工具

### 工作模式

- **safe 模式**：每次执行非只读工具前需要用户确认
- **auto 模式**：自动执行所有工具，无需确认

## 🏗️ 技术栈

| 组件 | 用途 |
|------|------|
| [Bubble Tea v2](https://github.com/charmbracelet/bubbletea) | TUI 框架 |
| [Bubbles v2](https://github.com/charmbracelet/bubbles) | 文本输入、视口组件 |
| [Lipgloss v2](https://github.com/charmbracelet/lipgloss) | 样式渲染 |
| [Pulse](https://github.com/Luo-root/pulse) | AI Agent 核心框架 |
| GORM + SQLite | 数据持久化 |
| Ollama | 本地嵌入模型 |

## 📄 许可证

[MIT License](LICENSE)

## 🤝 致谢

- [Charm](https://charm.sh/) 团队提供的优秀 TUI 工具链
- [Pulse](https://github.com/Luo-root/pulse) 框架提供 AI Agent 能力
