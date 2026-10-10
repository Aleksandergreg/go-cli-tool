package sandbox

import "errors"

// Exit statuses the lab models beyond 0 and 1. Any other failure exits with
// 1, as most GNU tools do, and $? reports these values like a real shell.
const (
	// statusTrouble is sh's status for a syntax error and the documented
	// status of ls, grep, and sort for errors such as a missing file.
	statusTrouble = 2
	// statusCannotExecute is for a script that exists but cannot run.
	statusCannotExecute = 126
	// statusCommandNotFound is for a name the lab does not provide.
	statusCommandNotFound = 127
)

// exitStatusError gives a failure its shell exit status.
type exitStatusError struct {
	status int
	err    error
}

func (e exitStatusError) Error() string { return e.err.Error() }

func (e exitStatusError) Unwrap() error { return e.err }

// withExitStatus marks err with status. A silent failing status, such as grep
// finding no match, keeps its status of 1.
func withExitStatus(status int, err error) error {
	if err == nil || errors.Is(err, errFailureStatus) {
		return err
	}
	return exitStatusError{status: status, err: err}
}

// exitStatus is the shell status of a pipeline's outcome: 0 on success, the
// outermost marked status of a failure, or 1.
func exitStatus(err error, statusFailed bool) int {
	var marked exitStatusError
	switch {
	case errors.As(err, &marked):
		return marked.status
	case err != nil || statusFailed:
		return 1
	}
	return 0
}
