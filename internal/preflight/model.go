package preflight

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// ── Messages ───────────────────────────────────────────────────────────

type checkDoneMsg struct {
	index  int
	status CheckStatus
}

type installProgressMsg struct {
	index int
	done  bool
	err   error
}

type preflightDoneMsg struct{}

// ── Model ──────────────────────────────────────────────────────────────

type Model struct {
	checks      []Check
	current     int // 当前正在处理的检查项索引
	autoInstall bool
	width       int
	done        bool
	failed      bool
	spinner     spinner
}

type spinner struct {
	frames []string
	index  int
}

func newSpinner() spinner {
	return spinner{
		frames: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
	}
}

func (s *spinner) Next() string {
	f := s.frames[s.index%len(s.frames)]
	s.index++
	return f
}

func New(autoInstall bool) *Model {
	return &Model{
		checks:      DefaultChecks(),
		autoInstall: autoInstall,
		spinner:     newSpinner(),
	}
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.tickSpinner(), m.runNextCheck())
}

// ── Commands ───────────────────────────────────────────────────────────

func (m *Model) tickSpinner() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg {
		return spinnerTickMsg{}
	})
}

type spinnerTickMsg struct{}

func (m *Model) runNextCheck() tea.Cmd {
	if m.current >= len(m.checks) {
		return func() tea.Msg {
			return preflightDoneMsg{}
		}
	}
	idx := m.current
	check := m.checks[idx]
	return func() tea.Msg {
		found := CheckBinary(check.Binary)
		status := StatusOK
		if !found {
			status = StatusMissing
		}
		return checkDoneMsg{index: idx, status: status}
	}
}

func (m *Model) runInstall(idx int) tea.Cmd {
	check := m.checks[idx]
	return func() tea.Msg {
		err := autoInstallBinary(check)
		if err != nil {
			return installProgressMsg{index: idx, done: true, err: err}
		}
		return installProgressMsg{index: idx, done: true, err: nil}
	}
}

// ── Update ─────────────────────────────────────────────────────────────

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinnerTickMsg:
		// 动画持续，直到所有检查完成
		if !m.done {
			return m, m.tickSpinner()
		}
		return m, nil

	case preflightDoneMsg:
		// 全部通过时写缓存
		if !m.failed {
			saveCache(m.checks)
		}
		return m, tea.Quit

	case checkDoneMsg:
		m.checks[msg.index].Status = msg.status

		if msg.status == StatusMissing {
			if m.autoInstall {
				// 自动安装
				m.checks[msg.index].Status = StatusInstalling
				return m, tea.Batch(m.tickSpinner(), m.runInstall(msg.index))
			}
			// 不自动安装，标记为 missing，继续下一个
			if m.checks[msg.index].Required {
				m.failed = true
			}
		}

		// 检查下一个
		m.current++
		if m.current >= len(m.checks) {
			if m.failed {
				m.done = true
				return m, nil
			}
			m.done = true
			return m, func() tea.Msg { return preflightDoneMsg{} }
		}
		return m, tea.Batch(m.tickSpinner(), m.runNextCheck())

	case installProgressMsg:
		if msg.err != nil {
			m.checks[msg.index].Status = StatusFailed
			if m.checks[msg.index].Required {
				m.failed = true
			}
		} else {
			m.checks[msg.index].Status = StatusInstalled
		}
		// 继续下一个
		m.current++
		if m.current >= len(m.checks) {
			m.done = true
			if m.failed {
				return m, nil
			}
			return m, func() tea.Msg { return preflightDoneMsg{} }
		}
		return m, tea.Batch(m.tickSpinner(), m.runNextCheck())

	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil
	}

	return m, nil
}

func (m *Model) Done() bool      { return m.done }
func (m *Model) Failed() bool    { return m.failed }
func (m *Model) AllPassed() bool { return m.done && !m.failed }
