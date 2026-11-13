package game

import (
	"testing"

	"github.com/aleksandergregersen/opsquest/internal/mission"
	"github.com/aleksandergregersen/opsquest/internal/sandbox"
)

func TestNewLinuxMissionsAcceptAlternativesAndRejectIncompleteOutcomes(t *testing.T) {
	catalog, err := mission.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		id          string
		alternative []string
		incomplete  []string
	}{
		{id: "linux-read-handoff", alternative: []string{"less handoff.txt"}, incomplete: []string{"cat handoff.old"}},
		{id: "linux-log-preview", alternative: []string{"head -3 /var/log/ledger/startup.log"}, incomplete: []string{"tail -n 3 startup.log"}},
		{id: "linux-error-headcount", alternative: []string{"grep -c ERROR deploy.log"}, incomplete: []string{"grep -c WARN deploy.log"}},
		{id: "linux-runbook-runner", alternative: []string{"chmod 750 publish-health.sh", "./publish-health.sh"}, incomplete: []string{"cat publish-health.sh"}},
	}
	for _, test := range tests {
		item, found := catalog.Find(test.id)
		if !found {
			t.Fatalf("mission %q not found", test.id)
		}
		run := func(t *testing.T, commands []string) bool {
			t.Helper()
			box, err := sandbox.New(item.Setup, item.StartDir)
			if err != nil {
				t.Fatal(err)
			}
			lastOutput := ""
			for _, command := range commands {
				result, err := box.Execute(command)
				if err != nil {
					t.Fatalf("Execute(%q) error = %v", command, err)
				}
				lastOutput = result.Output
			}
			complete, err := Validate(item.Validation, box, lastOutput)
			if err != nil {
				t.Fatal(err)
			}
			return complete
		}
		t.Run(test.id+" alternative", func(t *testing.T) {
			if !run(t, test.alternative) {
				t.Fatal("alternative observable outcome did not complete mission")
			}
		})
		t.Run(test.id+" incomplete", func(t *testing.T) {
			if run(t, test.incomplete) {
				t.Fatal("incomplete observable outcome completed mission")
			}
		})
	}
}

func TestSearchMissionRejectsUnfilteredFindOutput(t *testing.T) {
	catalog, _ := mission.LoadCatalog()
	item, _ := catalog.Find("linux-find-logs")
	box, err := sandbox.New(item.Setup, item.StartDir)
	if err != nil {
		t.Fatal(err)
	}
	result, err := box.Execute(`find . -name "*.log"`)
	if err != nil {
		t.Fatal(err)
	}
	complete, err := Validate(item.Validation, box, result.Output)
	if err != nil {
		t.Fatal(err)
	}
	if complete {
		t.Fatalf("unfiltered output unexpectedly completed mission: %q", result.Output)
	}
}

