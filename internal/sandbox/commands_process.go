package sandbox

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

func (s *Sandbox) cmdPS(args []string) (string, error) {
	for _, arg := range args {
		if arg != "-e" && arg != "-ef" && arg != "aux" && arg != "-A" {
			return "", fmt.Errorf("unsupported option %s", arg)
		}
	}
	pids := make([]int, 0, len(s.Processes))
	for pid, process := range s.Processes {
		if process.Running {
			pids = append(pids, pid)
		}
	}
	sort.Ints(pids)
	var output commandOutputBuffer
	output.WriteString("  PID COMMAND\n")
	for _, pid := range pids {
		output.WriteString(fmt.Sprintf("%5d %s\n", pid, s.Processes[pid].Command))
	}
	return output.Result()
}

func (s *Sandbox) cmdKill(args []string) (string, error) {
	var pids []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			signal := strings.TrimPrefix(arg, "-")
			if signal != "9" && signal != "15" && signal != "TERM" && signal != "KILL" {
				return "", fmt.Errorf("unsupported signal %s", arg)
			}
			continue
		}
		pids = append(pids, arg)
	}
	if len(pids) == 0 {
		return "", fmt.Errorf("missing PID")
	}
	for _, value := range pids {
		pid, err := strconv.Atoi(value)
		if err != nil {
			return "", fmt.Errorf("%s: PID must be a number", value)
		}
		process, exists := s.Processes[pid]
		if !exists || !process.Running {
			return "", fmt.Errorf("%d: no such process", pid)
		}
		process.Running = false
	}
	return "", nil
}
