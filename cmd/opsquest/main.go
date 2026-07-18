package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/aleksandergregersen/opsquest/internal/cli"
	"github.com/aleksandergregersen/opsquest/internal/ui"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	errorStyle := ui.Auto(os.Stderr)
	app, err := cli.NewWithContext(ctx, os.Stdin, os.Stdout, os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, errorStyle.Failure("opsquest: "+err.Error()))
		os.Exit(1)
	}

	if err := app.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, errorStyle.Failure("opsquest: "+err.Error()))
		os.Exit(1)
	}
}
