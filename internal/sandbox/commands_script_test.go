package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeTestScript(t *testing.T, box *Sandbox, name, content string, mode uint32) {
	t.Helper()
	if err := box.FS.WriteFile(box.Resolve(name), content, mode); err != nil {
		t.Fatalf("write script %s: %v", name, err)
	}
}

func TestShRunsTheTeachingShellSubset(t *testing.T) {
	box := testSandbox(t)
	writeTestScript(t, box, "report.sh", `#!/bin/sh
# The script sees the mission environment and its own earlier exports.
export LABEL="$TARGET report"
export CURRENT="$LABEL" # inline comments are supported
grep ERROR *.log | awk '{print $2}' | sort -u > "/out/services report.txt"
echo "$CURRENT"
cat "/out/services report.txt"
`, 0o644)

	result, err := box.Execute("sh report.sh")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Output != "staging report\napi\nworker\n" {
		t.Errorf("output = %q", result.Output)
	}
	wantCommands := []string{"sh", "export", "export", "grep", "awk", "sort", "echo", "cat"}
	if !reflect.DeepEqual(result.Commands, wantCommands) {
		t.Errorf("commands = %v, want %v", result.Commands, wantCommands)
	}
	if result.PipelineWidth != 3 {
		t.Errorf("pipeline width = %d, want 3", result.PipelineWidth)
	}
	content, err := box.FS.ReadFile("/out/services report.txt")
	if err != nil || content != "api\nworker\n" {
		t.Fatalf("redirected report = %q, %v", content, err)
	}
}

func TestShAcceptsQuotedPathsAndCRLF(t *testing.T) {
	box := testSandbox(t)
	writeTestScript(t, box, "daily report.sh", "#!/bin/sh\r\n# windows newlines\r\nprintf 'ready\\n'\r\n", 0o644)

	result, err := box.Execute(`sh "daily report.sh"`)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Output != "ready\n" {
		t.Errorf("output = %q, want %q", result.Output, "ready\n")
	}
	if !reflect.DeepEqual(result.Commands, []string{"sh", "printf"}) {
		t.Errorf("commands = %v", result.Commands)
	}
}

func TestExecutableScriptsRequirePermissionAndSupportedShebang(t *testing.T) {
	box := testSandbox(t)
	writeTestScript(t, box, "deploy.sh", "#!/bin/sh\necho deployed\n", 0o644)
	writeTestScript(t, box, "plain.sh", "echo plain\n", 0o755)
	writeTestScript(t, box, "env-sh.sh", "#!/usr/bin/env sh\necho env\n", 0o755)

	if _, err := box.Execute("./deploy.sh"); err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("non-executable script error = %v", err)
	}
	if _, err := box.Execute("chmod 750 deploy.sh"); err != nil {
		t.Fatalf("chmod script: %v", err)
	}
	result, err := box.Execute("./deploy.sh")
	if err != nil {
		t.Fatalf("execute chmodded script: %v", err)
	}
	if result.Output != "deployed\n" || !reflect.DeepEqual(result.Commands, []string{"sh", "echo"}) {
		t.Errorf("executable result = output %q commands %v", result.Output, result.Commands)
	}

	if _, err := box.Execute("./plain.sh"); err == nil || !strings.Contains(err.Error(), "require #!/bin/sh") {
		t.Fatalf("bad shebang error = %v", err)
	}
	result, err = box.Execute("sh plain.sh")
	if err != nil || result.Output != "plain\n" {
		t.Fatalf("sh should not require mode or shebang: output %q error %v", result.Output, err)
	}
	result, err = box.Execute("./env-sh.sh")
	if err != nil || result.Output != "env\n" {
		t.Fatalf("env shebang result = output %q error %v", result.Output, err)
	}
}

func TestShRejectsInvalidUsageTargetsAndIncomingInput(t *testing.T) {
	box := testSandbox(t)
	writeTestScript(t, box, "ok.sh", "echo ok\n", 0o644)
	writeTestScript(t, box, "executable.sh", "#!/bin/sh\necho ok\n", 0o755)

	tests := []struct {
		name string
		line string
		want string
	}{
		{name: "missing operand", line: "sh", want: "usage: sh FILE"},
		{name: "missing file", line: "sh missing.sh", want: "no such file"},
		{name: "directory", line: "sh /work", want: "is a directory"},
		{name: "option", line: "sh -x ok.sh", want: "options"},
		{name: "positional argument", line: "sh ok.sh extra", want: "positional arguments"},
		{name: "missing executable path", line: "./missing.sh", want: "no such file"},
		{name: "executable argument", line: "./executable.sh extra", want: "positional arguments"},
		{name: "pipeline input", line: "echo input | sh ok.sh", want: "input from pipelines or redirection"},
		{name: "executable pipeline input", line: "echo input | ./executable.sh", want: "input from pipelines or redirection"},
		{name: "redirected input", line: "sh ok.sh < events.log", want: "input from pipelines or redirection"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := box.Execute(test.line)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Execute(%q) error = %v, want substring %q", test.line, err, test.want)
			}
		})
	}
}

