package worker

import (
	"github.com/Luo-root/pulse/components/memory"
	"github.com/Luo-root/pulse/components/schema"
	"github.com/Luo-root/pulse/components/tools"
	"gorm.io/gorm/logger"
)

func (m *Manager) InitMemoryController() error {
	wm := memory.NewWindowManager(
		memory.WindowConfig{
			MaxHistoryMessages: m.config.Memory.MaxHistoryMessages,
			ReserveTokens:      m.config.Memory.ReserveTokens,
		},
		m.chatModel,
		nil,
	)

	sm := memory.NewWindowShortMemory(wm, m.chatModel, memory.DefaultSummaryFunc())

	lgc := memory.DefaultGormStoreConfig()
	lgc.DBPath = m.config.Memory.Path
	lgc.EmbeddingDimension = m.config.Embedding.EmbeddingDimension
	lgc.ChunkSize = m.config.Embedding.ChunkSize
	lgc.LogLevel = logger.Error

	lm, err := memory.NewGormStore(lgc, m.embedder.Embed)
	if err != nil {
		return err
	}

	systemPrompt, err := tools.NewPromptLoader(m.config.Memory.PromptPath).LoadAllDefaultPrompt()
	if err != nil {
		return err
	}

	m.memory = memory.NewController([]*schema.Message{schema.SystemMessage(systemPrompt)}, sm, lm)
	return nil
}
