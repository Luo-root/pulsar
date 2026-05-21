// internal/commands/search.go
package commands

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// SearchResult 单条搜索结果
type SearchResult struct {
	Index   int
	Role    string
	Content string
	Time    time.Time
	Snippet string // 包含高亮的片段
}

// SearchMsg 由 /search 命令触发
type SearchMsg struct {
	Query   string
	Results []SearchResult
}

// 搜索结果样式
var (
	searchTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#89B4FA"))

	searchMatchStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#F9E2AF")).
				Bold(true)

	searchDimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6C7086"))

	searchResultBorder = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#45475A")).
				Padding(0, 1)
)

// SearchMessages 在消息列表中搜索，返回匹配结果
func SearchMessages(messages []Searchable, query string) []SearchResult {
	if query == "" {
		return nil
	}

	queryLower := strings.ToLower(query)
	var results []SearchResult

	for _, msg := range messages {
		content := msg.Content()
		if !strings.Contains(strings.ToLower(content), queryLower) {
			continue
		}

		// 生成包含匹配的片段（最多 80 字符围绕匹配位置）
		snippet := extractSnippet(content, query)

		results = append(results, SearchResult{
			Index:   msg.Index(),
			Role:    msg.Role(),
			Content: content,
			Time:    msg.Time(),
			Snippet: snippet,
		})
	}

	return results
}

// extractSnippet 从内容中提取包含匹配词的片段
func extractSnippet(content, query string) string {
	lower := strings.ToLower(content)
	idx := strings.Index(lower, strings.ToLower(query))
	if idx < 0 {
		return content
	}

	// 取匹配位置前后各 40 字符
	start := max(0, idx-40)
	end := min(len(content), idx+len(query)+40)

	snippet := content[start:end]
	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(content) {
		snippet = snippet + "..."
	}

	// 高亮匹配词（不区分大小写）
	return highlightMatch(snippet, query)
}

// highlightMatch 在文本中高亮匹配的词
func highlightMatch(text, query string) string {
	lower := strings.ToLower(text)
	queryLower := strings.ToLower(query)

	var result strings.Builder
	pos := 0
	for {
		idx := strings.Index(lower[pos:], queryLower)
		if idx < 0 {
			result.WriteString(text[pos:])
			break
		}
		// 匹配前的文本
		result.WriteString(text[pos : pos+idx])
		// 高亮匹配部分
		matchEnd := pos + idx + len(query)
		result.WriteString(searchMatchStyle.Render(text[pos+idx : min(matchEnd, len(text))]))
		pos = matchEnd
	}

	return result.String()
}

// RenderSearchResults 将搜索结果渲染为终端友好的字符串
func RenderSearchResults(msg SearchMsg, width int) string {
	if len(msg.Results) == 0 {
		return searchResultBorder.Width(min(width-2, 50)).Render(
			searchTitleStyle.Render("No results") + "\n" +
				searchDimStyle.Render(fmt.Sprintf("  Nothing matched \"%s\"", msg.Query)),
		)
	}

	var lines []string
	header := fmt.Sprintf("Search: \"%s\" — %d match(es)", msg.Query, len(msg.Results))
	lines = append(lines, searchTitleStyle.Render(header))
	lines = append(lines, "")

	for _, r := range msg.Results {
		// "#3 14:30 You:"
		meta := searchDimStyle.Render(fmt.Sprintf("#%d  %s  %s", r.Index+1, r.Time.Format("15:04"), r.Role))
		lines = append(lines, meta)
		// 匹配内容（已高亮）
		lines = append(lines, "  "+r.Snippet)
		lines = append(lines, "")
	}

	lines = append(lines, searchDimStyle.Render("  /search <query> to search again"))

	inner := strings.Join(lines, "\n")
	bw := min(width-2, 70)
	return searchResultBorder.Width(bw).Render(inner)
}

// NewSearchCommand 创建 /search 命令
func NewSearchCommand() Command {
	return Command{
		Name:        "search",
		Aliases:     []string{"find", "grep"},
		Description: "Search conversation history",
		Usage:       "/search <query>",
		Handler: func(args string) tea.Cmd {
			// 注意：这里只发消息，不访问消息列表
			// 搜索逻辑在 model 的 Update 中处理
			// 这样 commands 包不依赖 model 的具体类型
			return func() tea.Msg {
				return SearchRequestMsg{Query: args}
			}
		},
	}
}

// SearchRequestMsg 搜索请求，model 收到后执行搜索
type SearchRequestMsg struct {
	Query string
}