func TestShPreflightsIncomingInputBeforeEarlierPipelineSideEffects(t *testing.T) {
	box := testSandbox(t)
	writeTestScript(t, box, "ok.sh", "echo ok\n", 0o644)
	writeTestScript(t, box, "executable.sh", "#!/bin/sh\necho ok\n", 0o755)

	tests := []struct {
		line string
		path string
	}{
		{line: "touch /out/before-sh | sh ok.sh", path: "/out/before-sh"},
		{line: "touch /out/before-executable | ./executable.sh", path: "/out/before-executable"},
	}
	for _, test := range tests {
		_, err := box.Execute(test.line)
		if err == nil || !strings.Contains(err.Error(), "input from pipelines or redirection") {
			t.Fatalf("Execute(%q) error = %v", test.line, err)
		}
		if box.FS.Exists(test.path) {
			t.Errorf("rejected pipeline created %s", test.path)
		}
	}
}

func TestShStopsAtLineNumberedErrorAndKeepsEarlierFilesystemChanges(t *testing.T) {
	box := testSandbox(t)
	writeTestScript(t, box, "failing.sh", "touch /out/before\ncat /missing\ntouch /out/after\n", 0o644)

	_, err := box.Execute("sh failing.sh")
	if err == nil {
		t.Fatal("failing script unexpectedly succeeded")
	}
	for _, expected := range []string{"/work/failing.sh:2:", "cat:", "no such file"} {
		if !strings.Contains(err.Error(), expected) {
			t.Errorf("error %q does not contain %q", err, expected)
		}
	}
	if !box.FS.Exists("/out/before") {
		t.Error("filesystem mutation before the error was rolled back")
	}
	if box.FS.Exists("/out/after") {
		t.Error("command after the error was executed")
	}
}

func TestShRestoresWorkingDirectoryAndEnvironmentWhileKeepingFiles(t *testing.T) {
	tests := []struct {
		name      string
		script    string
		wantError bool
		persisted string
	}{
		{
			name:      "success",
			script:    "cd /out\nexport TARGET=changed ADDED=value\ntouch success.txt\n",
			persisted: "/out/success.txt",
		},
		{
			name:      "error",
			script:    "cd /out\nexport TARGET=broken ADDED=value\ntouch before-error\ncat /missing\n",
			wantError: true,
			persisted: "/out/before-error",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			box := testSandbox(t)
			box.Env["OLDPWD"] = "/previous"
			beforeEnv := cloneEnvironment(box.Env)
			writeTestScript(t, box, "state.sh", test.script, 0o644)

			_, err := box.Execute("sh state.sh")
			if (err != nil) != test.wantError {
				t.Fatalf("Execute() error = %v, wantError %v", err, test.wantError)
			}
			if box.CWD != "/work" {
				t.Errorf("restored CWD = %q, want /work", box.CWD)
			}
			if !reflect.DeepEqual(box.Env, beforeEnv) {
				t.Errorf("restored environment = %#v, want %#v", box.Env, beforeEnv)
			}
			if !box.FS.Exists(test.persisted) {
				t.Errorf("script filesystem change %s did not persist", test.persisted)
			}
		})
	}
}

func TestShNestedScriptsShareTraceButOnlyTopLevelHistory(t *testing.T) {
	box := testSandbox(t)
	writeTestScript(t, box, "inner.sh", "printf 'inner\\n'\n", 0o644)
	writeTestScript(t, box, "outer.sh", "echo outer\nsh inner.sh\necho done\n", 0o644)

	result, err := box.Execute("sh outer.sh")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Output != "outer\ninner\ndone\n" {
		t.Errorf("output = %q", result.Output)
	}
	wantCommands := []string{"sh", "echo", "sh", "printf", "echo"}
	if !reflect.DeepEqual(result.Commands, wantCommands) {
		t.Errorf("commands = %v, want %v", result.Commands, wantCommands)
	}
	if !reflect.DeepEqual(box.History, []string{"sh outer.sh"}) {
		t.Errorf("history = %v, want only top-level input", box.History)
	}
}

