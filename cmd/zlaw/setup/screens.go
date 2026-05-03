package setup

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbletea"

	"github.com/zsomething/zlaw/internal/config"
	"github.com/zsomething/zlaw/internal/llm"
)

// LLM presets.
var llmPresets = []string{
	"minimax",
	"anthropic",
	"openai",
	"openrouter",
	"ollama",
}

// Env vars required by each LLM preset.
var llmEnvVars = map[string]string{
	"minimax":    "MINIMAX_API_KEY",
	"anthropic":  "ANTHROPIC_API_KEY",
	"openai":     "OPENAI_API_KEY",
	"openrouter": "OPENROUTER_API_KEY",
	"ollama":     "", // no env var needed for local
}

// newAgentTextInput creates a fresh textinput for agent ID.
func newAgentTextInput() textinput.Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = "agent-id"
	ti.CharLimit = 32
	ti.Width = 30
	ti.Focus()
	return ti
}

// === Bootstrap ===

func (m *Model) viewBootstrap() string {
	var b strings.Builder
	b.WriteString(headerView("Bootstrap Zlaw Home"))
	b.WriteString("\n\n")

	// Path display
	b.WriteString("Target path:\n\n")
	b.WriteString(Styles.Item.Render("  "+m.state.HomePath) + "\n")
	if m.state.EnvVarSet {
		b.WriteString(Styles.ItemDim.Render("  From ZLAW_HOME env var") + "\n")
	}
	b.WriteString("\n")

	// Check if showing confirmation dialog
	if m.bootstrap.confirming {
		b.WriteString(m.viewConfirmReset())
		b.WriteString("\n\n" + divider())
		b.WriteString(footer("[Enter] Yes   [N] No, Cancel   [Q] Quit"))
		return b.String()
	}

	// Status display when bootstrapped
	if m.state.BootstrapStatus == BootstrapReady {
		b.WriteString(Styles.StatusOK.Render("✓ Already configured") + "\n\n")
		b.WriteString("What would you like to do?\n\n")
		b.WriteString(m.option("Reset", m.bootstrap.cursor == MenuItem0) + "\n")
		b.WriteString(m.option("Keep", m.bootstrap.cursor == MenuItem1) + "\n")
		b.WriteString(m.option("Cancel", m.bootstrap.cursor == MenuItem2))
	} else {
		// Not bootstrapped or incomplete
		statusText, statusStyle := bootstrapStatusText(m.state.BootstrapStatus)
		b.WriteString(statusStyle.Render(statusText) + "\n\n")
		b.WriteString(m.option("Create", m.bootstrap.cursor == MenuItem0) + "\n")
		b.WriteString(m.option("Cancel", m.bootstrap.cursor == MenuItem1))
	}

	// Error message
	if m.bootstrap.errMsg != "" {
		b.WriteString("\n\n")
		b.WriteString(Styles.StatusErr.Render("⚠ " + m.bootstrap.errMsg))
	}

	b.WriteString("\n\n" + divider())
	b.WriteString(footer("[↑↓] Navigate   [Enter] Select   [←] Back   [Q] Quit"))
	return b.String()
}

func (m *Model) viewConfirmReset() string {
	var b strings.Builder
	b.WriteString(Styles.StatusWarn.Render("⚠️ Reset Bootstrap?") + "\n\n")
	b.WriteString("This will recreate the zlaw home structure.\n")
	b.WriteString("Existing files will be preserved where possible.\n\n")
	b.WriteString(m.option("Yes, Reset", m.bootstrap.cursor == MenuItem0) + "\n")
	b.WriteString(m.option("No, Cancel", m.bootstrap.cursor == MenuItem1))
	return b.String()
}

