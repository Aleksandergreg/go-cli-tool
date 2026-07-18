package sandbox

import (
	"reflect"
	"strings"
	"testing"

	"github.com/aleksandergregersen/opsquest/internal/mission"
)

func TestExpandedTokenLimitPreflightsBeforeMutation(t *testing.T) {
	box := testSandbox(t)
	value := strings.Repeat("x", 64*1024)
	box.Env["AMPLIFY"] = value
	references := maxExpandedTokenBytes/len(value) + 1
	line := `touch /out/token-side-effect | echo "` + strings.Repeat("$AMPLIFY", references) + `"`

	if _, err := box.Execute(line); err == nil || !strings.Contains(err.Error(), "token limit") {
		t.Fatalf("Execute() error = %v, want expanded-token limit", err)
	}
	if box.FS.Exists("/out/token-side-effect") {
		t.Fatal("expanded-token rejection ran an earlier pipeline stage")
	}
}

func TestExpandedArgumentLimitPreflightsBeforeMutation(t *testing.T) {
	box := testSandbox(t)
	patterns := maxExpandedArguments/2 + 1 // testSandbox has two matching .log files.
	line := "touch /out/argument-side-effect | echo " + strings.Repeat("*.log ", patterns)

	if _, err := box.Execute(line); err == nil || !strings.Contains(err.Error(), "argument limit") {
		t.Fatalf("Execute() error = %v, want expanded-argument limit", err)
	}
	if box.FS.Exists("/out/argument-side-effect") {
		t.Fatal("expanded-argument rejection ran an earlier pipeline stage")
	}
}

func TestPipelineStageLimitIsPreflighted(t *testing.T) {
	box := testSandbox(t)
	atLimit := make([]string, maxPipelineStages)
	for index := range atLimit {
		atLimit[index] = "pwd"
	}
	result, err := box.Execute(strings.Join(atLimit, " | "))
	if err != nil || result.Output != "/work\n" {
		t.Fatalf("pipeline at limit = output %q, error %v", result.Output, err)
	}

	overLimit := append([]string(nil), atLimit...)
	overLimit[0] = "touch /out/pipeline-side-effect"
	overLimit = append(overLimit, "pwd")
	if _, err := box.Execute(strings.Join(overLimit, " | ")); err == nil || !strings.Contains(err.Error(), "pipeline stage limit") {
		t.Fatalf("pipeline over limit error = %v", err)
	}
	if box.FS.Exists("/out/pipeline-side-effect") {
		t.Fatal("pipeline-stage rejection ran an earlier stage")
	}
}

func TestDispatchLimitCoversFindExec(t *testing.T) {
	box := testSandbox(t)
	line := "find " + strings.Repeat("events.log ", maxExecutionDispatchSteps) + `-exec pwd \;`

	result, err := box.Execute(line)
	if err == nil || !strings.Contains(err.Error(), "command dispatch limit") {
		t.Fatalf("find -exec dispatch error = %v", err)
	}
	if len(result.Commands) != maxExecutionDispatchSteps {
		t.Errorf("recorded commands = %d, want bounded trace of %d", len(result.Commands), maxExecutionDispatchSteps)
	}
}

func TestTarExtractionFailureIsTransactional(t *testing.T) {
	box := testSandbox(t)
	if err := box.FS.WriteFile("/out/existing.tar", "original\n", 0o600); err != nil {
		t.Fatal(err)
	}
	existingMetadata := Archive{Entries: []mission.ArchiveEntry{{Path: "original.txt", Content: "keep\n"}}}
	box.Archives["/out/existing.tar"] = existingMetadata
	if err := box.FS.WriteFile("/out/blocker", "not a directory\n", 0o640); err != nil {
		t.Fatal(err)
	}
	if err := box.FS.WriteFile("/work/restore.tar", "virtual archive\n", 0o644); err != nil {
		t.Fatal(err)
	}
	box.Archives["/work/restore.tar"] = Archive{Entries: []mission.ArchiveEntry{
		{Path: "existing.tar", Content: "replacement\n", Mode: "0644"},
		{Path: "blocker/child.txt", Content: "cannot be written\n", Mode: "0644"},
	}}

	if _, err := box.Execute("tar -xf /work/restore.tar -C /out"); err == nil {
		t.Fatal("conflicting archive extraction unexpectedly succeeded")
	}
	content, err := box.FS.ReadFile("/out/existing.tar")
	if err != nil || content != "original\n" {
		t.Fatalf("failed extraction changed earlier target: content %q, error %v", content, err)
	}
	entry, _ := box.FS.Entry("/out/existing.tar")
	if entry.Mode != 0o600 {
		t.Errorf("failed extraction changed target mode to %04o", entry.Mode)
	}
	if actual := box.Archives["/out/existing.tar"]; !reflect.DeepEqual(actual, existingMetadata) {
		t.Errorf("failed extraction changed archive metadata: %#v", actual)
	}
	if box.FS.Exists("/out/blocker/child.txt") {
		t.Fatal("failed extraction left a later target")
	}
}

func TestTarRejectsUnsupportedChangeDirectorySemantics(t *testing.T) {
	box := testSandbox(t)
	for _, line := range []string{
		"tar -cf /out/events.tar -C /work events.log",
		"tar -tf /work/backup.tar -C /out",
	} {
		if _, err := box.Execute(line); err == nil || !strings.Contains(err.Error(), "-C is supported only when extracting") {
			t.Errorf("Execute(%q) error = %v", line, err)
		}
	}
	if box.FS.Exists("/out/events.tar") {
		t.Fatal("rejected tar create wrote an archive")
	}
}

func TestCDDashUsesOLDPWDAsTheSourceOfTruth(t *testing.T) {
	box := testSandbox(t)
	if _, err := box.Execute("export OLDPWD=/out"); err != nil {
		t.Fatal(err)
	}
	result, err := box.Execute("cd -")
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "/out\n" || box.CWD != "/out" || box.Env["PWD"] != "/out" || box.Env["OLDPWD"] != "/work" {
		t.Errorf("cd - = output %q, CWD %q, PWD %q, OLDPWD %q", result.Output, box.CWD, box.Env["PWD"], box.Env["OLDPWD"])
	}
}

func TestNewPropagatesFileOwnerErrors(t *testing.T) {
	_, err := New(mission.Setup{
		Directories: []mission.DirectorySpec{{Path: "/work"}},
		Files: []mission.FileSpec{{
			Path:  "/work/report.txt",
			Owner: strings.Repeat("x", maxVirtualOwnerBytes+1),
		}},
	}, "/work")
	if err == nil || !strings.Contains(err.Error(), "file /work/report.txt") || !strings.Contains(err.Error(), "owner") {
		t.Fatalf("New() error = %v, want path-qualified owner error", err)
	}
}

func TestShellHelpReportsSeparateResourceBudgets(t *testing.T) {
	box := testSandbox(t)
	result, err := box.Execute("help")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"2048 KiB expanded tokens",
		"4096 expanded arguments",
		"64 pipeline stages",
		"512 command dispatches",
		"8 MiB filesystem content and 8 MiB archive payload",
		"4096 filesystem entries and 4096 archive entries",
	} {
		if !strings.Contains(result.Output, expected) {
			t.Errorf("help output does not contain %q:\n%s", expected, result.Output)
		}
	}
}
