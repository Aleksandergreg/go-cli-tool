package main

import (
	"context"
	"fmt"

	"github.com/aleksandergregersen/opsquest/internal/dockerlab"
	"github.com/aleksandergregersen/opsquest/internal/game"
	"github.com/aleksandergregersen/opsquest/internal/kubelab"
	"github.com/aleksandergregersen/opsquest/internal/mission"
)

// environmentRouter selects one backend per mission. Doctor uses the janitor
// independently of mission selection.
type environmentRouter struct {
	factories map[string]game.Factory
	game.ResourceJanitor
}

func newEnvironmentRouter() environmentRouter {
	docker := dockerlab.NewFactory()
	return environmentRouter{
		factories: map[string]game.Factory{
			mission.EnvironmentSimulated:  game.SandboxFactory{},
			mission.EnvironmentDocker:     docker,
			mission.EnvironmentKubernetes: kubelab.Factory{},
		},
		ResourceJanitor: docker,
	}
}

func (r environmentRouter) factory(item mission.Mission) (game.Factory, error) {
	kind := item.EffectiveEnvironment()
	factory := r.factories[kind]
	if factory == nil {
		return nil, fmt.Errorf("unsupported mission environment %q", kind)
	}
	return factory, nil
}

func (r environmentRouter) Create(ctx context.Context, item mission.Mission) (game.Environment, error) {
	factory, err := r.factory(item)
	if err != nil {
		return nil, err
	}
	return factory.Create(ctx, item)
}

func (r environmentRouter) Availability(ctx context.Context, item mission.Mission) game.Availability {
	factory, err := r.factory(item)
	if err != nil {
		return game.Availability{Detail: err.Error()}
	}
	return game.EnvironmentAvailability(ctx, factory, item)
}
