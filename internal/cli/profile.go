package cli

import (
	"fmt"
	"strings"

	"github.com/aleksandergregersen/opsquest/internal/game"
	"github.com/aleksandergregersen/opsquest/internal/mission"
	"github.com/aleksandergregersen/opsquest/internal/profile"
)

func (a *App) runProfile(args []string) error {
	flags := a.newFlagSet("profile", func() { a.printProfileUsage(a.errOut) })
	name := flags.String("name", "", "update the operator display name")
	if help, err := parseFlags(flags, args); help || err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("profile does not accept positional arguments")
	}
	nameProvided := flagProvided(flags, "name")
	player, err := a.loadPlayer()
	if err != nil {
		return err
	}
	if nameProvided {
		if err := profile.ValidateName(*name); err != nil {
			return err
		}
		player.Name = strings.TrimSpace(*name)
		if err := a.store.Save(player); err != nil {
			return err
		}
		fmt.Fprintln(a.out, a.style.Success("Profile name updated."))
	}
	completed := completedMissions(a.catalog.All(), player)
	type worldProgress struct {
		name      string
		track     string
		number    int
		completed int
		total     int
	}
	progress := make([]worldProgress, 0)
	for _, track := range []string{mission.TrackLinux, mission.TrackDocker} {
		for _, world := range a.catalog.Worlds(track) {
			item := worldProgress{name: world.Name, track: track, number: world.Number, completed: completedMissions(world.Missions, player), total: len(world.Missions)}
			progress = append(progress, item)
		}
	}
	linuxMissions, dockerMissions := a.catalog.InTrack(mission.TrackLinux), a.catalog.InTrack(mission.TrackDocker)
	linuxCompleted, linuxTotal := completedMissions(linuxMissions, player), len(linuxMissions)
	dockerCompleted, dockerTotal := completedMissions(dockerMissions, player), len(dockerMissions)
	fmt.Fprintln(a.out, a.style.Header("PROFILE"))
	fmt.Fprintln(a.out)
	fmt.Fprintf(a.out, "%s %s\n", a.style.Accent("Operator:"), player.Name)
	fmt.Fprintf(a.out, "%s %s\n", a.style.Accent("Rank:"), player.Rank())
	fmt.Fprintf(a.out, "%s %d · %s\n", a.style.Accent("Level:"), player.Level(), a.style.Reward(fmt.Sprintf("%d XP", player.XP)))
	if nextRank, needed, exists := player.NextRank(); exists {
		fmt.Fprintf(a.out, "%s %s in %s\n", a.style.Muted("Next rank:"), nextRank, a.style.Reward(fmt.Sprintf("%d XP", needed)))
	}
	fmt.Fprintln(a.out)
	labelWidth := 0
	for _, world := range progress {
		label := fmt.Sprintf("World %d · %s", world.number, world.name)
		labelWidth = max(labelWidth, len(label))
	}
	printWorlds := func(track string) {
		for _, world := range progress {
			if world.track != track {
				continue
			}
			label := fmt.Sprintf("World %d · %s", world.number, world.name)
			fmt.Fprintf(a.out, "  %s %s %3d%%\n", a.style.World(fmt.Sprintf("%-*s", labelWidth, label)), styledProgressBar(a.style, world.completed, world.total, 10), percentage(world.completed, world.total))
		}
	}
	fmt.Fprintf(a.out, "%s  %s %3d%%\n", a.style.Section("Linux"), styledProgressBar(a.style, linuxCompleted, linuxTotal, 20), percentage(linuxCompleted, linuxTotal))
	printWorlds(mission.TrackLinux)
	dockerState := "readiness: run opsquest doctor"
	fmt.Fprintf(a.out, "%s %s %3d%%  %s\n", a.style.Section("Docker"), styledProgressBar(a.style, dockerCompleted, dockerTotal, 20), percentage(dockerCompleted, dockerTotal), dockerState)
	printWorlds(mission.TrackDocker)
	fmt.Fprintf(a.out, "%s\n\n", a.style.Muted(fmt.Sprintf("K8s    %s  locked", progressBar(0, 20, 20))))
	fmt.Fprintf(a.out, "Commands mastered: %d\n", len(game.PracticedCommands(player)))
	fmt.Fprintf(a.out, "Missions completed: %d\n", completed)
	completedHints, activeHints := player.HintsUsed(), player.ActiveHints()
	fmt.Fprintf(a.out, "Hints used: %d (completed: %d · active: %d)\n", completedHints+activeHints, completedHints, activeHints)
	fmt.Fprintf(a.out, "Achievements: %d/%d\n", player.AchievementCount(), len(profile.AchievementDefinitions()))
	return nil
}

func (a *App) runCommands(args []string) error {
	flags := a.newFlagSet("commands", func() { fmt.Fprintln(a.errOut, "Usage: opsquest commands") })
	if help, err := parseFlags(flags, args); help || err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("commands does not accept positional arguments")
	}
	player, err := a.loadPlayer()
	if err != nil {
		return err
	}
	commands := game.PracticedCommands(player)
	if len(commands) == 0 {
		fmt.Fprintln(a.out, "No commands mastered yet. Start with 'opsquest play'.")
		return nil
	}
	fmt.Fprintf(a.out, "%s\n\n", a.style.Header(fmt.Sprintf("COMMAND MASTERY (%d)", len(commands))))
	for _, command := range commands {
		name := fmt.Sprintf("%-12s", command)
		fmt.Fprintf(a.out, "  %s used successfully %d %s\n", a.style.Accent(name), player.Commands[command], plural(player.Commands[command], "time", "times"))
	}
	return nil
}

func (a *App) runAchievements(args []string) error {
	flags := a.newFlagSet("achievements", func() { fmt.Fprintln(a.errOut, "Usage: opsquest achievements") })
	if help, err := parseFlags(flags, args); help || err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("achievements does not accept positional arguments")
	}
	player, err := a.loadPlayer()
	if err != nil {
		return err
	}
	definitions := profile.AchievementDefinitions()
	fmt.Fprintln(a.out, a.style.Header("ACHIEVEMENTS"))
	for _, achievement := range definitions {
		unlockedAt, unlocked := player.Unlocked[achievement.ID]
		if unlocked {
			title := fmt.Sprintf("%-22s", achievement.Title)
			fmt.Fprintf(a.out, "  %s %s %s  %s\n", a.style.Achievement("★"), a.style.Achievement(title), achievement.Description, a.style.Muted("["+unlockedAt.Local().Format("2006-01-02")+"]"))
		} else {
			fmt.Fprintf(a.out, "  %s %-22s %s\n", a.style.Muted("☆"), achievement.Title, achievement.Description)
		}
	}
	fmt.Fprintf(a.out, "\n%d/%d unlocked\n", player.AchievementCount(), len(definitions))
	return nil
}
