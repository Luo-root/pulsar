package view

import (
	"context"
	"fmt"
	"os"
	"pulse-tui/internal/commands"
	"pulse-tui/internal/planner"
	"pulse-tui/internal/worker"
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/Luo-root/pulse/components/agent"
	"github.com/Luo-root/pulse/components/flowchart/node"
	"github.com/Luo-root/pulse/components/schema"
	"github.com/Luo-root/pulse/components/tools"
	"github.com/charmbracelet/glamour"
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
	roleSystem // ← 新增
)

type message struct {
	role            role
	content         string
	toolCalls       []toolCallRecord // ← 新增：持久化的工具调用
	ts              time.Time
	renderedContent string
}

// indexedMessage 包装 message 带上索引，供 /search 使用
type indexedMessage struct {
	message
	index int
}

func (im indexedMessage) Role() string {
	switch im.role {
	case roleUser:
		return "You"
	case roleAI:
		return "AI"
	case roleSystem:
		return "System"
	}
	return ""
}
func (im indexedMessage) Content() string { return im.content }
func (im indexedMessage) Time() time.Time { return im.ts }
func (im indexedMessage) Index() int      { return im.index }

type thinkingTickMsg struct{}
type streamStartMsg struct{ ch <-chan tea.Msg }
type streamChunkMsg struct{ delta string }
type streamDoneMsg struct{}
type streamErrMsg struct{ err error }

type planStartedMsg struct {
	ch     <-chan commands.PlanUpdateMsg
	cancel context.CancelFunc
}

type planTickMsg struct{}

type planDoneMsg struct{}

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
	toolEvents       chan toolEvent   // 钩子写入，TUI 读取
	toolCalls        []toolCallRecord // 当前轮次活跃的工具调用
	toolCtx          context.Context  // 用于取消 tool 事件轮询
	toolCancel       context.CancelFunc
	showAllToolCalls bool // false=只显示最新, true=显示全部

	// ── 新增 ──
	confirmCh    chan toolConfirmEvent // hook 写入确认请求
	confirmQueue []toolConfirmEvent    // 待确认队列
	confirmState *confirmState         // 共享白名单
	mode         string                // "safe" | "auto"

	showHelp bool // ← 新增：是否显示帮助面板

	manager   *worker.Manager
	ctx       context.Context
	gRenderer *glamour.TermRenderer
	commands  *commands.Registry

	// ── Plan execution ──
	planActive    bool
	planCompleted bool
	planView      *commands.PlanView
	planPhase     commands.PlanPhase
	planMessage   string
	planCh        <-chan commands.PlanUpdateMsg
	planStart     time.Time
	planTick      int
	planCancel    context.CancelFunc
	planningAgent agent.AgentInterface // 规划用（无工具）
	taskAgent     agent.AgentInterface // 执行用（带工具）
}

func initialModel(
	ctx context.Context,
	manager *worker.Manager,
	toolEvents chan toolEvent,
	confirmCh chan toolConfirmEvent,
	mode string,
	confirmSt *confirmState,
	planningAgent agent.AgentInterface,
	taskAgent agent.AgentInterface,
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

	reg := commands.NewRegistry()
	reg.Register(commands.NewSearchCommand())
	reg.Register(commands.NewHelpCommand(reg))
	reg.Register(commands.NewPlanCommand())

	return model{
		textarea:      ta,
		manager:       manager,
		ctx:           ctx,
		toolEvents:    toolEvents,
		confirmCh:     confirmCh,
		confirmState:  confirmSt,
		mode:          mode,
		commands:      reg,
		planningAgent: planningAgent,
		taskAgent:     taskAgent,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, tea.RequestBackgroundColor)
}

// ── Plan helpers ───────────────────────────────────────────────────────

