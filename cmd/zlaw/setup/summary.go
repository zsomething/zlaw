package setup

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbletea"

	"github.com/zsomething/zlaw/internal/config"
)

// summaryScreenState holds state for the summary screen.
type summaryScreenState struct {
	cursor int
}

// summaryView renders the configuration summary.
func summaryView(m *Model) string {
	if m.summary == nil {
		m.summary = &summaryScreenState{}
	}

	lines := []string{}

	lines = append(lines, Styles.ItemDim.Render(strings.Repeat("─", 40)))
	lines = append(lines, "")

	// Zlaw Home path.
	lines = append(lines, Styles.Item.Render("Zlaw Home: ")+Styles.Selected.Render(m.state.Home))
	lines = append(lines, "")

	// Agent count.
	agents := m.state.Config.Agents
	agentCount := len(agents)
	if agentCount == 0 {
		lines = append(lines, Styles.Item.Render("Agents: ")+Styles.ItemDim.Render("None configured"))
	} else {
		lines = append(lines, Styles.Item.Render(fmt.Sprintf("Agents (%d):", agentCount)))
		for i, agent := range agents {
			prefix := "  "
			if m.summary.cursor == i {
				prefix = "> "
			}
			name := agent.ID
			if m.summary.cursor == i {
				lines = append(lines, Styles.Selected.Render(prefix+name))
			} else {
				lines = append(lines, Styles.Item.Render(prefix+name))
			}
		}
	}
	lines = append(lines, "")

	// Selected agent details.
	if agentCount > 0 && m.summary.cursor < agentCount {
		agent := agents[m.summary.cursor]
		lines = append(lines, Styles.Heading.Render("Agent: "+agent.ID))
		lines = append(lines, Styles.ItemDim.Render(strings.Repeat("─", 32)))
		lines = append(lines, "")

		// Get agent dir.
		agentDir := agent.Dir
		if agentDir == "" {
			agentDir = m.state.Home + "/agents/" + agent.ID
		}

		// Load agent config for LLM and adapter info.
		agentCfg, err := config.LoadAgentConfigFile(agentDir + "/agent.toml")

		// LLM config.
		llmBackend := "Not configured"
		llmModel := ""
		if err == nil && agentCfg.LLM.Backend != "" {
			llmBackend = agentCfg.LLM.Backend
			llmModel = agentCfg.LLM.Model
		}
		lines = append(lines, Styles.Item.Render("  LLM:  ")+Styles.Item.Render(llmBackend))
		if llmModel != "" {
			lines = append(lines, Styles.Item.Render("  Model: ")+Styles.Item.Render(llmModel))
		}

		// Adapter config.
		adapterName := "None"
		adapterSecret := ""
		if err == nil && len(agentCfg.Adapter) > 0 {
			adapterName = agentCfg.Adapter[0].Backend
			// Look for adapter secret in env vars.
			for _, ev := range agent.EnvVars {
				if ev.Name == adapterEnvVars[adapterName] {
					adapterSecret = ev.FromSecret
					break
				}
			}
		}
		lines = append(lines, Styles.Item.Render("  Adapter: ")+Styles.Item.Render(adapterName))
		if adapterSecret != "" {
			lines = append(lines, Styles.Item.Render("  Adapter Secret: ")+Styles.Item.Render(adapterSecret))
		}

		// Skills count.
		skillsDir := agentDir + "/skills"
		skillCount := 0
		if entries, err := os.ReadDir(skillsDir); err == nil {
			skillCount = len(entries)
		}
		lines = append(lines, Styles.Item.Render("  Skills: ")+Styles.Item.Render(fmt.Sprintf("%d installed", skillCount)))

		lines = append(lines, "")
	}

	// Secrets count.
	secrets := config.ListSecrets()
	lines = append(lines, Styles.Item.Render("Secrets: ")+Styles.Item.Render(fmt.Sprintf("%d stored", len(secrets))))

	lines = append(lines, "")
	lines = append(lines, Styles.ItemDim.Render(strings.Repeat("─", 40)))

	content := strings.Join(lines, "\n")
	helpText := "[↑/↓] Select agent  [←] Back  [Q] Quit"
	return windowView("Configuration Summary", content, helpText)
}

// updateSummary handles keyboard events for the summary screen.
func updateSummary(m *Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.summary == nil {
		m.summary = &summaryScreenState{}
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.summary.cursor > 0 {
				m.summary.cursor--
			}
			return m, nil

		case "down", "j":
			agentCount := len(m.state.Config.Agents)
			if m.summary.cursor < agentCount-1 {
				m.summary.cursor++
			}
			return m, nil

		case "left", "h":
			m.screen = ScreenMainMenu
			m.summary = nil
			return m, nil

		case "q", "Q", "ctrl+c":
			m.quit = true
			return m, tea.Quit
		}
	}

	return m, nil
}
