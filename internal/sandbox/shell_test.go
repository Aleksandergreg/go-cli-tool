package sandbox

import (
	"sort"
	"strings"
	"testing"

	"github.com/aleksandergregersen/opsquest/internal/mission"
)

func TestCommandNamesAreSortedUniqueAndDocumented(t *testing.T) {
	commands := CommandNames()
	if !sort.StringsAreSorted(commands) {
		t.Fatalf("CommandNames() is not sorted: %v", commands)
	}
	seen := make(map[string]bool, len(commands))
	for _, command := range commands {
		if seen[command] {
			t.Fatalf("CommandNames() contains duplicate %q", command)
		}
		seen[command] = true
		if _, documented := commandManuals[command]; !documented {
			t.Errorf("command %q has no focused help", command)
		}
	}
}

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
	if strings.Join(result.Commands, ",") != "find,grep,grep" {
		t.Errorf("nested command trace = %v", result.Commands)
	}
}

func TestQuotedGlobRemainsLiteral(t *testing.T) {
	box := testSandbox(t)
	if _, err := box.Execute(`cat "*.log"`); err == nil {
		t.Fatal("quoted glob unexpectedly expanded")
	}
	result, err := box.Execute(`cat *.log`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Output, "ERROR worker stuck") || !strings.Contains(result.Output, "INFO all good") {
		t.Errorf("unquoted glob output = %q", result.Output)
	}
}

func TestGlobCanReachAParentDirectory(t *testing.T) {
	box := testSandbox(t)
	if err := box.FS.EnsureDir("/shared", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := box.FS.WriteFile("/shared/note.txt", "outside cwd\n", 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := box.Execute(`cat ../shared/*.txt`)
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "outside cwd\n" {
		t.Errorf("output = %q", result.Output)
	}
}

func TestRedirectionBelongsToItsPipelineStage(t *testing.T) {
	box := testSandbox(t)
	result, err := box.Execute(`echo alpha > /out/middle.txt | wc -c`)
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "0\n" {
		t.Errorf("pipeline output = %q, want empty input byte count", result.Output)
	}
	content, _ := box.FS.ReadFile("/out/middle.txt")
	if content != "alpha\n" {
		t.Errorf("redirected content = %q", content)
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
	if _, err := box.Execute(`rm -rf .`); err == nil {
		t.Fatal("removing current directory unexpectedly succeeded")
	}
}

func TestCopyDoesNotReplaceDirectoriesWithFiles(t *testing.T) {
	box := testSandbox(t)
	if _, err := box.Execute(`mkdir /out/events.log`); err != nil {
		t.Fatal(err)
	}
	if _, err := box.Execute(`cp events.log /out`); err == nil {
		t.Fatal("copy replaced a directory with a regular file")
	}
	if !box.FS.IsDir("/out/events.log") {
		t.Fatal("destination directory was corrupted")
	}
	for _, command := range []string{`mkdir -p tree`, `touch tree/node`, `mkdir -p /out/tree/node`} {
		if _, err := box.Execute(command); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := box.Execute(`cp -r tree /out`); err == nil {
		t.Fatal("recursive copy replaced a nested directory with a file")
	}
	if !box.FS.IsDir("/out/tree/node") {
		t.Fatal("nested destination directory was corrupted")
	}
}

func TestWCCountsNewlineCharacters(t *testing.T) {
	box := testSandbox(t)
	result, err := box.Execute(`printf no-newline | wc -l`)
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "0\n" {
		t.Errorf("wc -l output = %q, want 0", result.Output)
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

func TestTarMetadataFollowsFilesAndRejectsUnsafeEntries(t *testing.T) {
	box := testSandbox(t)
	if _, err := box.Execute(`tar -cfbundle.tar events.log`); err != nil {
		t.Fatalf("create attached archive: %v", err)
	}
	if _, err := box.Execute(`cp bundle.tar /out/copy.tar`); err != nil {
		t.Fatal(err)
	}
	result, err := box.Execute(`tar -tf /out/copy.tar`)
	if err != nil || result.Output != "events.log\n" {
		t.Fatalf("copied archive list = %q, %v", result.Output, err)
	}
	if _, err := box.Execute(`mv /out/copy.tar /out/moved.tar`); err != nil {
		t.Fatal(err)
	}
	if _, err := box.Execute(`tar -tf /out/moved.tar`); err != nil {
		t.Fatalf("moved archive lost metadata: %v", err)
	}
	if _, err := box.Execute(`echo broken > /out/moved.tar`); err != nil {
		t.Fatal(err)
	}
	if _, err := box.Execute(`tar -tf /out/moved.tar`); err == nil {
		t.Fatal("overwritten archive retained stale metadata")
	}

	if err := box.FS.WriteFile("/work/unsafe.tar", "virtual\n", 0o644); err != nil {
		t.Fatal(err)
	}
	box.Archives["/work/unsafe.tar"] = Archive{Entries: []mission.ArchiveEntry{{Path: "../escaped", Content: "bad"}}}
	if _, err := box.Execute(`tar -xf unsafe.tar -C /out`); err == nil {
		t.Fatal("unsafe archive path unexpectedly extracted")
	}
	if box.FS.Exists("/escaped") {
		t.Fatal("unsafe archive escaped extraction directory")
	}
}

func TestSedTrDuStatAndNavigationEnhancements(t *testing.T) {
	box := testSandbox(t)
	if _, err := box.Execute(`sed -i 's/INFO/NOTICE/g' quiet.log`); err != nil {
		t.Fatal(err)
	}
	content, _ := box.FS.ReadFile("/work/quiet.log")
	if content != "NOTICE all good\n" {
		t.Errorf("sed content = %q", content)
	}
	result, err := box.Execute(`printf 'a   b' | tr -s ' '`)
	if err != nil || result.Output != "a b" {
		t.Errorf("tr output = %q, %v", result.Output, err)
	}
	result, err = box.Execute(`du -b *.log | sort -n | tail -n 1`)
	if err != nil || !strings.Contains(result.Output, "events.log") {
		t.Errorf("du pipeline output = %q, %v", result.Output, err)
	}
	result, err = box.Execute(`stat quiet.log`)
	if err != nil || !strings.Contains(result.Output, "Access: (0644/-rw-r--r--)") {
		t.Errorf("stat output = %q, %v", result.Output, err)
	}
	if _, err := box.Execute(`cd /out`); err != nil {
		t.Fatal(err)
	}
	result, err = box.Execute(`cd -`)
	if err != nil || result.Output != "/work\n" || box.CWD != "/work" {
		t.Errorf("cd - = %q, cwd %s, %v", result.Output, box.CWD, err)
	}
	if err := box.FS.EnsureDir("/home/operator", 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := box.Execute(`cd ~/`); err != nil {
		t.Fatalf("tilde navigation: %v", err)
	}
}
