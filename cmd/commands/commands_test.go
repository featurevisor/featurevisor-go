package commands

import (
	"bytes"
	"flag"
	"io"
	"reflect"
	"testing"
)

func TestCLIOptionAliasesAndValueForms(t *testing.T) {
	for alias, original := range map[string]string{
		"key-pattern": "keyPattern", "assertion-pattern": "assertionPattern",
		"entity-type": "entityType", "only-failures": "onlyFailures",
		"show-datafile": "showDatafile", "populate-uuid": "populateUuid",
		"project-directory-path": "projectDirectoryPath", "root-directory-path": "projectDirectoryPath",
		"rootDirectoryPath": "projectDirectoryPath",
	} {
		value := `a space and "quote" = value`
		if alias == "entity-type" {
			value = "feature"
		}
		boolean := alias == "only-failures" || alias == "show-datafile"
		if boolean {
			value = "true"
		}
		want, err := parseCLIOptions([]string{"--" + original + "=" + value}, io.Discard)
		if err != nil {
			t.Fatal(err)
		}
		forms := [][]string{{"--" + alias + "=" + value}}
		if boolean {
			forms = append(forms, []string{"--" + alias})
		} else {
			forms = append(forms, []string{"--" + alias, value})
		}
		for _, args := range forms {
			got, err := parseCLIOptions(args, io.Discard)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("%v: %v, %#v != %#v", args, err, got, want)
			}
		}
	}
	for _, flag := range []string{"--n", "-n"} {
		for _, args := range [][]string{{flag + "=2"}, {flag, "2"}} {
			got, err := parseCLIOptions(args, io.Discard)
			if err != nil || got.N != 2 {
				t.Fatalf("%v: %v, %#v", args, err, got)
			}
		}
	}
	opts, err := parseCLIOptions([]string{"--target", "web", "--populate-uuid", "userId", "--target=mobile", "--populateUuid=deviceId", "--target=web", "--populate-uuid=accountId"}, io.Discard)
	if err != nil || !reflect.DeepEqual(opts.Targets, []string{"web", "mobile"}) || !reflect.DeepEqual(opts.PopulateUuid, []string{"userId", "deviceId", "accountId"}) {
		t.Fatalf("%v: %#v", err, opts)
	}
	context := `{"name":"Ada Lovelace", "quote":"it's a \\\"test\\\"", "literal":"$(touch marker)"}`
	opts, err = parseCLIOptions([]string{"--context", context}, io.Discard)
	if err != nil || opts.Context != context {
		t.Fatalf("context changed: %v, %q", err, opts.Context)
	}
	if got := ParseCLIOptions(nil).ProjectDirectoryPath; got != getCurrentDir() {
		t.Fatalf("default directory: %q", got)
	}
}

func TestCLIOptionValidationAndHelp(t *testing.T) {
	invalid := [][]string{{"--typo"}, {"extra"}, {"--target=web", "extra"}, {"--", "extra"}, {"--only-failures=wat"}, {"--entity-type=typo"}}
	for _, name := range []string{"target", "populate-uuid", "key-pattern", "project-directory-path", "context", "set", "n", "inflate"} {
		invalid = append(invalid, []string{"--" + name}, []string{"--" + name, "--quiet"})
	}
	for _, name := range []string{"target", "populate-uuid", "project-directory-path", "context", "feature", "entity-type"} {
		invalid = append(invalid, []string{"--" + name + "="})
	}
	for _, name := range []string{"--n", "-n", "--inflate"} {
		for _, value := range []string{"0", "-1", "1.5", "NaN", "", "999999999999999999999"} {
			invalid = append(invalid, []string{name + "=" + value}, []string{name, value})
		}
	}
	for _, context := range []string{"[]", "null", "42", "true", "{"} {
		invalid = append(invalid, []string{"--context", context})
	}
	for _, args := range invalid {
		if _, err := parseCLIOptions(args, io.Discard); err == nil {
			t.Errorf("accepted invalid arguments: %v", args)
		}
	}
	for _, arg := range []string{"--help", "-h"} {
		var output bytes.Buffer
		if _, err := parseCLIOptions([]string{arg}, &output); err != flag.ErrHelp {
			t.Fatalf("help: %v", err)
		}
		for _, option := range []string{"key-pattern", "populate-uuid", "root-directory-path"} {
			if !bytes.Contains(output.Bytes(), []byte(option)) {
				t.Errorf("help missing %s", option)
			}
		}
	}
}

