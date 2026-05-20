package view

import (
	"context"
	"fmt"
	"os"
	"pulse-tui/internal/worker"
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/Luo-root/pulse/components/schema"
	"github.com/Luo-root/pulse/components/tools"
)

/*
handleSend
  │  thinking = true
  │  thinkingStart = time.Now()
  ├─ thinkingTick() ───→  100ms 后 ──→ thinkingTickMsg
  │                                        │
  │                       thinking == true? ├─ yes → refreshContent (View 重绘，renderThinking 重算 elapsed)
  │                                        │         → 返回 thinkingTick() 继续链
  │                                        └─ no  → 停止链，动画结束
  │
  └─ fetchAIResponse() ─→  goroutine 中...
                              │  SendStream 回调写入 ch
                              ▼
                         streamChunkMsg (第一个)
                              │
                              │  thinking = false  ← 切断 tick 链
                              │  streaming = true
                              │  创建 AI 占位气泡
                              ▼
                         refreshContent → View 中不再调用 renderThinking
                                          计时器彻底消失
*/

// ── Catppuccin Mocha ───────────────────────────────────────────────────

var (
	cSurface1 = lipgloss.Color("#45475A")
	cOverlay0 = lipgloss.Color("#6C7086")
	cText     = lipgloss.Color("#CDD6F4")
	cMauve    = lipgloss.Color("#CBA6F7")
	cGreen    = lipgloss.Color("#A6E3A1")
)

// ── Border frame styles (from pager) ───────────────────────────────────

var (
	// ╭── Pulse ├──
	titleStyle = func() lipgloss.Style {
		b := lipgloss.RoundedBorder()
		b.Right = "├"
		return lipgloss.NewStyle().
			BorderStyle(b).
			BorderForeground(cMauve).
			Padding(0, 1).
			Foreground(cMauve).
			Bold(true)
	}()

	// ┤ 45%:0% ──╯
	infoStyle = func() lipgloss.Style {
		b := lipgloss.RoundedBorder()
		b.Left = "┤"
		return lipgloss.NewStyle().
			BorderStyle(b).
			BorderForeground(cSurface1).
			Padding(0, 1).
			Foreground(cOverlay0)
	}()

	dimStyle = lipgloss.NewStyle().Foreground(cOverlay0).Italic(true)
	tsStyle  = lipgloss.NewStyle().Foreground(cSurface1).Faint(true)
)

// gutterWidth 是 LeftGutterFunc 返回的固定宽度
// "   1 │ " = 7 chars
const gutterWidth = 7

// ── Message types ──────────────────────────────────────────────────────

type role int

const (
	roleUser role = iota
	roleAI
)

type message struct {
	role      role
	content   string
	toolCalls []toolCallRecord // ← 新增：持久化的工具调用
	ts        time.Time
}

type thinkingTickMsg struct{}
type streamStartMsg struct{ ch <-chan tea.Msg }
type streamChunkMsg struct{ delta string }
type streamDoneMsg struct{}
type streamErrMsg struct{ err error }

// ── Tool call types ────────────────────────────────────────────────────

type toolEvent struct {
	Phase    string // "before" | "after"
	Name     string
	Args     map[string]any
	IsError  bool
	Duration time.Duration
}

type toolEventMsg struct{ event toolEvent }

// ── Tool confirm types ────────────────────────────────────────────────

type toolConfirmEvent struct {
	Name       string
	Args       map[string]any
	Permission tools.ToolPermission
	Resp       chan bool
}

type toolConfirmReqMsg struct {
	event toolConfirmEvent
}

// confirmState 与 hook 共享，通过 mutex 保护
type confirmState struct {
	mu          sync.Mutex
	autoAllowed map[string]bool
}

type toolCallRecord struct {
	name      string
	args      map[string]any
	startTime time.Time
	duration  time.Duration
	done      bool
	isError   bool
	denied    bool
}

// ── Model ──────────────────────────────────────────────────────────────

