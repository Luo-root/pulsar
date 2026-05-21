// internal/commands/help.go
package commands

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// HelpMsg 由 /help 命令触发，model 收到后渲染帮助信息
type HelpMsg struct {
	Commands []Command
}

// help 样式
var (
	helpTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#CBA6F7"))

	helpCmdStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A6E3A1")).
			Bold(true)

	helpDescStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A6ADC8"))

	helpBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#45475A")).
			Padding(0, 1)
)

// RenderHelp 将帮助信息渲染为终端友好的字符串
func RenderHelp(cmds []Command, width int) string {
	var lines []string
	lines = append(lines, helpTitleStyle.Render("Available Commands"))
	lines = append(lines, "")

	for _, cmd := range cmds {
		// "/search <query>    Search conversation history"
		usage := helpCmdStyle.Render(fmt.Sprintf("  %-20s", cmd.Usage))
		desc := helpDescStyle.Render(cmd.Description)
		lines = append(lines, usage+desc)
		if len(cmd.Aliases) > 0 {
			aliasLine := fmt.Sprintf("    aliases: %s", strings.Join(cmd.Aliases, ", "))
			lines = append(lines, helpDescStyle.Render(aliasLine))
		}
	}

	lines = append(lines, "")
	lines = append(lines, helpDescStyle.Render("  Type a message to chat normally."))

	inner := strings.Join(lines, "\n")

	// 限制边框宽度
	bw := min(width-2, 60)
	return helpBorder.Width(bw).Render(inner)
}

// NewHelpCommand 创建 /help 命令
func NewHelpCommand(registry *Registry) Command {
	return Command{
		Name:        "help",
		Aliases:     []string{"h", "?"},
		Description: "Show available commands",
		Usage:       "/help",
		Handler: func(args string) tea.Cmd {
			return func() tea.Msg {
				return HelpMsg{Commands: registry.List()}
			}
		},
	}
}