func TestRebalancedMissionOutcomesRemainRouteIndependent(t *testing.T) {
	catalog, err := mission.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}

	run := func(t *testing.T, missionID string, commands ...string) (mission.Mission, *sandbox.Sandbox, string) {
		t.Helper()
		item, found := catalog.Find(missionID)
		if !found {
			t.Fatalf("mission %q not found", missionID)
		}
		box, err := sandbox.New(item.Setup, item.StartDir)
		if err != nil {
			t.Fatal(err)
		}
		lastOutput := ""
		for _, command := range commands {
			result, err := box.Execute(command)
			if err != nil {
				t.Fatalf("Execute(%q) error = %v", command, err)
			}
			lastOutput = result.Output
		}
		return item, box, lastOutput
	}
	assertComplete := func(t *testing.T, item mission.Mission, box *sandbox.Sandbox, output string, want bool) {
		t.Helper()
		complete, err := Validate(item.Validation, box, output)
		if err != nil {
			t.Fatal(err)
		}
		if complete != want {
			t.Fatalf("mission complete = %v, want %v", complete, want)
		}
	}

	t.Run("search accepts an explicit log-file grep route", func(t *testing.T) {
		item, box, output := run(t, "linux-find-logs", `grep -l ERROR api/*.log worker/*.log assets/*.log`)
		assertComplete(t, item, box, output, true)
	})

	t.Run("release move is complete", func(t *testing.T) {
		item, box, output := run(t, "linux-release-shuffle", "mv incident-104.txt /archive/2026/incident-104.txt")
		assertComplete(t, item, box, output, true)
	})

	t.Run("release copy without source removal is incomplete", func(t *testing.T) {
		item, box, output := run(t, "linux-release-shuffle", "cp incident-104.txt /archive/2026/incident-104.txt")
		assertComplete(t, item, box, output, false)
	})

	t.Run("pipeline accepts cut and sort unique", func(t *testing.T) {
		item, box, output := run(t, "linux-pipeline-report", `grep ERROR incidents.log | cut -d " " -f 3 | sort -u > /reports/error-services.txt`)
		assertComplete(t, item, box, output, true)
	})

	t.Run("pipeline without deduplication is incomplete", func(t *testing.T) {
		item, box, output := run(t, "linux-pipeline-report", `grep ERROR incidents.log | awk '{print $3}' | sort > /reports/error-services.txt`)
		assertComplete(t, item, box, output, false)
	})

	t.Run("production boss accepts an alternative route", func(t *testing.T) {
		item, box, output := run(t, "linux-production-friday",
			"tar -xf release.tar -C /deploy",
			`printf 'SERVICE=checkout\nLOG_LEVEL=info\n' > /deploy/app/app.env`,
			"chmod 0750 /deploy/app/deploy.sh",
			"kill -TERM 9001",
		)
		assertComplete(t, item, box, output, true)
	})

	t.Run("production boss rejects collateral process damage", func(t *testing.T) {
		item, box, output := run(t, "linux-production-friday",
			"tar -xf release.tar -C /deploy",
			`sed -i 's/LOG_LEVEL=debug/LOG_LEVEL=info/' /deploy/app/app.env`,
			"chmod 750 /deploy/app/deploy.sh",
			"kill 9001 9002",
		)
		assertComplete(t, item, box, output, false)
	})
}

func TestScriptMissionsAcceptAlternativesAndRejectIncompleteOutcomes(t *testing.T) {
	catalog, err := mission.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}

	run := func(t *testing.T, missionID string, commands ...string) (mission.Mission, *sandbox.Sandbox) {
		t.Helper()
		item, found := catalog.Find(missionID)
		if !found {
			t.Fatalf("mission %q not found", missionID)
		}
		box, err := sandbox.New(item.Setup, item.StartDir)
		if err != nil {
			t.Fatal(err)
		}
		for _, command := range commands {
			if _, err := box.Execute(command); err != nil {
				t.Fatalf("Execute(%q) error = %v", command, err)
			}
		}
		return item, box
	}
	assertComplete := func(t *testing.T, item mission.Mission, box *sandbox.Sandbox, want bool) {
		t.Helper()
		complete, err := Validate(item.Validation, box, "")
		if err != nil {
			t.Fatal(err)
		}
		if complete != want {
			t.Fatalf("mission complete = %v, want %v", complete, want)
		}
	}

	t.Run("different pipeline and sh execution are accepted", func(t *testing.T) {
		item, box := run(t, "linux-report-on-repeat",
			`printf '#!/bin/sh\ngrep ERROR /workspace/incidents.log | cut -d " " -f 3 | sort -u > /reports/error-services.txt\n' > error-report.sh`,
			"chmod 750 error-report.sh",
			"sh error-report.sh",
		)
		assertComplete(t, item, box, true)
	})

	t.Run("repair without execution is incomplete", func(t *testing.T) {
		item, box := run(t, "linux-report-on-repeat",
			`sed -i 's/grep WARN/grep ERROR/' error-report.sh`,
			"chmod 750 error-report.sh",
		)
		assertComplete(t, item, box, false)
	})

	t.Run("manual boss commands leak caller state", func(t *testing.T) {
		item, box := run(t, "linux-scope-creep",
			`sed -i 's/grep INFO/grep ERROR/' night-shift.sh`,
			"chmod 750 night-shift.sh",
			"cd /srv/night-shift",
			"export REPORT_LABEL=night-shift",
			`grep ERROR events.log | awk '{print $3}' | sort | uniq > /reports/night-services.txt`,
			`echo "$REPORT_LABEL" > /reports/run-label.txt`,
		)
		assertComplete(t, item, box, false)
	})

	t.Run("sh preserves boss child scope", func(t *testing.T) {
		item, box := run(t, "linux-scope-creep",
			`sed -i 's/grep INFO/grep ERROR/' night-shift.sh`,
			"chmod 750 night-shift.sh",
			"sh night-shift.sh",
		)
		assertComplete(t, item, box, true)
	})
}
