package game

import (
	"context"
	"io"

	"github.com/aleksandergregersen/opsquest/internal/terminal"
)

type CommandLineReader = terminal.CommandLineReader

var missionControls = []string{
	"?", ":q", "exit", "guide", "hint", "list", "map", "missions",
	"next", "objective", "opsquest", "play", "prev", "previous", "quit", "restart", "status", "world", "worlds",
}

func NewCommandLineReaderContext(ctx context.Context, in io.Reader, out io.Writer) CommandLineReader {
	return terminal.NewReader(ctx, in, out, missionControls)
}
