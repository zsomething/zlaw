package setup

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbletea"
)

// skillsScreenState holds state for the skills management screen.
type skillsScreenState struct {
	mode         string // "list", "add", "confirm"
	skills       []string
	cursor       int
	newSkillName string
	deleteTarget string
	errMsg       string
}

// skillsInit initializes the skills screen state.
func (m *Model) skillsInit() {
	m.skills = &skillsScreenState{
		mode:   "list",
		cursor: 0,
	}
	skillsLoad(m)
}

// skillsLoad loads the list of skills from the skills directory.
func skillsLoad(m *Model) {
	if m.skills == nil {
		return
	}
	skillsDir := filepath.Join(m.state.Home, "agents", m.state.SelectedAgent, "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		m.skills.skills = []string{}
		return
	}
	skills := []string{}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			skills = append(skills, strings.TrimSuffix(e.Name(), ".md"))
		}
	}
	m.skills.skills = skills
}

// skillsView renders the skills management screen.
func skillsView(m *Model) string {
	if m.skills == nil {
		m.skillsInit()
	}

	var contentLines []string
	var helpText string

	switch m.skills.mode {
	case "list":
		contentLines = append(contentLines, Styles.Heading.Render("Manage Skills — "+m.state.SelectedAgent))
		contentLines = append(contentLines, "")
		if len(m.skills.skills) == 0 {
			contentLines = append(contentLines, Styles.ItemDim.Render("No skills installed."))
			contentLines = append(contentLines, Styles.ItemDim.Render("Press [Enter] to add a skill."))
		} else {
			contentLines = append(contentLines, Styles.Item.Render("Installed skills:"))
			contentLines = append(contentLines, Styles.ItemDim.Render(strings.Repeat("─", 32)))
			for i, name := range m.skills.skills {
				prefix := "  "
				if m.skills.cursor == i {
					prefix = "▶ "
					contentLines = append(contentLines, Styles.Selected.Render(prefix+name))
				} else {
					contentLines = append(contentLines, Styles.Item.Render(prefix+name))
				}
			}
		}
		contentLines = append(contentLines, "")
		contentLines = append(contentLines, Styles.ItemDim.Render(strings.Repeat("─", 32)))
		helpText = "[↑↓] Navigate  [Enter] Add  [←] Back"

	case "add":
		contentLines = append(contentLines, Styles.Heading.Render("Add Skill"))
		contentLines = append(contentLines, "")
		contentLines = append(contentLines, Styles.Item.Render("Enter skill name:"))
		contentLines = append(contentLines, Styles.Selected.Render("> "+m.skills.newSkillName)+"_")
		contentLines = append(contentLines, Styles.ItemDim.Render("  Creates: skills/<name>.md"))
		contentLines = append(contentLines, "")
		contentLines = append(contentLines, Styles.ItemDim.Render(strings.Repeat("─", 32)))
		helpText = "[Enter] Create  [←] Cancel"

	case "confirm":
		contentLines = append(contentLines, Styles.Heading.Render("Delete Skill"))
		contentLines = append(contentLines, "")
		contentLines = append(contentLines, Styles.Item.Render("Delete skill?"))
		contentLines = append(contentLines, "")
		contentLines = append(contentLines, Styles.StatusErr.Render("  "+m.skills.deleteTarget))
		contentLines = append(contentLines, "")
		contentLines = append(contentLines, Styles.StatusErr.Render("  This action cannot be undone."))
		contentLines = append(contentLines, "")
		contentLines = append(contentLines, Styles.ItemDim.Render(strings.Repeat("─", 32)))
		helpText = "[Enter] Delete  [←] Cancel"
	}

	if m.skills.errMsg != "" {
		contentLines = append(contentLines, "")
		contentLines = append(contentLines, Styles.StatusErr.Render("Error: "+m.skills.errMsg))
	}

	content := strings.Join(contentLines, "\n")
	return windowView("zlaw setup", content, helpText)
}

// updateSkills handles keyboard events for the skills management screen.
func updateSkills(m *Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.skills == nil {
		m.skillsInit()
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.skills.cursor > 0 {
				m.skills.cursor--
			}
			return m, nil

		case "down", "j":
			if m.skills.cursor < len(m.skills.skills)-1 {
				m.skills.cursor++
			}
			return m, nil

		case "enter":
			switch m.skills.mode {
			case "list":
				// Navigate to add mode.
				m.skills.mode = "add"
				m.skills.newSkillName = ""
				return m, nil
			case "add":
				m2, cmd := skillsAdd(m)
				return m2, cmd
			case "confirm":
				// Delete confirmed.
				m2, cmd := skillsDelete(m)
				return m2, cmd
			}
			return m, nil

		case "left", "h":
			switch m.skills.mode {
			case "add":
				m.skills.mode = "list"
			case "confirm":
				m.skills.mode = "list"
				m.skills.deleteTarget = ""
			default:
				m.screen = ScreenMainMenu
				m.skills = nil
			}
			return m, nil

		case "q", "Q", "ctrl+c":
			m.quit = true
			return m, tea.Quit

		default:
			// Character input for skill name.
			if m.skills.mode == "add" && len(msg.Runes) == 1 {
				r := msg.Runes[0]
				if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
					m.skills.newSkillName += string(r)
				} else if msg.String() == "backspace" && len(m.skills.newSkillName) > 0 {
					m.skills.newSkillName = m.skills.newSkillName[:len(m.skills.newSkillName)-1]
				}
			}
			return m, nil
		}
	}

	return m, nil
}

// skillsAdd creates a new skill.
func skillsAdd(m *Model) (tea.Model, tea.Cmd) {
	if m.skills.newSkillName == "" {
		m.skills.errMsg = "Skill name is required"
		return m, nil
	}

	skillsDir := filepath.Join(m.state.Home, "agents", m.state.SelectedAgent, "skills")
	skillPath := filepath.Join(skillsDir, m.skills.newSkillName+".md")

	content := "# " + m.skills.newSkillName + "\n\nDescribe this skill here.\n"
	if err := os.WriteFile(skillPath, []byte(content), 0o644); err != nil {
		m.skills.errMsg = err.Error()
		return m, nil
	}

	m.skills.mode = "list"
	m.skills.newSkillName = ""
	skillsLoad(m)
	return m, nil
}

// skillsDelete deletes a skill.
func skillsDelete(m *Model) (tea.Model, tea.Cmd) {
	skillsDir := filepath.Join(m.state.Home, "agents", m.state.SelectedAgent, "skills")
	skillPath := filepath.Join(skillsDir, m.skills.deleteTarget+".md")

	if err := os.Remove(skillPath); err != nil {
		m.skills.errMsg = err.Error()
		return m, nil
	}

	m.skills.mode = "list"
	m.skills.deleteTarget = ""
	if m.skills.cursor >= len(m.skills.skills)-1 && m.skills.cursor > 0 {
		m.skills.cursor--
	}
	skillsLoad(m)
	return m, nil
}
