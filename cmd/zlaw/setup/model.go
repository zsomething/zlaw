package setup

import (
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbletea"
	"github.com/zsomething/zlaw/internal/config"
)

// Model is the root Bubble Tea model for the setup wizard.
type Model struct {
	state       *AppState
	screen      ScreenType
	screenStack []ScreenType // history for back navigation
	quit        bool
	cursor      int

	// Screen-specific state
	bootstrap *bootstrapState
	agent     *agentScreenState
	llm       *llmScreenState
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	m.screen = ScreenMainMenu
	return nil
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "Q", "ctrl+c":
			m.quit = true
			return m, tea.Quit
		}
	}

	switch m.screen {
	case ScreenMainMenu:
		return m.updateMainMenu(msg)
	case ScreenBootstrap:
		return m.updateBootstrap(msg)
	case ScreenAgentCreate:
		return m.updateAgentCreate(msg)
	case ScreenAgentConfig:
		return m.updateAgentConfig(msg)
	case ScreenLLMConfig:
		return m.updateLLMConfig(msg)
	case ScreenAdapterConfig:
		return m.updateAdapterConfig(msg)
	case ScreenIdentityEdit:
		return m.updateIdentityEdit(msg)
	case ScreenSoulEdit:
		return m.updateSoulEdit(msg)
	case ScreenSkills:
		return m.updateSkills(msg)
	case ScreenSecrets:
		return m.updateSecrets(msg)
	case ScreenSummary:
		return m.updateSummary(msg)
	}

	return m, nil
}

// View implements tea.Model.
func (m *Model) View() string {
	switch m.screen {
	case ScreenMainMenu:
		return m.viewMainMenu()
	case ScreenBootstrap:
		return m.viewBootstrap()
	case ScreenAgentCreate:
		return m.viewAgentCreate()
	case ScreenAgentConfig:
		return m.viewAgentConfig()
	case ScreenLLMConfig:
		return m.viewLLMConfig()
	case ScreenAdapterConfig:
		return m.viewAdapterConfig()
	case ScreenIdentityEdit:
		return m.viewIdentityEdit()
	case ScreenSoulEdit:
		return m.viewSoulEdit()
	case ScreenSkills:
		return m.viewSkills()
	case ScreenSecrets:
		return m.viewSecrets()
	case ScreenSummary:
		return m.viewSummary()
	default:
		return m.placeholder()
	}
}

// pushScreen adds current screen to history and navigates to new screen.
func (m *Model) pushScreen(s ScreenType) {
	m.screenStack = append(m.screenStack, m.screen)
	m.screen = s
	m.cursor = 0

	// Initialize screen-specific state
	switch s {
	case ScreenBootstrap:
		if m.bootstrap == nil {
			m.bootstrap = &bootstrapState{cursor: 0}
		}
	case ScreenAgentCreate:
		if m.agent == nil {
			ti := textinput.New()
			ti.Prompt = ""
			ti.Placeholder = "agent-id"
			ti.CharLimit = 32
			ti.Width = 30
			m.agent = &agentScreenState{agentID: ti, cursor: 0}
		}
		// Always focus textinput when entering this screen
		m.agent.agentID.Focus()
	case ScreenLLMConfig:
		if m.llm == nil {
			ti := textinput.New()
			ti.Placeholder = "ENV_VAR_NAME"
			ti.Focus()
			vi := textinput.New()
			secrets := config.ListSecrets()
			m.llm = &llmScreenState{
				cursor:           0,
				secretKeyInput:   ti,
				secretValueInput: vi,
				existingSecrets:  secrets,
			}
		}
	}
}

// popScreen returns to previous screen.
func (m *Model) popScreen() {
	if len(m.screenStack) > 0 {
		last := len(m.screenStack) - 1
		m.screen = m.screenStack[last]
		m.screenStack = m.screenStack[:last]
		m.cursor = 0
	} else {
		m.screen = ScreenMainMenu
	}
}

func (m *Model) placeholder() string {
	return "zlaw setup\n\n[Screen: " + m.screen.String() + "]\n\nPress Q to quit.\n"
}

// Screen state types.
type bootstrapState struct {
	cursor     int
	errMsg     string
	confirming bool
}

type agentScreenState struct {
	cursor  int
	agentID textinput.Model
	errMsg  string
}

type llmScreenState struct {
	cursor int
	errMsg string

	// Secret phase
	secretPhase       bool
	secretCursor      int
	selectedSecretIdx int
	secretKeyInput    textinput.Model
	secretValueInput  textinput.Model
	existingSecrets   []string
}