func (m *Model) updateBootstrap(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Determine max cursor based on state
	maxCursor := MenuItem1 // Create, Cancel
	if m.state.BootstrapStatus == BootstrapReady {
		maxCursor = MenuItem2 // Reset, Keep, Cancel
	}

	// Handle confirmation dialog mode
	if m.bootstrap.confirming {
		return m.updateBootstrapConfirm(msg)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up":
			if m.bootstrap.cursor > MenuItem0 {
				m.bootstrap.cursor--
			}
		case "down":
			if m.bootstrap.cursor < maxCursor {
				m.bootstrap.cursor++
			}
		case "enter":
			m.bootstrap.errMsg = ""
			if m.state.BootstrapStatus == BootstrapReady {
				switch m.bootstrap.cursor {
				case MenuItem0: // Reset - show confirmation
					m.bootstrap.confirming = true
					m.bootstrap.cursor = 0
					return m, nil
				case MenuItem1: // Keep
					m.popScreen()
				case MenuItem2: // Cancel
					m.popScreen()
				}
			} else {
				switch m.bootstrap.cursor {
				case MenuItem0: // Create
					cfg := config.BootstrapConfig{
						Home:  m.state.HomePath,
						Force: m.state.BootstrapStatus == BootstrapIncomplete,
					}
					if err := cfg.CreateZlawHome(); err != nil {
						m.bootstrap.errMsg = err.Error()
						return m, nil
					}
					m.state.BootstrapStatus = BootstrapReady
					m.popScreen()
					return m, nil
				case MenuItem1: // Cancel
					m.popScreen()
				}
			}
		case "escape", "left":
			m.popScreen()
		}
	}
	return m, nil
}

func (m *Model) updateBootstrapConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up":
			if m.bootstrap.cursor > MenuItem0 {
				m.bootstrap.cursor--
			}
		case "down":
			if m.bootstrap.cursor < MenuItem1 {
				m.bootstrap.cursor++
			}
		case "enter":
			if m.bootstrap.cursor == MenuItem0 {
				cfg := config.BootstrapConfig{
					Home:  m.state.HomePath,
					Force: true,
				}
				if err := cfg.CreateZlawHome(); err != nil {
					m.bootstrap.confirming = false
					m.bootstrap.errMsg = err.Error()
					return m, nil
				}
				m.bootstrap.confirming = false
				m.state.BootstrapStatus = BootstrapReady
				m.popScreen()
				return m, nil
			} else {
				m.bootstrap.confirming = false
				m.bootstrap.cursor = 0
			}
		case "n", "N", "escape", "left":
			m.bootstrap.confirming = false
			m.bootstrap.cursor = 0
		}
	}
	return m, nil
}

// === Agent Create ===

func (m *Model) viewAgentCreate() string {
	var b strings.Builder
	b.WriteString(headerView("Create Agent"))
	b.WriteString("\n\n")

	b.WriteString(Styles.Item.Render("Agent ID") + "\n")
	b.WriteString(m.agent.agentID.View() + "\n\n")

	b.WriteString(Styles.Item.Render("Executor") + "\n")
	b.WriteString(m.option("subprocess", m.agent.cursor == MenuItem1) + "\n\n")

	b.WriteString(Styles.Item.Render("Target") + "\n")
	b.WriteString(m.option("local", m.agent.cursor == MenuItem2) + "\n\n")

	b.WriteString(Styles.Item.Render("Restart Policy") + "\n")
	b.WriteString(m.option("on-failure", m.agent.cursor == MenuItem3) + "\n\n")

	if m.agent.errMsg != "" {
		b.WriteString(Styles.StatusErr.Render("⚠ "+m.agent.errMsg) + "\n\n")
	}

	b.WriteString(m.option("Create Agent", m.agent.cursor == MenuItem4) + "   ")
	b.WriteString(m.option("Cancel", m.agent.cursor == MenuItem5))

	b.WriteString("\n\n" + divider())
	b.WriteString(footer("[↑↓] Navigate   [Tab] Next   [Enter] Select   [←] Back   [Q] Quit"))
	return b.String()
}