func TestParseCLIOptionsDistinguishesOmittedInflation(t *testing.T) {
	if opts := ParseCLIOptions(nil); opts.Inflate != 0 {
		t.Fatalf("omitted inflation: got %d, want sentinel 0", opts.Inflate)
	}
	for _, args := range [][]string{{"--inflate=1"}, {"--inflate", "1"}} {
		if opts := ParseCLIOptions(args); opts.Inflate != 1 {
			t.Fatalf("explicit inflation: got %d, want 1", opts.Inflate)
		}
	}
}

func TestParseCLIOptionsAcceptsLegacyIgnoredFlags(t *testing.T) {
	opts := ParseCLIOptions([]string{"--with-scopes", "--with-tags", "--schemaVersion=1", "--schema-version=2"})

	if !opts.WithScopes {
		t.Fatalf("expected WithScopes to be true")
	}
	if !opts.WithTags {
		t.Fatalf("expected WithTags to be true")
	}
	if opts.SchemaVersion != "2" {
		t.Fatalf("expected SchemaVersion to be parsed for compatibility")
	}
}

func TestParseCLIOptionsAcceptsRepeatedTargets(t *testing.T) {
	opts := ParseCLIOptions([]string{"--target=web", "--target=mobile", "--target=web"})
	if len(opts.Targets) != 2 || opts.Targets[0] != "web" || opts.Targets[1] != "mobile" {
		t.Fatalf("unexpected targets: %#v", opts.Targets)
	}
}

func TestParseCLIOptionsAcceptsReportingOptions(t *testing.T) {
	opts := ParseCLIOptions([]string{"--json", "--pretty", "--case=opaque", "--coverage", "--uncovered"})
	if !opts.JSON || !opts.Pretty || opts.Case != "opaque" || !opts.Coverage || !opts.Uncovered {
		t.Fatalf("unexpected reporting options: %#v", opts)
	}
}

func TestTargetDatafileCacheKey(t *testing.T) {
	if got := targetDatafileCacheKey(nil, "checkout"); got != `[null,"checkout"]` {
		t.Fatalf("unexpected flat target key: %s", got)
	}

	environment := "production"
	if got := targetDatafileCacheKey(&environment, "checkout"); got != `["production","checkout"]` {
		t.Fatalf("unexpected configured target key: %s", got)
	}
}

func TestDatafileCacheKeyForTargetAssertion(t *testing.T) {
	cache := map[string]interface{}{
		`["production",null]`:       map[string]interface{}{"kind": "base"},
		`["production","checkout"]`: map[string]interface{}{"kind": "target"},
	}

	got := datafileCacheKeyForAssertion(map[string]interface{}{
		"environment": "production",
		"target":      "checkout",
	}, cache)

	if got != `["production","checkout"]` {
		t.Fatalf("unexpected target key: %s", got)
	}
}

func TestDatafileCacheKeyForTargetAssertionDoesNotFallBackToBase(t *testing.T) {
	cache := map[string]interface{}{
		`["production",null]`: map[string]interface{}{"kind": "base"},
	}

	got := datafileCacheKeyForAssertion(map[string]interface{}{
		"environment": "production",
		"target":      "checkout",
	}, cache)

	if got != `["production","checkout"]` {
		t.Fatalf("unexpected target key: %s", got)
	}
}

func TestDatafileCacheKeyForNoEnvironmentTargetAssertion(t *testing.T) {
	cache := map[string]interface{}{
		`[null,null]`:       map[string]interface{}{"kind": "base"},
		`[null,"checkout"]`: map[string]interface{}{"kind": "target"},
	}

	got := datafileCacheKeyForAssertion(map[string]interface{}{
		"target": "checkout",
	}, cache)

	if got != `[null,"checkout"]` {
		t.Fatalf("unexpected flat target key: %s", got)
	}
}
