package commands

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// Command 定义一个斜杠命令
type Command struct {
	Name        string   // "search"
	Aliases     []string // ["find", "grep"] 可选别名
	Description string   // "Search conversation history"
	Usage       string   // "/search <query>"
	Handler     func(args string) tea.Cmd
}

// Registry 管理所有斜杠命令
type Registry struct {
	commands map[string]Command
}

func NewRegistry() *Registry {
	return &Registry{
		commands: make(map[string]Command),
	}
}

// Register 注册命令，别名也会被索引
func (r *Registry) Register(cmd Command) {
	r.commands[cmd.Name] = cmd
	for _, alias := range cmd.Aliases {
		r.commands[alias] = cmd
	}
}

// ParseResult 解析结果
type ParseResult struct {
	Matched bool
	Command Command
	Args    string // 命令后面的参数部分
	Raw     string // 原始输入
}

// Parse 检查输入是否是斜杠命令
//
//	"/search hello world" → Matched=true, Command=search, Args="hello world"
//	"hello world"         → Matched=false
func (r *Registry) Parse(input string) ParseResult {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		return ParseResult{Matched: false, Raw: input}
	}

	// 去掉 "/" 前缀，分离命令名和参数
	trimmed := strings.TrimPrefix(input, "/")
	parts := strings.SplitN(trimmed, " ", 2)
	name := strings.ToLower(parts[0])
	args := ""
	if len(parts) > 1 {
		args = strings.TrimSpace(parts[1])
	}

	if cmd, ok := r.commands[name]; ok {
		return ParseResult{
			Matched: true,
			Command: cmd,
			Args:    args,
			Raw:     input,
		}
	}

	return ParseResult{Matched: false, Raw: input}
}

// List 返回所有唯一命令（去重，不含别名）
func (r *Registry) List() []Command {
	seen := make(map[string]bool)
	var cmds []Command
	for _, cmd := range r.commands {
		if seen[cmd.Name] {
			continue
		}
		seen[cmd.Name] = true
		cmds = append(cmds, cmd)
	}
	return cmds
}

// Messages 用于搜索的消息接口
// model 中的 message 类型实现这个接口即可

type Searchable interface {
	Role() string
	Content() string
	Time() time.Time
	Index() int
}