func (m *Model) updateAgentCreate(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Don't update textinput on navigation/action keys
		keyStr := msg.String()
		if keyStr != "up" && keyStr != "down" &&
			keyStr != "left" && keyStr != "escape" &&
			keyStr != "enter" && keyStr != "tab" && keyStr != "q" && keyStr != "Q" {
			var cmd tea.Cmd
			m.agent.agentID, cmd = m.agent.agentID.Update(msg)
			if cmd != nil {
				return m, cmd
			}
		}

		switch keyStr {
		case "up":
			if m.agent.cursor > MenuItem0 {
				m.agent.cursor--
			}
		case "down":
			if m.agent.cursor < MenuItem5 {
				m.agent.cursor++
			}
		case "tab":
			m.agent.cursor = (m.agent.cursor + 1) % 6
		case "enter":
			m.agent.errMsg = ""
			switch m.agent.cursor {
			case MenuItem4: // Create
				agentID := strings.TrimSpace(m.agent.agentID.Value())
				if agentID == "" {
					m.agent.errMsg = "Agent ID is required"
					return m, nil
				}
				if err := addAgentToHub(agentID); err != nil {
					m.agent.errMsg = err.Error()
					return m, nil
				}
				cfg := config.SetupAgentConfig{
					ID:   agentID,
					Home: m.state.HomePath,
				}
				if err := cfg.CreateAgent(); err != nil {
					m.agent.errMsg = err.Error()
					return m, nil
				}
				// Update state and return to main menu
				m.state.Agents = append(m.state.Agents, agentID)
				m.state.SelectedAgent = agentID
				m.clearAgentState() // Clean up agent screen state
				m.screen = ScreenMainMenu
				m.screenStack = nil // Clear stack to go directly to main
				m.cursor = 0
			case 5: // Cancel
				m.popScreen()
			default:
				m.agent.cursor = 4
			}
		case "escape", "left":
			m.popScreen()
		}
	}
	return m, nil
}

func addAgentToHub(agentID string) error {
	hub, err := config.LoadHubConfig(config.DefaultHubConfigPath())
	if err != nil {
		return fmt.Errorf("load hub config: %w", err)
	}
	// Check for duplicate ID in zlaw.toml (source of truth, not agent homes)
	if _, found := hub.FindAgent(agentID); found {
		return fmt.Errorf("agent %q already exists", agentID)
	}
	hub.Agents = append(hub.Agents, config.AgentEntry{
		ID: agentID,
	})
	if err := hub.Save(); err != nil {
		return fmt.Errorf("save hub: %w", err)
	}
	return nil
}

// === Agent Config ===

func (m *Model) viewAgentConfig() string {
	var b strings.Builder
	b.WriteString(headerView("Select Agent"))
	b.WriteString("\n\n")

	for _, name := range m.state.Agents {
		prefix := "  "
		style := Styles.Item
		if name == m.state.SelectedAgent {
			prefix = "▸ "
			style = Styles.ItemSelected
		}
		b.WriteString(style.Render(prefix+name) + "\n")
	}

	b.WriteString("\n" + divider())
	b.WriteString(footer("[↑↓] Select   [Enter] Confirm   [←] Back   [Q] Quit"))
	return b.String()
}

func (m *Model) updateAgentConfig(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Sync cursor with selected agent
	for i, name := range m.state.Agents {
		if name == m.state.SelectedAgent {
			m.cursor = i
			break
		}
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up":
			if m.cursor > 0 {
				m.cursor--
				m.state.SelectedAgent = m.state.Agents[m.cursor]
			}
		case "down":
			if m.cursor < len(m.state.Agents)-1 {
				m.cursor++
				m.state.SelectedAgent = m.state.Agents[m.cursor]
			}
		case "enter", "escape", "left":
			m.popScreen()
		}
	}
	return m, nil
}

// === LLM Config ===

func (m *Model) viewLLMConfig() string {
	if m.llm.secretPhase {
		return m.viewLLMSecretPhase()
	}
	return m.viewLLMConfigPhase1()
}

