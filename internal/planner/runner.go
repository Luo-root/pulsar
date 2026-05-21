package planner

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/Luo-root/pulse/components/agent"
	"github.com/Luo-root/pulse/components/flowchart"
	"github.com/Luo-root/pulse/components/flowchart/node"
)

type RunnerPhase string

const (
	RunnerPlanning  RunnerPhase = "planning"
	RunnerExecuting RunnerPhase = "executing"
	RunnerDone      RunnerPhase = "done"
	RunnerFailed    RunnerPhase = "failed"
	RunnerCancelled RunnerPhase = "cancelled"
)

type RunnerEvent struct {
	Phase   RunnerPhase
	Plan    *node.Plan
	Message string
}

type EventCallback func(event RunnerEvent)

func RunPlan(
	ctx context.Context,
	goal string,
	planningAgent agent.AgentInterface, // 规划用：无工具，干净上下文
	taskAgent agent.AgentInterface, // 执行用：带工具循环 + hook
	emit EventCallback,
) {
	emit(RunnerEvent{Phase: RunnerPlanning, Message: "正在分析目标，生成执行计划..."})

	// Phase 1: 规划（用 planningAgent，无工具干扰）
	plan, err := runPlannerPhase(ctx, goal, planningAgent)
	if err != nil {
		emit(RunnerEvent{
			Phase:   RunnerFailed,
			Message: fmt.Sprintf("规划失败: %v", err),
		})
		return
	}

	emit(RunnerEvent{
		Phase:   RunnerExecuting,
		Plan:    plan,
		Message: fmt.Sprintf("计划已生成，共 %d 个任务，开始执行...", len(plan.Tasks)),
	})

	// Phase 2: 执行（用 taskAgent，带工具循环 + hook）
	err = runExecutionPhase(ctx, "planner", plan, taskAgent, emit)
	if err != nil {
		if ctx.Err() != nil {
			emit(RunnerEvent{
				Phase:   RunnerCancelled,
				Plan:    plan,
				Message: "计划已取消",
			})
			return
		}
		emit(RunnerEvent{
			Phase:   RunnerFailed,
			Plan:    plan,
			Message: fmt.Sprintf("执行失败: %v", err),
		})
		return
	}

	emit(RunnerEvent{
		Phase:   RunnerDone,
		Plan:    plan,
		Message: "计划执行完成",
	})
}

func runPlannerPhase(ctx context.Context, goal string, planAgent agent.AgentInterface) (*node.Plan, error) {
	wf, err := flowchart.NewWorkflow(ctx, 4)
	if err != nil {
		return nil, fmt.Errorf("create planner workflow: %w", err)
	}
	defer wf.Close()

	wf.Input("user_goal", goal)

	plannerNode := node.NewPlannerNode("planner", planAgent)
	if err := wf.AddNode(plannerNode); err != nil {
		return nil, fmt.Errorf("add planner node: %w", err)
	}

	if err := wf.Run(nil); err != nil {
		return nil, fmt.Errorf("planner workflow: %w", err)
	}

	val, err := wf.Get("planner_plan")
	if err != nil {
		return nil, fmt.Errorf("get plan: %w", err)
	}

	plan, ok := val.(*node.Plan)
	if !ok || plan == nil {
		return nil, fmt.Errorf("plan is nil or wrong type: %T", val)
	}

	if len(plan.Tasks) == 0 {
		return nil, fmt.Errorf("plan has no tasks")
	}

	return plan, nil
}

