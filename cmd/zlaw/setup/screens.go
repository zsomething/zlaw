package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

// === Bootstrap ===

func (m *Model) viewBootstrap() string {
	var b strings.Builder
	b.WriteString(headerView("Bootstrap Zlaw Home"))
	b.WriteString("\n\n")

	// Path display
	b.WriteString("Target path:\n\n")
	b.WriteString(Styles.Item.Render("  " + m.state.HomePath) + "\n")
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
		b.WriteString(m.option("Reset", m.bootstrap.cursor == 0) + "\n")
		b.WriteString(m.option("Keep", m.bootstrap.cursor == 1) + "\n")
		b.WriteString(m.option("Cancel", m.bootstrap.cursor == 2))
	} else {
		// Not bootstrapped or incomplete
		statusText, statusStyle := bootstrapStatusText(m.state.BootstrapStatus)
		b.WriteString(statusStyle.Render(statusText) + "\n\n")
		b.WriteString(m.option("Create", m.bootstrap.cursor == 0) + "\n")
		b.WriteString(m.option("Cancel", m.bootstrap.cursor == 1))
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
	b.WriteString(m.option("Yes, Reset", m.bootstrap.cursor == 0) + "\n")
	b.WriteString(m.option("No, Cancel", m.bootstrap.cursor == 1))
	return b.String()
}

func (m *Model) updateBootstrap(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Determine max cursor based on state
	maxCursor := 1 // Create, Cancel
	if m.state.BootstrapStatus == BootstrapReady {
		maxCursor = 2 // Reset, Keep, Cancel
	}

	// Handle confirmation dialog mode
	if m.bootstrap.confirming {
		return m.updateBootstrapConfirm(msg)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.bootstrap.cursor > 0 {
				m.bootstrap.cursor--
			}
		case "down", "j":
			if m.bootstrap.cursor < maxCursor {
				m.bootstrap.cursor++
			}
		case "enter":
			m.bootstrap.errMsg = ""
			if m.state.BootstrapStatus == BootstrapReady {
				switch m.bootstrap.cursor {
				case 0: // Reset - show confirmation
					m.bootstrap.confirming = true
					m.bootstrap.cursor = 0
					return m, nil
				case 1: // Keep
					m.popScreen()
				case 2: // Cancel
					m.popScreen()
				}
			} else {
				switch m.bootstrap.cursor {
				case 0: // Create
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
				case 1: // Cancel
					m.popScreen()
				}
			}
		case "escape", "left", "h":
			m.popScreen()
		}
	}
	return m, nil
}

func (m *Model) updateBootstrapConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.bootstrap.cursor > 0 {
				m.bootstrap.cursor--
			}
		case "down", "j":
			if m.bootstrap.cursor < 1 {
				m.bootstrap.cursor++
			}
		case "enter":
			if m.bootstrap.cursor == 0 {
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
		case "n", "N", "escape", "left", "h":
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
	b.WriteString(m.option("subprocess", m.agent.cursor == 1) + "\n\n")

	b.WriteString(Styles.Item.Render("Target") + "\n")
	b.WriteString(m.option("local", m.agent.cursor == 2) + "\n\n")

	b.WriteString(Styles.Item.Render("Restart Policy") + "\n")
	b.WriteString(m.option("on-failure", m.agent.cursor == 3) + "\n\n")

	if m.agent.errMsg != "" {
		b.WriteString(Styles.StatusErr.Render("⚠ "+m.agent.errMsg) + "\n\n")
	}

	b.WriteString(m.option("Create Agent", m.agent.cursor == 4) + "   ")
	b.WriteString(m.option("Cancel", m.agent.cursor == 5))

	b.WriteString("\n\n" + divider())
	b.WriteString(footer("[↑↓] Navigate   [Tab] Next   [Enter] Select   [←] Back   [Q] Quit"))
	return b.String()
}

func (m *Model) updateAgentCreate(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.agent.agentID, cmd = m.agent.agentID.Update(msg)
	if cmd != nil {
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.agent.cursor > 0 {
				m.agent.cursor--
			}
		case "down", "j":
			if m.agent.cursor < 5 {
				m.agent.cursor++
			}
		case "tab":
			m.agent.cursor = (m.agent.cursor + 1) % 6
		case "enter":
			m.agent.errMsg = ""
			switch m.agent.cursor {
			case 4: // Create
				agentID := strings.TrimSpace(m.agent.agentID.Value())
				if agentID == "" {
					m.agent.errMsg = "Agent ID is required"
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
				if err := addAgentToHub(agentID); err != nil {
					m.agent.errMsg = err.Error()
					return m, nil
				}
				m.state.Agents = append(m.state.Agents, agentID)
				m.state.SelectedAgent = agentID
				m.popScreen()
			case 5: // Cancel
				m.popScreen()
			default:
				m.agent.cursor = 4
			}
		case "escape", "left", "h":
			m.popScreen()
		}
	}
	return m, nil
}

func addAgentToHub(agentID string) error {
	hub, err := config.LoadHubConfig(config.DefaultHubConfigPath())
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("load hub config: %w", err)
	}
	hub.Agents = append(hub.Agents, config.AgentEntry{
		ID: agentID,
	})
	return hub.Save()
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
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.state.SelectedAgent = m.state.Agents[m.cursor]
			}
		case "down", "j":
			if m.cursor < len(m.state.Agents)-1 {
				m.cursor++
				m.state.SelectedAgent = m.state.Agents[m.cursor]
			}
		case "enter", "escape", "left", "h":
			m.popScreen()
		}
	}
	return m, nil
}

// === LLM Config ===

func (m *Model) viewLLMConfig() string {
	var b strings.Builder
	b.WriteString(headerView("Configure LLM — "+m.state.SelectedAgent))
	b.WriteString("\n\n")

	b.WriteString("Select LLM preset:\n\n")
	for i, name := range llmPresets {
		prefix := "  "
		style := Styles.Item
		if m.llm.cursor == i {
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
		b.WriteString(Styles.StatusErr.Render("⚠ "+m.llm.errMsg))
	}

	b.WriteString("\n" + divider())
	b.WriteString(footer("[↑↓] Navigate   [Enter] Select   [←] Back   [Q] Quit"))
	return b.String()
}

func (m *Model) updateLLMConfig(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.llm.cursor > 0 {
				m.llm.cursor--
			}
		case "down", "j":
			if m.llm.cursor < len(llmPresets)-1 {
				m.llm.cursor++
			}
		case "enter":
			m.llm.errMsg = ""
			presetName := llmPresets[m.llm.cursor]
			preset, err := llm.LookupPreset(presetName)
			if err != nil {
				m.llm.errMsg = err.Error()
				return m, nil
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
				m.llm.errMsg = err.Error()
				return m, nil
			}
			m.state.LLMStatus = StateConfigured
			m.popScreen()
		case "escape", "left", "h":
			m.popScreen()
		}
	}
	return m, nil
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
		case "enter", "escape", "left", "h":
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
		case "enter", "escape", "left", "h":
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
		case "enter", "escape", "left", "h":
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
		case "enter", "escape", "left", "h":
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
		case "enter", "escape", "left", "h":
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
	b.WriteString(Styles.Item.Render("Path:     " + m.state.HomePath) + "\n")
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
		case "enter", "escape", "left", "h":
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