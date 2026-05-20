package view

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/glamour"
)

func (m *model) setupGlamour() {
	cw := m.contentWidth()
	bubbleW := max(30, cw*75/100)
	innerW := bubbleW - 4 // 与 renderAIBubble 中的 innerW 完全一致
	wrapWidth := max(40, innerW)

	styleJSON := []byte(`{
        "document": { "color": "#CDD6F4" },
        "code_block": { "color": "#CDD6F4" },
        "link": { "color": "#CBA6F7" },
        "image": { "color": "#CBA6F7" },
        "heading": { "color": "#CBA6F7", "bold": true },
        "h1": { "color": "#CBA6F7", "bold": true },
        "h2": { "color": "#CBA6F7", "bold": true },
        "h3": { "color": "#CBA6F7", "bold": true },
        "blockquote": { "color": "#6C7086" },
        "strong": { "bold": true },
        "em": { "italic": true },
        "hr": { "color": "#45475A" },
        "list": { "color": "#CDD6F4" },
        "item": { "color": "#CDD6F4" },
        "table": { "color": "#CDD6F4" },
        "codespan": { "color": "#CBA6F7" },
        "strikethrough": { "crossed_out": true }
    }`)

	renderer, err := glamour.NewTermRenderer(
		glamour.WithStylesFromJSONBytes(styleJSON),
		glamour.WithWordWrap(wrapWidth),
		glamour.WithPreservedNewLines(),
	)
	if err == nil {
		m.gRenderer = renderer
	}
}

func (m model) glamourRender(markdown string) string {
	if m.gRenderer == nil {
		return lipgloss.NewStyle().Foreground(cText).Render(markdown)
	}
	out, err := m.gRenderer.RenderBytes([]byte(markdown))
	if err != nil {
		return lipgloss.NewStyle().Foreground(cText).Render(markdown)
	}
	return strings.TrimRight(string(out), "\n")
}
