# Pulse-TUI

一个基于 Go 和 Charm Bubble Tea v2 构建的终端 AI 聊天界面，支持多厂商大语言模型、工具调用、MCP 协议、对话记忆和智能规划执行。

## ✨ 特性

- **多厂商支持**：支持 OpenAI 和 Anthropic 两种 API 风格，内置 Kimi、DeepSeek、Xiaomi MiMo 等厂商配置
- **交互式初始化**：首次启动时通过精美的 TUI 引导完成 API 配置和模型选择
- **流式响应**：实时显示 AI 的思考和回复过程，带有动态动画效果
- **工具调用**：支持工具注册、执行和权限管理（safe/auto 两种模式）
- **MCP 协议**：支持 Model Context Protocol，可连接外部工具服务
- **对话记忆**：基于 SQLite 的长期记忆存储，支持向量嵌入检索
- **技能系统**：可加载自定义技能（Skill）扩展 AI 能力
- **智能规划**：支持目标导向的任务规划与自动执行，包含失败重试和重规划机制
- **斜杠命令**：支持 `/search`、`/help`、`/plan` 等命令扩展
- **工作模式**：支持 safe（安全模式）和 auto（自动模式）切换
- **精美界面**：采用 Catppuccin Mocha 配色方案，气泡式聊天界面，支持行号显示

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
│   ├── commands/            # 斜杠命令系统
│   │   ├── registry.go      # 命令注册中心
│   │   ├── help.go          # /help 命令
│   │   ├── search.go        # /search 命令
│   │   └── plan.go          # /plan 命令与计划渲染
│   ├── planner/             # 智能规划执行器
│   │   └── runner.go        # 规划与执行工作流
│   ├── view/
│   │   ├── main_view.go     # 主聊天界面
│   │   ├── handle.go        # 消息处理逻辑
│   │   ├── keybindings.go   # 快捷键管理
│   │   ├── render.go        # 渲染逻辑
│   │   └── glamour.go       # Markdown 渲染
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
├── .pulse-tui/              # 运行时数据目录
│   ├── config.yaml          # 用户配置文件
│   ├── memory.db            # 对话记忆数据库
│   ├── pulse-mcp.json       # MCP 服务配置
│   └── skills/              # 自定义技能目录
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
| `Ctrl+L` | 清空对话历史 |
| `Ctrl+T` | 切换显示所有工具调用 |
| `Ctrl+E` | 切换工作模式（safe/auto） |
| `F1` | 显示/隐藏帮助面板 |
| `PageUp` | 向上滚动 |
| `PageDown` | 向下滚动 |
| `Esc` | 关闭帮助面板 |
| `Ctrl+C` | 退出程序 / 取消计划 |

### 斜杠命令

Pulse-TUI 支持以下斜杠命令：

| 命令 | 别名 | 用法 | 描述 |
|------|------|------|------|
| `/search` | `/find`, `/grep` | `/search <query>` | 搜索对话历史记录 |
| `/help` | `/h`, `/?` | `/help` | 显示可用命令列表 |
| `/plan` | `/p` | `/plan <goal>` | 创建并执行智能任务计划 |

### 工作模式

Pulse-TUI 支持两种工作模式：

#### Safe 模式（默认）
- 每次执行非只读工具前需要用户确认
- 工具调用时显示确认提示：`⚠ dangerous_tool(args)  [y]es [n]ip [a]llow`
- **y**：允许本次执行
- **n**：跳过本次执行
- **a**：始终允许该工具

#### Auto 模式
- 自动执行所有工具，无需确认
- 适合需要快速执行的场景
- 可通过 `Ctrl+E` 快捷键切换模式

### 智能规划功能

使用 `/plan` 命令可以创建目标导向的任务计划：

```
/plan 创建一个计算器应用
```

规划功能包含：
- **自动任务分解**：AI 自动分析目标并分解为可执行的任务
- **依赖关系管理**：自动处理任务间的依赖关系
- **并行执行**：独立任务可并行执行以提高效率
- **失败重试**：任务失败时自动重规划，最多重试 3 次
- **实时进度**：显示任务执行状态、进度条和详细信息
- **双阶段执行**：
  1. **规划阶段**：使用专用规划Agent生成执行计划
  2. **执行阶段**：使用任务Agent执行具体任务，支持工具调用

### 工具调用确认（Safe 模式）

当 AI 需要执行工具时，会显示确认提示：

```
⚠ dangerous_tool(args)  [y]es [n]ip [a]llow
```

- **y**：允许本次执行
- **n**：跳过本次执行
- **a**：始终允许该工具

### 界面特性

- **行号显示**：消息列表左侧显示行号，便于引用
- **工具调用面板**：实时显示工具调用状态、参数和执行时间
- **进度条**：计划执行时显示详细的进度信息
- **Markdown 渲染**：AI 回复支持 Markdown 格式渲染
- **动态动画**：思考和流式响应时显示动态加载动画
- **工作模式指示**：头部显示当前工作模式（safe/auto）
- **Token 统计**：显示已使用的 Token 数量

## 🏗️ 技术栈

| 组件 | 用途 |
|------|------|
| [Bubble Tea v2](https://github.com/charmbracelet/bubbletea) | TUI 框架 |
| [Bubbles v2](https://github.com/charmbracelet/bubbles) | 文本输入、视口组件 |
| [Lipgloss v2](https://github.com/charmbracelet/lipgloss) | 样式渲染 |
| [Pulse](https://github.com/Luo-root/pulse) | AI Agent 核心框架 |
| [Glamour](https://github.com/charmbracelet/glamour) | Markdown 渲染 |
| GORM + SQLite | 数据持久化 |
| Ollama | 本地嵌入模型 |
| [Pulse Flowchart](https://github.com/Luo-root/pulse) | 工作流引擎（规划与执行） |

## 🔧 开发说明

### 构建依赖

```bash
# 更新依赖
go mod tidy

# 运行测试
go test ./...

# 构建并运行
go run ./cmd/pulse-tui
```

### 扩展功能

1. **添加新命令**：在 `internal/commands/` 目录下创建新文件，实现 `Command` 结构体
2. **自定义技能**：在 `.pulse-tui/skills/` 目录下添加技能文件
3. **扩展工具**：在 `internal/worker/tool_registry.go` 中注册新工具
4. **修改界面**：编辑 `internal/view/main_view.go` 中的样式和布局

### 架构设计

- **模块化设计**：各组件职责清晰，便于维护和扩展
- **事件驱动**：使用 Bubble Tea 的消息驱动架构
- **并发安全**：使用 mutex 和 channel 保证并发安全
- **可扩展性**：通过接口和插件机制支持功能扩展

## 📄 许可证

[MIT License](LICENSE)

## 🤝 致谢

- [Charm](https://charm.sh/) 团队提供的优秀 TUI 工具链
- [Pulse](https://github.com/Luo-root/pulse) 框架提供 AI Agent 能力
- [Catppuccin](https://github.com/catppuccin) 提供的精美配色方案