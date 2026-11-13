package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

func (a *App) newFlagSet(name string, usage func()) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	// Parse errors are returned and reported once by the caller; the flag
	// package would otherwise print the same message first.
	flags.SetOutput(io.Discard)
	flags.Usage = usage
	return flags
}

func parseFlags(flags *flag.FlagSet, args []string) (bool, error) {
	err := flags.Parse(args)
	if errors.Is(err, flag.ErrHelp) {
		return true, nil
	}
	return false, err
}

// parseInterspersedFlags parses flags before or after positional arguments,
// so `opsquest play 4 --once` works like `opsquest play --once 4`. A "--"
// terminator still makes every later argument positional.
func parseInterspersedFlags(flags *flag.FlagSet, args []string) ([]string, bool, error) {
	var positionals []string
	for {
		if help, err := parseFlags(flags, args); help || err != nil {
			return nil, help, err
		}
		remaining := flags.Args()
		if len(remaining) == 0 {
			return positionals, false, nil
		}
		if consumed := len(args) - len(remaining); consumed > 0 && args[consumed-1] == "--" {
			return append(positionals, remaining...), false, nil
		}
		positionals = append(positionals, remaining[0])
		args = remaining[1:]
	}
}

// parseNoArgs handles commands that accept only -h/--help.
func (a *App) parseNoArgs(name string, args []string) (bool, error) {
	flags := a.newFlagSet(name, func() { fmt.Fprintf(a.errOut, "Usage: opsquest %s\n", name) })
	if help, err := parseFlags(flags, args); help || err != nil {
		return help, err
	}
	if flags.NArg() > 0 {
		return false, fmt.Errorf("%s does not accept arguments", name)
	}
	return false, nil
}

func flagProvided(flags *flag.FlagSet, name string) bool {
	provided := false
	flags.Visit(func(item *flag.Flag) { provided = provided || item.Name == name })
	return provided
}

// hasFlag reports whether args set the named flag in any form the flag
// package accepts: -name, --name, -name=value, or --name=value.
func hasFlag(args []string, name string) bool {
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		trimmed := strings.TrimPrefix(strings.TrimPrefix(arg, "-"), "-")
		if trimmed == arg {
			continue
		}
		if trimmed == name || strings.HasPrefix(trimmed, name+"=") {
			return true
		}
	}
	return false
}
