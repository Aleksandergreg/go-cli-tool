package sandbox

import (
	"fmt"
	"strings"
)

func (s *Sandbox) cmdExport(args []string) (string, error) {
	if len(args) == 0 {
		return s.cmdEnv(nil)
	}
	environment := cloneEnvironment(s.Env)
	for _, assignment := range args {
		key, value, found := strings.Cut(assignment, "=")
		if !found || !validVariableName(key) {
			return "", fmt.Errorf("expected NAME=value, got %q", assignment)
		}
		environment[key] = value
	}
	if err := validateEnvironment(environment); err != nil {
		return "", err
	}
	s.Env = environment
	return "", nil
}

func (s *Sandbox) cmdEnv(args []string) (string, error) {
	if len(args) > 0 {
		return "", fmt.Errorf("running a command through env is not supported in this lab")
	}
	var output commandOutputBuffer
	for _, key := range sortedKeys(s.Env) {
		output.WriteString(key + "=" + s.Env[key] + "\n")
	}
	return output.Result()
}

func validVariableName(name string) bool {
	if name == "" {
		return false
	}
	for index, char := range name {
		if !(char == '_' || char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || index > 0 && char >= '0' && char <= '9') {
			return false
		}
	}
	return true
}
