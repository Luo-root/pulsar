package worker

import (
	"github.com/Luo-root/pulse/components/sandbox"
	"github.com/Luo-root/pulse/components/tools"
)

func (m *Manager) InitToolRegistry() error {
	m.registry = tools.NewToolRegistry()
	tools.RegisterEnvTools(m.registry)
	tools.RegisterWebTools(m.registry)
	sb := sandbox.NewProcessSandbox(sandbox.ProcessConfig{})
	err := sandbox.RegisterSandboxTools(m.registry, sb)
	if err != nil {
		return err
	}
	return nil
}