type model struct {
	messages      []message
	ready         bool
	width         int
	height        int
	viewport      viewport.Model
	textarea      textarea.Model
	thinking      bool
	thinkingStart time.Time      // ← 新增：thinking 起始时间
	streaming     bool           // ← 新增：是否正在流式接收
	streamCh      <-chan tea.Msg // ← 新增：流式数据通道

	// ── 新增 ──
	toolEvents chan toolEvent   // 钩子写入，TUI 读取
	toolCalls  []toolCallRecord // 当前轮次活跃的工具调用
	toolCtx    context.Context  // 用于取消 tool 事件轮询
	toolCancel context.CancelFunc

	// ── 新增 ──
	confirmCh    chan toolConfirmEvent // hook 写入确认请求
	confirmQueue []toolConfirmEvent    // 待确认队列
	confirmState *confirmState         // 共享白名单
	mode         string                // "safe" | "auto"

	manager *worker.Manager
	ctx     context.Context
}

func initialModel(
	ctx context.Context,
	manager *worker.Manager,
	toolEvents chan toolEvent,
	confirmCh chan toolConfirmEvent,
	mode string,
	confirmSt *confirmState,
) model {
	ta := textarea.New()
	ta.Placeholder = "Send a message... (Ctrl+S to send)"
	ta.SetVirtualCursor(false)
	ta.Focus()
	ta.Prompt = "┃ "
	ta.CharLimit = 0
	ta.ShowLineNumbers = false
	ta.KeyMap.PageUp.SetEnabled(false)
	ta.KeyMap.PageDown.SetEnabled(false)

	s := ta.Styles()
	s.Focused.CursorLine = lipgloss.NewStyle()
	ta.SetStyles(s)

	return model{
		textarea:     ta,
		manager:      manager,
		ctx:          ctx,
		toolEvents:   toolEvents,
		confirmCh:    confirmCh,
		confirmState: confirmSt,
		mode:         mode,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, tea.RequestBackgroundColor)
}

// ── Commands ───────────────────────────────────────────────────────────

func thinkingTick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return thinkingTickMsg{}
	})
}

func readNextCmd(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return streamDoneMsg{}
		}
		return msg
	}
}

// 轮询工具事件
func readToolEventCmd(ctx context.Context, ch <-chan toolEvent) tea.Cmd {
	return func() tea.Msg {
		select {
		case event := <-ch:
			return toolEventMsg{event: event}
		case <-ctx.Done():
			return nil
		}
	}
}

func readConfirmCmd(ctx context.Context, ch <-chan toolConfirmEvent) tea.Cmd {
	return func() tea.Msg {
		select {
		case event := <-ch:
			return toolConfirmReqMsg{event: event}
		case <-ctx.Done():
			return nil
		}
	}
}

func (m *model) applyToolEvent(event toolEvent) {
	switch event.Phase {
	case "before":
		m.toolCalls = append(m.toolCalls, toolCallRecord{
			name:      event.Name,
			args:      event.Args,
			startTime: time.Now(),
		})
	case "after":
		for i := len(m.toolCalls) - 1; i >= 0; i-- {
			if m.toolCalls[i].name == event.Name && !m.toolCalls[i].done {
				m.toolCalls[i].done = true
				m.toolCalls[i].duration = event.Duration
				m.toolCalls[i].isError = event.IsError
				break
			}
		}
	case "denied":
		m.toolCalls = append(m.toolCalls, toolCallRecord{
			name:   event.Name,
			args:   event.Args,
			done:   true,
			denied: true,
		})
	}
}

