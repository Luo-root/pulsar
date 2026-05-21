package commands

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

/*
用户输入: /plan 创建一个计算器应用
         │
         ▼
    PlanRequestMsg
         │
         ▼
  handlePlanRequest ──── launchPlan ───→ goroutine
         │                                    │
         ▼                                    ▼
  planActive=true                    planner.RunPlan(ctx, goal, agent, emit)
  planPhase=planning                         │
         │                          ┌────────┴────────┐
         ▼                          ▼                 │
  View: spinner +                  Phase 1:           │
  "Generating plan..."             runPlannerPhase    │
         │                          │                 │
         │                          ▼                 │
         │◄──── PlanUpdateMsg ──── Plan{3 tasks}      │
         │                          │                 │
         ▼                          ▼                 │
  View: task list                  Phase 2:           │
  ○ task_1  wait                   runExecutionPhase  │
  ○ task_2  wait                   │                  │
  ○ task_3  wait                   ▼                  │
         │                  wf.AddNode(topo)          │
         │                  wf.Run()                  │
         │                          │                 │
         │◄──── PlanUpdateMsg ─────┤  (状态变更通知)    │
         ▼                          │                 │
  View: 更新状态                    │                   │
  ● task_1  run                     │                 │
  ○ task_2  wait                    │                 │
  ○ task_3  wait                    │                 │
         │                          ▼                 │
         │◄──── PlanUpdateMsg ──── 完成/失败           │
         ▼                          │                 │
  View: 最终状态                    ▼                   │
  ✓ task_1  done          PlanUpdateMsg(Done) ────────┘
  ✓ task_2  done                  │
  ✓ task_3  done                  ▼
         │               planActive=false
         ▼               planCompleted=true
  消息流: "计划执行完成"         │
                               ▼
                         继续正常对话
*/

// ── 命令 ────────────────────────────────────────────────────────────────

type PlanRequestMsg struct {
	Goal string
}

func NewPlanCommand() Command {
	return Command{
		Name:        "plan",
		Aliases:     []string{"p"},
		Description: "Plan and execute a goal",
		Usage:       "/plan <goal>",
		Handler: func(args string) tea.Cmd {
			return func() tea.Msg {
				return PlanRequestMsg{Goal: args}
			}
		},
	}
}

// ── 阶段枚举 ────────────────────────────────────────────────────────────

type PlanPhase string

const (
	PlanPhasePlanning  PlanPhase = "planning"
	PlanPhaseExecuting PlanPhase = "executing"
	PlanPhaseDone      PlanPhase = "done"
	PlanPhaseFailed    PlanPhase = "failed"
	PlanPhaseCancelled PlanPhase = "cancelled"
)

func (p PlanPhase) IsTerminal() bool {
	return p == PlanPhaseDone || p == PlanPhaseFailed || p == PlanPhaseCancelled
}

// ── 任务状态（与 node.TaskState 对齐）──────────────────────────────────

type PlanTaskState string

const (
	PlanTaskPending   PlanTaskState = "pending"
	PlanTaskRunning   PlanTaskState = "running"
	PlanTaskSuccess   PlanTaskState = "success"
	PlanTaskFailed    PlanTaskState = "failed"
	PlanTaskCancelled PlanTaskState = "cancelled"
)

// ── 视图模型（与 node 包解耦）───────────────────────────────────────────

type TaskView struct {
	ID          string
	Description string
	Inputs      []string
	Outputs     []string
	State       PlanTaskState
	Error       string
	Result      map[string]any
}

type PlanView struct {
	Goal  string
	Tasks []TaskView
}

// ── 更新消息 ────────────────────────────────────────────────────────────

type PlanUpdateMsg struct {
	Phase   PlanPhase
	Plan    *PlanView
	Message string
}

// ── 样式 ────────────────────────────────────────────────────────────────

