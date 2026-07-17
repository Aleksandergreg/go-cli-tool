package game

import (
	"testing"

	"github.com/aleksandergregersen/opsquest/internal/mission"
	"github.com/aleksandergregersen/opsquest/internal/sandbox"
)

func TestEveryMissionHasAWorkingOutcome(t *testing.T) {
	catalog, err := mission.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	solutions := map[int][]string{
		1:  {"pwd"},
		2:  {"cd /srv/web/config/live"},
		3:  {"mkdir -p reports/daily", "touch reports/daily/summary.txt"},
		4:  {`find . -name "*.log" -exec grep -l "ERROR" {} \;`},
		5:  {"cp incident-104.txt /archive/2026/incident-104.txt", "rm incident-104.txt"},
		6:  {"chmod 750 deploy.sh"},
		7:  {"export DEPLOY_ENV=staging"},
		8:  {"ps", "kill 4242"},
		9:  {"tar -xf status-site.tar -C /restore"},
		10: {`grep ERROR incidents.log | awk '{print $3}' | sort | uniq > /reports/error-services.txt`},
		11: {"tail -n 3 gateway.log"},
		12: {"sort raw.txt | uniq -c > /reports/alert-counts.txt"},
		13: {`sed -i 's/LOG_LEVEL=debug/LOG_LEVEL=info/' app.env`},
		14: {"chown web secrets.env", "chmod 640 secrets.env"},
		15: {"du -b * | sort -n | tail -n 1"},
		16: {
			"tar -xf release.tar -C /deploy",
			`sed -i 's/LOG_LEVEL=debug/LOG_LEVEL=info/' /deploy/app/app.env`,
			"chmod 750 /deploy/app/deploy.sh",
			"kill 9001",
		},
	}

	for _, item := range catalog.All() {
		item := item
		t.Run(item.ID, func(t *testing.T) {
			box, err := sandbox.New(item.Setup, item.StartDir)
			if err != nil {
				t.Fatal(err)
			}
			lastOutput := ""
			for _, command := range solutions[item.Number] {
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
			if !complete {
				t.Fatalf("canonical outcome did not complete mission; last output %q", lastOutput)
			}
		})
	}
}

func TestSearchMissionRejectsUnfilteredFindOutput(t *testing.T) {
	catalog, _ := mission.LoadCatalog()
	item, _ := catalog.Find("4")
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
