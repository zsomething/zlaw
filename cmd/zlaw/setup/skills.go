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

	lines := []string{
		Styles.Title.Render("zlaw setup"),
		"",
		Styles.Heading.Render("Manage Skills — " + m.state.SelectedAgent),
		"",
	}

	switch m.skills.mode {
	case "list":
		if len(m.skills.skills) == 0 {
			lines = append(lines, Styles.Dim.Render("No skills installed."))
			lines = append(lines, Styles.Dim.Render("Press [A] to add a skill."))
		} else {
			lines = append(lines, Styles.Item.Render("Installed skills:"))
			lines = append(lines, Styles.Dim.Render(strings.Repeat("─", 32)))
			for i, name := range m.skills.skills {
				prefix := "  "
				if m.skills.cursor == i {
					prefix = "> "
					lines = append(lines, Styles.Selected.Render(prefix+itoa(i+1)+". "+name))
				} else {
					lines = append(lines, Styles.Item.Render(prefix+itoa(i+1)+". "+name))
				}
			}
		}
		lines = append(lines, "")
		lines = append(lines, Styles.Dim.Render(strings.Repeat("─", 32)))
		lines = append(lines, Styles.Footer.Render("[A] Add  [R] Remove  [B] Back"))

	case "add":
		lines = append(lines, Styles.Item.Render("Add new skill:"))
		lines = append(lines, "")
		lines = append(lines, Styles.Selected.Render("> Skill name: ")+Styles.Item.Render(m.skills.newSkillName+"_"))
		lines = append(lines, Styles.Dim.Render("  Creates: skills/<name>.md"))
		lines = append(lines, "")
		lines = append(lines, Styles.Dim.Render(strings.Repeat("─", 32)))
		lines = append(lines, Styles.Footer.Render("[Enter] Create  [B] Cancel"))

	case "confirm":
		lines = append(lines, Styles.Item.Render("Delete skill?"))
		lines = append(lines, "")
		lines = append(lines, Styles.Heading.Render("  "+m.skills.deleteTarget))
		lines = append(lines, "")
		lines = append(lines, Styles.StatusErr.Render("  This action cannot be undone."))
		lines = append(lines, "")
		lines = append(lines, Styles.Dim.Render(strings.Repeat("─", 32)))
		lines = append(lines, Styles.Footer.Render("[D] Delete  [B] Cancel"))
	}

	if m.skills.errMsg != "" {
		lines = append(lines, "")
		lines = append(lines, Styles.StatusErr.Render("Error: "+m.skills.errMsg))
	}

	return strings.Join(lines, "\n")
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

		case "a", "A":
			if m.skills.mode == "list" {
				m.skills.mode = "add"
				m.skills.newSkillName = ""
			}
			return m, nil

		case "r", "R":
			if m.skills.mode == "list" && len(m.skills.skills) > 0 && m.skills.cursor < len(m.skills.skills) {
				m.skills.mode = "confirm"
				m.skills.deleteTarget = m.skills.skills[m.skills.cursor]
			}
			return m, nil

		case "d", "D":
			if m.skills.mode == "confirm" {
				m2, cmd := skillsDelete(m)
				return m2, cmd
			}
			return m, nil

		case "enter":
			switch m.skills.mode {
			case "list":
				return m, nil
			case "add":
				m2, cmd := skillsAdd(m)
				return m2, cmd
			}
			return m, nil

		case "left", "h", "b", "B":
			switch m.skills.mode {
			case "add":
				m.skills.mode = "list"
				m.skills.newSkillName = ""
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
