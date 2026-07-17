package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aleksandergregersen/opsquest/internal/mission"
)

func TestViReturnsVirtualEditorRequest(t *testing.T) {
	box := testSandbox(t)
	result, err := box.Execute(`vi "events.log"`)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Editor == nil {
		t.Fatal("Execute() returned no editor request")
	}
	want := EditorRequest{
		Command:     "vi",
		Path:        "/work/events.log",
		DisplayPath: "events.log",
		Content:     "INFO api ready\nERROR worker stuck\nERROR api timeout\n",
	}
	if *result.Editor != want {
		t.Errorf("editor request = %#v, want %#v", *result.Editor, want)
	}
	if strings.Join(result.Commands, ",") != "vi" {
		t.Errorf("commands = %v, want [vi]", result.Commands)
	}
	if result.PipelineWidth != 1 {
		t.Errorf("pipeline width = %d, want 1", result.PipelineWidth)
	}
}

func TestViHelpDocumentsTheTeachingSubset(t *testing.T) {
	box := testSandbox(t)
	result, err := box.Execute("help vi")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"h/j/k/l", "dd", ":wq", ":q!", "256 KiB", "shell escapes"} {
		if !strings.Contains(result.Output, expected) {
			t.Errorf("help vi did not contain %q:\n%s", expected, result.Output)
		}
	}
}

func TestViResolvesQuotedAbsoluteAndHomePaths(t *testing.T) {
	box := testSandbox(t)
	if err := box.FS.WriteFile("/work/notes with spaces.txt", "quoted\n", 0o644); err != nil {
		t.Fatal(err)
	}
	if err := box.FS.EnsureDir("/home/operator", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := box.FS.WriteFile("/home/operator/profile.txt", "home\n", 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		line        string
		wantPath    string
		wantDisplay string
		wantContent string
	}{
		{line: `vi "notes with spaces.txt"`, wantPath: "/work/notes with spaces.txt", wantDisplay: "notes with spaces.txt", wantContent: "quoted\n"},
		{line: `vi /work/quiet.log`, wantPath: "/work/quiet.log", wantDisplay: "/work/quiet.log", wantContent: "INFO all good\n"},
		{line: `vi ~/profile.txt`, wantPath: "/home/operator/profile.txt", wantDisplay: "~/profile.txt", wantContent: "home\n"},
	}
	for _, test := range tests {
		t.Run(test.line, func(t *testing.T) {
			result, err := box.Execute(test.line)
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if result.Editor == nil {
				t.Fatal("Execute() returned no editor request")
			}
			if result.Editor.Path != test.wantPath || result.Editor.DisplayPath != test.wantDisplay || result.Editor.Content != test.wantContent {
				t.Errorf("editor request = %#v", *result.Editor)
			}
		})
	}
}

func TestViCreatesOnlyWhenSavedAndPreservesVirtualMetadata(t *testing.T) {
	box := testSandbox(t)
	result, err := box.Execute(`vi /out/new.txt`)
	if err != nil {
		t.Fatal(err)
	}
	if result.Editor == nil || result.Editor.Content != "" {
		t.Fatalf("new-file request = %#v", result.Editor)
	}
	if box.FS.Exists("/out/new.txt") {
		t.Fatal("opening a new file created it before :w")
	}
	box.Archives["/out/new.txt"] = Archive{Entries: []mission.ArchiveEntry{{Path: "old"}}}
	if err := box.SaveEditorFile(result.Editor.Path, "saved\n"); err != nil {
		t.Fatalf("SaveEditorFile() error = %v", err)
	}
	content, err := box.FS.ReadFile("/out/new.txt")
	if err != nil || content != "saved\n" {
		t.Fatalf("saved content = %q, %v", content, err)
	}
	entry, _ := box.FS.Entry("/out/new.txt")
	if entry.Mode != 0o644 || entry.Owner != "operator" {
		t.Errorf("new file metadata = mode %04o owner %q", entry.Mode, entry.Owner)
	}
	if _, exists := box.Archives["/out/new.txt"]; exists {
		t.Fatal("saved file retained stale archive metadata")
	}

	if err := box.FS.Chmod("/work/events.log", 0o600); err != nil {
		t.Fatal(err)
	}
	if err := box.FS.Chown("/work/events.log", "root"); err != nil {
		t.Fatal(err)
	}
	if err := box.SaveEditorFile("/work/events.log", "updated\n"); err != nil {
		t.Fatal(err)
	}
	entry, _ = box.FS.Entry("/work/events.log")
	if entry.Mode != 0o600 || entry.Owner != "root" {
		t.Errorf("existing file metadata = mode %04o owner %q", entry.Mode, entry.Owner)
	}
}

func TestViRejectsUsageAndInvalidTargets(t *testing.T) {
	box := testSandbox(t)
	tests := []struct {
		line string
		want string
	}{
		{line: `vi`, want: "usage: vi FILE"},
		{line: `vi events.log quiet.log`, want: "usage: vi FILE"},
		{line: `vi -R`, want: "options are not supported"},
		{line: `vi *.log`, want: "usage: vi FILE"},
		{line: `vi /work`, want: "is a directory"},
		{line: `vi /missing/note.txt`, want: "no such directory"},
	}
	for _, test := range tests {
		t.Run(test.line, func(t *testing.T) {
			result, err := box.Execute(test.line)
			if err == nil {
				t.Fatal("Execute() unexpectedly succeeded")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("error = %q, want substring %q", err, test.want)
			}
			if result.Editor != nil {
				t.Errorf("invalid command returned editor request %#v", result.Editor)
			}
			if strings.Join(result.Commands, ",") != "vi" {
				t.Errorf("commands = %v, want [vi]", result.Commands)
			}
		})
	}
}

func TestViRejectsShellCompositionBeforePipelineSideEffects(t *testing.T) {
	box := testSandbox(t)
	tests := []struct {
		line        string
		wantError   string
		absentPaths []string
	}{
		{line: `touch /out/before | vi events.log`, wantError: "pipelines are not supported", absentPaths: []string{"/out/before"}},
		{line: `vi events.log | touch /out/after`, wantError: "pipelines are not supported", absentPaths: []string{"/out/after"}},
		{line: `echo changed > /out/redirected | vi events.log`, wantError: "pipelines are not supported", absentPaths: []string{"/out/redirected"}},
		{line: `vi events.log > /out/transcript`, wantError: "redirection is not supported", absentPaths: []string{"/out/transcript"}},
		{line: `vi events.log < quiet.log`, wantError: "redirection is not supported"},
	}
	for _, test := range tests {
		t.Run(test.line, func(t *testing.T) {
			_, err := box.Execute(test.line)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Execute() error = %v, want substring %q", err, test.wantError)
			}
			for _, name := range test.absentPaths {
				if box.FS.Exists(name) {
					t.Errorf("rejected command mutated %s", name)
				}
			}
		})
	}
}