func runExecutionPhase(
	ctx context.Context,
	plannerNodeID string,
	plan *node.Plan,
	planAgent agent.AgentInterface,
	emit EventCallback,
) error {
	const maxRetries = 3

	for attempt := 0; attempt < maxRetries; attempt++ {
		// 为 emitPlanEvents 创建子 context，确保返回前 goroutine 退出
		emitCtx, emitCancel := context.WithCancel(ctx)
		var emitWg sync.WaitGroup
		emitWg.Add(1)
		go func() {
			defer emitWg.Done()
			emitPlanEvents(emitCtx, plan, emit)
		}()

		err := runSingleAttempt(ctx, plannerNodeID, plan, planAgent)

		// ★ 先取消 emit 并等待退出，再处理结果
		emitCancel()
		emitWg.Wait()

		if err == nil {
			// ★ 关键修复：wfErr==nil 不代表所有 task 成功（软失败不产生 wfErr）
			if failedTask := plan.FindFailedTask(); failedTask != nil {
				err = fmt.Errorf("task %s failed: %s", failedTask.ID, failedTask.Error)
			}
		}

		if err == nil {
			return nil // 全部成功
		}

		// Context 已取消，不再重规划
		if ctx.Err() != nil {
			return err
		}

		// 无失败任务但 workflow 报错（不应该到这里，但防御性处理）
		failedTask := plan.FindFailedTask()
		if failedTask == nil {
			return err
		}

		// 达到最大重试次数
		if attempt >= maxRetries-1 {
			return fmt.Errorf("max retries (%d) reached, last failed task: %s - %s",
				maxRetries, failedTask.ID, failedTask.Error)
		}

		// 重规划
		log.Printf("[planner] attempt %d: task %s failed, replanning...", attempt+1, failedTask.ID)
		emit(RunnerEvent{
			Phase:   RunnerPlanning,
			Plan:    plan,
			Message: fmt.Sprintf("任务 %s 失败，正在重规划... (%d/%d)", failedTask.ID, attempt+1, maxRetries),
		})

		newPlan, replanErr := node.RePlan(ctx, plan, failedTask, planAgent)
		if replanErr != nil {
			return fmt.Errorf("replan failed: %w", replanErr)
		}
		plan = newPlan

		emit(RunnerEvent{
			Phase:   RunnerExecuting,
			Plan:    plan,
			Message: fmt.Sprintf("重规划完成，继续执行... (%d/%d)", attempt+1, maxRetries),
		})
	}

	return fmt.Errorf("max retries (%d) reached", maxRetries)
}

// runSingleAttempt 构建 workflow 并执行一轮
func runSingleAttempt(
	ctx context.Context,
	plannerNodeID string,
	plan *node.Plan,
	planAgent agent.AgentInterface,
) error {
	wf, err := flowchart.NewWorkflow(ctx, 8)
	if err != nil {
		return fmt.Errorf("create execution workflow: %w", err)
	}
	defer wf.Close()

	planName := fmt.Sprintf("%s_plan", plannerNodeID)
	wf.Input(planName, plan)

	taskNodes := node.BatchNewTaskNode(plannerNodeID, plan, planAgent)
	if len(taskNodes) == 0 {
		return fmt.Errorf("no task nodes created from plan")
	}

	// 收集所有 TaskNode 的输出 key
	var outputKeys []string
	for _, t := range plan.Tasks {
		outputKeys = append(outputKeys, t.Outputs...)
	}

	// 尝试用 TopologicalNode 编排
	topo, topoErr := node.NewTopologicalNode("task_exec", toNodes(taskNodes), uniqueStrings(outputKeys))

	if topoErr == nil {
		log.Printf("[planner] using TopologicalNode: %d layers", len(topo.GetLayers()))
		if err := wf.AddNode(topo); err != nil {
			return fmt.Errorf("add topological node: %w", err)
		}
	} else {
		log.Printf("[planner] TopologicalNode not applicable (%v), adding nodes directly", topoErr)
		for _, n := range taskNodes {
			if err := wf.AddNode(n); err != nil {
				return fmt.Errorf("add task node %s: %w", n.ID(), err)
			}
		}
	}

	return wf.Run(nil)
}

func emitPlanEvents(ctx context.Context, plan *node.Plan, emit EventCallback) {
	stateCh := plan.GetStateChannel()
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-stateCh:
			if !ok {
				return
			}
			emit(RunnerEvent{
				Phase: RunnerExecuting,
				Plan:  plan,
			})
		}
	}
}

func toNodes(sn []*node.SimpleNode) []node.Node {
	r := make([]node.Node, len(sn))
	for i, n := range sn {
		r[i] = n
	}
	return r
}

func uniqueStrings(ss []string) []string {
	seen := make(map[string]bool)
	var r []string
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			r = append(r, s)
		}
	}
	return r
}
