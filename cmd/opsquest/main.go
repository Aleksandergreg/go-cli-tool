package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/aleksandergregersen/opsquest/internal/cli"
	"github.com/aleksandergregersen/opsquest/internal/dockerlab"
	"github.com/aleksandergregersen/opsquest/internal/game"
	"github.com/aleksandergregersen/opsquest/internal/mission"
	"github.com/aleksandergregersen/opsquest/internal/profile"
	"github.com/aleksandergregersen/opsquest/internal/ui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, ui.Auto(os.Stderr).Failure("opsquest: "+err.Error()))
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	catalog, err := mission.LoadCatalog()
	if err != nil {
		return fmt.Errorf("load mission catalog: %w", err)
	}
	store, err := profile.DefaultStore()
	if err != nil {
		return fmt.Errorf("initialize profile store: %w", err)
	}
	app := cli.New(cli.Config{
		Context: ctx,
		In:      os.Stdin,
		Out:     os.Stdout,
		ErrOut:  os.Stderr,
		Catalog: catalog,
		Store:   store,
		Factory: dockerlab.NewFactory(game.SandboxFactory{}),
	})
	return app.Run(os.Args[1:])
}