// planToView 将 node.Plan 转换为 commands.PlanView（视图模型，与 node 解耦）
func planToView(p *node.Plan) *commands.PlanView {
	if p == nil {
		return nil
	}
	plan := p.Snapshot()
	v := &commands.PlanView{Goal: plan.Goal}
	for _, t := range plan.Tasks {
		v.Tasks = append(v.Tasks, commands.TaskView{
			ID:          t.ID,
			Description: t.Description,
			Inputs:      t.Inputs,
			Outputs:     t.Outputs,
			State:       commands.PlanTaskState(t.State),
			Error:       t.Error,
		})
	}
	return v
}

func planTickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return planTickMsg{}
	})
}

// ── Plan execution methods ─────────────────────────────────────────────

func (m *model) handlePlanRequest(goal string) (tea.Model, tea.Cmd) {
	if m.planActive {
		m.messages = append(m.messages, message{
			role: roleSystem, content: "A plan is already running.", ts: time.Now(),
		})
		m.refreshContent()
		m.viewport.GotoBottom()
		return m, nil
	}

	m.planActive = true
	m.planCompleted = false
	m.planView = nil
	m.planPhase = commands.PlanPhasePlanning
	m.planMessage = "正在分析目标..."
	m.planStart = time.Now()
	m.planTick = 0

	m.refreshContent()
	m.viewport.GotoBottom()

	return m, tea.Batch(m.launchPlan(goal), planTickCmd())
}

func (m model) launchPlan(goal string) tea.Cmd {
	return func() tea.Msg {
		ch := make(chan commands.PlanUpdateMsg, 64)
		ctx, cancel := context.WithCancel(m.ctx)

		go func() {
			defer cancel()
			defer close(ch)
			defer func() {
				if r := recover(); r != nil {
					ch <- commands.PlanUpdateMsg{
						Phase:   commands.PlanPhaseFailed,
						Message: fmt.Sprintf("Plan panicked: %v", r),
					}
				}
			}()

			planner.RunPlan(ctx, goal, m.planningAgent, m.taskAgent, func(event planner.RunnerEvent) {
				ch <- commands.PlanUpdateMsg{
					Phase:   commands.PlanPhase(event.Phase),
					Plan:    planToView(event.Plan),
					Message: event.Message,
				}
			})
		}()

		return planStartedMsg{ch: ch, cancel: cancel}
	}
}

func (m model) readPlanUpdateCmd() tea.Cmd {
	ch := m.planCh
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		update, ok := <-ch
		if !ok {
			return planDoneMsg{}
		}
		return update
	}
}

