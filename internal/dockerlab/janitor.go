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
	networkNamePattern   = regexp.MustCompile(`^opsquest-([a-f0-9]{24})-n[0-9]{2}$`)
)

// orphanMinimumAge is when a resource without a verifiable owner process is
// considered abandoned. Every fixture process exits by then, so removing it
// cannot interrupt a playable attempt.
const orphanMinimumAge = fixtureLifetime

// orphanSet holds exact IDs of abandoned resources. Containers are removed
// before networks because Docker refuses to remove a network in use.
type orphanSet struct {
	containers []string
	networks   []string
}

func (o orphanSet) count() int {
	return len(o.containers) + len(o.networks)
}

// OrphanedResources counts OpsQuest containers and networks left behind by
// processes that exited without cleanup. It never modifies Docker state.
func (f *Factory) OrphanedResources(ctx context.Context) (int, error) {
	orphans, err := f.findOrphans(ctx)
	return orphans.count(), err
}

// RemoveOrphanedResources removes OpsQuest containers and networks left
// behind by processes that exited without cleanup and reports how many were
// removed.
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
	for _, id := range orphans.containers {
		result, err := runDocker(ctx, f.runner, "container", "rm", "--force", id)
		if err != nil && !isMissingContainer(result) {
			removeErrors = append(removeErrors, fmt.Errorf("remove orphaned Docker fixture %s: %w", id[:12], err))
			continue
		}
		if err == nil {
			removed++
		}
	}
	for _, id := range orphans.networks {
		result, err := runDocker(ctx, f.runner, "network", "rm", id)
		if err != nil && !isMissingNetwork(result) {
			removeErrors = append(removeErrors, fmt.Errorf("remove orphaned Docker network %s: %w", id[:12], err))
			continue
		}
		if err == nil {
			removed++
		}
	}
	return removed, errors.Join(removeErrors...)
}

// findOrphans lists managed containers and networks and keeps only exact IDs
// whose complete ownership labels and generated name prove OpsQuest created
// them and whose owner is provably gone.
func (f *Factory) findOrphans(ctx context.Context) (orphanSet, error) {
	if f.lookupErr != nil || f.runner == nil {
		return orphanSet{}, fmt.Errorf("docker executable not found in PATH")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var orphans orphanSet
	containerIDs, err := f.listManaged(ctx, "container", "ls", "--all")
	if err != nil {
		return orphanSet{}, err
	}
	for _, id := range containerIDs {
		inspection, exists, err := inspectReference(ctx, f.runner, id)
		if err != nil {
			return orphanSet{}, err
		}
		if !exists {
			continue
		}
		if inspection.ID != id {
			return orphanSet{}, fmt.Errorf("Docker inspection returned an unexpected container ID")
		}
		if f.isOrphan(inspection.Config.Labels, inspection.Name, inspection.Created, generatedNamePattern) {
			orphans.containers = append(orphans.containers, id)
		}
	}
	networkIDs, err := f.listManaged(ctx, "network", "ls")
	if err != nil {
		return orphanSet{}, err
	}
	for _, id := range networkIDs {
		inspection, exists, err := inspectNetworkReference(ctx, f.runner, id)
		if err != nil {
			return orphanSet{}, err
		}
		if !exists {
			continue
		}
		if inspection.ID != id {
			return orphanSet{}, fmt.Errorf("Docker inspection returned an unexpected network ID")
		}
		if f.isOrphan(inspection.Labels, inspection.Name, inspection.Created, networkNamePattern) {
			orphans.networks = append(orphans.networks, id)
		}
	}
	return orphans, nil
}

// listManaged returns the exact IDs of resources carrying the managed label.
func (f *Factory) listManaged(ctx context.Context, command ...string) ([]string, error) {
	args := append(append([]string(nil), command...), "--quiet", "--no-trunc", "--filter", "label="+managedLabel+"=true")
	result, err := runDocker(ctx, f.runner, args...)
	if err != nil {
		return nil, err
	}
	ids := strings.Fields(result.stdout)
	for _, id := range ids {
		if !containerIDPattern.MatchString(id) {
			return nil, fmt.Errorf("Docker %s listing returned an invalid %s ID", command[0], command[0])
		}
	}
	return ids, nil
}

func (f *Factory) isOrphan(labels map[string]string, name, created string, namePattern *regexp.Regexp) bool {
	session := labels[sessionLabel]
	if labels[managedLabel] != "true" || labels[schemaLabel] != "1" || !sessionIDPattern.MatchString(session) ||
		!mission.ValidDockerLogicalName(labels[missionLabel]) || !mission.ValidDockerLogicalName(labels[aliasLabel]) {
		return false
	}
	match := namePattern.FindStringSubmatch(name)
	if match == nil || match[1] != session {
		return false
	}
	// An owner on this host can be checked directly. Owners on another host,
	// and resources created before owner labels existed, fall back to age.
	if host := labels[ownerHostLabel]; host != "" && host == f.owner.host && f.owner.alive != nil {
		if pid, err := strconv.Atoi(labels[ownerPIDLabel]); err == nil && pid > 0 && !f.owner.alive(pid) {
			return true
		}
	}
	createdAt, err := time.Parse(time.RFC3339Nano, created)
	if err != nil || f.owner.now == nil {
		return false
	}
	return f.owner.now().Sub(createdAt) > orphanMinimumAge
}
