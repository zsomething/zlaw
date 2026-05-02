package setup

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbletea"
)

// identityScreenState holds state for the identity editor screen.
type identityScreenState struct {
	agentDir string
	path     string
	status   string // "ready", "editing", "done"
}

// identityInit initializes the identity screen state.
func (m *Model) identityInit() {
	if m.state.SelectedAgent == "" {
		return
	}
	agent, ok := m.state.Config.FindAgent(m.state.SelectedAgent)
	if !ok {
		return
	}
	dir := agentDir(m.state.Home, agent)
	m.identity = &identityScreenState{
		agentDir: dir,
		path:     filepath.Join(dir, "IDENTITY.md"),
		status:   "ready",
	}
}

// identityView renders the identity editor screen.
func identityView(m *Model) string {
	if m.identity == nil {
		m.identityInit()
	}

	lines := []string{
		Styles.Title.Render("zlaw setup"),
		"",
		Styles.Heading.Render("Edit Identity — " + m.state.SelectedAgent),
		"",
	}

	if m.identity.path != "" {
		lines = append(lines, Styles.Item.Render("File: "+m.identity.path))
	} else {
		lines = append(lines, Styles.Dim.Render("File: (no agent selected)"))
	}

	lines = append(lines, "")
	lines = append(lines, Styles.Item.Render("Status: "+m.identity.status))
	lines = append(lines, "")

	// Show whether file exists.
	if _, err := os.Stat(m.identity.path); os.IsNotExist(err) {
		lines = append(lines, Styles.Dim.Render("File does not exist yet."))
	} else if err == nil {
		lines = append(lines, Styles.StatusOK.Render("File exists."))
	}

	lines = append(lines, "")
	lines = append(lines, Styles.Dim.Render(strings.Repeat("─", 32)))
	lines = append(lines, Styles.Footer.Render("[E] Open editor  [B] Back"))

	return strings.Join(lines, "\n")
}

// updateIdentity handles keyboard events for the identity editor screen.
func updateIdentity(m *Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.identity == nil {
		m.identityInit()
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "e", "E":
			return identityEdit(m)

		case "left", "h", "b", "B":
			m.screen = ScreenMainMenu
			m.identity = nil
			return m, nil

		case "q", "Q", "ctrl+c":
			m.quit = true
			return m, tea.Quit
		}
	}

	return m, nil
}

// identityEdit opens the identity file in $EDITOR.
func identityEdit(m *Model) (tea.Model, tea.Cmd) {
	if m.identity == nil || m.identity.path == "" {
		return m, nil
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	// Ensure file exists (create with default content if needed).
	if _, err := os.Stat(m.identity.path); os.IsNotExist(err) {
		content := "# Identity\n\nYour name, role, and background.\n"
		if err := os.WriteFile(m.identity.path, []byte(content), 0644); err != nil {
			m.identity.status = "error: " + err.Error()
			return m, nil
		}
	}

	m.identity.status = "editing..."

	cmd := exec.Command(editor, m.identity.path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		m.identity.status = "error: " + err.Error()
		return m, nil
	}

	m.identity.status = "done"
	m.screen = ScreenMainMenu
	m.identity = nil
	return m, nil
}