func TestShRejectsDirectAndIndirectRecursion(t *testing.T) {
	t.Run("direct", func(t *testing.T) {
		box := testSandbox(t)
		writeTestScript(t, box, "self.sh", "sh self.sh\n", 0o644)
		_, err := box.Execute("sh self.sh")
		if err == nil || !strings.Contains(err.Error(), "recursive script invocation") {
			t.Fatalf("direct recursion error = %v", err)
		}
	})

	t.Run("indirect", func(t *testing.T) {
		box := testSandbox(t)
		writeTestScript(t, box, "a.sh", "sh b.sh\n", 0o644)
		writeTestScript(t, box, "b.sh", "sh a.sh\n", 0o644)
		_, err := box.Execute("sh a.sh")
		if err == nil || !strings.Contains(err.Error(), "recursive script invocation") {
			t.Fatalf("indirect recursion error = %v", err)
		}
	})
}

func TestShEnforcesDepthStepSizeLineAndOutputLimits(t *testing.T) {
	t.Run("depth", func(t *testing.T) {
		box := testSandbox(t)
		for index := 0; index <= maxScriptDepth; index++ {
			content := "echo deepest\n"
			if index < maxScriptDepth {
				content = fmt.Sprintf("sh /work/depth-%d.sh\n", index+1)
			}
			writeTestScript(t, box, fmt.Sprintf("depth-%d.sh", index), content, 0o644)
		}
		_, err := box.Execute("sh depth-0.sh")
		if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("nesting limit of %d", maxScriptDepth)) {
			t.Fatalf("depth limit error = %v", err)
		}
	})

	t.Run("steps", func(t *testing.T) {
		box := testSandbox(t)
		writeTestScript(t, box, "steps.sh", strings.Repeat("pwd\n", maxScriptSteps+1), 0o644)
		result, err := box.Execute("sh steps.sh")
		if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("command limit of %d", maxScriptSteps)) {
			t.Fatalf("step limit error = %v", err)
		}
		if len(result.Commands) != maxScriptSteps+1 { // top-level sh plus the allowed commands
			t.Errorf("command trace length = %d, want %d", len(result.Commands), maxScriptSteps+1)
		}
	})

	t.Run("script size", func(t *testing.T) {
		box := testSandbox(t)
		writeTestScript(t, box, "large.sh", strings.Repeat("#", maxScriptBytes+1), 0o644)
		_, err := box.Execute("sh large.sh")
		if err == nil || !strings.Contains(err.Error(), "script exceeds") {
			t.Fatalf("script size error = %v", err)
		}
	})

	t.Run("line size", func(t *testing.T) {
		box := testSandbox(t)
		writeTestScript(t, box, "long-line.sh", strings.Repeat("x", maxScriptLineBytes+1)+"\n", 0o644)
		_, err := box.Execute("sh long-line.sh")
		if err == nil || !strings.Contains(err.Error(), ":1: line exceeds") {
			t.Fatalf("line size error = %v", err)
		}
	})

	t.Run("output", func(t *testing.T) {
		box := testSandbox(t)
		if err := box.FS.WriteFile("/work/huge.txt", strings.Repeat("x", maxScriptOutputBytes+1), 0o644); err != nil {
			t.Fatal(err)
		}
		writeTestScript(t, box, "output.sh", "cat huge.txt\n", 0o644)
		_, err := box.Execute("sh output.sh")
		if err == nil || !strings.Contains(err.Error(), "script output exceeds") {
			t.Fatalf("output limit error = %v", err)
		}
	})
}

func TestShRejectsUnsupportedShellLanguage(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{name: "semicolon", line: "echo one; echo two", want: "control syntax"},
		{name: "and", line: "echo one && echo two", want: "control syntax"},
		{name: "or", line: "echo one || echo two", want: "operator ||"},
		{name: "parentheses", line: "(echo one)", want: "control syntax"},
		{name: "backticks", line: "echo `pwd`", want: "command substitution"},
		{name: "substitution", line: "echo $(pwd)", want: "command substitution"},
		{name: "quoted substitution", line: `echo "$(pwd)"`, want: "command substitution"},
		{name: "special parameter", line: "echo $?", want: "special parameters"},
		{name: "positional parameter", line: "echo ${1}", want: "special parameters"},
		{name: "if keyword", line: "if echo yes", want: "keyword"},
		{name: "for keyword", line: "for item", want: "keyword"},
		{name: "function keyword", line: "function task", want: "keyword"},
		{name: "assignment", line: "TARGET=production", want: "standalone assignments"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			box := testSandbox(t)
			writeTestScript(t, box, "unsupported.sh", test.line+"\n", 0o644)
			_, err := box.Execute("sh unsupported.sh")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("line %q error = %v, want substring %q", test.line, err, test.want)
			}
			if !strings.Contains(err.Error(), "/work/unsupported.sh:1:") {
				t.Errorf("error lacks virtual line location: %v", err)
			}
		})
	}
}