func (m *model) drainToolEvents() {
	for {
		select {
		case event := <-m.toolEvents:
			m.applyToolEvent(event)
		default:
			return
		}
	}
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

func formatArgsBrief(args map[string]any) string {
	if len(args) == 0 {
		return ""
	}
	var parts []string
	for k, v := range args {
		val := fmt.Sprintf("%v", v)
		if len(val) > 30 {
			val = val[:27] + "..."
		}
		parts = append(parts, fmt.Sprintf("%s=%q", k, val))
	}
	s := strings.Join(parts, " ")
	if len(s) > 55 {
		s = s[:52] + "..."
	}
	return "(" + s + ")"
}

// API 调用
func (m model) fetchAIResponse(userMsg string) tea.Cmd {
	return func() tea.Msg {
		ch := make(chan tea.Msg, 64)
		go func() {
			defer close(ch)
			_, err := m.manager.GetWorker().SendStream(
				m.ctx, userMsg,
				func(msg *schema.Message, isToolCall bool) bool {
					if msg != nil && !isToolCall && msg.Content != "" {
						ch <- streamChunkMsg{delta: msg.Content}
					}
					return true
				},
			)
			if err != nil {
				ch <- streamErrMsg{err: err}
			}
			// close(ch) 由 defer 处理，readNextCmd 收到 ok=false 时发 streamDoneMsg
		}()
		return streamStartMsg{ch: ch}
	}
}

// ── Update ─────────────────────────────────────────────────────────────

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if len(m.confirmQueue) > 0 {
			switch msg.String() {
			case "ctrl+c":
				for _, ev := range m.confirmQueue {
					ev.Resp <- false
				}
				m.confirmQueue = nil
				return m, tea.Quit
			case "y":
				ev := m.confirmQueue[0]
				m.confirmQueue = m.confirmQueue[1:]
				ev.Resp <- true
			case "n":
				ev := m.confirmQueue[0]
				m.confirmQueue = m.confirmQueue[1:]
				ev.Resp <- false
			case "a":
				ev := m.confirmQueue[0]
				m.confirmQueue = m.confirmQueue[1:]
				m.confirmState.mu.Lock()
				m.confirmState.autoAllowed[ev.Name] = true
				m.confirmState.mu.Unlock()
				ev.Resp <- true
			default:
				return m, nil // 其他按键全部忽略
			}
			m.refreshContent()
			m.viewport.GotoBottom()
			// 持续监听下一个确认
			return m, readConfirmCmd(m.toolCtx, m.confirmCh)
		}

		// ── 正常模式 ──────────────────────────────────
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "ctrl+s":
			return m.handleSend()
		case "pgup":
			m.viewport.PageUp()
			return m, nil
		case "pgdown":
			m.viewport.PageDown()
			return m, nil
		}

		// ── 确认请求到达 ────────────────────────────────
	case toolConfirmReqMsg:
		m.confirmQueue = append(m.confirmQueue, msg.event)
		m.refreshContent()
		m.viewport.GotoBottom()
		return m, readConfirmCmd(m.toolCtx, m.confirmCh)

	// ── 工具事件 ───────────────────────────────────────
	case toolEventMsg:
		m.applyToolEvent(msg.event)
		m.refreshContent()
		m.viewport.GotoBottom()
		// 持续轮询（thinking 或 streaming 期间）
		if m.thinking || m.streaming {
			return m, readToolEventCmd(m.toolCtx, m.toolEvents)
		}
		return m, nil

	case thinkingTickMsg:
		needTick := m.thinking
		if !needTick {
			for _, tc := range m.toolCalls {
				if !tc.done {
					needTick = true
					break
				}
			}
		}
		if needTick {
			m.refreshContent()
			return m, thinkingTick()
		}
		return m, nil

	case streamStartMsg:
		// 流已启动，保存 channel，开始读第一个 chunk
		// thinking 动画保持，等到第一个 chunk 到达再停止
		m.streamCh = msg.ch
		return m, readNextCmd(m.streamCh)

	case streamChunkMsg:
		if !m.streaming {
			m.streaming = true
			m.thinking = false // ← thinking 动画到此停止
			m.messages = append(m.messages, message{
				role: roleAI, content: "", ts: time.Now(),
			})
		}
		if len(m.messages) > 0 {
			last := &m.messages[len(m.messages)-1]
			if last.role == roleAI {
				last.content += msg.delta
			}
		}
		m.refreshContent()
		m.viewport.GotoBottom()
		return m, readNextCmd(m.streamCh)

	case streamDoneMsg:
		// 流结束
		m.streaming = false
		m.streamCh = nil
		m.toolCancel()      // 停止工具事件轮询
		m.drainToolEvents() // 排空剩余事件
		// 将工具调用记录附加到最后一条 AI 消息
		if len(m.toolCalls) > 0 && len(m.messages) > 0 {
			last := &m.messages[len(m.messages)-1]
			if last.role == roleAI {
				last.toolCalls = m.toolCalls
			}
		}
		m.toolCalls = nil
		m.refreshContent()
		m.viewport.GotoBottom()
		return m, nil

	case streamErrMsg:
		// 流出错
		m.streaming = false
		m.thinking = false
		m.streamCh = nil
		errText := fmt.Sprintf("Error: %v", msg.err)
		if len(m.messages) > 0 && m.messages[len(m.messages)-1].role == roleAI {
			m.messages[len(m.messages)-1].content = errText
		} else {
			m.messages = append(m.messages, message{
				role: roleAI, content: errText, ts: time.Now(),
			})
		}
		m.refreshContent()
		return m, nil

	case tea.BackgroundColorMsg:
		m.textarea.SetStyles(textarea.DefaultStyles(msg.IsDark()))

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()
		return m, nil
	}

	var cmd tea.Cmd
	switch msg.(type) {
	case tea.MouseWheelMsg:
		m.viewport, cmd = m.viewport.Update(msg)
	case tea.KeyPressMsg:
		m.textarea, cmd = m.textarea.Update(msg)
	default:
		m.textarea, cmd = m.textarea.Update(msg)
	}
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *model) handleSend() (tea.Model, tea.Cmd) {
	val := strings.TrimSpace(m.textarea.Value())
	if val == "" || m.streaming || m.thinking {
		return m, nil // 流式进行中禁止重复发送
	}
	m.messages = append(m.messages, message{
		role: roleUser, content: val, ts: time.Now(),
	})

	m.textarea.Reset()
	m.thinking = true
	m.thinkingStart = time.Now() // ← 记录起始时间
	m.toolCalls = nil            // 清理上一轮工具调用
	m.confirmQueue = nil
	m.toolCtx, m.toolCancel = context.WithCancel(m.ctx)

	m.refreshContent()
	m.viewport.GotoBottom()

	cmds := []tea.Cmd{
		thinkingTick(),
		m.fetchAIResponse(val),
		readToolEventCmd(m.toolCtx, m.toolEvents),
	}
	// safe 模式才启动确认监听
	if m.mode == "safe" {
		cmds = append(cmds, readConfirmCmd(m.toolCtx, m.confirmCh))
	}
	return m, tea.Batch(cmds...)
}

