package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"context"

	"github.com/chickenzord/zlaw/internal/dotenv"
)

func main() {
	_ = dotenv.LoadCwd()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel(),
	}))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fs := flag.NewFlagSet("zlaw-hub", flag.ContinueOnError)
	fs.Usage = func() { printUsage(fs) }

	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(1)
	}

	args := fs.Args()
	if len(args) == 0 {
		printUsage(fs)
		os.Exit(1)
	}

	var err error
	switch args[0] {
	case "init":
		err = runInit(args[1:])
	case "auth":
		err = runAuth(args[1:])
	case "start":
		err = runStart(ctx, args[1:], logger)
	case "status":
		err = runStatus(args[1:])
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
	fmt.Fprintln(os.Stderr, "usage: zlaw-hub <command> [args]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "environment variables:")
	fmt.Fprintln(os.Stderr, "  ZLAW_HOME              root directory for hub data (default: $HOME/.zlaw)")
	fmt.Fprintln(os.Stderr, "  ZLAW_CREDENTIALS_FILE  override credentials file path")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "commands:")
	fmt.Fprintln(os.Stderr, "  init    scaffold $ZLAW_HOME with hub and manager agent configuration")
	fmt.Fprintln(os.Stderr, "  auth    manage hub credential profiles")
	fmt.Fprintln(os.Stderr, "  start   start the hub (stub — Phase 2)")
	fmt.Fprintln(os.Stderr, "  status  show hub status (stub — Phase 2)")
}

func logLevel() slog.Level {
	if os.Getenv("ZLAW_LOG_LEVEL") == "debug" {
		return slog.LevelDebug
	}
	return slog.LevelInfo
}
