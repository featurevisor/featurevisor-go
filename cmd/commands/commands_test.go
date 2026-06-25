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

func TestDatafileCacheKeyForTargetAssertion(t *testing.T) {
	cache := map[string]interface{}{
		"production":                 map[string]interface{}{"kind": "base"},
		"production-target-checkout": map[string]interface{}{"kind": "target"},
	}

	got := datafileCacheKeyForAssertion(map[string]interface{}{
		"environment": "production",
		"target":      "checkout",
	}, cache)

	if got != "production-target-checkout" {
		t.Fatalf("expected production-target-checkout, got %s", got)
	}
}

func TestDatafileCacheKeyForTargetAssertionFallsBackToBase(t *testing.T) {
	cache := map[string]interface{}{
		"production": map[string]interface{}{"kind": "base"},
	}

	got := datafileCacheKeyForAssertion(map[string]interface{}{
		"environment": "production",
		"target":      "checkout",
	}, cache)

	if got != "production" {
		t.Fatalf("expected production, got %s", got)
	}
}

func TestDatafileCacheKeyForNoEnvironmentTargetAssertion(t *testing.T) {
	cache := map[string]interface{}{
		noEnvironmentKey:        map[string]interface{}{"kind": "base"},
		"false-target-checkout": map[string]interface{}{"kind": "target"},
	}

	got := datafileCacheKeyForAssertion(map[string]interface{}{
		"target": "checkout",
	}, cache)

	if got != "false-target-checkout" {
		t.Fatalf("expected false-target-checkout, got %s", got)
	}
}