func (m *Model) viewLLMConfigPhase1() string {
	var b strings.Builder
	b.WriteString(headerView("Configure LLM — " + m.state.SelectedAgent))
	b.WriteString("\n\n")

	b.WriteString("Select LLM preset:\n\n")
	for i, name := range llmPresets {
		prefix := "  "
		style := Styles.Item
		if m.llm.cursor == MenuCursor(i) {
			prefix = "▸ "
			style = Styles.ItemSelected
		}
		envVar := llmEnvVars[name]
		if envVar != "" {
			b.WriteString(style.Render(prefix+name) + "  " + Styles.ItemDim.Render("("+envVar+")") + "\n")
		} else {
			b.WriteString(style.Render(prefix+name) + "\n")
		}
	}

	if m.llm.errMsg != "" {
		b.WriteString("\n")
		b.WriteString(Styles.StatusErr.Render("⚠ " + m.llm.errMsg))
	}

	b.WriteString("\n" + divider())
	b.WriteString(footer("[↑↓] Navigate   [Enter] Select   [←] Back   [Q] Quit"))
	return b.String()
}

func (m *Model) viewLLMSecretPhase() string {
	var b strings.Builder
	b.WriteString(headerView("Configure LLM — " + m.state.SelectedAgent))
	b.WriteString("\n\n")

	presetName := llmPresets[m.llm.cursor]
	envVarName := llmEnvVars[presetName]

	if envVarName == "" {
		b.WriteString("No secret required for " + presetName + ".\n\n")
		b.WriteString(m.option("Configure", true) + "   ")
		b.WriteString(m.option("Back", false) + "\n\n")
		b.WriteString(divider())
		b.WriteString(footer("[Enter] Configure   [←] Back   [Q] Quit"))
		return b.String()
	}

	b.WriteString(envVarName + " is required\n\n")

	if len(m.llm.existingSecrets) > 0 {
		b.WriteString("Use existing secret:\n")
		selectedSecret := "(none)"
		if m.llm.selectedSecretIdx >= 0 && m.llm.selectedSecretIdx < len(m.llm.existingSecrets) {
			selectedSecret = m.llm.existingSecrets[m.llm.selectedSecretIdx]
		}
		prefix := "  "
		if m.llm.secretCursor == SecretCursorDropdown {
			prefix = "▸ "
		}
		b.WriteString(Styles.Item.Render(prefix+"["+selectedSecret+" ▼]") + "\n")
		b.WriteString(m.option("Use Secret", m.llm.secretCursor == SecretCursorUseSecret) + "\n\n")
		b.WriteString(Styles.ItemDim.Render(strings.Repeat("─", 40)) + "\n\n")
	}
	b.WriteString("Or create new secret:\n\n")
	b.WriteString("Key\n")
	prefix := "  "
	if m.llm.secretCursor == SecretCursorKey {
		prefix = "▸ "
	}
	b.WriteString(Styles.Item.Render(prefix))
	b.WriteString(m.llm.secretKeyInput.View())
	b.WriteString("\n\n")

	b.WriteString("Value\n")
	prefix = "  "
	if m.llm.secretCursor == SecretCursorValue {
		prefix = "▸ "
	}
	b.WriteString(Styles.Item.Render(prefix))
	b.WriteString(m.llm.secretValueInput.View())
	b.WriteString("\n\n")

	b.WriteString(m.option("Create Secret", m.llm.secretCursor == SecretCursorCreate))

	if m.llm.errMsg != "" {
		b.WriteString("\n\n")
		b.WriteString(Styles.StatusErr.Render("⚠ " + m.llm.errMsg))
	}

	b.WriteString("\n\n" + divider())
	b.WriteString(footer("[↑↓] Navigate   [Tab] Next   [Enter] Select   [←] Back   [Q] Quit"))
	return b.String()
}

func (m *Model) updateLLMConfig(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.llm.secretPhase {
		return m.updateLLMSecretPhase(msg)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up":
			if m.llm.cursor > 0 {
				m.llm.cursor--
			}
		case "down":
			if m.llm.cursor < MenuCursor(len(llmPresets)-1) {
				m.llm.cursor++
			}
		case "enter":
			m.llm.errMsg = ""
			presetName := llmPresets[m.llm.cursor]
			envVarName := llmEnvVars[presetName]

			if envVarName == "" {
				if err := m.configureLLMWithPreset(presetName, "", ""); err != nil {
					m.llm.errMsg = err.Error()
					return m, nil
				}
				m.state.LLMStatus = StateConfigured
				m.popScreen()
				return m, nil
			}

			// Enter secret phase
			m.llm.secretPhase = true
			m.llm.secretCursor = SecretCursorKey
			m.llm.selectedSecretIdx = -1
			m.llm.secretKeyInput.Placeholder = envVarName
			m.llm.secretKeyInput.SetValue("")
			m.llm.secretValueInput.SetValue("")
			m.llm.secretKeyInput.Focus()
		case "escape", "left":
			m.popScreen()
		}
	}
	return m, nil
}

