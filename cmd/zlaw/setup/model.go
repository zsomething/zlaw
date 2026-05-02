package setup

import (
	"fmt"

	"github.com/charmbracelet/bubbletea"
)

// Model is the root Bubble Tea model for the setup wizard.
// It delegates to screen-specific update functions based on the current screen.
type Model struct {
	state  *State
	screen Screen
	quit   bool
	cursor int // cursor position in the current screen's item list

	// Screen-specific state.
	bootstrap *bootstrapState
	agent     *agentScreenState
	llm       *llmScreenState
	errMsg    string
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	// Default to main menu if not configured, otherwise bootstrap
	if m.state.IsConfigured() {
		m.screen = ScreenMainMenu
	} else {
		m.screen = ScreenBootstrap
	}
	return nil
}

// Update implements tea.Model by dispatching to the appropriate screen update.
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
		return updateMenu(m, msg)
	case ScreenBootstrap:
		return updateBootstrap(m, msg)
	case ScreenCreateAgent:
		return updateCreateAgent(m, msg)
	case ScreenDeleteAgent:
		return updateDeleteAgent(m, msg)
	case ScreenLLM:
		return updateLLM(m, msg)
	case ScreenLLMSecret:
		return updateLLMSecret(m, msg)
	case ScreenAdapter:
		return updateAdapter(m, msg)
	case ScreenAdapterSecret:
		return updateAdapterSecret(m, msg)
	case ScreenIdentity:
		return updateIdentity(m, msg)
	case ScreenSoul:
		return updateSoul(m, msg)
	case ScreenSkills:
		return updateSkills(m, msg)
	case ScreenSecrets:
		return updateSecrets(m, msg)
	case ScreenSummary:
		return updateSummary(m, msg)
	}

	return m, nil
}

// View implements tea.Model.
func (m *Model) View() string {
	switch m.screen {
	case ScreenMainMenu:
		return menuView(m)
	case ScreenBootstrap:
		return bootstrapView(m)
	case ScreenCreateAgent, ScreenDeleteAgent:
		return agentView(m)
	case ScreenLLM, ScreenLLMSecret:
		return llmView(m)
	default:
		return placeholderView(m)
	}
}

func placeholderView(m *Model) string {
	return fmt.Sprintf("zlaw setup\n\n[Screen: %s]\n\nPress Q to quit.\n", m.screen)
}
