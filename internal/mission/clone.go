package mission

import (
	"maps"
	"slices"
)

func cloneMission(item Mission) Mission {
	cloned := item
	cloned.SuggestedCommands = slices.Clone(item.SuggestedCommands)
	cloned.Hints = slices.Clone(item.Hints)
	cloned.Setup.Directories = slices.Clone(item.Setup.Directories)
	cloned.Setup.Files = slices.Clone(item.Setup.Files)
	cloned.Setup.Processes = slices.Clone(item.Setup.Processes)
	cloned.Setup.Archives = make([]ArchiveSpec, len(item.Setup.Archives))
	for index, archive := range item.Setup.Archives {
		cloned.Setup.Archives[index] = archive
		cloned.Setup.Archives[index].Entries = slices.Clone(archive.Entries)
	}
	cloned.Setup.Environment = maps.Clone(item.Setup.Environment)
	if item.Docker != nil {
		dockerSetup := *item.Docker
		dockerSetup.Images = slices.Clone(item.Docker.Images)
		dockerSetup.Networks = slices.Clone(item.Docker.Networks)
		dockerSetup.Containers = slices.Clone(item.Docker.Containers)
		for index, container := range dockerSetup.Containers {
			dockerSetup.Containers[index].Networks = slices.Clone(container.Networks)
			if container.ExitCode != nil {
				exitCode := *container.ExitCode
				dockerSetup.Containers[index].ExitCode = &exitCode
			}
		}
		cloned.Docker = &dockerSetup
	}
	cloned.Validation.All = make([]Condition, len(item.Validation.All))
	for index, condition := range item.Validation.All {
		cloned.Validation.All[index] = condition
		cloned.Validation.All[index].Values = slices.Clone(condition.Values)
		cloned.Validation.All[index].Containers = slices.Clone(condition.Containers)
		if condition.Count != nil {
			count := *condition.Count
			cloned.Validation.All[index].Count = &count
		}
	}
	return cloned
}

func cloneMissions(items []Mission) []Mission {
	cloned := make([]Mission, len(items))
	for index, item := range items {
		cloned[index] = cloneMission(item)
	}
	return cloned
}
