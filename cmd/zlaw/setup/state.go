package setup

import (
	"os"
	"path/filepath"

	"github.com/zsomething/zlaw/internal/config"
	"github.com/zsomething/zlaw/internal/secrets"
)

// State holds the shared state for the setup wizard.
// It is loaded once at startup and refreshed when returning from sub-screens.
type State struct {
	Home          string
	Config        *config.HubConfig
	Secrets       secrets.Store
	SelectedAgent string // agent ID or ""
}

// LoadState reads the current configuration from disk.
func LoadState() (*State, error) {
	home := config.ZlawHome()
	zlawPath := filepath.Join(home, "zlaw.toml")

	var cfg *config.HubConfig
	if _, err := os.Stat(zlawPath); os.IsNotExist(err) {
		cfg = nil
	} else if err != nil {
		return nil, err
	} else {
		loaded, err := config.LoadHubConfig(zlawPath)
		if err != nil {
			return nil, err
		}
		cfg = &loaded
	}

	sec, err := secrets.Load(secrets.DefaultSecretsPath())
	if err != nil {
		return nil, err
	}

	return &State{
		Home:          home,
		Config:        cfg,
		Secrets:       sec,
		SelectedAgent: "",
	}, nil
}

// IsConfigured returns true if zlaw.toml exists at Home.
func (s *State) IsConfigured() bool {
	if s.Config == nil {
		return false
	}
	path := filepath.Join(s.Home, "zlaw.toml")
	_, err := os.Stat(path)
	return err == nil
}

// HasAgents returns true if there is at least one agent.
func (s *State) HasAgents() bool {
	return s.Config != nil && len(s.Config.Agents) > 0
}
