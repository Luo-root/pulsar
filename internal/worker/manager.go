package worker

import (
	"context"
	"fmt"
	"pulse-tui/internal/bootloader"

	"github.com/Luo-root/pulse/components/agent"
	"github.com/Luo-root/pulse/components/chatmodel"
	"github.com/Luo-root/pulse/components/mcp"
	"github.com/Luo-root/pulse/components/memory"
	"github.com/Luo-root/pulse/components/skill"
	"github.com/Luo-root/pulse/components/tools"
)

type Manager struct {
	ctx       context.Context
	config    *bootloader.Config
	registry  *tools.ToolRegistry
	mcp       *mcp.Manager
	skill     *skill.SkillLoader
	chatModel chatmodel.BaseModel
	embedder  memory.Embedder
	memory    *memory.Controller
	worker    *agent.Agent
}

func NewManager(ctx context.Context, config *bootloader.Config) (*Manager, error) {
	m := &Manager{ctx: ctx, config: config}

	err := m.InitToolRegistry()
	if err != nil {
		return nil, err
	}

	errs := m.InitMcpManager()
	if errs != nil {
		return nil, fmt.Errorf("%v", errs)
	}

	err = m.InitSkillLoader()
	if err != nil {
		return nil, err
	}

	err = m.InitChatModel()
	if err != nil {
		return nil, err
	}

	m.InitEmbedder()

	err = m.InitMemoryController()
	if err != nil {
		return nil, err
	}

	m.InitWorker()

	return m, nil
}

func (m *Manager) GetWorker() *agent.Agent {
	return m.worker
}

func (m *Manager) GetRegistry() *tools.ToolRegistry {
	return m.registry
}

func (m *Manager) GetMode() string {
	return m.config.Worker.Mode
}
