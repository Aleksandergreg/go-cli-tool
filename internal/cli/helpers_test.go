package cli

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/aleksandergregersen/opsquest/internal/game"
	"github.com/aleksandergregersen/opsquest/internal/mission"
	"github.com/aleksandergregersen/opsquest/internal/profile"
)

var sgrPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

var linuxMissionIDsThroughArchive = []string{
	"linux-orientation",
	"linux-config-crawl",
	"linux-read-handoff",
	"linux-workspace",
	"linux-find-logs",
	"linux-release-shuffle",
	"linux-permissions",
	"linux-environment",
	"linux-log-preview",
	"linux-runaway",
	"linux-archive-rescue",
}

func testApp(t *testing.T, input string, store profile.Store) (*App, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	catalog, err := mission.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	return New(Config{
		In:      strings.NewReader(input),
		Out:     out,
		ErrOut:  errOut,
		Catalog: catalog,
		Store:   store,
	}), out, errOut
}

type testWebCompanion struct {
	events     []game.AttemptEvent
	closeCount int
	closeErr   error
}

func (c *testWebCompanion) ReportAttempt(event game.AttemptEvent) {
	c.events = append(c.events, game.CloneAttemptEvent(event))
}

func (c *testWebCompanion) URL() string { return "http://127.0.0.1:43210/pair?token=test" }

func (c *testWebCompanion) Close(context.Context) error {
	c.closeCount++
	return c.closeErr
}

func seedCompletedMissions(t *testing.T, store profile.Store, missionIDs ...string) {
	t.Helper()
	player := profile.New("alex")
	for _, id := range missionIDs {
		player.Complete(id, 0, 0, time.Unix(1, 0))
	}
	if err := store.Save(player); err != nil {
		t.Fatal(err)
	}
}

func eventTypesForCLI(events []game.AttemptEvent) []game.AttemptEventType {
	types := make([]game.AttemptEventType, len(events))
	for index, event := range events {
		types[index] = event.Type
	}
	return types
}
