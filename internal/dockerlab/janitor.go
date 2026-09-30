package dockerlab

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/aleksandergregersen/opsquest/internal/mission"
)

var (
	sessionIDPattern     = regexp.MustCompile(`^[a-f0-9]{24}$`)
	generatedNamePattern = regexp.MustCompile(`^/?opsquest-([a-f0-9]{24})-c[0-9]{2}$`)
)

// orphanMinimumAge is when a fixture without a verifiable owner process is
// considered abandoned. Every fixture process exits by then, so removing it
// cannot interrupt a playable attempt.
const orphanMinimumAge = fixtureLifetime

// OrphanedResources counts OpsQuest fixtures left behind by processes that
// exited without cleanup. It never modifies Docker state.
func (f *Factory) OrphanedResources(ctx context.Context) (int, error) {
	orphans, err := f.findOrphans(ctx)
	return len(orphans), err
}

// RemoveOrphanedResources removes OpsQuest fixtures left behind by processes
// that exited without cleanup and reports how many were removed.
func (f *Factory) RemoveOrphanedResources(ctx context.Context) (int, error) {
	return f.removeOrphans(ctx)
}

func (f *Factory) removeOrphans(ctx context.Context) (int, error) {
	orphans, err := f.findOrphans(ctx)
	if err != nil {
		return 0, err
	}
	removed := 0
	var removeErrors []error
	for _, id := range orphans {
		result, err := runDocker(ctx, f.runner, "container", "rm", "--force", id)
		if err != nil && !isMissingContainer(result) {
			removeErrors = append(removeErrors, fmt.Errorf("remove orphaned Docker fixture %s: %w", id[:12], err))
			continue
		}
		if err == nil {
			removed++
		}
	}
	return removed, errors.Join(removeErrors...)
}

// findOrphans lists managed containers and keeps only exact IDs whose
// complete ownership labels and generated name prove OpsQuest created them
// and whose owner is provably gone.
func (f *Factory) findOrphans(ctx context.Context) ([]string, error) {
	if f.lookupErr != nil || f.runner == nil {
		return nil, fmt.Errorf("docker executable not found in PATH")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := runDocker(ctx, f.runner,
		"container", "ls", "--all", "--quiet", "--no-trunc",
		"--filter", "label="+managedLabel+"=true",
	)
	if err != nil {
		return nil, err
	}
	var orphans []string
	for _, id := range strings.Fields(result.stdout) {
		if !containerIDPattern.MatchString(id) {
			return nil, fmt.Errorf("Docker container listing returned an invalid container ID")
		}
		inspection, exists, err := inspectReference(ctx, f.runner, id)
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		if inspection.ID != id {
			return nil, fmt.Errorf("Docker inspection returned an unexpected container ID")
		}
		if f.isOrphan(inspection) {
			orphans = append(orphans, id)
		}
	}
	return orphans, nil
}

func (f *Factory) isOrphan(inspection containerInspection) bool {
	labels := inspection.Config.Labels
	session := labels[sessionLabel]
	if labels[managedLabel] != "true" || labels[schemaLabel] != "1" || !sessionIDPattern.MatchString(session) ||
		!mission.ValidDockerLogicalName(labels[missionLabel]) || !mission.ValidDockerLogicalName(labels[aliasLabel]) {
		return false
	}
	match := generatedNamePattern.FindStringSubmatch(inspection.Name)
	if match == nil || match[1] != session {
		return false
	}
	// An owner on this host can be checked directly. Owners on another host,
	// and fixtures created before owner labels existed, fall back to age.
	if host := labels[ownerHostLabel]; host != "" && host == f.owner.host && f.owner.alive != nil {
		if pid, err := strconv.Atoi(labels[ownerPIDLabel]); err == nil && pid > 0 && !f.owner.alive(pid) {
			return true
		}
	}
	created, err := time.Parse(time.RFC3339Nano, inspection.Created)
	if err != nil || f.owner.now == nil {
		return false
	}
	return f.owner.now().Sub(created) > orphanMinimumAge
}
