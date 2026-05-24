package bootloader

import (
	"fmt"
	"net/url"
	"os"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/exp/charmtone"
)

// 定义选择步骤
type step int

const (
	stepAPI step = iota
	stepVendor
	stepConfig
	stepSelectModel
	stepDone
)

const (
	Openai    string = "openai"
	Anthropic string = "anthropic"
)

type model struct {
	currentStep step

	// API 风格选择
	cursor      int
	apiChoices  []string
	selectedAPI int

	// 模型厂商选择
	vendorCursor   int
	vendorChoices  []string
	selectedVendor int

	// 填写模型基础信息
	input         inputModel
	modelCursor   int
	modelChoices  []string
	selectedModel int
	isLoading     bool
	loadError     error

	// 配置保存状态
	configSaved bool
	configPath  string
	saveError   error

	choiceStyle  lipgloss.Style
	titleStyle   lipgloss.Style
	inputStyle   lipgloss.Style
	errStyle     lipgloss.Style
	helpStyle    lipgloss.Style
	successStyle lipgloss.Style
}

type inputModel struct {
	baseUrlInput textinput.Model
	apiKeyInput  textinput.Model
	focusedInput int
	err          error
}

func initialModel() model {
	baseUrlInput := textinput.New()
	baseUrlInput.Focus() // 初始聚焦
	baseUrlInput.Placeholder = "https://api.example.com/v1"
	baseUrlInput.SetWidth(30)
	baseUrlInput.Prompt = "" // 隐藏提示符
	baseUrlInput.Validate = func(s string) error {
		if s == "" {
			return nil // 空的时候不报错，等用户输完
		}
		if _, err := url.ParseRequestURI(s); err != nil {
			return fmt.Errorf("无效的 URL 格式")
		}
		return nil
	}

	apiKeyInput := textinput.New()
	apiKeyInput.Blur() // 初始不聚焦
	apiKeyInput.Placeholder = "sk-..."
	apiKeyInput.CharLimit = 100
	apiKeyInput.SetWidth(100)
	apiKeyInput.Prompt = ""
	apiKeyInput.EchoMode = textinput.EchoPassword
	apiKeyInput.Validate = func(s string) error {
		if s != "" && len(s) < 10 {
			return fmt.Errorf("密钥长度过短")
		}
		return nil
	}

	return model{
		currentStep: stepAPI,

		apiChoices:  []string{Openai, Anthropic},
		selectedAPI: -1,

		vendorChoices:  []string{Kimi, DeepSeek, XiaomiMimo},
		selectedVendor: -1,

		input: inputModel{
			baseUrlInput: baseUrlInput,
			apiKeyInput:  apiKeyInput,
			focusedInput: 0,
		},
		modelChoices:  make([]string, 0),
		selectedModel: -1,
		isLoading:     false,
		loadError:     nil,

		configSaved: false,
		configPath:  "",
		saveError:   nil,

		choiceStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color("2")),
		titleStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color("4")).Bold(true),
		inputStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color("6")),
		errStyle:     lipgloss.NewStyle().Foreground(lipgloss.Color(charmtone.Cherry.Hex())),
		successStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(charmtone.Guac.Hex())),
		helpStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
	}
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	// 关键：始终将消息传递给 textinput（支持粘贴）
	if m.currentStep == stepConfig {
		if m.input.focusedInput == 0 {
			m.input.baseUrlInput, cmd = m.input.baseUrlInput.Update(msg)
		} else {
			m.input.apiKeyInput, cmd = m.input.apiKeyInput.Update(msg)
		}
	}

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit

		case "up", "k":
			if m.currentStep == stepAPI {
				if m.cursor > 0 {
					m.cursor--
				}
			} else if m.currentStep == stepVendor {
				if m.vendorCursor > 0 {
					m.vendorCursor--
				}
			} else if m.currentStep == stepSelectModel {
				if m.modelCursor > 0 {
					m.modelCursor--
				}
			} else if m.currentStep == stepConfig {
				// 在配置页面切换输入框焦点
				if m.input.focusedInput == 1 {
					m.input.focusedInput = 0
					m.input.baseUrlInput.Focus()
					m.input.apiKeyInput.Blur()
				}
			}

		case "down", "j":
			if m.currentStep == stepAPI {
				if m.cursor < len(m.apiChoices)-1 {
					m.cursor++
				}
			} else if m.currentStep == stepVendor {
				if m.vendorCursor < len(m.vendorChoices)-1 {
					m.vendorCursor++
				}
			} else if m.currentStep == stepSelectModel {
				if m.modelCursor < len(m.modelChoices)-1 {
					m.modelCursor++
				}
			} else if m.currentStep == stepConfig {
				// 在配置页面切换输入框焦点
				if m.input.focusedInput == 0 {
					m.input.focusedInput = 1
					m.input.apiKeyInput.Focus()
					m.input.baseUrlInput.Blur()
				}
			}

		case "enter", "space":
			if m.currentStep == stepAPI {
				m.selectedAPI = m.cursor
				m.currentStep = stepVendor
				m.vendorCursor = 0
			} else if m.currentStep == stepVendor {
				m.selectedVendor = m.vendorCursor
				m.currentStep = stepConfig
				m.input.baseUrlInput.Focus()
				m.input.focusedInput = 0
			} else if m.currentStep == stepConfig {
				// 验证输入
				if !m.input.saveConfig() {
					return m, nil
				}
				// 开始加载模型列表
				m.currentStep = stepSelectModel
				m.modelCursor = 0

				// 使用异步命令获取模型列表
				vendor := m.vendorChoices[m.selectedVendor]
				apiKey := m.input.apiKeyInput.Value()
				return m, fetchModelsCmd(vendor, apiKey)
			} else if m.currentStep == stepSelectModel {
				if len(m.modelChoices) > 0 {
					m.selectedModel = m.modelCursor
					// 保存配置
					config := &Config{
						Version:   "1.0.0",
						CreatedAt: time.Now(),
						API: APIConfig{
							Style:   m.apiChoices[m.selectedAPI],
							BaseURL: m.input.baseUrlInput.Value(),
							APIKey:  m.input.apiKeyInput.Value(),
						},
						Model: ModelConfig{
							Vendor:  m.vendorChoices[m.selectedVendor],
							ModelID: m.modelChoices[m.selectedModel],
						},
					}

					// 获取默认配置路径
					configPath, err := GetDefaultConfigPath()
					if err != nil {
						m.configSaved = false
						m.saveError = err
					} else {
						m.configPath = configPath
						if err := SaveConfig(config, configPath); err != nil {
							m.configSaved = false
							m.saveError = err
						} else {
							m.configSaved = true
							m.saveError = nil
						}
					}

					m.currentStep = stepDone
				}
			} else if m.currentStep == stepDone {
				return m, tea.Quit
			}

		case "esc":
			if m.currentStep == stepVendor {
				m.currentStep = stepAPI
			} else if m.currentStep == stepConfig {
				m.currentStep = stepVendor
			} else if m.currentStep == stepSelectModel {
				m.currentStep = stepConfig
			} else if m.currentStep == stepDone {
				return m, tea.Quit
			}

		case "tab":
			if m.currentStep == stepConfig {
				if m.input.focusedInput == 0 {
					m.input.baseUrlInput.Blur()
					m.input.apiKeyInput.Focus()
					m.input.focusedInput = 1
				} else {
					m.input.apiKeyInput.Blur()
					m.input.baseUrlInput.Focus()
					m.input.focusedInput = 0
				}
			}
		}

	// 处理异步消息
	case modelsLoadedMsg:
		m.modelChoices = msg.models
		m.isLoading = false
		m.loadError = nil

	case modelsLoadErrMsg:
		m.isLoading = false
		m.loadError = msg.err
	}

	return m, cmd
}

