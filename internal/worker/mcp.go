package worker

import (
	"github.com/Luo-root/pulse/components/mcp"
)

func (m *Manager) InitMcpManager() []error {
	mgr := mcp.NewManager(m.registry)
	configs, err := mcp.LoadConfig(m.config.Mcp.Path)
	if err != nil {
		return []error{err}
	}
	errs := mgr.ConnectAll(m.ctx, configs)
	if len(errs) != 0 {
		return errs
	}

	return nil
}
