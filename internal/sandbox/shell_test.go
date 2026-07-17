package sandbox

import (
	"strings"
	"testing"

	"github.com/aleksandergregersen/opsquest/internal/mission"
)

func testSandbox(t *testing.T) *Sandbox {
	t.Helper()
	box, err := New(mission.Setup{
		Directories: []mission.DirectorySpec{{Path: "/work"}, {Path: "/out"}},
		Files: []mission.FileSpec{
			{Path: "/work/events.log", Content: "INFO api ready\nERROR worker stuck\nERROR api timeout\n"},
			{Path: "/work/quiet.log", Content: "INFO all good\n"},
		},
		Environment: map[string]string{"TARGET": "staging"},
	}, "/work")
	if err != nil {
		t.Fatal(err)
	}
	return box
}

func TestPipelineAndRedirectionStayInVirtualFilesystem(t *testing.T) {
	box := testSandbox(t)
	result, err := box.Execute(`grep ERROR *.log | awk '{print $2}' | sort -u > /out/services.txt`)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Output != "" {
		t.Errorf("redirected output = %q, want empty", result.Output)
	}
	content, err := box.FS.ReadFile("/out/services.txt")
	if err != nil {
		t.Fatal(err)
	}
	if content != "api\nworker\n" {
		t.Errorf("services.txt = %q", content)
	}
	wantCommands := []string{"grep", "awk", "sort"}
	if strings.Join(result.Commands, ",") != strings.Join(wantCommands, ",") {
		t.Errorf("commands = %v, want %v", result.Commands, wantCommands)
	}
}

func TestFindExecSupportsTheTeachingExample(t *testing.T) {
	box := testSandbox(t)
	result, err := box.Execute(`find . -name "*.log" -exec grep -l "ERROR" {} \;`)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Output != "./events.log\n" {
		t.Errorf("output = %q, want matching log only", result.Output)
	}
}

func TestVariablesQuotesAndAppendRedirection(t *testing.T) {
	box := testSandbox(t)
	if _, err := box.Execute(`echo "$TARGET release" > /out/deploy.txt`); err != nil {
		t.Fatal(err)
	}
	if _, err := box.Execute(`echo done >> /out/deploy.txt`); err != nil {
		t.Fatal(err)
	}
	content, _ := box.FS.ReadFile("/out/deploy.txt")
	if content != "staging release\ndone\n" {
		t.Errorf("content = %q", content)
	}
}

func TestHostCommandsAndRootRemovalAreRejected(t *testing.T) {
	box := testSandbox(t)
	if _, err := box.Execute(`sh -c "touch /tmp/escaped"`); err == nil {
		t.Fatal("host shell command unexpectedly succeeded")
	}
	if _, err := box.Execute(`rm -rf /`); err == nil {
		t.Fatal("removing virtual root unexpectedly succeeded")
	}
}

func TestUnterminatedInputIsRejected(t *testing.T) {
	box := testSandbox(t)
	if _, err := box.Execute(`echo "unfinished`); err == nil {
		t.Fatal("unterminated quote unexpectedly succeeded")
	}
	if _, err := box.Execute(`echo ok |`); err == nil {
		t.Fatal("trailing pipeline unexpectedly succeeded")
	}
}
