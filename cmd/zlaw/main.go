package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/alecthomas/kong"

	"github.com/zsomething/zlaw/internal/dotenv"
)

var cli struct {
	Init  InitCmd  `cmd:"" help:"bootstrap $ZLAW_HOME or create a named agent"`
	Auth  AuthCmd  `cmd:"" help:"manage credential profiles"`
	Hub   HubCmd   `cmd:"" help:"manage the zlaw hub"`
	Agent AgentCmd `cmd:"" help:"manage and run agents"`
}

func main() {
	_ = dotenv.LoadCwd()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel(),
	}))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	kctx := kong.Parse(&cli,
		kong.Name("zlaw"),
		kong.Description("multi-agent personal assistant platform"),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{Compact: true}),
		kong.BindTo(ctx, (*context.Context)(nil)),
		kong.Bind(logger),
	)
	err := kctx.Run()
	kctx.FatalIfErrorf(err)
}
