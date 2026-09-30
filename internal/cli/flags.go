package cli

import (
	"errors"
	"flag"
	"strings"
)

func (a *App) newFlagSet(name string, usage func()) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(a.errOut)
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

func flagProvided(flags *flag.FlagSet, name string) bool {
	provided := false
	flags.Visit(func(item *flag.Flag) { provided = provided || item.Name == name })
	return provided
}

func hasFlag(args []string, name string) bool {
	for _, arg := range args {
		if arg == name || strings.HasPrefix(arg, name+"=") {
			return true
		}
	}
	return false
}
