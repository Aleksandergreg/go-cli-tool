package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aleksandergregersen/opsquest/internal/buildinfo"
	"github.com/aleksandergregersen/opsquest/internal/game"
	"github.com/aleksandergregersen/opsquest/internal/mission"
	"github.com/aleksandergregersen/opsquest/internal/profile"
	"github.com/aleksandergregersen/opsquest/internal/ui"
	"github.com/aleksandergregersen/opsquest/internal/webapp"
)

type App struct {
	in             io.Reader
	out            io.Writer
	errOut         io.Writer
	catalog        mission.Catalog
	store          profile.Store
	ctx            context.Context
	factory        game.Factory
	startCompanion CompanionStarter
	style          ui.Style
	errorStyle     ui.Style
}

// Companion is the lifecycle and presentation boundary used by web-assisted
// play. It receives sanitized game snapshots and never accepts player command
// text or completion decisions.
type Companion interface {
	game.AttemptReporter
	URL() string
	Close(context.Context) error
}

// CompanionStarter creates one companion for the lifetime of a play command.
type CompanionStarter func(context.Context) (Companion, error)

// Config contains the process-level dependencies required by the CLI. The
// executable supplies persistent storage and the combined environment factory;
// focused tests may omit Context, Factory, and StartCompanion to use safe local
// defaults.
type Config struct {
	Context        context.Context
	In             io.Reader
	Out            io.Writer
	ErrOut         io.Writer
	Catalog        mission.Catalog
	Store          profile.Store
	Factory        game.Factory
	StartCompanion CompanionStarter
}

func New(config Config) *App {
	if config.In == nil {
		config.In = strings.NewReader("")
	}
	if config.Out == nil {
		config.Out = io.Discard
	}
	if config.ErrOut == nil {
		config.ErrOut = io.Discard
	}
	if config.Context == nil {
		config.Context = context.Background()
	}
	if config.Factory == nil {
		config.Factory = game.SandboxFactory{}
	}
	if config.StartCompanion == nil {
		config.StartCompanion = func(ctx context.Context) (Companion, error) {
			return webapp.Start(ctx)
		}
	}
	return &App{
		in:             config.In,
		out:            config.Out,
		errOut:         config.ErrOut,
		catalog:        config.Catalog,
		store:          config.Store,
		ctx:            config.Context,
		factory:        config.Factory,
		startCompanion: config.StartCompanion,
		style:          ui.Auto(config.Out),
		errorStyle:     ui.Auto(config.ErrOut),
	}
}

func (a *App) loadPlayer() (profile.Profile, error) {
	player, err := a.store.Load()
	if err != nil {
		return profile.Profile{}, err
	}
	if unlocked := game.ReconcileAchievements(&player, a.catalog, time.Now()); len(unlocked) > 0 {
		if err := a.store.Save(player); err != nil {
			return profile.Profile{}, err
		}
	}
	return player, nil
}

func (a *App) Run(args []string) error {
	if len(args) == 0 {
		a.printUsage()
		return nil
	}
	switch args[0] {
	case "play":
		return a.runPlay(args[1:])
	case "guide", "tutorial":
		return a.runGuide(args[1:])
	case "list", "campaign", "map", "worlds":
		return a.runList(args[1:])
	case "profile":
		return a.runProfile(args[1:])
	case "commands":
		return a.runCommands(args[1:])
	case "achievements":
		return a.runAchievements(args[1:])
	case "show", "mission":
		return a.runShow(args[1:])
	case "doctor":
		return a.runDoctor(args[1:])
	case "reset":
		return a.runReset(args[1:])
	case "version", "--version", "-v":
		if help, err := a.parseNoArgs("version", args[1:]); help || err != nil {
			return err
		}
		fmt.Fprintf(a.out, "%s %s\n", a.style.Header("OpsQuest"), a.style.Accent(buildinfo.Version))
		return nil
	case "help", "--help", "-h":
		return a.runHelp(args[1:])
	default:
		return fmt.Errorf("unknown command %q; run 'opsquest help'", args[0])
	}
}
