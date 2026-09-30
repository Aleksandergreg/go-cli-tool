package dockerlab

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/aleksandergregersen/opsquest/internal/mission"
)

const (
	maxDockerCommandLineBytes = 64 * 1024
	maxLogTailLines           = 10000
	maxListFilters            = 4
)

const dockerHelp = `Docker lab commands:
  docker ps [-a|--all]              List mission containers
  docker ps --filter KEY=VALUE      Filter by status=created|running|restarting|exited
                                    or health=starting|healthy|unhealthy|none
  docker container ls [OPTIONS]     List mission containers
  docker start ALIAS                Start a mission container
  docker container start ALIAS      Start a mission container
  docker restart ALIAS              Restart a mission container
  docker container restart ALIAS    Restart a mission container
  docker stop ALIAS                 Stop a mission container
  docker container stop ALIAS       Stop a mission container
  docker rm ALIAS                   Remove a stopped mission container
  docker container rm ALIAS         Remove a stopped mission container
  docker inspect ALIAS              Inspect logical container state
  docker container inspect ALIAS    Inspect logical container state
  docker logs ALIAS                 Read a mission container's logs
  docker logs --tail N ALIAS        Read only the last N log lines
  docker container logs ALIAS       Read a mission container's logs
  docker network ls                 List mission networks
  docker network inspect NETWORK    Inspect a network and its containers
  docker network create NETWORK     Create an internal mission network
  docker network rm NETWORK         Remove a network no container uses
  docker network connect NETWORK ALIAS
                                    Attach a container to a network
  docker network disconnect NETWORK ALIAS
                                    Detach a container from a network
  help                              Show this help
`

type actionKind uint8

const (
	actionHelp actionKind = iota
	actionList
	actionStart
	actionRestart
	actionStop
	actionRemove
	actionInspect
	actionLogs
	actionNetworkList
	actionNetworkCreate
	actionNetworkRemove
	actionNetworkInspect
	actionNetworkConnect
	actionNetworkDisconnect
)

// listFilter is one parsed docker ps filter. Filters are evaluated in Go over
// sanitized inspection data and are never forwarded to the Docker CLI.
type listFilter struct {
	key   string
	value string
}

type dockerAction struct {
	kind    actionKind
	alias   string
	all     bool
	filters []listFilter
	// tail is the number of trailing log lines to show; negative means all.
	tail int
	// network is the logical network name for docker network actions.
	network string
}

// aliasActions maps single-alias container subcommands to their actions.
var aliasActions = map[string]actionKind{"start": actionStart, "restart": actionRestart, "stop": actionStop, "inspect": actionInspect}

// networkActions maps single-network subcommands to their actions.
var networkActions = map[string]actionKind{"create": actionNetworkCreate, "rm": actionNetworkRemove, "remove": actionNetworkRemove, "inspect": actionNetworkInspect}

var listFilterValues = map[string]map[string]bool{
	"status": {"created": true, "running": true, "restarting": true, "exited": true},
	"health": {"starting": true, "healthy": true, "unhealthy": true, "none": true},
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
		return dockerAction{}, fmt.Errorf("usage: docker ps [-a|--all] | docker start ALIAS | docker stop ALIAS | docker rm ALIAS | docker inspect ALIAS | docker logs ALIAS")
	}

	if fields[0] == "network" {
		return parseNetwork(fields[1:])
	}
	if fields[0] == "container" {
		fields = fields[1:]
		if len(fields) == 0 {
			return dockerAction{}, fmt.Errorf("usage: docker container ls [-a|--all] | docker container start ALIAS | docker container stop ALIAS | docker container rm ALIAS | docker container inspect ALIAS | docker container logs ALIAS")
		}
	}
	switch fields[0] {
	case "ps", "ls":
		return parseList(fields[1:])
	case "logs":
		return parseLogs(fields[1:])
	case "rm":
		for _, field := range fields[1:] {
			if field == "-f" || field == "--force" {
				return dockerAction{}, fmt.Errorf("docker rm --force is outside this teaching subset; stop the container first, then remove it")
			}
		}
		if len(fields) != 2 || !mission.ValidDockerLogicalName(fields[1]) {
			return dockerAction{}, fmt.Errorf("usage: docker rm ALIAS")
		}
		return dockerAction{kind: actionRemove, alias: fields[1]}, nil
	case "start", "restart", "stop", "inspect":
		if len(fields) != 2 || !mission.ValidDockerLogicalName(fields[1]) {
			return dockerAction{}, fmt.Errorf("usage: docker %s ALIAS", fields[0])
		}
		return dockerAction{kind: aliasActions[fields[0]], alias: fields[1]}, nil
	default:
		return dockerAction{}, fmt.Errorf("docker %s is outside this mission's teaching subset; type help", fields[0])
	}
}

