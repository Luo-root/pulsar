package worker

import (
	"github.com/Luo-root/pulse/components/agent"
)

func (m *Manager) InitWorker() {
	m.worker = agent.NewAgent(
		m.chatModel,
		m.registry,
		agent.WithUsageTracker(agent.NewUsageTracker()),
		agent.WithMemoryController(m.memory),
		agent.WithSessionID(m.config.Worker.SessionID),
		agent.WithMaxToolRounds(m.config.Worker.MaxToolRounds),
	)
}

func (m *Manager) DisposableWorker() *agent.Agent {
	return agent.NewAgent(
		m.InitChatModelNoTools(),
		nil,
		agent.WithUsageTracker(m.worker.GetUsageTracker()),
		agent.WithMaxToolRounds(m.config.Worker.MaxToolRounds),
	)
}
