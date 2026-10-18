package featurevisor_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestReadmeInitialization(t *testing.T) {
	readme, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile("(?s)## Initialization\\n.*?```go\\n(.*?)\\n```").FindSubmatch(readme)
	if match == nil {
		t.Fatal("README initialization fence not found")
	}
	const fixture = `{"schemaVersion":"2","revision":"readme","segments":{},"features":{"checkout":{"hash":"checkout","bucketBy":"userId","traffic":[{"key":"all","segments":"*","percentage":100000}]}},"variables":{}}`
	status, body := http.StatusOK, fixture
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	defer server.Close()
	if strings.Count(string(match[1]), "https://cdn.yoursite.com/featurevisor-all.json") != 1 {
		t.Fatal("expected exactly one fixture URL to replace")
	}
	source := strings.ReplaceAll(string(match[1]), "https://cdn.yoursite.com/featurevisor-all.json", server.URL)
	directory := t.TempDir()
	file := filepath.Join(directory, "main.go")
	if err := os.WriteFile(file, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(directory, "readme")
	run := func(args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		command := exec.CommandContext(ctx, args[0], args[1:]...)
		command.Env = append(os.Environ(), "GOWORK=off", "GOPROXY=off", "GOSUMDB=off")
		return command.CombinedOutput()
	}
	for _, args := range [][]string{{"go", "vet", file}, {"go", "build", "-o", binary, file}} {
		if output, err := run(args...); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, output)
		}
	}
	for _, test := range []struct {
		name    string
		status  int
		body    string
		want    string
		failure bool
	}{
		{"success", 200, fixture, "true\n", false},
		{"http error with valid JSON", 503, fixture, "datafile request failed: 503", true},
		{"malformed JSON", 200, "{", "panic:", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			mu.Lock()
			status, body = test.status, test.body
			mu.Unlock()
			output, err := run(binary)
			if (err != nil) != test.failure || !strings.Contains(string(output), test.want) {
				t.Fatalf("error=%v output=%s", err, output)
			}
		})
	}
	server.Close()
	t.Run("network failure", func(t *testing.T) {
		if output, err := run(binary); err == nil || !strings.Contains(string(output), "connection refused") {
			t.Fatalf("error=%v output=%s", err, output)
		}
	})
}
