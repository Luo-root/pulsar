package worker

import (
	"fmt"
	"pulse-tui/internal/bootloader"

	"github.com/Luo-root/pulse/components/chatmodel"
	"github.com/Luo-root/pulse/components/chatmodel/anthropic"
	"github.com/Luo-root/pulse/components/chatmodel/openai"
)

func (m *Manager) InitChatModel() error {
	switch m.config.API.Style {

	case bootloader.Openai:
		thinkingType := openai.Disabled
		if m.config.Model.Thinking {
			thinkingType = openai.Enabled
		}

		model, err := openai.NewChatModel(&openai.ChatModelConfig{
			BaseURL: m.config.API.BaseURL,
			APIKey:  m.config.API.APIKey,
			Model:   m.config.Model.ModelID,
			Thinking: openai.Thinking{
				Type: thinkingType,
			},
			Tools: m.registry.GetEnabledTools(),
		})
		if err != nil {
			return err
		}

		m.chatModel = model
		return nil

	case bootloader.Anthropic:
		thinkingType := anthropic.Disabled
		if m.config.Model.Thinking {
			thinkingType = anthropic.Enabled
		}

		model, err := anthropic.NewChatModel(&anthropic.ChatModelConfig{
			BaseURL: m.config.API.BaseURL,
			APIKey:  m.config.API.APIKey,
			Model:   m.config.Model.ModelID,
			Thinking: &anthropic.Thinking{
				Type:         thinkingType,
				BudgetTokens: 2000,
			},
			Tools: m.registry.GetEnabledTools(),
		})
		if err != nil {
			return err
		}

		m.chatModel = model
		return nil

	default:
		return fmt.Errorf("this API type does not exist")
	}
}

func (m *Manager) InitChatModelNoTools() chatmodel.BaseModel {
	switch m.config.API.Style {

	case bootloader.Openai:
		thinkingType := openai.Disabled
		if m.config.Model.Thinking {
			thinkingType = openai.Enabled
		}

		model, _ := openai.NewChatModel(&openai.ChatModelConfig{
			BaseURL: m.config.API.BaseURL,
			APIKey:  m.config.API.APIKey,
			Model:   m.config.Model.ModelID,
			Thinking: openai.Thinking{
				Type: thinkingType,
			},
		})

		return model

	case bootloader.Anthropic:
		thinkingType := anthropic.Disabled
		if m.config.Model.Thinking {
			thinkingType = anthropic.Enabled
		}

		model, _ := anthropic.NewChatModel(&anthropic.ChatModelConfig{
			BaseURL: m.config.API.BaseURL,
			APIKey:  m.config.API.APIKey,
			Model:   m.config.Model.ModelID,
			Thinking: &anthropic.Thinking{
				Type:         thinkingType,
				BudgetTokens: 2000,
			},
		})

		return model

	default:
		return nil
	}
}