func (m model) renderPlanPanel() string {
	msg := commands.PlanUpdateMsg{
		Phase:   m.planPhase,
		Plan:    m.planView,
		Message: m.planMessage,
	}
	elapsed := time.Since(m.planStart)
	return commands.RenderPlan(msg, m.contentWidth(), elapsed, m.planTick)
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
		case "ctrl+t":
			m.showAllToolCalls = !m.showAllToolCalls
			m.refreshContent()
			return m, nil
		case "ctrl+l":
			m.messages = nil
			m.showHelp = false
			m.planCompleted = false // ← 新增
			m.planView = nil        // ← 新增
			m.refreshContent()
			return m, nil
		case "f1":
			m.showHelp = !m.showHelp
			if m.showHelp {
				m.viewport.SetContent(m.renderHelp())
			} else {
				m.refreshContent()
			}
			return m, nil
		case "esc":
			if m.showHelp {
				m.showHelp = false
				m.refreshContent()
				return m, nil
			}
		case "pgup":
			if !m.showHelp {
				m.viewport.PageUp()
			}
			return m, nil
		case "pgdown":
			if !m.showHelp {
				m.viewport.PageDown()
			}
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
		// ★ 持续轮询：thinking、streaming 或 plan 执行期间
		if m.thinking || m.streaming || m.planActive {
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

		// ── 渲染 markdown ──
		if len(m.messages) > 0 {
			last := &m.messages[len(m.messages)-1]
			if last.role == roleAI && last.renderedContent == "" {
				last.renderedContent = m.glamourRender(last.content)
			}
		}

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
		m.setupGlamour()
		// 窗口大小变了，重新渲染所有已完成的消息
		for i := range m.messages {
			if m.messages[i].role == roleAI && m.messages[i].renderedContent != "" {
				m.messages[i].renderedContent = m.glamourRender(m.messages[i].content)
			}
		}
		m.refreshContent()
		return m, nil

	case commands.HelpMsg:
		m.showHelp = false // 不走 showHelp 面板，走消息流
		helpText := commands.RenderHelp(msg.Commands, m.contentWidth())
		m.messages = append(m.messages, message{
			role: roleSystem, content: helpText, ts: time.Now(),
		})
		m.refreshContent()
		m.viewport.GotoBottom()
		return m, nil

	case commands.SearchRequestMsg:
		return m.handleSearch(msg.Query)
		// ── Plan execution ─────────────────────────────────────────────
	case commands.PlanRequestMsg:
		return m.handlePlanRequest(msg.Goal)

	case planStartedMsg:
		m.planCh = msg.ch
		m.planCancel = msg.cancel
		return m, m.readPlanUpdateCmd()

	case commands.PlanUpdateMsg:
		m.planPhase = msg.Phase
		m.planView = msg.Plan
		m.planMessage = msg.Message

		if msg.Phase.IsTerminal() {
			m.planActive = false
			m.planCompleted = true
			m.planCh = nil

			// ★ 清理 toolCtx，停止 readToolEventCmd
			if m.toolCancel != nil {
				m.toolCancel()
				m.toolCancel = nil
			}
			m.confirmQueue = nil

			if msg.Message != "" {
				m.messages = append(m.messages, message{
					role: roleSystem, content: msg.Message, ts: time.Now(),
				})
			}
		}

		m.refreshContent()
		m.viewport.GotoBottom()

		if msg.Phase.IsTerminal() {
			return m, nil
		}
		return m, m.readPlanUpdateCmd()

	case planTickMsg:
		if !m.planActive {
			return m, nil
		}
		m.planTick++
		m.refreshContent()
		return m, planTickCmd()

	case planDoneMsg:
		if m.planActive {
			m.planActive = false
			m.planCompleted = true
			m.planCh = nil
			m.refreshContent()
		}
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

func (m *model) refreshContent() {
	if m.viewport.Width() == 0 {
		return
	}
	if m.showHelp {
		m.viewport.SetContent(m.renderHelp())
		return
	}

	var sections []string
	hasActive := len(m.toolCalls) > 0 || len(m.confirmQueue) > 0

	for i, msg := range m.messages {
		if m.streaming && hasActive && i == len(m.messages)-1 && msg.role == roleAI {
			sections = append(sections, m.renderActiveSection()...)
		}
		sections = append(sections, m.renderMessage(msg))
	}

	if !m.streaming && hasActive {
		sections = append(sections, m.renderActiveSection()...)
	}

	if m.thinking {
		sections = append(sections, m.renderThinking())
	}

	// ── Plan 面板放在消息列表之后 ──
	if m.planActive || m.planCompleted {
		sections = append(sections, m.renderPlanPanel())
	}

	m.viewport.SetContent(lipgloss.JoinVertical(lipgloss.Left, sections...))
}

// contentWidth 返回 viewport 内容区宽度（减去 gutter）
func (m model) contentWidth() int {
	return max(10, m.viewport.Width()-gutterWidth)
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
				select {
				case toolCh <- toolEvent{Phase: "before", Name: toolName, Args: args}:
				default:
				}
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
			select {
			case toolCh <- toolEvent{
				Phase:    "after",
				Name:     toolName,
				IsError:  result.IsError,
				Duration: duration,
			}:
			default:
			}
		})
	}

	m := initialModel(ctx, manager, toolCh, confirmCh, mode, state, manager.DisposableWorker(), manager.GetWorker())

	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
