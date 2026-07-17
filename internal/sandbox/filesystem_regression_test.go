package sandbox

import "testing"

func TestFileSystemMoveSamePathPreservesRegularFile(t *testing.T) {
	fs := NewFileSystem()
	if err := fs.EnsureDir("/work", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := fs.WriteFile("/work/report.txt", "keep me\n", 0o640); err != nil {
		t.Fatal(err)
	}
	if err := fs.Chown("/work/report.txt", "reviewer"); err != nil {
		t.Fatal(err)
	}

	// Returning an error or treating this as a no-op are both acceptable; the
	// important invariant is that a same-path move cannot delete the entry.
	_ = fs.Move("/work/report.txt", "/work/report.txt")

	content, err := fs.ReadFile("/work/report.txt")
	if err != nil {
		t.Fatalf("same-path move removed the source: %v", err)
	}
	if content != "keep me\n" {
		t.Errorf("content after same-path move = %q", content)
	}
	entry, exists := fs.Entry("/work/report.txt")
	if !exists || entry.Kind != Regular || entry.Mode != 0o640 || entry.Owner != "reviewer" {
		t.Errorf("entry after same-path move = %#v, exists %v", entry, exists)
	}
}
