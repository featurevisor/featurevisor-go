package commands

import "testing"

func TestParseCLIOptionsWithScopesAndTags(t *testing.T) {
	opts := ParseCLIOptions([]string{"--with-scopes", "--with-tags"})

	if !opts.WithScopes {
		t.Fatalf("expected WithScopes to be true")
	}
	if !opts.WithTags {
		t.Fatalf("expected WithTags to be true")
	}
}
