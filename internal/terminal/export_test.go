package terminal

import "io"

func RunTestEditor(in io.Reader, out io.Writer, request EditorRequest, save SaveFunc, size func() (int, int)) error {
	return runViEditor(newTerminalKeyReader(in), out, request, save, size)
}
