package view

import (
	"context"
	"fmt"
	"pulse-tui/internal/commands"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

func (m *model) handleSend() (tea.Model, tea.Cmd) {
	val := strings.TrimSpace(m.textarea.Value())
	if val == "" || m.streaming || m.thinking {
		return m, nil
	}
	m.textarea.Reset() // ← 先 reset，不管走哪条分支

	if m.planCompleted {
		m.planCompleted = false
		m.planView = nil
	}

	// ── 斜杠命令优先 ──
	result := m.commands.Parse(val)
	if result.Matched {
		if result.Args == "" && result.Command.Name != "help" {
			m.messages = append(m.messages, message{
				role: roleUser, content: val, ts: time.Now(),
			})
			m.messages = append(m.messages, message{
				role: roleSystem, content: fmt.Sprintf("Usage: %s", result.Command.Usage), ts: time.Now(),
			})
			m.refreshContent()
			m.viewport.GotoBottom()
			return m, nil
		}
		m.messages = append(m.messages, message{
			role: roleUser, content: val, ts: time.Now(),
		})
		m.refreshContent()
		m.viewport.GotoBottom()

		// ★ 修复：/plan 命令需要启动 toolCtx + readToolEventCmd
		cmds := []tea.Cmd{result.Command.Handler(result.Args)}
		if result.Command.Name == "plan" {
			planCtx, planCancel := context.WithCancel(m.ctx)
			m.toolCtx = planCtx
			m.toolCancel = planCancel
			cmds = append(cmds, readToolEventCmd(m.toolCtx, m.toolEvents))
			if m.mode == "safe" {
				cmds = append(cmds, readConfirmCmd(m.toolCtx, m.confirmCh))
			}
		}
		return m, tea.Batch(cmds...)
	}

	// ── 普通消息（只添加一次） ──
	m.messages = append(m.messages, message{
		role: roleUser, content: val, ts: time.Now(),
	})
	m.thinking = true
	m.thinkingStart = time.Now()
	m.toolCalls = nil
	m.confirmQueue = nil
	m.toolCtx, m.toolCancel = context.WithCancel(m.ctx)

	m.refreshContent()
	m.viewport.GotoBottom()

	cmds := []tea.Cmd{
		thinkingTick(),
		m.fetchAIResponse(val),
		readToolEventCmd(m.toolCtx, m.toolEvents),
	}
	if m.mode == "safe" {
		cmds = append(cmds, readConfirmCmd(m.toolCtx, m.confirmCh))
	}
	return m, tea.Batch(cmds...)
}

// handleSearch 执行 /search 命令
func (m *model) handleSearch(query string) (tea.Model, tea.Cmd) {
	// 构建带索引的可搜索消息列表
	searchable := make([]commands.Searchable, len(m.messages))
	for i, msg := range m.messages {
		searchable[i] = indexedMessage{message: msg, index: i}
	}

	results := commands.SearchMessages(searchable, query)
	resultMsg := commands.SearchMsg{Query: query, Results: results}
	resultText := commands.RenderSearchResults(resultMsg, m.contentWidth())

	m.messages = append(m.messages, message{
		role: roleSystem, content: resultText, ts: time.Now(),
	})
	m.refreshContent()
	m.viewport.GotoBottom()
	return m, nil
}