func (m *Model) updateLLMSecretPhase(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Only update textinput on character input, not navigation keys
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		keyStr := keyMsg.String()
		// Don't update textinput on navigation or action keys
		if keyStr != "up" && keyStr != "down" &&
			keyStr != "left" && keyStr != "escape" &&
			keyStr != "enter" && keyStr != "tab" && keyStr != "q" && keyStr != "Q" {
			if m.llm.secretCursor == SecretCursorKey {
				m.llm.secretKeyInput, _ = m.llm.secretKeyInput.Update(msg)
			} else if m.llm.secretCursor == SecretCursorValue {
				m.llm.secretValueInput, _ = m.llm.secretValueInput.Update(msg)
			}
		}
	}

	presetName := llmPresets[m.llm.cursor]
	envVarName := llmEnvVars[presetName]
	noSecret := envVarName == ""

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up":
			if m.llm.secretCursor > SecretCursorDropdown {
				m.llm.secretCursor--
			}
		case "down":
			if len(m.llm.existingSecrets) == 0 {
				// No existing secrets: skip dropdown (cursor 0), navigate Key->Value->Create
				maxCursor := SecretCursorCreate
				if m.llm.secretCursor >= SecretCursorUseSecret && m.llm.secretCursor < maxCursor {
					m.llm.secretCursor++
				}
			} else {
				if m.llm.secretCursor < SecretCursorCreate {
					m.llm.secretCursor++
				}
			}
		case "left", "escape":
			m.llm.secretPhase = false
			m.llm.secretCursor = SecretCursorDropdown
		case "enter":
			m.llm.errMsg = ""
			if noSecret {
				if err := m.configureLLMWithPreset(presetName, "", ""); err != nil {
					m.llm.errMsg = err.Error()
					return m, nil
				}
				m.state.LLMStatus = StateConfigured
				m.popScreen()
				return m, nil
			}

			switch m.llm.secretCursor {
			case SecretCursorDropdown:
				if len(m.llm.existingSecrets) > 0 {
					m.llm.selectedSecretIdx = (m.llm.selectedSecretIdx + 1) % (len(m.llm.existingSecrets) + 1)
					if m.llm.selectedSecretIdx == len(m.llm.existingSecrets) {
						m.llm.selectedSecretIdx = -1
					}
				}
			case SecretCursorUseSecret:
				if m.llm.selectedSecretIdx >= 0 {
					secretName := m.llm.existingSecrets[m.llm.selectedSecretIdx]
					if err := m.configureLLMWithPreset(presetName, envVarName, secretName); err != nil {
						m.llm.errMsg = err.Error()
						return m, nil
					}
					m.state.LLMStatus = StateConfigured
					m.popScreen()
				}
			case SecretCursorKey:
				m.llm.secretKeyInput.Focus()
			case SecretCursorValue:
				m.llm.secretValueInput.Focus()
			case SecretCursorCreate:
				key := m.llm.secretKeyInput.Value()
				value := m.llm.secretValueInput.Value()
				if key == "" || value == "" {
					m.llm.errMsg = "Key and value are required"
					return m, nil
				}
				if err := config.AddSecret(key, value); err != nil {
					m.llm.errMsg = err.Error()
					return m, nil
				}
				if err := m.configureLLMWithPreset(presetName, key, key); err != nil {
					m.llm.errMsg = err.Error()
					return m, nil
				}
				m.state.LLMStatus = StateConfigured
				m.popScreen()
			}
		case "tab":
			if noSecret {
				m.llm.secretCursor = (m.llm.secretCursor + 1) % 2
			} else {
				m.llm.secretCursor = (m.llm.secretCursor + 1) % 5
				// Focus appropriate textinput
				if m.llm.secretCursor == SecretCursorKey {
					m.llm.secretKeyInput.Focus()
				} else if m.llm.secretCursor == SecretCursorValue {
					m.llm.secretValueInput.Focus()
				}
			}
		}
	}
	return m, nil
}

