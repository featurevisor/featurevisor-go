package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestEvaluationCommandsSelectProjectsAndFailWithNonzeroStatus(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses a POSIX shell")
	}
	directory := filepath.Join(t.TempDir(), "project with 'quotes' and spaces")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(directory, "featurevisor-go")
	build := exec.Command("go", "build", "-o", binary, "../main.go")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}
	write := func(name, contents string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(directory, name), []byte(contents), 0700); err != nil {
			t.Fatal(err)
		}
	}
	write("npx", `#!/bin/sh
printf '%s\n' "$*" >> calls.log
case "$2" in
  config) cat config.json ;;
  list)
    case "$*" in
      *--tests*) printf '%s\n' '{"kind":"featurevisor.test-execution","schemaVersion":1,"tests":[{"key":"smoke","scenario":"smoke","_test":{"source":"tests/scenarios/smoke.spec.yml","set":null,"entityType":"scenario"},"assertions":[{"_case":{"id":"case-smoke","assertionIndex":0,"matrixIndex":null,"matrixValues":{}},"checks":[{"feature":"checkout","enabled":true}]}]}]}' ;;
      *) printf '%s\n' '[]' ;;
    esac ;;
  build)
    case "$*" in
      *--target=bad*|*--set=bad*) echo 'delegated selection error' >&2; exit 7 ;;
    esac
    cat datafile.json ;;
  *) exit 8 ;;
esac
`)
	write("datafile.json", `{"schemaVersion":"2","revision":"1","segments":{},"features":{"checkout":{"bucketBy":"userId","traffic":[{"key":"all","segments":"*","percentage":100000,"enabled":true}]}},"variables":{"subject":{"type":"string","defaultValue":"ready"}}}`)
	for _, set := range []string{"alpha", "beta"} {
		if err := os.MkdirAll(filepath.Join(directory, "sets", set), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	setsConfig, _ := json.Marshal(map[string]interface{}{"sets": true, "setsDirectoryPath": filepath.Join(directory, "sets")})
	for _, command := range []string{"benchmark", "assess-distribution"} {
		for _, test := range []struct {
			name, config string
			args         []string
			failure      bool
			builds       int
			want         string
		}{
			{"flat", `{}`, nil, false, 1, ""},
			{"configured", `{"environments":["production"]}`, []string{"--environment=production"}, false, 1, "--environment=production"},
			{"missing environment", `{"environments":["production"]}`, nil, true, 0, ""},
			{"unknown environment", `{"environments":["production"]}`, []string{"--environment=typo"}, true, 0, ""},
			{"unexpected environment", `{}`, []string{"--environment=production"}, true, 0, ""},
			{"all sets", string(setsConfig), nil, false, 2, "--set=beta"},
			{"selected set", string(setsConfig), []string{"--set=alpha"}, false, 1, "--set=alpha"},
			{"targets", string(setsConfig), []string{"--set=alpha", "--target=web", "--target=mobile", "--target=web"}, false, 2, "--target=mobile"},
			{"target separate value", `{}`, []string{"--target", "web"}, false, 1, "--target=web"},
			{"bad target", `{}`, []string{"--target=bad"}, true, 1, "--target=bad"},
			{"bad set", string(setsConfig), []string{"--set=bad"}, true, 1, "--set=bad"},
			{"zero iterations", `{}`, []string{"--n=0"}, true, 0, ""},
			{"negative iterations", `{}`, []string{"--n=-1"}, true, 0, ""},
			{"zero inflate", `{}`, []string{"--inflate=0"}, true, 0, ""},
			{"array context", `{}`, []string{"--context=[]"}, true, 0, ""},
			{"null context", `{}`, []string{"--context=null"}, true, 0, ""},
			{"broken context", `{}`, []string{"--context={"}, true, 0, ""},
			{"broken config", `{`, nil, true, 0, ""},
		} {
			t.Run(command+"/"+test.name, func(t *testing.T) {
				write("config.json", test.config)
				write("calls.log", "")
				args := append([]string{command, "--projectDirectoryPath=" + directory, "--feature=checkout", "--n=1"}, test.args...)
				output, err := exec.Command(binary, args...).CombinedOutput()
				if (err != nil) != test.failure {
					t.Fatalf("status: %v\n%s", err, output)
				}
				if test.failure && len(output) == 0 {
					t.Fatalf("missing diagnostic: %s", output)
				}
				calls, _ := os.ReadFile(filepath.Join(directory, "calls.log"))
				if strings.Count(string(calls), "featurevisor build") != test.builds || !strings.Contains(string(calls), test.want) {
					t.Fatalf("unexpected delegated calls:\n%s", calls)
				}
				if test.name == "flat" && strings.Contains(string(calls), "--environment") {
					t.Fatalf("flat build received an environment: %s", calls)
				}
			})
		}
		for _, args := range [][]string{{}, {"--n=1"}} {
			if output, err := exec.Command(binary, append([]string{command}, args...)...).CombinedOutput(); err == nil {
				t.Fatalf("missing feature succeeded: %s", output)
			}
		}
	}
	write("config.json", `{}`)
	for _, command := range []string{"test", "benchmark", "assess-distribution"} {
		for _, flags := range [][]string{{"--typo"}, {"extra"}, {"--target=web", "extra"}, {"--target"}, {"--populate-uuid", "--quiet"}, {"--n=0"}, {"-n", "1.5"}, {"--context", "[]"}, {"--entity-type=typo"}} {
			write("calls.log", "")
			args := append([]string{command, "--root-directory-path", directory, "--feature=checkout"}, flags...)
			process := exec.Command(binary, args...)
			var diagnostics bytes.Buffer
			process.Stderr = &diagnostics
			stdout, err := process.Output()
			calls, _ := os.ReadFile(filepath.Join(directory, "calls.log"))
			if err == nil || len(stdout) != 0 || diagnostics.Len() == 0 || len(calls) != 0 {
				t.Fatalf("usage validation %v: %v stdout=%s stderr=%s calls=%s", args, err, stdout, diagnostics.String(), calls)
			}
		}
		for _, root := range []string{"project-directory-path", "root-directory-path", "rootDirectoryPath"} {
			write("calls.log", "")
			args := []string{command, "--" + root, directory, "--feature", "checkout", "-n", "1", "--target", "web", "--target=mobile", "--target=web", "--populate-uuid", "userId", "--populateUuid=deviceId"}
			if command == "test" {
				args = append(args, "--json", "--only-failures", "--key-pattern=*", "--entity-type=feature")
			}
			stdout, err := exec.Command(binary, args...).Output()
			if err != nil {
				t.Fatalf("aliases %v: %v, %s", args, err, stdout)
			}
			if command == "test" && !json.Valid(stdout) {
				t.Fatalf("invalid JSON stdout: %s", stdout)
			}
			calls, _ := os.ReadFile(filepath.Join(directory, "calls.log"))
			web, mobile := strings.Index(string(calls), "--target=web"), strings.Index(string(calls), "--target=mobile")
			if web < 0 || mobile <= web || strings.Count(string(calls), "--target=web") != 1 {
				t.Fatalf("target order or duplicate changed: %s", calls)
			}
		}
	}
	for _, args := range [][]string{{"--help"}, {"-h"}, {"help", "test"}, {"test", "--help"}, {"benchmark", "--help"}, {"assess-distribution", "-h"}} {
		write("calls.log", "")
		output, err := exec.Command(binary, args...).CombinedOutput()
		calls, _ := os.ReadFile(filepath.Join(directory, "calls.log"))
		if err != nil || !strings.Contains(string(output), "Usage:") || len(calls) != 0 {
			t.Fatalf("help %v: %v, %s", args, err, output)
		}
	}
	for _, args := range [][]string{{"typo"}, {"evaluate"}, {"help", "typo"}, {"help", "test", "extra"}} {
		if output, err := exec.Command(binary, args...).CombinedOutput(); err == nil {
			t.Fatalf("invalid command %v: %s", args, output)
		}
	}
	for _, command := range []string{"benchmark", "assess-distribution", "test"} {
		for _, flags := range [][]string{{"--set=unexpected"}, {"--set="}, {"--set", ""}} {
			t.Run(command+"/flat set/"+strings.Join(flags, " "), func(t *testing.T) {
				write("config.json", `{"sets":false}`)
				write("calls.log", "")
				args := []string{command, "--projectDirectoryPath=" + directory, "--feature=checkout", "--n=1"}
				output, err := exec.Command(binary, append(args, flags...)...).CombinedOutput()
				if err == nil || !strings.Contains(string(output), "Omit --set") {
					t.Fatalf("expected actionable rejection: %v\n%s", err, output)
				}
				calls, _ := os.ReadFile(filepath.Join(directory, "calls.log"))
				if strings.Contains(string(calls), "featurevisor build") || strings.Contains(string(calls), "featurevisor list") {
					t.Fatalf("invalid selection executed: %s", calls)
				}
			})
		}
	}
	write("config.json", `{}`)
	for _, command := range []string{"benchmark", "assess-distribution", "test"} {
		arguments := [][]string{nil, {"--inflate"}}
		for _, value := range []string{"1", "2", "0", "-1", "1.5", "invalid", ""} {
			arguments = append(arguments, []string{"--inflate=" + value}, []string{"--inflate", value})
		}
		for _, extra := range arguments {
			t.Run(command+"/inflate/"+strings.Join(extra, " "), func(t *testing.T) {
				write("calls.log", "")
				args := []string{command, "--projectDirectoryPath=" + directory}
				if command != "test" {
					args = append(args, "--feature=checkout", "--n=1")
				}
				output, err := exec.Command(binary, append(args, extra...)...).CombinedOutput()
				valid := len(extra) == 0
				value := ""
				if len(extra) > 0 {
					value = strings.TrimPrefix(extra[len(extra)-1], "--inflate=")
					valid = value == "1" || value == "2"
				}
				calls, _ := os.ReadFile(filepath.Join(directory, "calls.log"))
				if valid {
					if err != nil || strings.Count(string(calls), "featurevisor build") != 1 {
						t.Fatalf("valid inflation: %v\n%s\n%s", err, output, calls)
					}
					if len(extra) == 0 && strings.Contains(string(calls), "--inflate") {
						t.Fatalf("omitted inflation was delegated: %s", calls)
					}
					if value == "2" && !strings.Contains(string(calls), "--inflate=2") {
						t.Fatalf("inflation multiplier was not delegated: %s", calls)
					}
				} else if err == nil || len(calls) != 0 || !strings.Contains(string(output), "inflate") {
					t.Fatalf("invalid inflation reached Node or lacked diagnostic: %v\n%s\n%s", err, output, calls)
				}
			})
		}
	}
	output, err := exec.Command(binary, "benchmark", "--projectDirectoryPath="+directory, "--variable=subject", "--n=1").CombinedOutput()
	if err != nil || !strings.Contains(string(output), `Evaluated value : "ready"`) {
		t.Fatalf("global variable: %v\n%s", err, output)
	}
	if output, err := exec.Command(binary, "benchmark", "--feature=checkout", "--variation", "--variable=subject").CombinedOutput(); err == nil {
		t.Fatalf("conflicting modes succeeded: %s", output)
	}
}
