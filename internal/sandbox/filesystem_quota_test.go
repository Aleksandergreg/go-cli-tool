package sandbox

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestVirtualFileSizeLimitIsAtomic(t *testing.T) {
	fs := NewFileSystem()
	if err := fs.EnsureDir("/work", 0o755); err != nil {
		t.Fatal(err)
	}

	original := strings.Repeat("a", maxVirtualFileBytes)
	if err := fs.WriteFile("/work/report.txt", original, 0o640); err != nil {
		t.Fatalf("write at file limit: %v", err)
	}
	if err := fs.Chown("/work/report.txt", "reviewer"); err != nil {
		t.Fatal(err)
	}

	oversized := original + "x"
	if err := fs.WriteFile("/work/report.txt", oversized, 0o777); err == nil {
		t.Error("oversized overwrite unexpectedly succeeded")
	}
	content, err := fs.ReadFile("/work/report.txt")
	if err != nil {
		t.Fatalf("read after rejected overwrite: %v", err)
	}
	if content != original {
		t.Errorf("rejected overwrite changed content length to %d, want %d", len(content), len(original))
	}
	entry, exists := fs.Entry("/work/report.txt")
	if !exists || entry.Mode != 0o640 || entry.Owner != "reviewer" {
		t.Errorf("rejected overwrite changed metadata: %#v, exists %v", entry, exists)
	}

	if err := fs.WriteFile("/work/new.txt", oversized, 0o644); err == nil {
		t.Error("oversized new file unexpectedly succeeded")
	}
	if fs.Exists("/work/new.txt") {
		t.Error("rejected oversized write left a filesystem entry")
	}
}

func TestVirtualFileSystemContentLimitAndCapacityRelease(t *testing.T) {
	fs := NewFileSystem()
	if err := fs.EnsureDir("/work", 0o755); err != nil {
		t.Fatal(err)
	}
	payload := strings.Repeat("x", maxVirtualFileBytes)
	for index := 0; index < maxVirtualFileSystemBytes/maxVirtualFileBytes; index++ {
		name := fmt.Sprintf("/work/full-%d", index)
		if err := fs.WriteFile(name, payload, 0o644); err != nil {
			t.Fatalf("fill content budget with %s: %v", name, err)
		}
	}

	if err := fs.WriteFile("/work/overflow", "x", 0o644); err == nil {
		t.Error("write beyond total content limit unexpectedly succeeded")
	}
	if fs.Exists("/work/overflow") {
		t.Error("rejected total-limit write left a filesystem entry")
	}

	if err := fs.Remove("/work/full-0", false, false); err != nil {
		t.Fatalf("remove file to release capacity: %v", err)
	}
	if err := fs.WriteFile("/work/replacement", payload, 0o644); err != nil {
		t.Fatalf("released filesystem capacity was not reusable: %v", err)
	}
}

func TestVirtualEntryLimitPreflightsParentCreation(t *testing.T) {
	fs := NewFileSystem()
	// Leave one entry available. Creating this two-component path must fail
	// before either component is committed.
	for index := 0; index < maxVirtualEntries-2; index++ {
		name := fmt.Sprintf("/entry-%04d", index)
		if err := fs.Mkdir(name, false, 0o755); err != nil {
			t.Fatalf("fill entry budget with %s: %v", name, err)
		}
	}
	before := len(fs.Paths())
	if before != maxVirtualEntries-1 {
		t.Fatalf("entry setup count = %d, want %d", before, maxVirtualEntries-1)
	}

	if err := fs.Mkdir("/nested/child", true, 0o755); err == nil {
		t.Error("multi-component mkdir beyond entry limit unexpectedly succeeded")
	}
	if fs.Exists("/nested") || fs.Exists("/nested/child") {
		t.Errorf("rejected mkdir left partial parents: nested=%v child=%v", fs.Exists("/nested"), fs.Exists("/nested/child"))
	}
	if after := len(fs.Paths()); after != before {
		t.Errorf("rejected mkdir changed entry count from %d to %d", before, after)
	}

	if err := fs.Mkdir("/last-entry", false, 0o755); err != nil {
		t.Fatalf("create entry at exact limit: %v", err)
	}
	if err := fs.Mkdir("/one-too-many", false, 0o755); err == nil {
		t.Error("entry beyond exact limit unexpectedly succeeded")
	}
	if fs.Exists("/one-too-many") {
		t.Error("rejected entry-limit mkdir left an entry")
	}
}