var (
	planTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#CBA6F7"))

	planGoalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#CDD6F4")).
			Bold(true)

	planPhaseStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#89B4FA"))

	planBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#45475A")).
			Padding(0, 1)

	// Task block styles
	taskIDStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#89B4FA"))
	taskDescStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#BAC2DE")).Italic(true)
	taskTreeStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#45475A"))
	taskLabelStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#585B70"))
	taskValueStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#A6ADC8"))
	taskSuccessIcon = lipgloss.NewStyle().Foreground(lipgloss.Color("#A6E3A1"))
	taskFailIcon    = lipgloss.NewStyle().Foreground(lipgloss.Color("#F38BA8"))
	taskRunIcon     = lipgloss.NewStyle().Foreground(lipgloss.Color("#CBA6F7"))
	taskCancelIcon  = lipgloss.NewStyle().Foreground(lipgloss.Color("#6C7086"))
	taskPendingIcon = lipgloss.NewStyle().Foreground(lipgloss.Color("#6C7086"))
	taskSeparator   = lipgloss.NewStyle().Foreground(lipgloss.Color("#313244"))
)

// ── 状态渲染 ────────────────────────────────────────────────────────────

func taskStateIcon(s PlanTaskState) (string, lipgloss.Style) {
	switch s {
	case PlanTaskSuccess:
		return "✓", taskSuccessIcon
	case PlanTaskFailed:
		return "✗", taskFailIcon
	case PlanTaskRunning:
		frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		idx := int(time.Now().UnixMilli()/100) % len(frames)
		return frames[idx], taskRunIcon
	case PlanTaskCancelled:
		return "⊘", taskCancelIcon
	default:
		return "○", taskPendingIcon
	}
}

func taskStateLabel(s PlanTaskState) lipgloss.Style {
	switch s {
	case PlanTaskSuccess:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#A6E3A1"))
	case PlanTaskFailed:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#F38BA8"))
	case PlanTaskRunning:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#CBA6F7"))
	case PlanTaskCancelled:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#6C7086"))
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#6C7086"))
	}
}

func taskStateText(s PlanTaskState) string {
	switch s {
	case PlanTaskSuccess:
		return "done"
	case PlanTaskFailed:
		return "fail"
	case PlanTaskRunning:
		return "run"
	case PlanTaskCancelled:
		return "skip"
	default:
		return "wait"
	}
}

func phaseLabel(p PlanPhase) string {
	switch p {
	case PlanPhasePlanning:
		return "Planning"
	case PlanPhaseExecuting:
		return "Executing"
	case PlanPhaseDone:
		return "Completed"
	case PlanPhaseFailed:
		return "Failed"
	case PlanPhaseCancelled:
		return "Cancelled"
	}
	return string(p)
}

// ── 进度条 ──────────────────────────────────────────────────────────────

func renderProgressBar(done, fail, total, width int) string {
	if total == 0 {
		return ""
	}

	barW := width - 2
	if barW < 4 {
		barW = 4
	}

	fillDone := done * barW / total
	fillFail := fail * barW / total
	empty := barW - fillDone - fillFail

	return "[" +
		taskSuccessIcon.Render(strings.Repeat("█", fillDone)) +
		taskFailIcon.Render(strings.Repeat("█", fillFail)) +
		taskSeparator.Render(strings.Repeat("░", empty)) +
		"]"
}

// ── 任务块渲染（多行，含详情）──────────────────────────────────────────

func renderTaskBlock(t TaskView) string {
	var lines []string

	// ── 第一行：状态图标 + 任务 ID + 状态标签 ──
	icon, iconSt := taskStateIcon(t.State)
	stateSt := taskStateLabel(t.State)
	stateText := taskStateText(t.State)
	stateStr := stateSt.Width(8).Align(lipgloss.Right).Render(stateText)

	header := fmt.Sprintf("  %s  %s  %s",
		iconSt.Render(icon),
		taskIDStyle.Width(10).Render(t.ID),
		stateStr,
	)
	lines = append(lines, header)

	// ── 第二行：任务描述 ──
	lines = append(lines, "     "+taskDescStyle.Render(t.Description))

	// ── 详情行：输入 / 输出 ──
	type detailEntry struct {
		label string
		value string
	}
	var details []detailEntry

	if len(t.Inputs) > 0 {
		details = append(details, detailEntry{
			label: "inputs",
			value: strings.Join(t.Inputs, ", "),
		})
	}
	if len(t.Outputs) > 0 {
		details = append(details, detailEntry{
			label: "outputs",
			value: strings.Join(t.Outputs, ", "),
		})
	}

	for i, d := range details {
		isLast := i == len(details)-1
		connector := "├─"
		if isLast {
			connector = "└─"
		}
		line := fmt.Sprintf("     %s %-8s %s",
			taskTreeStyle.Render(connector),
			taskLabelStyle.Render(d.label),
			taskValueStyle.Render(d.value),
		)
		lines = append(lines, line)
	}

	// ── 错误行（仅失败时）──
	if t.State == PlanTaskFailed && t.Error != "" {
		errMsg := t.Error
		if len(errMsg) > 50 {
			errMsg = errMsg[:47] + "..."
		}
		lines = append(lines, taskFailIcon.Render("     ✗ "+errMsg))
	}

	return strings.Join(lines, "\n")
}

