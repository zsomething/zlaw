package setup

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbletea"
)

// soulScreenState holds state for the soul editor screen.
type soulScreenState struct {
	agentDir string
	path     string
	status   string // "ready", "editing", "done"
}

// soulInit initializes the soul screen state.
func (m *Model) soulInit() {
	if m.state.SelectedAgent == "" {
		return
	}
	agent, ok := m.state.Config.FindAgent(m.state.SelectedAgent)
	if !ok {
		return
	}
	dir := agentDir(m.state.Home, agent)
	m.soul = &soulScreenState{
		agentDir: dir,
		path:     filepath.Join(dir, "SOUL.md"),
		status:   "ready",
	}
}

// soulView renders the soul editor screen.
func soulView(m *Model) string {
	if m.soul == nil {
		m.soulInit()
	}

	lines := []string{
		Styles.Title.Render("zlaw setup"),
		"",
		Styles.Heading.Render("Edit Soul — " + m.state.SelectedAgent),
		"",
	}

	if m.soul.path != "" {
		lines = append(lines, Styles.Item.Render("File: "+m.soul.path))
	} else {
		lines = append(lines, Styles.Dim.Render("File: (no agent selected)"))
	}

	lines = append(lines, "")
	lines = append(lines, Styles.Item.Render("Status: "+m.soul.status))
	lines = append(lines, "")

	// Show whether file exists.
	if _, err := os.Stat(m.soul.path); os.IsNotExist(err) {
		lines = append(lines, Styles.Dim.Render("File does not exist yet."))
	} else if err == nil {
		lines = append(lines, Styles.StatusOK.Render("File exists."))
	}

	lines = append(lines, "")
	lines = append(lines, Styles.Dim.Render(strings.Repeat("─", 32)))
	lines = append(lines, Styles.Footer.Render("[E] Open editor  [B] Back"))

	return strings.Join(lines, "\n")
}

// updateSoul handles keyboard events for the soul editor screen.
func updateSoul(m *Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.soul == nil {
		m.soulInit()
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "e", "E":
			return soulEdit(m)

		case "left", "h", "b", "B":
			m.screen = ScreenMainMenu
			m.soul = nil
			return m, nil

		case "q", "Q", "ctrl+c":
			m.quit = true
			return m, tea.Quit
		}
	}

	return m, nil
}

// soulEdit opens the soul file in $EDITOR.
func soulEdit(m *Model) (tea.Model, tea.Cmd) {
	if m.soul == nil || m.soul.path == "" {
		return m, nil
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	// Ensure file exists (create with default content if needed).
	if _, err := os.Stat(m.soul.path); os.IsNotExist(err) {
		content := "# Soul\n\nYour personality, values, and guiding principles.\n"
		if err := os.WriteFile(m.soul.path, []byte(content), 0644); err != nil {
			m.soul.status = "error: " + err.Error()
			return m, nil
		}
	}

	m.soul.status = "editing..."

	cmd := exec.Command(editor, m.soul.path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		m.soul.status = "error: " + err.Error()
		return m, nil
	}

	m.soul.status = "done"
	m.screen = ScreenMainMenu
	m.soul = nil
	return m, nil
}
