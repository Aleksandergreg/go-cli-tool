package sandbox

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/aleksandergregersen/opsquest/internal/mission"
)

func sandboxAtArchivePayloadLimit(t *testing.T) (*Sandbox, string, string) {
	t.Helper()
	if maxVirtualArchiveBytes%maxVirtualFileBytes != 0 {
		t.Fatalf("archive payload limit %d is not divisible by file limit %d", maxVirtualArchiveBytes, maxVirtualFileBytes)
	}
	box := testSandbox(t)
	payload := strings.Repeat("p", maxVirtualFileBytes)
	sourcePath := "/work/archive-payload"
	if err := box.FS.WriteFile(sourcePath, payload, 0o640); err != nil {
		t.Fatalf("write archive source: %v", err)
	}
	for index := 0; index < maxVirtualArchiveBytes/maxVirtualFileBytes; index++ {
		archivePath := fmt.Sprintf("/out/quota-%d.tar", index)
		if _, err := box.Execute(fmt.Sprintf("tar -cf %s %s", archivePath, sourcePath)); err != nil {
			t.Fatalf("create archive at logical payload limit (%s): %v", archivePath, err)
		}
	}
	return box, sourcePath, payload
}

func TestTarCreateRejectsArchivePayloadBeyondLimit(t *testing.T) {
	box, sourcePath, payload := sandboxAtArchivePayloadLimit(t)
	overflowPath := "/out/overflow.tar"
	beforeCount := len(box.Archives)

	if _, err := box.Execute("tar -cf " + overflowPath + " " + sourcePath); err == nil {
		t.Error("archive creation beyond metadata payload limit unexpectedly succeeded")
	}
	if box.FS.Exists(overflowPath) {
		t.Error("rejected archive creation left a backing file")
	}
	if _, exists := box.Archives[overflowPath]; exists {
		t.Error("rejected archive creation left metadata")
	}
	if len(box.Archives) != beforeCount {
		t.Errorf("rejected archive creation changed archive count from %d to %d", beforeCount, len(box.Archives))
	}
	content, err := box.FS.ReadFile(sourcePath)
	if err != nil || content != payload {
		t.Errorf("rejected archive creation changed source: length %d, error %v", len(content), err)
	}
}

func TestCopyArchiveMetadataCannotBypassPayloadLimit(t *testing.T) {
	box, _, _ := sandboxAtArchivePayloadLimit(t)
	sourcePath := "/out/quota-0.tar"
	destinationPath := "/out/copied.tar"
	sourceMetadata := box.Archives[sourcePath]
	sourceEntry, exists := box.FS.Entry(sourcePath)
	if !exists {
		t.Fatalf("archive source backing file %s is missing", sourcePath)
	}
	sourceEntrySnapshot := *sourceEntry

	if _, err := box.Execute("cp " + sourcePath + " " + destinationPath); err == nil {
		t.Error("copying archive metadata beyond payload limit unexpectedly succeeded")
	}
	if box.FS.Exists(destinationPath) {
		t.Error("rejected archive copy left a destination backing file")
	}
	if _, exists := box.Archives[destinationPath]; exists {
		t.Error("rejected archive copy left destination metadata")
	}
	if actual := box.Archives[sourcePath]; !reflect.DeepEqual(actual, sourceMetadata) {
		t.Errorf("rejected archive copy changed source metadata")
	}
	actualEntry, exists := box.FS.Entry(sourcePath)
	if !exists || *actualEntry != sourceEntrySnapshot {
		t.Errorf("rejected archive copy changed source backing entry: %#v, exists %v", actualEntry, exists)
	}
}

func TestTarPayloadQuotaFailurePreservesExistingArchive(t *testing.T) {
	box := testSandbox(t)
	bigPayload := strings.Repeat("b", maxVirtualFileBytes)
	smallPayload := strings.Repeat("s", maxVirtualFileBytes/2)
	if err := box.FS.WriteFile("/work/big", bigPayload, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := box.FS.WriteFile("/work/small", smallPayload, 0o640); err != nil {
		t.Fatal(err)
	}

	commands := []string{
		"tar -cf /out/existing.tar small",
		"tar -cf /out/big-0.tar big",
		"tar -cf /out/big-1.tar big",
		"tar -cf /out/big-2.tar big",
		"tar -cf /out/small.tar small",
	}
	for _, command := range commands {
		if _, err := box.Execute(command); err != nil {
			t.Fatalf("prepare archive payload budget with %q: %v", command, err)
		}
	}
	if err := box.FS.Chmod("/out/existing.tar", 0o600); err != nil {
		t.Fatal(err)
	}
	if err := box.FS.Chown("/out/existing.tar", "reviewer"); err != nil {
		t.Fatal(err)
	}
	existingMetadata := box.Archives["/out/existing.tar"]
	existingEntry, _ := box.FS.Entry("/out/existing.tar")
	existingEntrySnapshot := *existingEntry

	// Replacing the 1 MiB archive with a 2 MiB payload would exceed the
	// already-full 8 MiB logical metadata budget.
	if _, err := box.Execute("tar -cf /out/existing.tar big"); err == nil {
		t.Error("archive replacement beyond payload limit unexpectedly succeeded")
	}
	if actual := box.Archives["/out/existing.tar"]; !reflect.DeepEqual(actual, existingMetadata) {
		t.Error("rejected archive replacement changed existing metadata")
	}
	actualEntry, exists := box.FS.Entry("/out/existing.tar")
	if !exists || *actualEntry != existingEntrySnapshot {
		t.Errorf("rejected archive replacement changed backing entry: %#v, exists %v", actualEntry, exists)
	}
	content, err := box.FS.ReadFile("/work/big")
	if err != nil || content != bigPayload {
		t.Errorf("rejected archive replacement changed source: length %d, error %v", len(content), err)
	}
}

func TestTarCreateRejectsArchiveEntryBeyondLimit(t *testing.T) {
	box := testSandbox(t)
	entries := make([]mission.ArchiveEntry, maxVirtualArchiveEntries)
	for index := range entries {
		entries[index] = mission.ArchiveEntry{Path: fmt.Sprintf("empty-%04d", index)}
	}
	box.Archives["/out/full.tar"] = Archive{Entries: entries}
	if err := box.FS.WriteFile("/out/full.tar", "OpsQuest virtual tar archive\n", 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateArchiveMetadata(box.Archives); err != nil {
		t.Fatalf("exact archive entry limit rejected: %v", err)
	}

	if _, err := box.Execute("tar -cf /out/overflow-entries.tar quiet.log"); err == nil {
		t.Error("archive creation beyond entry limit unexpectedly succeeded")
	}
	if box.FS.Exists("/out/overflow-entries.tar") {
		t.Error("rejected archive creation left a backing file")
	}
	if _, exists := box.Archives["/out/overflow-entries.tar"]; exists {
		t.Error("rejected archive creation left metadata")
	}
}
