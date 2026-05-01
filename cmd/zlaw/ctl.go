package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/zsomething/zlaw/internal/config"
)

// ── Templates ────────────────────────────────────────────────────────────────

// agentTOMLTemplate has agent name substituted for %s.
const agentTOMLTemplate = `[agent]
id = %q
description = ""

[llm]
backend = "anthropic"
model = "claude-sonnet-4-5"
auth_profile = "anthropic"
max_tokens = 4096
timeout_sec = 60

[tools]
allowed = []

# Uncomment and configure adapters after setting up credentials:
# [[adapter]]
# type = "telegram"
# auth_profile = "telegram"
`

const credentialsTOMLTemplate = `[profiles.anthropic]
name = "anthropic"
data = { api_key = "${ANTHROPIC_API_KEY}" }

[profiles.telegram]
name = "telegram"
data = { telegram_bot_token = "${TELEGRAM_BOT_TOKEN}" }

[profiles.fizzy]
name = "fizzy"
data = { fizzy_api_key = "${FIZZY_API_KEY}" }
`

const ctlSoulMDTemplate = `You are a helpful personal assistant.
`

// ctlIdentityMDTemplate has agent name substituted for %s.
const ctlIdentityMDTemplate = `# Identity

Your name is %s.
`

// ── CtlCmd ───────────────────────────────────────────────────────────────────

type CtlCmd struct {
	Create CtlCreateCmd `cmd:"" help:"create a resource"`
}

type CtlCreateCmd struct {
	Agent CtlCreateAgentCmd `cmd:"" help:"create and register a new agent"`
}

// ── ctl create agent ─────────────────────────────────────────────────────────

type CtlCreateAgentCmd struct {
	Name      string `arg:"true" help:"agent name"`
	AgentHome string `name:"agent-home" help:"absolute path for agent home (default: $ZLAW_HOME/agents/<name>)"`
	Start     bool   `help:"spawn the agent after registration"`
}

func (c *CtlCreateAgentCmd) Run(ctx context.Context, _ *slog.Logger) error {
	agentHome := c.AgentHome
	if agentHome == "" {
		agentHome = filepath.Join(config.ZlawHome(), "agents", c.Name)
	}
	if !filepath.IsAbs(agentHome) {
		abs, err := filepath.Abs(agentHome)
		if err != nil {
			return fmt.Errorf("resolve agent-home: %w", err)
		}
		agentHome = abs
	}

	// 1. Create agent home directory.
	if err := os.MkdirAll(agentHome, 0o700); err != nil {
		return fmt.Errorf("create agent home %s: %w", agentHome, err)
	}

	// 2. Create workspace/ subdir.
	workspaceDir := filepath.Join(agentHome, "workspace")
	if err := os.MkdirAll(workspaceDir, 0o700); err != nil {
		return fmt.Errorf("create workspace dir: %w", err)
	}

	// 3. Scaffold files (skip if already exist).
	type scaffold struct {
		path    string
		content string
		mode    os.FileMode
	}
	files := []scaffold{
		{filepath.Join(agentHome, "agent.toml"), fmt.Sprintf(agentTOMLTemplate, c.Name), 0o600},
		{filepath.Join(agentHome, "credentials.toml"), credentialsTOMLTemplate, 0o600},
		{filepath.Join(agentHome, "SOUL.md"), ctlSoulMDTemplate, 0o644},
		{filepath.Join(agentHome, "IDENTITY.md"), fmt.Sprintf(ctlIdentityMDTemplate, c.Name), 0o644},
	}
	for _, f := range files {
		if _, err := os.Stat(f.path); os.IsNotExist(err) {
			if err := os.WriteFile(f.path, []byte(f.content), f.mode); err != nil {
				return fmt.Errorf("write %s: %w", f.path, err)
			}
			fmt.Fprintf(os.Stdout, "  created  %s\n", f.path)
		} else {
			fmt.Fprintf(os.Stdout, "  exists   %s\n", f.path)
		}
	}

	// 4. Register with hub via control socket.
	method := "agent.create"
	params := map[string]any{
		"name": c.Name,
		"dir":  agentHome,
	}
	if err := ctlSocketCall(method, params); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not register with hub: %v\n", err)
		fmt.Fprintf(os.Stderr, "  start the hub and run: zlaw agent create %s --dir %s\n", c.Name, agentHome)
	} else {
		fmt.Fprintf(os.Stdout, "  registered agent %q with hub (dir: %s)\n", c.Name, agentHome)
	}

	fmt.Fprintf(os.Stdout, "\nAgent %q ready at %s\n", c.Name, agentHome)
	return nil
}

// ctlSocketCall sends a JSON-RPC request to the hub control socket.
func ctlSocketCall(method string, params map[string]any) error {
	conn, err := socketConn()
	if err != nil {
		return fmt.Errorf("connect to hub: %w", err)
	}
	defer conn.Close() //nolint:errcheck

	req := map[string]any{"method": method, "params": params}
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	conn.SetWriteDeadline(time.Now().Add(5 * time.Second)) //nolint:errcheck
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))  //nolint:errcheck
	if _, err := conn.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("send request: %w", err)
	}

	var resp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if !resp.OK {
		return fmt.Errorf("%s", resp.Error)
	}
	return nil
}