func TestViRejectsNestedFindExec(t *testing.T) {
	box := testSandbox(t)
	result, err := box.Execute(`find . -name "*.log" -exec vi {} \;`)
	if err == nil || !strings.Contains(err.Error(), "interactive commands are not supported inside find -exec") {
		t.Fatalf("Execute() error = %v", err)
	}
	if strings.Join(result.Commands, ",") != "find,vi" {
		t.Errorf("command trace = %v, want [find vi]", result.Commands)
	}
}

func TestViSaveCannotWriteToHostFilesystem(t *testing.T) {
	box := testSandbox(t)
	hostFile := filepath.Join(t.TempDir(), "host-proof.txt")
	virtualPath := filepath.ToSlash(hostFile)
	if err := box.FS.EnsureDir(filepath.ToSlash(filepath.Dir(hostFile)), 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := box.Execute(fmt.Sprintf("vi %q", virtualPath))
	if err != nil {
		t.Fatal(err)
	}
	if err := box.SaveEditorFile(result.Editor.Path, "virtual only\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(hostFile); !os.IsNotExist(err) {
		t.Fatalf("host path was changed or stat returned unexpected error: %v", err)
	}
	content, err := box.FS.ReadFile(virtualPath)
	if err != nil || content != "virtual only\n" {
		t.Fatalf("virtual content = %q, %v", content, err)
	}
}
