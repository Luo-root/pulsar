package worker

import "github.com/Luo-root/pulse/components/memory"

func (m *Manager) InitEmbedder() {
	m.embedder = memory.NewOllamaEmbedder(m.config.Embedding.BaseURL, m.config.Embedding.ModelID)
}