// ── 任务间的分隔线 ──────────────────────────────────────────────────────

func renderTaskSeparator(width int) string {
	// 用一段淡色虚线分隔任务块
	w := max(10, min(width, 50))
	return "     " + taskSeparator.Render(strings.Repeat("·", w))
}

// ── 主渲染入口 ──────────────────────────────────────────────────────────

// RenderPlan 渲染计划面板
func RenderPlan(msg PlanUpdateMsg, width int, elapsed time.Duration, tick int) string {
	var lines []string

	// ── 标题行 ──
	var timeStr string
	if elapsed < time.Minute {
		timeStr = fmt.Sprintf("%.1fs", elapsed.Seconds())
	} else {
		timeStr = fmt.Sprintf("%dm%02ds", int(elapsed.Minutes()), int(elapsed.Seconds())%60)
	}
	title := planTitleStyle.Render("Plan") + "  " +
		planPhaseStyle.Render(phaseLabel(msg.Phase)) + "  " +
		lipgloss.NewStyle().Foreground(lipgloss.Color("#45475A")).Render(timeStr)
	lines = append(lines, title)
	lines = append(lines, "")

	// ── 目标 ──
	if msg.Plan != nil && msg.Plan.Goal != "" {
		lines = append(lines, planGoalStyle.Render("Goal: ")+msg.Plan.Goal)
		lines = append(lines, "")
	}

	// ── 任务列表 ──
	if msg.Plan != nil && len(msg.Plan.Tasks) > 0 {
		tasks := msg.Plan.Tasks
		total := len(tasks)
		done, fail := 0, 0
		for _, t := range tasks {
			switch t.State {
			case PlanTaskSuccess:
				done++
			case PlanTaskFailed:
				fail++
			}
		}

		// 进度条
		barW := max(20, min(width-14, 40))
		progressBar := renderProgressBar(done, fail, total, barW)
		countStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6C7086"))
		lines = append(lines, fmt.Sprintf("  %s  %s",
			progressBar,
			countStyle.Render(fmt.Sprintf("%d/%d", done+fail, total)),
		))
		lines = append(lines, "")

		// 各任务块
		for i, t := range tasks {
			lines = append(lines, renderTaskBlock(t))
			if i < len(tasks)-1 {
				lines = append(lines, renderTaskSeparator(20))
			}
		}
	} else if msg.Phase == PlanPhasePlanning {
		// 规划中，还没出 plan
		frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		frame := frames[tick%len(frames)]
		lines = append(lines, fmt.Sprintf("  %s %s",
			planPhaseStyle.Render(frame),
			dimRender("Generating plan..."),
		))
	}

	// ── 状态消息 ──
	if msg.Message != "" {
		lines = append(lines, "")
		switch msg.Phase {
		case PlanPhaseFailed:
			lines = append(lines, taskFailIcon.Render("  ✗ "+msg.Message))
		case PlanPhaseDone:
			lines = append(lines, taskSuccessIcon.Render("  ✓ "+msg.Message))
		default:
			lines = append(lines, dimRender("  "+msg.Message))
		}
	}

	inner := strings.Join(lines, "\n")
	bw := min(width-2, 72)
	return planBorder.Width(bw).Render(inner)
}

func dimRender(s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#6C7086")).Render(s)
}
