package setup

// Screen identifies the currently active sub-screen in the setup wizard.
type Screen string

const (
	ScreenMainMenu      Screen = "main-menu"
	ScreenBootstrap     Screen = "bootstrap"
	ScreenCreateAgent   Screen = "create-agent"
	ScreenDeleteAgent   Screen = "delete-agent"
	ScreenLLM           Screen = "llm"
	ScreenLLMSecret     Screen = "llm-secret"
	ScreenAdapter       Screen = "adapter"
	ScreenAdapterSecret Screen = "adapter-secret"
	ScreenIdentity      Screen = "identity"
	ScreenSoul          Screen = "soul"
	ScreenSkills        Screen = "skills"
	ScreenSecrets       Screen = "secrets"
	ScreenSummary       Screen = "summary"
)
