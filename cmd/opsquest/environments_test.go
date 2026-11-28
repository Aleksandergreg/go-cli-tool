package main

import (
	"context"
	"errors"
	"testing"

	"github.com/aleksandergregersen/opsquest/internal/dockerlab"
	"github.com/aleksandergregersen/opsquest/internal/game"
	"github.com/aleksandergregersen/opsquest/internal/kubelab"
	"github.com/aleksandergregersen/opsquest/internal/mission"
)

func TestEnvironmentWiring(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	router := newEnvironmentRouter()
	ctx := context.Background()
	docker, ok := router.factories[mission.EnvironmentDocker].(*dockerlab.Factory)
	if !ok || router.ResourceJanitor != docker {
		t.Fatal("Docker missions and doctor must share the Docker factory")
	}
	if _, ok := router.factories[mission.EnvironmentKubernetes].(kubelab.Factory); !ok {
		t.Fatal("Kubernetes missions need the kind factory")
	}

	catalog, err := mission.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range catalog.All() {
		if got := router.Availability(ctx, item); got.Available != (item.EffectiveEnvironment() == mission.EnvironmentSimulated) {
			t.Fatalf("availability for %s with an empty PATH = %+v", item.ID, got)
		}
	}
	legacy, _ := catalog.Find("1")
	legacy.Environment = ""
	env, err := router.Create(ctx, legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Close(); err != nil {
		t.Fatal(err)
	}
	unknown := mission.Mission{Environment: "unknown"}
	if availability := router.Availability(ctx, unknown); availability.Available || availability.Detail == "" {
		t.Fatalf("unexpected availability for unknown environment: %+v", availability)
	}
	if env, err := router.Create(ctx, unknown); env != nil || err == nil {
		t.Fatalf("unexpected creation for unknown environment: %v, %v", env, err)
	}
}

func TestRouterPreservesPartialEnvironmentOnSetupFailure(t *testing.T) {
	type partialEnvironment struct{ game.Environment }
	partial := &partialEnvironment{}
	failure := errors.New("partial setup")
	router := environmentRouter{factories: map[string]game.Factory{
		mission.EnvironmentKubernetes: game.FactoryFunc(func(context.Context, mission.Mission) (game.Environment, error) {
			return partial, failure
		}),
	}}
	got, err := router.Create(context.Background(), mission.Mission{Environment: mission.EnvironmentKubernetes})
	if got != partial || err != failure {
		t.Fatalf("Create returned %v, %v", got, err)
	}
}
