package sandbox

import (
	"reflect"
	"testing"
)

func TestCopyRecursiveCurrentDirectorySucceeds(t *testing.T) {
	box := testSandbox(t)

	if _, err := box.Execute(`cp -r . /out/snapshot`); err != nil {
		t.Fatalf("cp -r current directory: %v", err)
	}

	content, err := box.FS.ReadFile("/out/snapshot/events.log")
	if err != nil {
		t.Fatalf("copied file missing: %v", err)
	}
	if content != "INFO api ready\nERROR worker stuck\nERROR api timeout\n" {
		t.Errorf("copied content = %q", content)
	}
	if box.CWD != "/work" || box.Env["PWD"] != "/work" {
		t.Errorf("copy changed navigation state: CWD=%q PWD=%q", box.CWD, box.Env["PWD"])
	}
}

func TestMoveCurrentDirectoryOrAncestorIsRejectedWithoutChangingPWD(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		target  string
		present string
	}{
		{name: "current directory", source: ".", target: "/out/current", present: "/work/nested"},
		{name: "ancestor", source: "/work", target: "/out/work", present: "/work"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			box := testSandbox(t)
			if _, err := box.Execute(`mkdir /work/nested`); err != nil {
				t.Fatal(err)
			}
			if _, err := box.Execute(`cd /work/nested`); err != nil {
				t.Fatal(err)
			}
			beforeCWD, beforePWD := box.CWD, box.Env["PWD"]

			if _, err := box.Execute("mv " + test.source + " " + test.target); err == nil {
				t.Fatalf("mv %s unexpectedly succeeded", test.source)
			}
			if box.CWD != beforeCWD || box.Env["PWD"] != beforePWD {
				t.Errorf("rejected move changed navigation state: CWD=%q PWD=%q, want %q", box.CWD, box.Env["PWD"], beforeCWD)
			}
			if !box.FS.IsDir(beforeCWD) || !box.FS.IsDir(test.present) {
				t.Errorf("rejected move removed current path or source; paths = %v", box.FS.Paths())
			}
			if box.FS.Exists(test.target) {
				t.Errorf("rejected move created destination %s", test.target)
			}
		})
	}
}

func TestCopyVirtualRootIsRejectedWithoutMutation(t *testing.T) {
	box := testSandbox(t)
	before := box.FS.Paths()

	if _, err := box.Execute(`cp -r / /out/root-copy`); err == nil {
		t.Fatal("copying virtual root unexpectedly succeeded")
	}
	if after := box.FS.Paths(); !reflect.DeepEqual(after, before) {
		t.Errorf("rejected root copy mutated filesystem:\n before: %v\n  after: %v", before, after)
	}
}
