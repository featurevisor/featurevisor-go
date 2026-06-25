package commands

import "testing"

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

func TestTargetDatafileCacheKey(t *testing.T) {
	if got := targetDatafileCacheKey(nil, "checkout"); got != "false-target-checkout" {
		t.Fatalf("expected false-target-checkout, got %s", got)
	}

	environment := "production"
	if got := targetDatafileCacheKey(&environment, "checkout"); got != "production-target-checkout" {
		t.Fatalf("expected production-target-checkout, got %s", got)
	}
}