// ── Layout ─────────────────────────────────────────────────────────────

func (m *model) layout() {
	if m.width == 0 || m.height == 0 {
		return
	}

	// header/footer 各 3 行（边框上+内容+边框下）
	headerH := lipgloss.Height(m.renderHeader())
	footerH := lipgloss.Height(m.renderFooter())
	const inputH = 5
	const inputSepH = 1
	contentH := m.height - headerH - footerH - inputH - inputSepH

	m.textarea.SetWidth(m.width)
	m.textarea.SetHeight(inputH)

	if !m.ready {
		m.viewport = viewport.New(
			viewport.WithWidth(m.width),
			viewport.WithHeight(max(1, contentH)),
		)
		m.viewport.YPosition = headerH
		m.viewport.KeyMap = viewport.KeyMap{}

		// 行号 gutter（pager 示例）
		m.viewport.LeftGutterFunc = func(info viewport.GutterContext) string {
			if info.Soft {
				return "     │ "
			}
			if info.Index >= info.TotalLines {
				return "   ~ │ "
			}
			return fmt.Sprintf("%4d │ ", info.Index+1)
		}

		m.viewport.SetContent(m.renderWelcome())
		m.ready = true
	} else {
		m.viewport.SetWidth(m.width)
		m.viewport.SetHeight(max(1, contentH))
		m.refreshContent()
	}
}

// ── Content rendering ──────────────────────────────────────────────────

func (m *model) refreshContent() {
	if m.viewport.Width() == 0 {
		return
	}
	var sections []string
	hasActive := len(m.toolCalls) > 0 || len(m.confirmQueue) > 0

	for i, msg := range m.messages {
		// 流式期间：活跃工具 + 确认插入到最后一条 AI 消息之前
		if m.streaming && hasActive &&
			i == len(m.messages)-1 && msg.role == roleAI {
			sections = append(sections, m.renderActiveSection()...)
		}
		sections = append(sections, m.renderMessage(msg))
	}

	// thinking 阶段（还没有 AI 消息）：追加在末尾
	if !m.streaming && hasActive {
		sections = append(sections, m.renderActiveSection()...)
	}

	if m.thinking {
		sections = append(sections, m.renderThinking())
	}
	m.viewport.SetContent(lipgloss.JoinVertical(lipgloss.Left, sections...))
}

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