// 添加异步命令类型
type modelsLoadedMsg struct {
	models []string
}

type modelsLoadErrMsg struct {
	err error
}

// 添加异步获取模型的命令
func fetchModelsCmd(vendor, apiKey string) tea.Cmd {
	return func() tea.Msg {
		models, err := getModelList(vendor, apiKey)
		if err != nil {
			return modelsLoadErrMsg{err: err}
		}
		return modelsLoadedMsg{models: models}
	}
}

func (im inputModel) saveConfig() bool {
	correctIsbnGiven := im.baseUrlInput.Err == nil && len(im.baseUrlInput.Value()) != 0
	correctTitleGiven := im.apiKeyInput.Err == nil && len(im.apiKeyInput.Value()) != 0

	return correctIsbnGiven && correctTitleGiven
}

func (m model) View() tea.View {
	var lines []string

	switch m.currentStep {
	case stepAPI:
		lines = append(lines, m.titleStyle.Render("步骤 1/4: 选择 API 风格"))
		lines = append(lines, "")

		for i, choice := range m.apiChoices {
			cursor := " "
			if m.cursor == i {
				cursor = ">"
			}

			line := fmt.Sprintf("%s [ ] %s", cursor, choice)
			lines = append(lines, line)
		}

		lines = append(lines, "")
		lines = append(lines, m.helpStyle.Render("↑↓ 选择 | Enter 确认 | q 退出"))

	case stepVendor:
		lines = append(lines, m.titleStyle.Render("步骤 2/4: 选择模型厂商"))
		lines = append(lines, "")

		apiInfo := fmt.Sprintf("API 风格: %s", m.apiChoices[m.selectedAPI])
		lines = append(lines, m.choiceStyle.Render(apiInfo))
		lines = append(lines, "")

		for i, choice := range m.vendorChoices {
			cursor := " "
			if m.vendorCursor == i {
				cursor = ">"
			}

			line := fmt.Sprintf("%s [ ] %s", cursor, choice)
			lines = append(lines, line)
		}

		lines = append(lines, "")
		lines = append(lines, m.helpStyle.Render("↑↓ 选择 | Enter 确认 | Esc 返回 | q 退出"))

	case stepConfig:
		lines = append(lines, m.titleStyle.Render("步骤 3/4: 配置 API"))
		lines = append(lines, "")

		vendorInfo := fmt.Sprintf("厂商: %s (%s)", m.vendorChoices[m.selectedVendor], m.apiChoices[m.selectedAPI])
		lines = append(lines, m.choiceStyle.Render(vendorInfo))
		lines = append(lines, "")

		// Base URL 输入
		baseUrlLabel := "请求地址 BaseURL:"
		if m.input.focusedInput == 0 {
			baseUrlLabel = m.inputStyle.Render("> 请求地址 BaseURL:")
		}
		lines = append(lines, baseUrlLabel)
		lines = append(lines, m.input.baseUrlInput.View())

		var baseUrlErrorText string
		if m.input.baseUrlInput.Value() != "" {
			if m.input.baseUrlInput.Err != nil {
				baseUrlErrorText = m.errStyle.Render(m.input.baseUrlInput.Err.Error())
			} else {
				baseUrlErrorText = m.choiceStyle.Render("✔")
			}
		}
		lines = append(lines, baseUrlErrorText)
		lines = append(lines, "")

		// API Key 输入
		apiKeyLabel := "密钥 Api-Key:"
		if m.input.focusedInput == 1 {
			apiKeyLabel = m.inputStyle.Render("> 密钥 Api-Key:")
		}
		lines = append(lines, apiKeyLabel)
		lines = append(lines, m.input.apiKeyInput.View())

		var apiKeyErrorText string
		if m.input.apiKeyInput.Value() != "" {
			if m.input.apiKeyInput.Err != nil {
				apiKeyErrorText = m.errStyle.Render(m.input.apiKeyInput.Err.Error())
			} else {
				apiKeyErrorText = m.choiceStyle.Render("✔")
			}
		}
		lines = append(lines, apiKeyErrorText)
		lines = append(lines, "")

		var continueText string
		if m.input.baseUrlInput.Value() != "" && m.input.apiKeyInput.Value() != "" {
			continueText = m.choiceStyle.Render("✓ 按 Enter 保存并获取模型列表")
		}
		lines = append(lines, continueText)
		lines = append(lines, "")
		lines = append(lines, m.helpStyle.Render("Tab 切换输入框 | Enter 确认 | Esc 返回"))

	case stepSelectModel:
		lines = append(lines, m.titleStyle.Render("步骤 4/4: 选择模型"))
		lines = append(lines, "")

		configInfo := fmt.Sprintf("API: %s | 厂商: %s", m.apiChoices[m.selectedAPI], m.vendorChoices[m.selectedVendor])
		lines = append(lines, m.choiceStyle.Render(configInfo))
		lines = append(lines, "")

		if m.isLoading {
			lines = append(lines, "⏳ 正在从 API 获取模型列表...")
			lines = append(lines, "")
			lines = append(lines, m.helpStyle.Render("请稍候..."))
		} else if m.loadError != nil {
			lines = append(lines, m.errStyle.Render("❌ 获取模型列表失败"))
			lines = append(lines, "")
			lines = append(lines, fmt.Sprintf("错误信息: %v", m.loadError))
			lines = append(lines, "")
			lines = append(lines, m.helpStyle.Render("按 Esc 返回重试"))
		} else {
			for i, modelID := range m.modelChoices {
				cursor := " "
				if m.modelCursor == i {
					cursor = ">"
				}

				line := fmt.Sprintf("%s [ ] %s", cursor, modelID)
				lines = append(lines, line)
			}

			lines = append(lines, "")
			lines = append(lines, m.helpStyle.Render(fmt.Sprintf("↑↓ 选择 (%d 个模型) | Enter 确认 | Esc 返回", len(m.modelChoices))))
		}

	case stepDone:
		lines = append(lines, m.titleStyle.Render("✅ 配置完成！"))
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("API 风格: %s", m.apiChoices[m.selectedAPI]))
		lines = append(lines, fmt.Sprintf("模型厂商: %s", m.vendorChoices[m.selectedVendor]))

		if len(m.modelChoices) > 0 && m.selectedModel >= 0 {
			lines = append(lines, fmt.Sprintf("选择模型: %s", m.modelChoices[m.selectedModel]))
		}

		lines = append(lines, fmt.Sprintf("Base URL: %s", m.input.baseUrlInput.Value()))
		apiKeyValue := m.input.apiKeyInput.Value()
		if len(apiKeyValue) > 8 {
			lines = append(lines, fmt.Sprintf("API Key: %s...", apiKeyValue[:8]))
		} else {
			lines = append(lines, fmt.Sprintf("API Key: %s", apiKeyValue))
		}
		lines = append(lines, "")
		// 显示保存状态
		if m.configSaved {
			saveInfo := fmt.Sprintf("💾 配置已保存至: %s", m.configPath)
			lines = append(lines, m.successStyle.Render(saveInfo))
		} else if m.saveError != nil {
			errorInfo := fmt.Sprintf("❌ 配置保存失败: %v", m.saveError)
			lines = append(lines, m.errStyle.Render(errorInfo))
		}

		lines = append(lines, m.helpStyle.Render("按任意键退出..."))
	}

	s := lipgloss.JoinVertical(lipgloss.Left, lines...)

	v := tea.NewView(s)
	v.WindowTitle = "Pulse - TUI"
	v.AltScreen = true

	// 只在配置页面显示光标
	if m.currentStep == stepConfig {
		if m.input.focusedInput == 0 {
			v.Cursor = m.input.baseUrlInput.Cursor()
		} else {
			v.Cursor = m.input.apiKeyInput.Cursor()
		}
	} else {
		v.Cursor = nil
	}

	return v
}

func BootLoader() {
	p := tea.NewProgram(initialModel())
	if _, err := p.Run(); err != nil {
		fmt.Printf("出了点问题呢: %v", err)
		os.Exit(1)
	}
}
