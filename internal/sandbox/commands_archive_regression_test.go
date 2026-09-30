package sandbox

import (
	"reflect"
	"testing"
)

func TestTarCreateFailureLeavesNoArchiveMetadata(t *testing.T) {
	box := testSandbox(t)
	archivePath := "/work/events.log/bundle.tar"
	before := len(box.Archives)

	if _, err := box.Execute(`tar -cf /work/events.log/bundle.tar quiet.log`); err == nil {
		t.Fatal("tar creation beneath a regular file unexpectedly succeeded")
	}
	if len(box.Archives) != before {
		t.Errorf("failed tar creation changed archive metadata: %#v", box.Archives)
	}
	if _, exists := box.Archives[archivePath]; exists {
		t.Fatalf("failed tar creation left metadata at %s", archivePath)
	}
	if box.FS.Exists(archivePath) {
		t.Fatalf("failed tar creation left a virtual file at %s", archivePath)
	}
}

func TestTarCreateFailurePreservesExistingArchiveMetadata(t *testing.T) {
	box := testSandbox(t)
	archivePath := "/out/existing.tar"
	if _, err := box.Execute(`tar -cf /out/existing.tar events.log`); err != nil {
		t.Fatalf("create original archive: %v", err)
	}
	original := box.Archives[archivePath]
	originalBacking, err := box.FS.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := box.Execute(`tar -cf /out/existing.tar quiet.log missing.log`); err == nil {
		t.Fatal("tar creation with a missing operand unexpectedly succeeded")
	}
	if actual := box.Archives[archivePath]; !reflect.DeepEqual(actual, original) {
		t.Errorf("failed tar creation replaced metadata: got %#v, want %#v", actual, original)
	}
	backing, err := box.FS.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("failed tar creation removed backing file: %v", err)
	}
	if backing != originalBacking {
		t.Errorf("failed tar creation changed backing file: got %q, want %q", backing, originalBacking)
	}
}
