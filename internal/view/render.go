package view

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/Luo-root/pulse/components/tools"
)

// ── Content rendering ──────────────────────────────────────────────────

func (m *model) renderActiveSection() []string {
	var lines []string
	for _, tc := range m.toolCalls {
		lines = append(lines, m.renderToolCallLine(tc))
	}
	for _, c := range m.confirmQueue {
		lines = append(lines, m.renderConfirmLine(c))
	}
	return lines
}

func (m model) renderConfirmLine(event toolConfirmEvent) string {
	var icon string

	if event.Permission == tools.PermDangerous {
		icon = "⚠"
	} else {
		icon = "⚡"
	}

	var nameStyle lipgloss.Style
	if event.Permission == tools.PermDangerous {
		nameStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#F38BA8")).Bold(true)
	} else {
		nameStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#F9E2AF")).Bold(true)
	}

	iconS := nameStyle.Render(icon) // 图标和名字用同样的颜色和加粗
	nameS := nameStyle.Render(event.Name)
	argsS := dimStyle.Render(formatArgsBrief(event.Args))

	yS := lipgloss.NewStyle().Foreground(cGreen).Bold(true).Render("y")
	nS := lipgloss.NewStyle().Foreground(lipgloss.Color("#F38BA8")).Bold(true).Render("n")
	aS := lipgloss.NewStyle().Foreground(cMauve).Bold(true).Render("a")
	hintS := lipgloss.NewStyle().Foreground(cOverlay0).Render(
		fmt.Sprintf("[%s]es [%s]kip [%s]llow", yS, nS, aS),
	)

	return fmt.Sprintf("  %s %s %s  %s", iconS, nameS, argsS, hintS)
}

// ── renderMessage 加 case ──
func (m model) renderMessage(msg message) string {
	var sections []string
	switch msg.role {
	case roleUser:
		sections = append(sections, m.renderUserBubble(msg.content))
	case roleAI:
		if len(msg.toolCalls) > 0 {
			sections = append(sections, m.renderToolCallsSection(msg.toolCalls))
		}
		sections = append(sections, m.renderAIBubble(msg.content, msg.renderedContent))
	case roleSystem:
		sections = append(sections, msg.content)
	}
	ts := tsStyle.Render("  " + msg.ts.Format("15:04"))
	sections = append(sections, ts)
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// ── 历史工具调用（已完成）─────────────────────────────────

func (m model) renderToolCallsSection(toolCalls []toolCallRecord) string {
	var lines []string
	for _, tc := range toolCalls {
		lines = append(lines, m.renderToolCallLine(tc))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// ── 单条工具调用（通用：进行中 / 完成 / 出错）───────────

func (m model) renderToolCallLine(tc toolCallRecord) string {
	var icon string
	var elapsed time.Duration
	var nameSt lipgloss.Style

	switch {
	case tc.denied:
		icon = "⊘"
		nameSt = lipgloss.NewStyle().Foreground(cSurface1)
		elapsed = 0
	case tc.done:
		elapsed = tc.duration
		if tc.isError {
			icon = "✗"
			nameSt = lipgloss.NewStyle().Foreground(lipgloss.Color("#F38BA8"))
		} else {
			icon = "✓"
			nameSt = lipgloss.NewStyle().Foreground(cGreen)
		}
	default:
		elapsed = time.Since(tc.startTime)
		frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		icon = frames[int(elapsed.Milliseconds()/100)%len(frames)]
		nameSt = lipgloss.NewStyle().Foreground(cMauve)
	}

	iconS := nameSt.Render(icon)
	nameS := nameSt.Render(tc.name)
	argsS := dimStyle.Render(formatArgsBrief(tc.args))

	var timeS string
	if tc.denied {
		timeS = dimStyle.Render("denied")
	} else {
		timeS = tsStyle.Render(formatDuration(elapsed))
	}

	return fmt.Sprintf("  %s %s%s  %s", iconS, nameS, argsS, timeS)
}

func (m model) renderUserBubble(content string) string {
	cw := m.contentWidth()
	bubbleW := max(30, cw*65/100)
	innerW := bubbleW - 4 // border(2) + padding(2)

	rendered := lipgloss.NewStyle().
		Width(innerW).Foreground(cText).Render(content)
	bubble := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(cGreen).
		Padding(0, 1).
		Render(rendered)

	// 外层 Width = 内容区宽度，Align(Right) 让气泡靠右
	return lipgloss.NewStyle().
		Width(cw).Align(lipgloss.Right).Render(bubble)
}

func (m model) renderAIBubble(content string, rendered string) string {
	cw := m.contentWidth()
	bubbleW := max(30, cw*75/100)
	innerW := bubbleW - 4

	var renderedContent string
	if rendered != "" {
		renderedContent = lipgloss.NewStyle().
			Width(innerW).Render(rendered)
	} else {
		renderedContent = lipgloss.NewStyle().
			Width(innerW).Foreground(cText).Render(content)
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(cMauve).
		Padding(0, 1).
		Render(renderedContent)
}

func (m model) renderThinking() string {
	elapsed := time.Since(m.thinkingStart)
	// 格式：秒数 < 60 显示 "X.Xs"，>= 60 显示 "Xm XXs"
	var timeStr string
	if elapsed < time.Minute {
		timeStr = fmt.Sprintf("%.1fs", elapsed.Seconds())
	} else {
		timeStr = fmt.Sprintf("%dm%02ds", int(elapsed.Minutes()), int(elapsed.Seconds())%60)
	}
	// 动态跳动的光标符号
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	frame := frames[int(elapsed.Milliseconds()/100)%len(frames)]
	return dimStyle.Render(fmt.Sprintf("  %s Thinking  %s", frame, timeStr))
}

func (m model) renderWelcome() string {
	icon := lipgloss.NewStyle().Foreground(cMauve).Bold(true).Render("✦")
	title := lipgloss.NewStyle().Foreground(cText).Bold(true).Render("Pulse - TUI")
	desc := dimStyle.Render("Type a message and press Ctrl+S to send.\nPageUp/PageDown to scroll.")
	content := lipgloss.JoinVertical(lipgloss.Center, icon+" "+title, "", desc)
	return lipgloss.Place(
		m.viewport.Width(), m.viewport.Height(),
		lipgloss.Center, lipgloss.Center, content,
	)
}

// ── Header / Footer (pager border frame) ───────────────────────────────

func (m model) renderHeader() string {
	title := titleStyle.Render("Pulse - TUI")
	// ─ 颜色匹配边框，让 ├ 无缝连接
	line := lipgloss.NewStyle().Foreground(cMauve).Render(
		strings.Repeat("─", max(0, m.width-lipgloss.Width(title))),
	)
	return lipgloss.JoinHorizontal(lipgloss.Center, title, line)
}

func (m model) renderFooter() string {
	info := infoStyle.Render(fmt.Sprintf(
		" %3.f%%:%3.f%% ",
		m.viewport.ScrollPercent()*100,
		m.viewport.HorizontalScrollPercent()*100,
	))

	hints := lipgloss.NewStyle().Foreground(cOverlay0).Render(m.footerHints())
	hintsW := lipgloss.Width(hints)
	infoW := lipgloss.Width(info)
	lineW := max(0, m.width-hintsW-infoW)

	line := "\n" + hints +
		lipgloss.NewStyle().Foreground(cSurface1).Render(
			strings.Repeat("─", lineW))

	return lipgloss.JoinHorizontal(lipgloss.Left, line, info)
}
