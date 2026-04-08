package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel(),
	}))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Global flags — parsed before the subcommand.
	fs := flag.NewFlagSet("zlaw-agent", flag.ContinueOnError)
	agentName := fs.String("agent", "", "agent name; resolves to $ZLAW_HOME/agents/<name>")
	agentDir := fs.String("agent-dir", "", "explicit path to agent directory (overrides --agent)")
	fs.Usage = func() { printUsage(fs) }

	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(1)
	}

	// Env-var fallbacks.
	if *agentDir == "" {
		*agentDir = os.Getenv("ZLAW_AGENT_DIR")
	}
	if *agentName == "" {
		*agentName = os.Getenv("ZLAW_AGENT")
	}

	args := fs.Args()
	if len(args) == 0 {
		printUsage(fs)
		os.Exit(1)
	}

	var err error
	switch args[0] {
	case "auth":
		err = runAuth(args[1:])
	case "run":
		err = runRun(ctx, args[1:], *agentName, *agentDir, logger)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", args[0])
		printUsage(fs)
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func printUsage(fs *flag.FlagSet) {
	fmt.Fprintln(os.Stderr, "usage: zlaw-agent [global flags] <command> [args]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "global flags:")
	fs.PrintDefaults()
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "environment variables:")
	fmt.Fprintln(os.Stderr, "  ZLAW_HOME          root directory for agents, sessions, and credentials (default: $PWD)")
	fmt.Fprintln(os.Stderr, "  ZLAW_AGENT         agent name (same as --agent)")
	fmt.Fprintln(os.Stderr, "  ZLAW_AGENT_DIR     explicit agent directory (same as --agent-dir)")
	fmt.Fprintln(os.Stderr, "  ZLAW_CREDENTIALS_FILE  override credentials file path")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "commands:")
	fmt.Fprintln(os.Stderr, "  auth    manage authentication credentials")
	fmt.Fprintln(os.Stderr, "  run     start the agent (interactive or stdin)")
}

// logLevel returns slog.LevelDebug when ZLAW_LOG_LEVEL=debug, else Info.
func logLevel() slog.Level {
	if os.Getenv("ZLAW_LOG_LEVEL") == "debug" {
		return slog.LevelDebug
	}
	return slog.LevelInfo
}