func (m *Model) configureLLMWithPreset(presetName, envVarName, secretName string) error {
	preset, err := llm.LookupPreset(presetName)
	if err != nil {
		return err
	}
	agentDir := filepath.Join(m.state.HomePath, "agents", m.state.SelectedAgent)
	llmCfg := config.LLMConfig{
		Backend: preset.Backend,
		Model:   preset.DefaultModel,
		ClientConfig: map[string]any{
			"base_url": preset.ClientConfig["base_url"],
		},
	}
	if err := config.WriteLLMConfig(agentDir, llmCfg); err != nil {
		return err
	}
	if envVarName != "" && secretName != "" {
		if err := config.SetAgentEnvVar(m.state.SelectedAgent, envVarName, secretName); err != nil {
			return err
		}
	}
	return nil
}

// === Adapter Config (stub) ===

func (m *Model) viewAdapterConfig() string {
	var b strings.Builder
	b.WriteString(headerView("Configure Adapter"))
	b.WriteString("\n\n")
	b.WriteString(Styles.ItemDim.Render("  Not implemented yet") + "\n\n")
	b.WriteString(m.option("Back", true))
	b.WriteString("\n\n" + divider())
	b.WriteString(footer("[Enter] Back   [Q] Quit"))
	return b.String()
}

func (m *Model) updateAdapterConfig(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter", "escape", "left":
			m.popScreen()
		}
	}
	return m, nil
}

// === Identity Edit (stub) ===

func (m *Model) viewIdentityEdit() string {
	var b strings.Builder
	b.WriteString(headerView("Edit Identity"))
	b.WriteString("\n\n")
	b.WriteString("Agent: ")
	b.WriteString(Styles.ItemSelected.Render(m.state.SelectedAgent))
	b.WriteString("\n\n")
	b.WriteString(Styles.Item.Render("File: IDENTITY.md") + "\n\n")
	b.WriteString(Styles.ItemDim.Render("  Not implemented yet") + "\n\n")
	b.WriteString(m.option("Back", true))
	b.WriteString("\n\n" + divider())
	b.WriteString(footer("[Enter] Back   [Q] Quit"))
	return b.String()
}

func (m *Model) updateIdentityEdit(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter", "escape", "left":
			m.popScreen()
		}
	}
	return m, nil
}

// === Soul Edit (stub) ===

func (m *Model) viewSoulEdit() string {
	var b strings.Builder
	b.WriteString(headerView("Edit Soul"))
	b.WriteString("\n\n")
	b.WriteString("Agent: ")
	b.WriteString(Styles.ItemSelected.Render(m.state.SelectedAgent))
	b.WriteString("\n\n")
	b.WriteString(Styles.Item.Render("File: SOUL.md") + "\n\n")
	b.WriteString(Styles.ItemDim.Render("  Not implemented yet") + "\n\n")
	b.WriteString(m.option("Back", true))
	b.WriteString("\n\n" + divider())
	b.WriteString(footer("[Enter] Back   [Q] Quit"))
	return b.String()
}

func (m *Model) updateSoulEdit(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter", "escape", "left":
			m.popScreen()
		}
	}
	return m, nil
}

// === Skills (stub) ===

