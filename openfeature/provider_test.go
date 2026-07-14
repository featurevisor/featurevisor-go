package openfeature

import (
	"context"
	"testing"

	featurevisor "github.com/featurevisor/featurevisor-go"
	of "github.com/open-feature/go-sdk/openfeature"
)

const testDatafile = `{
  "schemaVersion":"2","revision":"openfeature-test","segments":{},
  "features":{
    "checkout":{
      "bucketBy":"userId",
      "variations":[{"value":"on","variables":{"title":"Hello","count":3,"ratio":1.5,"visible":true,"items":["a"],"config":{"color":"blue"},"json":"{\"nested\":true}"}}],
      "variablesSchema":{
        "title":{"type":"string","defaultValue":"Default"},
        "count":{"type":"integer","defaultValue":0},
        "ratio":{"type":"double","defaultValue":0},
        "visible":{"type":"boolean","defaultValue":false},
        "items":{"type":"array","defaultValue":[]},
        "config":{"type":"object","defaultValue":{}},
        "json":{"type":"json","defaultValue":"{}"}
      },
      "force":[{"conditions":{"attribute":"userId","operator":"equals","value":"forced-user"},"enabled":true,"variation":"on"}],
      "traffic":[{"key":"all","segments":"*","percentage":100000,"variation":"on"}]
    },
    "empty":{"bucketBy":"userId","variations":[],"traffic":[{"key":"all","segments":"*","percentage":100000,"allocation":[]}]}
  }
}`

func newTestProvider(options ...func(*Options)) *Provider {
	level := featurevisor.LogLevelFatal
	resolved := Options{FeaturevisorOptions: featurevisor.FeaturevisorOptions{Datafile: testDatafile, LogLevel: &level}}
	for _, option := range options {
		option(&resolved)
	}
	return NewProvider(resolved)
}

func TestProviderResolvesEveryType(t *testing.T) {
	p := newTestProvider()
	ctx := of.FlattenedContext{of.TargetingKey: "forced-user"}
	if result := p.BooleanEvaluation(context.Background(), "checkout", false, ctx); !result.Value || result.Reason != of.TargetingMatchReason {
		t.Fatalf("unexpected flag: %#v", result)
	}
	if result := p.StringEvaluation(context.Background(), "checkout:variation", "fallback", ctx); result.Value != "on" || result.Variant != "on" {
		t.Fatalf("unexpected variation: %#v", result)
	}
	if result := p.StringEvaluation(context.Background(), "checkout:title", "fallback", ctx); result.Value != "Hello" {
		t.Fatalf("unexpected string: %#v", result)
	}
	if result := p.IntEvaluation(context.Background(), "checkout:count", 0, ctx); result.Value != 3 {
		t.Fatalf("unexpected integer: %#v", result)
	}
	if result := p.FloatEvaluation(context.Background(), "checkout:ratio", 0, ctx); result.Value != 1.5 {
		t.Fatalf("unexpected float: %#v", result)
	}
	if result := p.BooleanEvaluation(context.Background(), "checkout:visible", false, ctx); !result.Value {
		t.Fatalf("unexpected boolean variable: %#v", result)
	}
	if result := p.ObjectEvaluation(context.Background(), "checkout:items", []any{}, ctx); len(result.Value.([]any)) != 1 {
		t.Fatalf("unexpected array: %#v", result)
	}
	if result := p.ObjectEvaluation(context.Background(), "checkout:config", map[string]any{}, ctx); result.Value.(map[string]any)["color"] != "blue" {
		t.Fatalf("unexpected object: %#v", result)
	}
	if result := p.ObjectEvaluation(context.Background(), "checkout:json", map[string]any{}, ctx); result.Value.(map[string]any)["nested"] != true {
		t.Fatalf("unexpected json: %#v", result)
	}
}

func TestProviderErrorsGrammarTrackingAndLifecycle(t *testing.T) {
	tracked := false
	p := newTestProvider(func(options *Options) {
		options.KeySeparator = "/"
		options.VariationKey = "$variation"
		options.OnTrack = func(event TrackingEvent) { tracked = event.Name == "purchase" }
	})
	ctx := of.FlattenedContext{of.TargetingKey: "user"}
	if result := p.StringEvaluation(context.Background(), "checkout/$variation", "fallback", ctx); result.Value != "on" {
		t.Fatalf("custom grammar failed: %#v", result)
	}
	if result := p.StringEvaluation(context.Background(), "missing", "fallback", ctx); result.ResolutionDetail().ErrorCode != of.TypeMismatchCode {
		t.Fatalf("expected type mismatch: %#v", result)
	}
	if result := p.BooleanEvaluation(context.Background(), "missing", true, ctx); result.ResolutionDetail().ErrorCode != of.FlagNotFoundCode || result.Value != true {
		t.Fatalf("expected missing fallback: %#v", result)
	}
	if result := p.StringEvaluation(context.Background(), "empty/$variation", "fallback", ctx); result.ResolutionDetail().ErrorCode != of.FlagNotFoundCode {
		t.Fatalf("expected no variations error: %#v", result)
	}
	p.Track(context.Background(), "purchase", of.NewEvaluationContext("user", nil), of.NewTrackingEventDetails(1))
	if !tracked {
		t.Fatal("tracking callback was not called")
	}
	p.Shutdown()
}

func TestProviderWorksThroughOpenFeatureSDK(t *testing.T) {
	p := newTestProvider()
	if err := of.SetProviderAndWait(p); err != nil {
		t.Fatal(err)
	}
	client := of.NewClient("")
	value, err := client.BooleanValue(context.Background(), "checkout", false, of.NewEvaluationContext("forced-user", nil))
	if err != nil || !value {
		t.Fatalf("unexpected OpenFeature result: %v %v", value, err)
	}
}

func TestMalformedDatafileReportsParseError(t *testing.T) {
	level := featurevisor.LogLevelFatal
	p := NewProvider(Options{FeaturevisorOptions: featurevisor.FeaturevisorOptions{Datafile: "{", LogLevel: &level}})
	result := p.BooleanEvaluation(context.Background(), "checkout", false, nil)
	if result.ResolutionDetail().ErrorCode != of.ParseErrorCode {
		t.Fatalf("expected parse error: %#v", result)
	}
	p.Featurevisor().SetDatafile(testDatafile, true)
	if recovered := p.BooleanEvaluation(context.Background(), "checkout", false, of.FlattenedContext{of.TargetingKey: "forced-user"}); !recovered.Value || recovered.Error() != nil {
		t.Fatalf("expected provider to recover after valid datafile: %#v", recovered)
	}
}

func TestProviderBorrowsExistingFeaturevisor(t *testing.T) {
	closed := false
	level := featurevisor.LogLevelFatal
	fv := featurevisor.CreateFeaturevisor(featurevisor.FeaturevisorOptions{
		Datafile: testDatafile,
		LogLevel: &level,
		Modules: []*featurevisor.FeaturevisorModule{{
			Name:  "owner",
			Close: func() { closed = true },
		}},
	})
	p := NewProvider(Options{Featurevisor: fv})
	if p.Featurevisor() != fv {
		t.Fatal("provider did not reuse the supplied Featurevisor instance")
	}
	p.Shutdown()
	if closed {
		t.Fatal("provider closed a caller-owned Featurevisor instance")
	}
	fv.Close()
	if !closed {
		t.Fatal("caller could not close its Featurevisor instance")
	}
}
