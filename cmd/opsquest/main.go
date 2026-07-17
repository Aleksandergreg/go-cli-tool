package main

import (
	"fmt"
	"os"

	"github.com/aleksandergregersen/opsquest/internal/cli"
)

func main() {
	app, err := cli.New(os.Stdin, os.Stdout, os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "opsquest: %v\n", err)
		os.Exit(1)
	}

	if err := app.Run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "opsquest: %v\n", err)
		os.Exit(1)
	}
}
