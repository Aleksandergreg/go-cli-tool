package dockerlab

import (
	"fmt"
	"strings"
	"unicode"
)

const maxDockerCommandLineBytes = 64 * 1024

const dockerHelp = `Docker lab commands:
  docker ps [-a|--all]              List mission containers
  docker container ls [-a|--all]    List mission containers
  docker start ALIAS                Start a mission container
  docker container start ALIAS      Start a mission container
  docker restart ALIAS              Restart a mission container
  docker container restart ALIAS    Restart a mission container
  docker inspect ALIAS              Inspect logical container state
  help                              Show this help
`

type actionKind uint8

const (
	actionHelp actionKind = iota
	actionList
	actionStart
	actionRestart
	actionInspect
)

type dockerAction struct {
	kind  actionKind
	alias string
	all   bool
}

func parseAction(line string) (dockerAction, error) {
	if len(line) > maxDockerCommandLineBytes {
		return dockerAction{}, fmt.Errorf("command line exceeds the %d KiB limit", maxDockerCommandLineBytes/1024)
	}
	for _, value := range line {
		if value == 0 || unicode.IsControl(value) && value != '\t' && value != '\n' && value != '\r' {
			return dockerAction{}, fmt.Errorf("Docker command contains an unsupported control character")
		}
	}
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return dockerAction{}, fmt.Errorf("enter a Docker lab command; type help to see the supported subset")
	}
	if len(fields) == 1 && fields[0] == "help" || len(fields) == 2 && fields[0] == "help" && fields[1] == "docker" || len(fields) == 2 && fields[0] == "docker" && (fields[1] == "help" || fields[1] == "--help") {
		return dockerAction{kind: actionHelp}, nil
	}
	if fields[0] != "docker" {
		return dockerAction{}, fmt.Errorf("%s: command not available in this Docker lab; type help", fields[0])
	}
	fields = fields[1:]
	if len(fields) == 0 {
		return dockerAction{}, fmt.Errorf("usage: docker ps [-a|--all] | docker start ALIAS | docker inspect ALIAS")
	}

	if fields[0] == "container" {
		fields = fields[1:]
		if len(fields) == 0 {
			return dockerAction{}, fmt.Errorf("usage: docker container ls [-a|--all] | docker container start ALIAS")
		}
	}
	switch fields[0] {
	case "ps", "ls":
		if len(fields) == 1 {
			return dockerAction{kind: actionList}, nil
		}
		if len(fields) == 2 && (fields[1] == "-a" || fields[1] == "--all") {
			return dockerAction{kind: actionList, all: true}, nil
		}
		return dockerAction{}, fmt.Errorf("usage: docker ps [-a|--all]")
	case "start", "restart", "inspect":
		if len(fields) != 2 || !logicalNamePattern.MatchString(fields[1]) {
			return dockerAction{}, fmt.Errorf("usage: docker %s ALIAS", fields[0])
		}
		kind := actionStart
		if fields[0] == "restart" {
			kind = actionRestart
		} else if fields[0] == "inspect" {
			kind = actionInspect
		}
		return dockerAction{kind: kind, alias: fields[1]}, nil
	default:
		return dockerAction{}, fmt.Errorf("docker %s is outside this mission's teaching subset; type help", fields[0])
	}
}
