package view

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// ── Key binding registry ───────────────────────────────────────────────

// KeyBinding 表示单个快捷键
type KeyBinding struct {
	Key         string
	Description string
}

// KeyGroup 表示一组相关的快捷键
type KeyGroup struct {
	Name     string
	Bindings []KeyBinding
}

// getKeyGroups 返回当前状态下所有快捷键分组
// 新增快捷键：在这里加一行 + Update 里加 case 即可
func (m model) getKeyGroups() []KeyGroup {
	groups := []KeyGroup{
		{
			Name: "General",
			Bindings: []KeyBinding{
				{"F1", "Toggle help"},
				{"Ctrl+C", "Quit"},
				{"Ctrl+L", "Clear conversation"},
			},
		},
		{
			Name: "Chat",
			Bindings: []KeyBinding{
				{"Ctrl+S", "Send message"},
			},
		},
		{
			Name: "Navigation",
			Bindings: []KeyBinding{
				{"PgUp", "Scroll up"},
				{"PgDn", "Scroll down"},
			},
		},
	}

	if m.mode == "safe" {
		groups = append(groups, KeyGroup{
			Name: "Tool Confirmation",
			Bindings: []KeyBinding{
				{"y", "Execute tool"},
				{"n", "Skip tool"},
				{"a", "Allow this tool for session"},
			},
		})
	}

	return groups
}

// ── Help view ──────────────────────────────────────────────────────────

func (m model) renderHelp() string {
	cw := m.contentWidth()
	var sections []string

	// Title
	icon := lipgloss.NewStyle().Foreground(cMauve).Bold(true).Render("✦")
	title := lipgloss.NewStyle().Foreground(cText).Bold(true).Render("Keyboard Shortcuts")
	sections = append(sections, "  "+icon+" "+title)
	sections = append(sections, "")

	// Groups
	for _, g := range m.getKeyGroups() {
		// Category header: ── General ──────────
		headPrefix := lipgloss.NewStyle().Foreground(cMauve).Bold(true).
			Render(fmt.Sprintf("  ── %s ", g.Name))
		headSuffix := lipgloss.NewStyle().Foreground(cSurface1).Render(
			strings.Repeat("─", max(0, cw-lipgloss.Width(headPrefix)-1)))
		sections = append(sections, headPrefix+headSuffix)
		sections = append(sections, "")

		// Bindings:    key         description
		for _, b := range g.Bindings {
			key := lipgloss.NewStyle().Foreground(cGreen).
				Width(14).Align(lipgloss.Right).Render(b.Key)
			desc := lipgloss.NewStyle().Foreground(cOverlay0).Render(b.Description)
			sections = append(sections, "    "+key+"  "+desc)
		}
		sections = append(sections, "")
	}

	// Footer hint
	hint := dimStyle.Render("  Press F1 or Esc to close")
	sections = append(sections, hint)

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// ── Footer hints (compact) ─────────────────────────────────────────────

func (m model) footerHints() string {
	if len(m.confirmQueue) > 0 {
		return " y exec · n skip · a allow · ctrl+c quit "
	}
	if m.streaming || m.thinking {
		return " F1 help · ctrl+c quit "
	}
	return " F1 help · ctrl+s send · ctrl+c quit "
}