func TestShAllowsQuotedShellPunctuationAsData(t *testing.T) {
	box := testSandbox(t)
	literal := "; && || () ` $() $? NAME=value if then"
	writeTestScript(t, box, "quoted.sh", "echo '"+literal+"' > /out/literal.txt\n", 0o644)

	if _, err := box.Execute("sh quoted.sh"); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	content, err := box.FS.ReadFile("/out/literal.txt")
	if err != nil || content != literal+"\n" {
		t.Fatalf("literal output = %q, %v", content, err)
	}
}

func TestShRejectsInteractiveVi(t *testing.T) {
	box := testSandbox(t)
	writeTestScript(t, box, "edit.sh", "vi events.log\n", 0o644)

	result, err := box.Execute("sh edit.sh")
	if err == nil || !strings.Contains(err.Error(), "vi: interactive commands are not supported in scripts") {
		t.Fatalf("vi script error = %v", err)
	}
	if result.Editor != nil {
		t.Errorf("script returned editor request %#v", result.Editor)
	}
}

func TestShOutputSupportsOutgoingPipelinesAndRedirection(t *testing.T) {
	box := testSandbox(t)
	writeTestScript(t, box, "producer.sh", "echo alpha\necho beta\n", 0o644)

	result, err := box.Execute("sh producer.sh | grep beta > /out/filtered.txt")
	if err != nil {
		t.Fatalf("pipeline Execute() error = %v", err)
	}
	if result.Output != "" {
		t.Errorf("redirected pipeline output = %q", result.Output)
	}
	content, err := box.FS.ReadFile("/out/filtered.txt")
	if err != nil || content != "beta\n" {
		t.Fatalf("filtered output = %q, %v", content, err)
	}
	if !reflect.DeepEqual(result.Commands, []string{"sh", "echo", "echo", "grep"}) {
		t.Errorf("pipeline trace = %v", result.Commands)
	}

	if _, err := box.Execute("sh producer.sh > /out/all.txt"); err != nil {
		t.Fatalf("redirection Execute() error = %v", err)
	}
	content, err = box.FS.ReadFile("/out/all.txt")
	if err != nil || content != "alpha\nbeta\n" {
		t.Fatalf("redirected output = %q, %v", content, err)
	}
}

func TestShRedirectionClearsVirtualArchiveMetadata(t *testing.T) {
	box := testSandbox(t)
	if err := box.FS.WriteFile("/out/bundle.tar", "virtual archive\n", 0o644); err != nil {
		t.Fatal(err)
	}
	box.Archives["/out/bundle.tar"] = Archive{}
	writeTestScript(t, box, "overwrite.sh", "echo replaced > /out/bundle.tar\n", 0o644)

	if _, err := box.Execute("sh overwrite.sh"); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if _, exists := box.Archives["/out/bundle.tar"]; exists {
		t.Fatal("script redirection retained stale archive metadata")
	}
	content, _ := box.FS.ReadFile("/out/bundle.tar")
	if content != "replaced\n" {
		t.Errorf("archive replacement = %q", content)
	}
}

func TestShCannotReadExecuteOrWriteHostPaths(t *testing.T) {
	box := testSandbox(t)
	hostFile := filepath.Join(t.TempDir(), "script-escape.txt")
	virtualFile := filepath.ToSlash(hostFile)
	if err := box.FS.EnsureDir(filepath.ToSlash(filepath.Dir(hostFile)), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestScript(t, box, "isolation.sh", fmt.Sprintf("echo virtual-only > %q\n/bin/sh\n", virtualFile), 0o644)

	_, err := box.Execute("sh isolation.sh")
	if err == nil || !strings.Contains(err.Error(), "/bin/sh: no such file") {
		t.Fatalf("virtual /bin/sh error = %v", err)
	}
	if _, err := os.Stat(hostFile); !os.IsNotExist(err) {
		t.Fatalf("host path was changed or stat returned unexpected error: %v", err)
	}
	content, err := box.FS.ReadFile(virtualFile)
	if err != nil || content != "virtual-only\n" {
		t.Fatalf("virtual host-shaped path = %q, %v", content, err)
	}
}
