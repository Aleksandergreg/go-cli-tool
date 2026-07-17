package sandbox

import "testing"

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
