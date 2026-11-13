package dockerlab

import (
	"os"
	"strconv"
	"time"
)

// Every engine resource an attempt creates carries these labels. Cleanup and
// the orphan janitor remove a resource only when all of them match.
const (
	managedLabel = "com.opsquest.managed"
	schemaLabel  = "com.opsquest.schema"
	sessionLabel = "com.opsquest.session"
	missionLabel = "com.opsquest.mission"
	aliasLabel   = "com.opsquest.alias"
	// Owner labels let a later process recognize resources whose OpsQuest
	// process exited without cleanup. They are advisory: removal still
	// requires every ownership label above to match.
	ownerPIDLabel  = "com.opsquest.owner-pid"
	ownerHostLabel = "com.opsquest.owner-host"
)

// processOwner identifies the current OpsQuest process for orphan detection
// and decides whether another recorded owner process is still alive.
type processOwner struct {
	pid   int
	host  string
	alive func(int) bool
	now   func() time.Time
}

func currentProcessOwner() processOwner {
	host, err := os.Hostname()
	if err != nil {
		host = ""
	}
	return processOwner{pid: os.Getpid(), host: host, alive: processAlive, now: time.Now}
}

// ownershipLabels returns the --label arguments for one attempt resource
// with the given logical alias.
func (e *environment) ownershipLabels(alias string) []string {
	args := []string{
		"--label", managedLabel + "=true",
		"--label", schemaLabel + "=1",
		"--label", sessionLabel + "=" + e.sessionID,
		"--label", missionLabel + "=" + e.missionID,
		"--label", aliasLabel + "=" + alias,
	}
	if e.owner.host != "" && e.owner.pid > 0 {
		args = append(args,
			"--label", ownerPIDLabel+"="+strconv.Itoa(e.owner.pid),
			"--label", ownerHostLabel+"="+e.owner.host,
		)
	}
	return args
}

// ownedByAttempt reports whether labels prove a resource belongs to this
// attempt.
func (e *environment) ownedByAttempt(labels map[string]string) bool {
	return labels[managedLabel] == "true" && labels[schemaLabel] == "1" &&
		labels[sessionLabel] == e.sessionID && labels[missionLabel] == e.missionID
}

// owns reports whether labels prove a resource is this attempt's resource
// with the given logical alias.
func (e *environment) owns(labels map[string]string, alias string) bool {
	return e.ownedByAttempt(labels) && labels[aliasLabel] == alias
}
