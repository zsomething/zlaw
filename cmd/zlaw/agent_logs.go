package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/zsomething/zlaw/internal/config"
	"github.com/zsomething/zlaw/internal/logging"
	"github.com/zsomething/zlaw/internal/messaging"
)

// AgentLogsCmd streams agent logs from the hub.
type AgentLogsCmd struct {
	Agent    string        `short:"a" help:"agent name to filter logs (default: all agents)"`
	Level    string        `help:"minimum log level (debug/info/warn/error)"`
	Since    time.Duration `help:"show logs from the last N seconds (default: all)"`
	Follow   bool          `short:"f" help:"follow logs continuously (default: true)"`
	NatsURL  string        `env:"ZLAW_NATS_URL" help:"NATS server URL"`
	NatsCred string        `env:"ZLAW_NATS_CREDS" help:"NATS credentials token"`
}

func (c *AgentLogsCmd) Run(ctx context.Context) error {
	// Determine NATS URL
	natsURL := c.NatsURL
	if natsURL == "" {
		natsURL = os.Getenv("ZLAW_NATS_URL")
	}
	if natsURL == "" {
		// Try to get from hub config (Listen address)
		cfg, err := config.LoadHubConfig(config.DefaultHubConfigPath())
		if err == nil && cfg.NATS.Listen != "" {
			natsURL = "nats://" + cfg.NATS.Listen
		}
	}
	if natsURL == "" {
		return fmt.Errorf("NATS URL not configured: set ZLAW_NATS_URL or run 'zlaw hub start' first")
	}

	// Connect to NATS
	messenger, err := messaging.NewNATSMessenger(natsURL, "", c.NatsCred)
	if err != nil {
		return fmt.Errorf("connect to NATS: %w", err)
	}
	defer messenger.Close()

	// Determine subject to subscribe
	subject := "zlaw.logs"
	if c.Agent != "" {
		subject = fmt.Sprintf("agent.%s.logs", c.Agent)
	}

	// Set up signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(ctx)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	// Subscribe to logs
	logCh := make(chan []byte, 100)
	sub, err := messenger.Subscribe(ctx, subject, func(data []byte) {
		select {
		case logCh <- data:
		default:
			// Channel full, skip
		}
	})
	if err != nil {
		return fmt.Errorf("subscribe to %s: %w", subject, err)
	}
	defer sub.Unsubscribe() //nolint:errcheck

	noColor := logging.DetectNoColor()

	fmt.Fprintln(os.Stderr, "Streaming logs from hub...")
	if c.Agent != "" {
		fmt.Fprintf(os.Stderr, "Filtering by agent: %s\n", c.Agent)
	}
	if c.Level != "" {
		fmt.Fprintf(os.Stderr, "Minimum level: %s\n", c.Level)
	}
	fmt.Fprintf(os.Stderr, "Press Ctrl+C to stop\n\n")

	// Process log entries
	var minLevel int
	switch strings.ToLower(c.Level) {
	case "debug":
		minLevel = 0
	case "info":
		minLevel = 1
	case "warn", "warning":
		minLevel = 2
	case "error":
		minLevel = 3
	default:
		minLevel = 1 // default to info
	}

	since := c.Since
	if since == 0 {
		since = -1 // no filter
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case data := <-logCh:
			if err := c.printLogEntry(data, minLevel, since, noColor); err != nil {
				// Skip malformed entries silently
			}
		}
	}
}

func (c *AgentLogsCmd) printLogEntry(data []byte, minLevel int, since time.Duration, noColor bool) error {
	var entry struct {
		Time  string `json:"time"`
		Level string `json:"level"`
		Msg   string `json:"msg"`
		Agent string `json:"agent"`
	}
	if err := json.Unmarshal(data, &entry); err != nil {
		return err
	}

	// Filter by level
	entryLevel := levelToInt(entry.Level)
	if entryLevel < minLevel {
		return nil
	}

	// Filter by time
	if since > 0 {
		t, err := time.Parse(time.RFC3339Nano, entry.Time)
		if err == nil {
			if time.Since(t) > since {
				return nil
			}
		}
	}

	// Format and print
	label := fmt.Sprintf("[agent:%s]", entry.Agent)
	if c.Agent != "" {
		label = fmt.Sprintf("[agent:%s]", entry.Agent)
	}

	color := logging.AgentColor(entry.Agent)
	levelColor := levelColor(entry.Level)

	var sb strings.Builder
	if noColor {
		sb.WriteString(label)
		sb.WriteString(" ")
		sb.WriteString(strings.ToUpper(fmt.Sprintf("%-5s", entry.Level)))
	} else {
		sb.WriteString(logging.Colorize(color, label))
		sb.WriteString(" ")
		sb.WriteString(logging.Colorize(levelColor, strings.ToUpper(fmt.Sprintf("%-5s", entry.Level))))
	}
	sb.WriteString("  ")
	sb.WriteString(entry.Msg)

	// Print extra attributes
	var extra map[string]json.RawMessage
	if err := json.Unmarshal(data, &extra); err == nil {
		for k, v := range extra {
			if k == "time" || k == "level" || k == "msg" || k == "agent" {
				continue
			}
			sb.WriteString("  ")
			sb.WriteString(k)
			sb.WriteString("=")
			var val any
			json.Unmarshal(v, &val)
			sb.WriteString(formatAttr(val))
		}
	}

	fmt.Println(sb.String())
	return nil
}

func levelToInt(level string) int {
	switch strings.ToLower(level) {
	case "debug":
		return 0
	case "info":
		return 1
	case "warn", "warning":
		return 2
	case "error":
		return 3
	default:
		return 1
	}
}

func levelColor(level string) logging.Color {
	switch strings.ToLower(level) {
	case "debug":
		return logging.ColorGray
	case "info":
		return logging.ColorGreen
	case "warn", "warning":
		return logging.ColorYellow
	case "error":
		return logging.ColorRed
	default:
		return logging.ColorDefault
	}
}

func formatAttr(v any) string {
	switch val := v.(type) {
	case string:
		if strings.Contains(val, " ") || strings.Contains(val, "=") {
			return fmt.Sprintf("%q", val)
		}
		return val
	case nil:
		return "<nil>"
	default:
		return fmt.Sprintf("%v", val)
	}
}

func (c *AgentLogsCmd) SetFlags(fs *flag.FlagSet) {
	fs.StringVar(&c.Agent, "agent", "", "agent name to filter")
	fs.StringVar(&c.Level, "level", "", "minimum log level")
	fs.DurationVar(&c.Since, "since", 0, "show logs from the last N seconds")
	fs.BoolVar(&c.Follow, "follow", true, "follow logs continuously")
}

var _ flag.Value = (*AgentLogsCmd)(nil)

func (c *AgentLogsCmd) String() string {
	return ""
}

func (c *AgentLogsCmd) Set(s string) error {
	return nil
}