func (m model) renderMessage(msg message) string {
	var sections []string
	switch msg.role {
	case roleUser:
		sections = append(sections, m.renderUserBubble(msg.content))
	case roleAI:
		// 已完成的工具调用（历史消息中）
		if len(msg.toolCalls) > 0 {
			sections = append(sections, m.renderToolCallsSection(msg.toolCalls))
		}
		sections = append(sections, m.renderAIBubble(msg.content))
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

// contentWidth 返回 viewport 内容区宽度（减去 gutter）
func (m model) contentWidth() int {
	return max(10, m.viewport.Width()-gutterWidth)
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

func (m model) renderAIBubble(content string) string {
	cw := m.contentWidth()
	bubbleW := max(30, cw*75/100)
	innerW := bubbleW - 4

	rendered := lipgloss.NewStyle().
		Width(innerW).Foreground(cText).Render(content)
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(cMauve).
		Padding(0, 1).
		Render(rendered)
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
	line := lipgloss.NewStyle().Foreground(cSurface1).Render(
		strings.Repeat("─", max(0, m.width-lipgloss.Width(info))),
	)
	return lipgloss.JoinHorizontal(lipgloss.Center, line, info)
}

// ── View ───────────────────────────────────────────────────────────────

func (m model) View() tea.View {
	var v tea.View
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion

	if !m.ready {
		v.SetContent("\n  Initializing...")
		return v
	}

	header := m.renderHeader()
	vpView := m.viewport.View()
	inputSep := lipgloss.NewStyle().Foreground(cSurface1).
		Render(strings.Repeat("─", m.width))
	inputArea := m.textarea.View()
	footer := m.renderFooter()

	content := strings.Join([]string{
		header, vpView, inputSep, inputArea, footer,
	}, "\n")

	var c *tea.Cursor
	if !m.textarea.VirtualCursor() {
		c = m.textarea.Cursor()
		if c != nil {
			c.Y += lipgloss.Height(header) +
				lipgloss.Height(vpView) +
				lipgloss.Height(inputSep)
		}
	}

	v.SetContent(content)
	v.Cursor = c
	return v
}

// ── Main ───────────────────────────────────────────────────────────────

func UI(ctx context.Context, manager *worker.Manager) {
	toolCh := make(chan toolEvent, 64)
	confirmCh := make(chan toolConfirmEvent, 4)
	state := &confirmState{autoAllowed: make(map[string]bool)}
	mode := "safe"
	if m := manager.GetMode(); m != "" {
		mode = m
	}

	if registry := manager.GetRegistry(); registry != nil {
		// ── beforeExecute：权限检查 + 确认 ──
		registry.AddBeforeExecuteHook(func(ctx context.Context, toolName string, args map[string]any) error {
			if mode != "safe" {
				// auto 模式直接放行
				toolCh <- toolEvent{Phase: "before", Name: toolName, Args: args}
				return nil
			}

			// 检查白名单
			state.mu.Lock()
			autoAllowed := state.autoAllowed[toolName]
			state.mu.Unlock()

			// 检查权限级别
			tool, ok := registry.Get(toolName)
			needsConfirm := ok &&
				tool.Metadata.Permission != tools.PermReadOnly &&
				!autoAllowed

			if !needsConfirm {
				toolCh <- toolEvent{Phase: "before", Name: toolName, Args: args}
				return nil
			}

			// 需要确认 → 发送到 TUI，阻塞等响应
			resp := make(chan bool, 1)
			confirmCh <- toolConfirmEvent{
				Name:       toolName,
				Args:       args,
				Permission: tool.Metadata.Permission,
				Resp:       resp,
			}

			if !<-resp {
				toolCh <- toolEvent{Phase: "denied", Name: toolName, Args: args}
				return fmt.Errorf("operation denied by user")
			}

			// 确认通过
			toolCh <- toolEvent{Phase: "before", Name: toolName, Args: args}
			return nil
		})

		// ── afterExecute：记录结果 ──
		registry.AddAfterExecuteHook(func(ctx context.Context, toolName string, result schema.ToolResult, duration time.Duration) {
			toolCh <- toolEvent{
				Phase:    "after",
				Name:     toolName,
				IsError:  result.IsError,
				Duration: duration,
			}
		})
	}

	m := initialModel(ctx, manager, toolCh, confirmCh, mode, state)
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
