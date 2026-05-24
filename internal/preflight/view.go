package preflight

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ── Styles (Catppuccin Mocha) ──────────────────────────────────────────

var (
	checkOKStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A6E3A1")). // green
			Bold(true)

	checkFailStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F38BA8")). // red
			Bold(true)

	checkRunningStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#CBA6F7")). // mauve
				Bold(true)

	checkPendingStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#6C7086")) // overlay0

	checkInstalledStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#F9E2AF")). // yellow
				Bold(true)

	titlePreStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#CBA6F7")).
			Bold(true)

	descPreStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A6ADC8"))

	warnPreStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F9E2AF"))

	errPreStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F38BA8"))

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#45475A")).
			Padding(1, 2)
)

func (m *Model) View() tea.View {
	var lines []string

	// Title
	lines = append(lines, titlePreStyle.Render("  ✦ Pulse")+" "+descPreStyle.Render("Environment Check"))
	lines = append(lines, "")

	// Each check item
	for i, c := range m.checks {
		lines = append(lines, m.renderCheckItem(i, c))
	}

	// Summary
	lines = append(lines, "")
	if m.done {
		if m.failed {
			lines = append(lines, errPreStyle.Render("  ✗ Some required dependencies are missing."))
			lines = append(lines, descPreStyle.Render("  Install them and restart Pulse."))
		} else {
			lines = append(lines, checkOKStyle.Render("  ✓ All checks passed. Starting Pulse..."))
		}
	} else {
		// 正在进行中的提示
		spinFrame := m.spinner.frames[m.spinner.index%len(m.spinner.frames)]
		lines = append(lines, checkRunningStyle.Render(
			fmt.Sprintf("  %s Checking environment...", spinFrame),
		))
	}

	content := strings.Join(lines, "\n")

	var rendered string
	if m.width > 0 {
		rendered = boxStyle.Width(min(m.width-2, 70)).Render(content)
	} else {
		rendered = boxStyle.Render(content)
	}

	v := tea.NewView(rendered + "\n")
	return v
}

func (m *Model) renderCheckItem(idx int, c Check) string {
	var icon string
	var iconStyle lipgloss.Style
	var statusText string

	switch c.Status {
	case StatusPending:
		icon = "○"
		iconStyle = checkPendingStyle
		statusText = checkPendingStyle.Render("pending")

	case StatusRunning:
		spinFrame := m.spinner.frames[m.spinner.index%len(m.spinner.frames)]
		icon = spinFrame
		iconStyle = checkRunningStyle
		statusText = checkRunningStyle.Render("checking...")

	case StatusOK:
		icon = "✓"
		iconStyle = checkOKStyle
		version := GetVersion(c.Binary)
		if version != "" {
			statusText = checkOKStyle.Render(version)
		} else {
			statusText = checkOKStyle.Render("found")
		}

	case StatusMissing:
		icon = "✗"
		iconStyle = checkFailStyle
		hint := c.InstallHint()
		statusText = warnPreStyle.Render("not found") + "\n    " +
			descPreStyle.Render("  Install: "+hint)

	case StatusInstalling:
		spinFrame := m.spinner.frames[m.spinner.index%len(m.spinner.frames)]
		icon = spinFrame
		iconStyle = checkInstalledStyle
		statusText = checkInstalledStyle.Render("installing...")

	case StatusInstalled:
		icon = "✓"
		iconStyle = checkInstalledStyle
		statusText = checkInstalledStyle.Render("installed")

	case StatusFailed:
		icon = "✗"
		iconStyle = checkFailStyle
		errMsg := ""
		if c.Error != nil {
			errMsg = c.Error.Error()
		}
		statusText = errPreStyle.Render("install failed: " + errMsg)
	}

	name := fmt.Sprintf("%-10s", c.Name)
	required := ""
	if c.Required {
		required = checkFailStyle.Render(" [required]")
	} else {
		required = checkPendingStyle.Render(" [optional]")
	}

	mainLine := fmt.Sprintf("  %s  %s%s  %s",
		iconStyle.Render(icon),
		descPreStyle.Render(name),
		required,
		statusText,
	)

	return mainLine
}
