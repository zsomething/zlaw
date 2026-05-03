package setup

// ScreenType identifies the currently active screen.
type ScreenType int

const (
	ScreenMainMenu ScreenType = iota
	ScreenBootstrap
	ScreenAgentCreate
	ScreenAgentConfig
	ScreenLLMConfig
	ScreenAdapterConfig
	ScreenIdentityEdit
	ScreenSoulEdit
	ScreenSkills
	ScreenSecrets
	ScreenSummary
)

func (s ScreenType) String() string {
	switch s {
	case ScreenMainMenu:
		return "Main Menu"
	case ScreenBootstrap:
		return "Bootstrap"
	case ScreenAgentCreate:
		return "Create Agent"
	case ScreenAgentConfig:
		return "Configure Agent"
	case ScreenLLMConfig:
		return "Configure LLM"
	case ScreenAdapterConfig:
		return "Configure Adapter"
	case ScreenIdentityEdit:
		return "Edit Identity"
	case ScreenSoulEdit:
		return "Edit Soul"
	case ScreenSkills:
		return "Manage Skills"
	case ScreenSecrets:
		return "Manage Secrets"
	case ScreenSummary:
		return "Summary"
	default:
		return "Unknown"
	}
}

// BootstrapStatus represents the state of the zlaw home bootstrap.
type BootstrapStatus int

const (
	BootstrapNotReady BootstrapStatus = iota
	BootstrapReady
	BootstrapIncomplete // directory exists but zlaw.toml missing/malformed
)

func (s BootstrapStatus) String() string {
	switch s {
	case BootstrapNotReady:
		return "not initialized"
	case BootstrapReady:
		return "configured"
	case BootstrapIncomplete:
		return "incomplete setup"
	default:
		return "unknown"
	}
}

// ItemState represents the configuration state of a component.
type ItemState int

const (
	StateMissing ItemState = iota
	StateConfigured
	StateInvalid
	StateView // action-only items
)

func (s ItemState) String() string {
	switch s {
	case StateMissing:
		return "missing"
	case StateConfigured:
		return "configured"
	case StateInvalid:
		return "invalid"
	case StateView:
		return ""
	default:
		return ""
	}
}
