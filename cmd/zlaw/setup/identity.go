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
	agentPath := agentHomeDir(m.state.Home, agent)
	m.identity = &identityScreenState{
		agentDir: agentPath,
		path:     filepath.Join(agentPath, "IDENTITY.md"),
		status:   "ready",
	}
}

// identityView renders the identity editor screen.
func identityView(m *Model) string {
	if m.identity == nil {
		m.identityInit()
	}

	var content strings.Builder
	content.WriteString(Styles.Heading.Render("Edit Identity — " + m.state.SelectedAgent))
	content.WriteString("\n\n")

	if m.identity.path != "" {
		content.WriteString(Styles.Item.Render("File: " + m.identity.path))
	} else {
		content.WriteString(Styles.ItemDim.Render("File: (no agent selected)"))
	}

	content.WriteString("\n\n")
	content.WriteString(Styles.Item.Render("Status: " + m.identity.status))
	content.WriteString("\n\n")

	// Show whether file exists.
	if _, err := os.Stat(m.identity.path); os.IsNotExist(err) {
		content.WriteString(Styles.ItemDim.Render("File does not exist yet."))
	} else if err == nil {
		content.WriteString(Styles.StatusOK.Render("File exists."))
	}

	content.WriteString("\n\n")
	content.WriteString(Styles.ItemDim.Render(strings.Repeat("─", 32)))

	return windowView("zlaw setup", content.String(), "[E] Open editor  [B] Back")
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