func (m *Model) viewSkills() string {
	var b strings.Builder
	b.WriteString(headerView("Manage Skills"))
	b.WriteString("\n\n")
	b.WriteString("Agent: ")
	b.WriteString(Styles.ItemSelected.Render(m.state.SelectedAgent))
	b.WriteString("\n\n")
	b.WriteString("Installed: ")
	b.WriteString(Styles.Item.Render(itoa(m.state.SkillsCount)))
	b.WriteString("\n\n")
	b.WriteString(Styles.ItemDim.Render("  Not implemented yet") + "\n\n")
	b.WriteString(m.option("Back", true))
	b.WriteString("\n\n" + divider())
	b.WriteString(footer("[Enter] Back   [Q] Quit"))
	return b.String()
}

func (m *Model) updateSkills(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter", "escape", "left":
			m.popScreen()
		}
	}
	return m, nil
}

// === Secrets (stub) ===

func (m *Model) viewSecrets() string {
	var b strings.Builder
	b.WriteString(headerView("Manage Secrets"))
	b.WriteString("\n\n")
	b.WriteString("Count: ")
	b.WriteString(Styles.Item.Render(itoa(m.state.SecretsCount)))
	b.WriteString("\n\n")
	b.WriteString(Styles.ItemDim.Render("  Not implemented yet") + "\n\n")
	b.WriteString(m.option("Back", true))
	b.WriteString("\n\n" + divider())
	b.WriteString(footer("[Enter] Back   [Q] Quit"))
	return b.String()
}

func (m *Model) updateSecrets(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter", "escape", "left":
			m.popScreen()
		}
	}
	return m, nil
}

// === Summary ===

func (m *Model) viewSummary() string {
	var b strings.Builder
	b.WriteString(headerView("Configuration Summary"))
	b.WriteString("\n\n")

	b.WriteString(Styles.SectionLabel.Render("BOOTSTRAP") + "\n")
	b.WriteString(Styles.Item.Render("Path:     "+m.state.HomePath) + "\n")
	bsText, bsStyle := bootstrapStatusText(m.state.BootstrapStatus)
	b.WriteString(Styles.Item.Render("Status:  ") + bsStyle.Render(bsText) + "\n\n")

	b.WriteString(Styles.SectionLabel.Render("AGENTS") + "\n")
	for _, name := range m.state.Agents {
		prefix := "  "
		style := Styles.Item
		if name == m.state.SelectedAgent {
			prefix = "▸ "
			style = Styles.ItemSelected
		}
		b.WriteString(style.Render(prefix+name) + "\n")
	}
	b.WriteString("\n")

	if m.state.SelectedAgent != "" {
		b.WriteString(Styles.SectionLabel.Render("AGENT: "+m.state.SelectedAgent) + "\n")
		llmText, llmStyle := itemStatus(m.state.LLMStatus)
		b.WriteString(Styles.Item.Render("LLM:     ") + llmStyle.Render(llmText) + "\n")
		adapterText, adapterStyle := itemStatus(m.state.AdapterStatus)
		b.WriteString(Styles.Item.Render("Adapter: ") + adapterStyle.Render(adapterText) + "\n")
		identityText, identityStyle := itemStatus(m.state.IdentityStatus)
		b.WriteString(Styles.Item.Render("Identity: ") + identityStyle.Render(identityText) + "\n")
		soulText, soulStyle := itemStatus(m.state.SoulStatus)
		b.WriteString(Styles.Item.Render("Soul:     ") + soulStyle.Render(soulText) + "\n")
		b.WriteString(Styles.Item.Render("Skills:   ") + Styles.Item.Render(itoa(m.state.SkillsCount)) + "\n\n")
	}

	b.WriteString(Styles.SectionLabel.Render("SECRETS") + "\n")
	b.WriteString(Styles.Item.Render("Count: ") + Styles.Item.Render(itoa(m.state.SecretsCount)) + "\n")

	b.WriteString("\n" + divider())
	b.WriteString(footer("[←] Back   [Q] Quit"))
	return b.String()
}

func (m *Model) updateSummary(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter", "escape", "left":
			m.popScreen()
		}
	}
	return m, nil
}

// === Helpers ===

func (m *Model) option(label string, selected bool) string {
	if selected {
		return Styles.ItemSelected.Render("▶ " + label)
	}
	return Styles.Item.Render("  " + label)
}

func (m *Model) clearAgentState() {
	m.agent = nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
