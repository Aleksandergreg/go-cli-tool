package dockerlab

import (
	"context"
	"fmt"
	"strings"
)

func (e *environment) listContainers(ctx context.Context, all bool, filters []listFilter) (string, error) {
	type row struct {
		logicalID string
		image     string
		status    string
		alias     string
	}
	rows := make([]row, 0, len(e.containers))
	// Eight columns preserves the original listing layout for plain states.
	statusWidth := 8
	for _, tracked := range e.snapshotContainers() {
		inspection, exists, err := e.inspect(ctx, tracked.id)
		if err != nil {
			return "", err
		}
		// A status filter selects stopped containers even without --all,
		// matching docker ps.
		if !exists || !all && !hasFilterKey(filters, "status") && !inspection.State.Running || !matchesFilters(inspection, filters) {
			continue
		}
		status := displayStatus(inspection)
		statusWidth = max(statusWidth, len(status))
		rows = append(rows, row{logicalID: tracked.logicalID, image: tracked.imageAlias, status: status, alias: tracked.alias})
	}
	var output strings.Builder
	fmt.Fprintf(&output, "%-13s %-9s %-*s %s\n", "CONTAINER ID", "IMAGE", statusWidth, "STATUS", "NAMES")
	for _, item := range rows {
		fmt.Fprintf(&output, "%-13s %-9s %-*s %s\n", item.logicalID, item.image, statusWidth, item.status, item.alias)
	}
	return output.String(), nil
}

func displayStatus(inspection containerInspection) string {
	status := inspection.State.Status
	if status == "" {
		if inspection.State.Running {
			status = "running"
		} else {
			status = "stopped"
		}
	}
	if health := healthStatus(inspection); inspection.State.Running && health != "none" {
		status += " (" + health + ")"
	}
	return status
}

// healthStatus reports the probe verdict, or none for a fixture without a
// health probe. Like docker ps, listings only show it for running containers.
func healthStatus(inspection containerInspection) string {
	if inspection.State.Health == nil || inspection.State.Health.Status == "" {
		return "none"
	}
	return inspection.State.Health.Status
}

func hasFilterKey(filters []listFilter, key string) bool {
	for _, filter := range filters {
		if filter.key == key {
			return true
		}
	}
	return false
}

// matchesFilters follows docker ps semantics: values for one key are
// alternatives, and different keys must all match.
func matchesFilters(inspection containerInspection, filters []listFilter) bool {
	matched := make(map[string]bool, len(filters))
	for _, filter := range filters {
		if _, seen := matched[filter.key]; !seen {
			matched[filter.key] = false
		}
		var actual string
		switch filter.key {
		case "status":
			actual = inspection.State.Status
		case "health":
			actual = "none"
			if inspection.State.Running {
				actual = healthStatus(inspection)
			}
		}
		if actual == filter.value {
			matched[filter.key] = true
		}
	}
	for _, ok := range matched {
		if !ok {
			return false
		}
	}
	return true
}