func parseNetwork(fields []string) (dockerAction, error) {
	const usage = "usage: docker network ls | inspect NETWORK | create NETWORK | rm NETWORK | connect NETWORK ALIAS | disconnect NETWORK ALIAS"
	if len(fields) == 0 {
		return dockerAction{}, errors.New(usage)
	}
	for _, field := range fields[1:] {
		if strings.HasPrefix(field, "-") {
			if fields[0] == "create" {
				return dockerAction{}, fmt.Errorf("docker network create options are fixed in this lab: every network OpsQuest creates is internal")
			}
			return dockerAction{}, fmt.Errorf("docker network %s options are outside this teaching subset", fields[0])
		}
	}
	name := func(value string) error {
		if mission.ReservedDockerNetworkName(value) {
			return fmt.Errorf("the built-in %q network is outside this lab", value)
		}
		if !mission.ValidDockerNetworkName(value) {
			return fmt.Errorf("network name %q must be a lowercase logical name", value)
		}
		return nil
	}
	switch fields[0] {
	case "ls", "list":
		if len(fields) != 1 {
			return dockerAction{}, fmt.Errorf("usage: docker network ls")
		}
		return dockerAction{kind: actionNetworkList}, nil
	case "create", "rm", "remove", "inspect":
		if len(fields) != 2 {
			return dockerAction{}, fmt.Errorf("usage: docker network %s NETWORK", fields[0])
		}
		if err := name(fields[1]); err != nil {
			return dockerAction{}, err
		}
		return dockerAction{kind: networkActions[fields[0]], network: fields[1]}, nil
	case "connect", "disconnect":
		if len(fields) != 3 {
			return dockerAction{}, fmt.Errorf("usage: docker network %s NETWORK ALIAS", fields[0])
		}
		if err := name(fields[1]); err != nil {
			return dockerAction{}, err
		}
		if !mission.ValidDockerLogicalName(fields[2]) {
			return dockerAction{}, fmt.Errorf("usage: docker network %s NETWORK ALIAS", fields[0])
		}
		kind := actionNetworkConnect
		if fields[0] == "disconnect" {
			kind = actionNetworkDisconnect
		}
		return dockerAction{kind: kind, network: fields[1], alias: fields[2]}, nil
	default:
		return dockerAction{}, fmt.Errorf("docker network %s is outside this mission's teaching subset; type help", fields[0])
	}
}

func parseList(fields []string) (dockerAction, error) {
	const usage = "usage: docker ps [-a|--all] [--filter status=VALUE|health=VALUE]"
	action := dockerAction{kind: actionList}
	for index := 0; index < len(fields); index++ {
		field := fields[index]
		var filter string
		switch {
		case field == "-a" || field == "--all":
			if action.all {
				return dockerAction{}, errors.New(usage)
			}
			action.all = true
			continue
		case field == "-f" || field == "--filter":
			if index+1 >= len(fields) {
				return dockerAction{}, fmt.Errorf("docker ps %s requires KEY=VALUE", field)
			}
			index++
			filter = fields[index]
		case strings.HasPrefix(field, "--filter="):
			filter = strings.TrimPrefix(field, "--filter=")
		default:
			return dockerAction{}, errors.New(usage)
		}
		parsed, err := parseListFilter(filter)
		if err != nil {
			return dockerAction{}, err
		}
		if len(action.filters) == maxListFilters {
			return dockerAction{}, fmt.Errorf("docker ps accepts at most %d filters in this lab", maxListFilters)
		}
		action.filters = append(action.filters, parsed)
	}
	return action, nil
}

func parseListFilter(value string) (listFilter, error) {
	key, filterValue, found := strings.Cut(value, "=")
	allowed, knownKey := listFilterValues[key]
	if !found || !knownKey {
		return listFilter{}, fmt.Errorf("docker ps filter %q is outside this teaching subset; use status=VALUE or health=VALUE", value)
	}
	if !allowed[filterValue] {
		return listFilter{}, fmt.Errorf("docker ps filter %s accepts %s", key, strings.Join(sortedKeys(allowed), ", "))
	}
	return listFilter{key: key, value: filterValue}, nil
}

func parseLogs(fields []string) (dockerAction, error) {
	const usage = "usage: docker logs [--tail N] ALIAS"
	action := dockerAction{kind: actionLogs, tail: -1}
	tailSet := false
	for len(fields) > 1 {
		var value string
		switch field := fields[0]; {
		case field == "--tail" || field == "-n":
			if len(fields) < 3 {
				return dockerAction{}, errors.New(usage)
			}
			value = fields[1]
			fields = fields[2:]
		case strings.HasPrefix(field, "--tail="):
			value = strings.TrimPrefix(field, "--tail=")
			fields = fields[1:]
		default:
			return dockerAction{}, errors.New(usage)
		}
		if tailSet {
			return dockerAction{}, errors.New(usage)
		}
		tail, err := parseTail(value)
		if err != nil {
			return dockerAction{}, err
		}
		action.tail = tail
		tailSet = true
	}
	if len(fields) != 1 || !mission.ValidDockerLogicalName(fields[0]) {
		return dockerAction{}, errors.New(usage)
	}
	action.alias = fields[0]
	return action, nil
}

func parseTail(value string) (int, error) {
	if value == "all" {
		return -1, nil
	}
	if value == "" || strings.TrimLeft(value, "0123456789") != "" {
		return 0, fmt.Errorf("docker logs --tail requires a whole number or all")
	}
	tail, err := strconv.Atoi(value)
	if err != nil || tail > maxLogTailLines {
		return 0, fmt.Errorf("docker logs --tail accepts at most %d lines in this lab", maxLogTailLines)
	}
	return tail, nil
}

func sortedKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