func TestRecursiveCopyQuotaFailureIsAtomic(t *testing.T) {
	fs := NewFileSystem()
	for _, directory := range []string{"/src", "/dest"} {
		if err := fs.EnsureDir(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	halfFile := strings.Repeat("s", maxVirtualFileBytes/2)
	for _, name := range []string{"/src/first", "/src/second"} {
		if err := fs.WriteFile(name, halfFile, 0o644); err != nil {
			t.Fatalf("write source %s: %v", name, err)
		}
	}
	fullFile := strings.Repeat("f", maxVirtualFileBytes)
	for index := 0; index < 3; index++ {
		name := fmt.Sprintf("/filler-%d", index)
		if err := fs.WriteFile(name, fullFile, 0o644); err != nil {
			t.Fatalf("write filler %s: %v", name, err)
		}
	}

	if err := fs.Copy("/src", "/dest/snapshot", true); err == nil {
		t.Error("recursive copy beyond content limit unexpectedly succeeded")
	}
	for _, name := range []string{"/dest/snapshot", "/dest/snapshot/first", "/dest/snapshot/second"} {
		if fs.Exists(name) {
			t.Errorf("rejected recursive copy left partial destination %s", name)
		}
	}
}

func TestScriptAppendAmplificationPreservesFile(t *testing.T) {
	box := testSandbox(t)
	original := strings.Repeat("x", maxVirtualFileBytes/2+1)
	if err := box.FS.WriteFile("/work/growing.txt", original, 0o640); err != nil {
		t.Fatal(err)
	}
	writeTestScript(t, box, "grow.sh", "cat growing.txt >> growing.txt\n", 0o644)

	if _, err := box.Execute("sh grow.sh"); err == nil {
		t.Error("script append beyond file limit unexpectedly succeeded")
	}
	content, err := box.FS.ReadFile("/work/growing.txt")
	if err != nil {
		t.Fatal(err)
	}
	if content != original {
		t.Errorf("rejected append changed file length to %d, want %d", len(content), len(original))
	}
}

func TestOversizedRedirectionPreservesDestination(t *testing.T) {
	box := testSandbox(t)
	payload := strings.Repeat("x", maxVirtualFileBytes)
	for _, name := range []string{"/work/first", "/work/second"} {
		if err := box.FS.WriteFile(name, payload, 0o644); err != nil {
			t.Fatalf("write input %s: %v", name, err)
		}
	}
	if err := box.FS.WriteFile("/out/report", "keep\n", 0o640); err != nil {
		t.Fatal(err)
	}
	if err := box.FS.Chown("/out/report", "reviewer"); err != nil {
		t.Fatal(err)
	}

	if _, err := box.Execute("cat first second > /out/report"); err == nil {
		t.Error("oversized redirection unexpectedly succeeded")
	}
	content, err := box.FS.ReadFile("/out/report")
	if err != nil {
		t.Fatal(err)
	}
	if content != "keep\n" {
		t.Errorf("rejected redirection changed destination length to %d", len(content))
	}
	entry, exists := box.FS.Entry("/out/report")
	if !exists || entry.Mode != 0o640 || entry.Owner != "reviewer" {
		t.Errorf("rejected redirection changed metadata: %#v, exists %v", entry, exists)
	}
}

func TestCommandOutputLimitAppliesAtDispatcherBoundary(t *testing.T) {
	box := testSandbox(t)
	context := &executionContext{}
	payload := strings.Repeat("x", maxCommandOutputBytes)

	output, err := box.run(context, []string{"echo", "-n", payload}, "")
	if err != nil || len(output) != maxCommandOutputBytes {
		t.Fatalf("output at command limit: length %d, error %v", len(output), err)
	}
	if output, err = box.run(context, []string{"echo", payload}, ""); err == nil {
		t.Fatalf("output beyond command limit unexpectedly succeeded with %d bytes", len(output))
	}
	chunk := strings.Repeat("y", maxCommandOutputBytes/2+1)
	if output, err = box.run(context, []string{"printf", "%s%s", chunk, chunk}, ""); err == nil {
		t.Fatalf("incrementally built output beyond command limit unexpectedly succeeded with %d bytes", len(output))
	}
}

func TestMoveRejectsDestinationTreeBeyondPathLimitAtomically(t *testing.T) {
	fs := NewFileSystem()
	if err := fs.EnsureDir("/source", 0o750); err != nil {
		t.Fatal(err)
	}
	if err := fs.WriteFile("/source/report", "keep\n", 0o640); err != nil {
		t.Fatal(err)
	}
	longParent := "/" + strings.Repeat("d", maxVirtualPathBytes-1)
	if err := fs.EnsureDir(longParent, 0o755); err != nil {
		t.Fatalf("create parent at path limit: %v", err)
	}

	if err := fs.Move("/source", longParent); err == nil {
		t.Error("move producing an over-limit descendant unexpectedly succeeded")
	}
	content, err := fs.ReadFile("/source/report")
	if err != nil || content != "keep\n" {
		t.Errorf("rejected move changed source: content %q, error %v", content, err)
	}
	if fs.Exists(longParent + "/source") {
		t.Error("rejected move left a destination tree")
	}
}

func TestSedAmplificationIsRejectedBeforeInPlaceWrite(t *testing.T) {
	box := testSandbox(t)
	result, err := box.Execute(`printf 'aa\n' | sed 's/(a+)/$1$1/'`)
	if err != nil || result.Output != "aaaa\n" {
		t.Fatalf("bounded capture replacement output = %q, error %v", result.Output, err)
	}
	original := strings.Repeat("a", maxVirtualFileBytes)
	if err := box.FS.WriteFile("/work/large.txt", original, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := box.FS.Chown("/work/large.txt", "reviewer"); err != nil {
		t.Fatal(err)
	}

	if _, err := box.Execute(`sed -i 's/(a+)/$1$1/' large.txt`); err == nil {
		t.Error("amplifying sed replacement unexpectedly succeeded")
	}
	content, err := box.FS.ReadFile("/work/large.txt")
	if err != nil || content != original {
		t.Errorf("rejected sed changed content: length %d, error %v", len(content), err)
	}
	entry, exists := box.FS.Entry("/work/large.txt")
	if !exists || entry.Mode != 0o640 || entry.Owner != "reviewer" {
		t.Errorf("rejected sed changed metadata: %#v, exists %v", entry, exists)
	}
}

func TestCommandLineLimitDoesNotGrowHistory(t *testing.T) {
	box := testSandbox(t)
	atLimit := "#" + strings.Repeat("x", maxCommandLineBytes-1)
	if _, err := box.Execute(atLimit); err != nil {
		t.Fatalf("comment at command-line limit: %v", err)
	}
	if len(box.History) != 1 {
		t.Fatalf("history length at command-line limit = %d", len(box.History))
	}
	if _, err := box.Execute(atLimit + "x"); err == nil {
		t.Error("command beyond line limit unexpectedly succeeded")
	}
	if len(box.History) != 1 {
		t.Errorf("rejected command changed history length to %d", len(box.History))
	}
}

func TestEnvironmentQuotasAreAtomic(t *testing.T) {
	t.Run("content", func(t *testing.T) {
		box := testSandbox(t)
		before := cloneEnvironment(box.Env)
		assignments := []string{
			"SHOULD_NOT_APPEAR=value",
			"TOO_LARGE=" + strings.Repeat("x", maxVirtualEnvironmentBytes),
		}
		if _, err := box.cmdExport(assignments); err == nil {
			t.Error("oversized environment update unexpectedly succeeded")
		}
		if !reflect.DeepEqual(box.Env, before) {
			t.Errorf("rejected environment update changed state")
		}
	})

	t.Run("entries and navigation", func(t *testing.T) {
		box := testSandbox(t)
		assignments := make([]string, 0, maxVirtualEnvironmentEntries-len(box.Env))
		for index := 0; index < maxVirtualEnvironmentEntries-len(box.Env); index++ {
			assignments = append(assignments, fmt.Sprintf("QUOTA_%03d=x", index))
		}
		if _, err := box.cmdExport(assignments); err != nil {
			t.Fatalf("fill environment entry budget: %v", err)
		}
		before := cloneEnvironment(box.Env)
		if _, err := box.cmdExport([]string{"ONE_TOO_MANY=x"}); err == nil {
			t.Error("environment entry beyond limit unexpectedly succeeded")
		}
		if !reflect.DeepEqual(box.Env, before) {
			t.Error("rejected environment entry changed state")
		}
		beforeCWD := box.CWD
		if _, err := box.cmdCD([]string{"/out"}); err == nil {
			t.Error("cd that requires an over-limit OLDPWD entry unexpectedly succeeded")
		}
		if box.CWD != beforeCWD || !reflect.DeepEqual(box.Env, before) {
			t.Error("rejected cd changed navigation or environment state")
		}
	})
}

func TestOwnerLimitPreservesExistingMetadata(t *testing.T) {
	fs := NewFileSystem()
	if err := fs.WriteFile("/report", "ok\n", 0o640); err != nil {
		t.Fatal(err)
	}
	if err := fs.Chown("/report", "reviewer"); err != nil {
		t.Fatal(err)
	}
	if err := fs.Chown("/report", strings.Repeat("x", maxVirtualOwnerBytes+1)); err == nil {
		t.Error("owner beyond metadata limit unexpectedly succeeded")
	}
	entry, exists := fs.Entry("/report")
	if !exists || entry.Owner != "reviewer" || entry.Mode != 0o640 {
		t.Errorf("rejected owner update changed metadata: %#v, exists %v", entry, exists)
	}
}
