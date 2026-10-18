package commands

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDelegatedJSONKeepsDiagnosticsSeparate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses a POSIX shell")
	}
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "warning", true: "failure"}[failed], func(t *testing.T) {
			directory := t.TempDir()
			script := "#!/bin/sh\nprintf '{\"valid\":true}'\nprintf 'delegated warning\\n' >&2\n"
			if failed {
				script += "exit 7\n"
			}
			if err := os.WriteFile(filepath.Join(directory, "npx"), []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
			output, err := executeFeaturevisorCommand(directory, "build", "--json")
			if failed {
				if err == nil || !strings.Contains(err.Error(), "delegated warning") || !strings.Contains(err.Error(), `{"valid":true}`) {
					t.Fatalf("failure lost process output: %v", err)
				}
			} else if err != nil || output != `{"valid":true}` {
				t.Fatalf("warning corrupted JSON: output=%q error=%v", output, err)
			}
		})
	}
}

func TestDelegationPreservesArgumentBoundaries(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses a POSIX shell")
	}
	directory := filepath.Join(t.TempDir(), "project with 'quotes' and spaces")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > arguments\nprintf '{\"ok\":true}'\n"
	if err := os.WriteFile(filepath.Join(directory, "npx"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	args := []string{"list", "--keyPattern=a 'quoted' value", `--context={"name":"Ada Lovelace","literal":"$(touch marker)"}`, "--json"}
	output, err := executeFeaturevisorCommand(directory, args...)
	if err != nil || output != `{"ok":true}` {
		t.Fatalf("%v: %s", err, output)
	}
	captured, err := os.ReadFile(filepath.Join(directory, "arguments"))
	if err != nil || string(captured) != strings.Join(append([]string{"featurevisor"}, args...), "\n")+"\n" {
		t.Fatalf("arguments: %v, %s", err, captured)
	}
	if _, err := os.Stat(filepath.Join(directory, "marker")); !os.IsNotExist(err) {
		t.Fatal("arguments were interpreted by a shell")
	}
}
