package openfeature

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	featurevisor "github.com/featurevisor/featurevisor-go/v4"
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
  },
  "variables":{"supportEmail":{"type":"string","defaultValue":"support@example.com"},"settings":{"type":"object","defaultValue":{"enabled":true}}}
}`

const requirementsDatafile = `{
  "schemaVersion":"2","revision":"openfeature-requirements","segments":{},
  "features":{
    "guarded":{
      "bucketBy":"userId",
      "conditions":[{"variable":"gate","operator":"equals","value":true}],
      "variations":[],"traffic":[]
    }
  },
  "variables":{"gate":{"type":"boolean","defaultValue":false}}
}`

func TestInitialHealthIndependentOfLogging(t *testing.T) {
	var typedNil *featurevisor.DatafileContent
	for _, level := range []featurevisor.LogLevel{featurevisor.LogLevelError, featurevisor.LogLevelFatal} {
		for _, input := range []any{"{", "", "null", `{}`, `{"schemaVersion":"2","revision":"x","features":[],"segments":{}}`, map[string]any{"schemaVersion": "2", "revision": "x", "features": []any{}, "segments": map[string]any{}}, featurevisor.DatafileContent{}, typedNil, false} {
			var diagnostics []string
			var revisions []string
			closed := 0
			p := NewProvider(Options{FeaturevisorOptions: featurevisor.FeaturevisorOptions{
				Datafile: input, LogLevel: &level,
				OnDiagnostic: func(d featurevisor.FeaturevisorDiagnostic) { diagnostics = append(diagnostics, d.Code) },
				Modules:      []*featurevisor.FeaturevisorModule{{Setup: func(api featurevisor.FeaturevisorModuleApi) { revisions = append(revisions, api.GetRevision()) }, Close: func() { closed++ }}},
			}})
			if !reflect.DeepEqual(revisions, []string{"unknown"}) {
				t.Fatalf("setup revision: %v", revisions)
			}
			if p.Init(of.EvaluationContext{}) == nil {
				t.Fatalf("invalid initialization succeeded: %v", input)
			}
			if result := p.BooleanEvaluation(context.Background(), "checkout", false, nil); result.ResolutionDetail().ErrorCode != of.ParseErrorCode {
				t.Fatalf("missing parse error: %v", input)
			}
			if level == featurevisor.LogLevelFatal && len(diagnostics) != 0 {
				t.Fatalf("log filter bypassed: %v", diagnostics)
			}
			p.Featurevisor().SetDatafile(testDatafile, true)
			if event := <-p.EventChannel(); event.EventType != of.ProviderReady {
				t.Fatalf("recovery event: %v", event)
			}
			if err := p.Init(of.EvaluationContext{}); err != nil {
				t.Fatal(err)
			}
			p.Featurevisor().SetDatafile("{", true)
			if event := <-p.EventChannel(); event.EventType != of.ProviderError || event.ErrorCode != of.ParseErrorCode {
				t.Fatalf("error event: %v", event)
			}
			p.Shutdown()
			p.Shutdown()
			if closed != 1 {
				t.Fatalf("close count: %d", closed)
			}
		}
	}
}

func TestInvalidRegistrationAndRecoveryThroughClient(t *testing.T) {
	level := featurevisor.LogLevelFatal
	p := NewProvider(Options{FeaturevisorOptions: featurevisor.FeaturevisorOptions{Datafile: `{}`, LogLevel: &level}})
	if err := of.SetProviderAndWait(p); err == nil {
		t.Fatal("invalid registration succeeded")
	}
	defer of.Shutdown()
	client := of.NewClient("initial-health")
	if client.State() != of.ErrorState {
		t.Fatalf("invalid state: %v", client.State())
	}
	p.Featurevisor().SetDatafile(testDatafile, true)
	deadline := time.Now().Add(time.Second)
	for client.State() != of.ReadyState && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if client.State() != of.ReadyState {
		t.Fatalf("recovery state: %v", client.State())
	}
	value, err := client.BooleanValue(context.Background(), "checkout", false, of.EvaluationContext{})
	if err != nil || !value {
		t.Fatalf("recovery value: %v %v", value, err)
	}
	if p.Featurevisor().SetDatafile("{", true) {
		t.Fatal("invalid update accepted")
	}
	deadline = time.Now().Add(time.Second)
	for client.State() != of.ErrorState && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if client.State() != of.ErrorState {
		t.Fatalf("rejected update state: %v", client.State())
	}
	if result, err := client.BooleanValueDetails(context.Background(), "checkout", false, of.EvaluationContext{}); err == nil || result.Value {
		t.Fatalf("unhealthy default: %#v %v", result, err)
	}
	if !p.Featurevisor().SetDatafile(testDatafile, true) {
		t.Fatal("same data recovery rejected")
	}
	deadline = time.Now().Add(time.Second)
	for client.State() != of.ReadyState && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if value, err := client.BooleanValue(context.Background(), "checkout", false, of.EvaluationContext{}); err != nil || !value {
		t.Fatalf("same data recovery: %v %v", value, err)
	}
}

func TestSharedProviderConformance(t *testing.T) {
	var fixture struct {
		Kind          string          `json:"kind"`
		SchemaVersion int             `json:"schemaVersion"`
		Datafile      json.RawMessage `json:"datafile"`
		Cases         []struct {
			Name     string         `json:"name"`
			FlagKey  string         `json:"flagKey"`
			Type     string         `json:"type"`
			Default  any            `json:"default"`
			Context  map[string]any `json:"context"`
			Expected struct {
				Value     any    `json:"value"`
				Reason    string `json:"reason"`
				Variant   string `json:"variant"`
				ErrorCode string `json:"errorCode"`
			} `json:"expected"`
		} `json:"cases"`
		TargetingCases []struct {
			Name          string         `json:"name"`
			Context       map[string]any `json:"context"`
			ExpectedValue bool           `json:"expectedValue"`
		} `json:"targetingCases"`
		CustomTargetingCase struct {
			FlagKey       string         `json:"flagKey"`
			Context       map[string]any `json:"context"`
			ExpectedValue bool           `json:"expectedValue"`
		} `json:"customTargetingCase"`
	}
	contents, err := os.ReadFile("conformance/openfeature-provider.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(contents, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Kind != "featurevisor.openfeature-provider" || fixture.SchemaVersion != 1 || len(fixture.Cases) != 16 {
		t.Fatal("unsupported provider fixture")
	}
	level := featurevisor.LogLevelFatal
	options := Options{FeaturevisorOptions: featurevisor.FeaturevisorOptions{Datafile: string(fixture.Datafile), LogLevel: &level}}
	p := NewProvider(options)
	defer p.Shutdown()
	for _, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			before, _ := json.Marshal(tc.Context)
			var value any
			var reason, variant, errorCode string
			ctx := of.FlattenedContext(tc.Context)
			switch tc.Type {
			case "boolean":
				r := p.BooleanEvaluation(context.Background(), tc.FlagKey, tc.Default.(bool), ctx)
				value, reason, variant, errorCode = r.Value, string(r.Reason), r.Variant, string(r.ResolutionDetail().ErrorCode)
			case "string":
				r := p.StringEvaluation(context.Background(), tc.FlagKey, tc.Default.(string), ctx)
				value, reason, variant, errorCode = r.Value, string(r.Reason), r.Variant, string(r.ResolutionDetail().ErrorCode)
			case "object":
				r := p.ObjectEvaluation(context.Background(), tc.FlagKey, tc.Default, ctx)
				value, reason, variant, errorCode = r.Value, string(r.Reason), r.Variant, string(r.ResolutionDetail().ErrorCode)
			default:
				t.Fatalf("unknown type %s", tc.Type)
			}
			got, _ := json.Marshal(value)
			want, _ := json.Marshal(tc.Expected.Value)
			if string(got) != string(want) || (tc.Expected.Reason != "" && reason != tc.Expected.Reason) || variant != tc.Expected.Variant || errorCode != tc.Expected.ErrorCode {
				t.Fatalf("got %s %s %s %s; want %s %s %s %s", got, reason, variant, errorCode, want, tc.Expected.Reason, tc.Expected.Variant, tc.Expected.ErrorCode)
			}
			after, _ := json.Marshal(tc.Context)
			if string(before) != string(after) {
				t.Fatal("input context mutated")
			}
		})
	}
	for _, tc := range fixture.TargetingCases {
		t.Run(tc.Name, func(t *testing.T) {
			before, _ := json.Marshal(tc.Context)
			r := p.BooleanEvaluation(context.Background(), "targetOnly", false, of.FlattenedContext(tc.Context))
			if r.Value != tc.ExpectedValue {
				t.Fatalf("got %v; want %v", r.Value, tc.ExpectedValue)
			}
			after, _ := json.Marshal(tc.Context)
			if string(before) != string(after) {
				t.Fatal("input context mutated")
			}
		})
	}
	options.TargetingKeyField = "accountId"
	custom := NewProvider(options)
	defer custom.Shutdown()
	c := fixture.CustomTargetingCase
	if r := custom.BooleanEvaluation(context.Background(), c.FlagKey, false, of.FlattenedContext(c.Context)); r.Value != c.ExpectedValue {
		t.Fatalf("custom targeting: %#v", r)
	}
}

func TestClientTargetingKeyPresenceLimit(t *testing.T) {
	contents, err := os.ReadFile("conformance/openfeature-provider.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Datafile json.RawMessage `json:"datafile"`
	}
	if err := json.Unmarshal(contents, &fixture); err != nil {
		t.Fatal(err)
	}
	level := featurevisor.LogLevelFatal
	options := Options{FeaturevisorOptions: featurevisor.FeaturevisorOptions{Datafile: string(fixture.Datafile), LogLevel: &level}}
	p := NewProvider(options)
	defer p.Shutdown()
	if err := of.SetProviderAndWait(p); err != nil {
		t.Fatal(err)
	}
	client := of.NewClient("")
	value, _ := client.BooleanValue(context.Background(), "targetOnly", false, of.NewEvaluationContext("", map[string]any{"userId": "forced"}))
	if !value {
		t.Fatal("the upstream client omits an empty targeting key, so the existing field must remain")
	}
	value, _ = client.BooleanValue(context.Background(), "targetOnly", false, of.NewEvaluationContext("", map[string]any{"targetingKey": "", "userId": "forced"}))
	if value {
		t.Fatal("an explicit targetingKey attribute must override the old field")
	}
	value, _ = client.BooleanValue(context.Background(), "targetOnly", false, of.NewEvaluationContext("other", map[string]any{"userId": "forced"}))
	if value {
		t.Fatal("a nonempty targeting key must override the old field")
	}
	options.TargetingKeyField = "accountId"
	custom := NewProvider(options)
	defer custom.Shutdown()
	if err := of.SetProviderAndWait(custom); err != nil {
		t.Fatal(err)
	}
	client = of.NewClient("")
	value, _ = client.BooleanValue(context.Background(), "accountTargeted", false, of.NewEvaluationContext("", map[string]any{"targetingKey": "", "accountId": "old"}))
	if !value {
		t.Fatal("explicit targetingKey attribute must override the configured custom field")
	}
}

func TestBorrowedInstanceParseErrorRecoveryAndCleanup(t *testing.T) {
	level := featurevisor.LogLevelFatal
	f := featurevisor.CreateFeaturevisor(featurevisor.FeaturevisorOptions{Datafile: testDatafile, LogLevel: &level})
	defer f.Close()
	p := NewProvider(Options{Featurevisor: f})
	if f.SetDatafile("{") {
		t.Fatal("malformed update must be rejected")
	}
	if result := p.BooleanEvaluation(context.Background(), "checkout", false, nil); result.ResolutionDetail().ErrorCode != of.ParseErrorCode {
		t.Fatal("malformed update must return a parse error")
	}
	if !f.SetDatafile(testDatafile, true) {
		t.Fatal("valid recovery must be accepted")
	}
	if result := p.BooleanEvaluation(context.Background(), "checkout", false, nil); result.Error() != nil {
		t.Fatal(result.Error())
	}
	p.Shutdown()
	p.Shutdown()
	if f.SetDatafile("{") {
		t.Fatal("malformed update must still be rejected after provider shutdown")
	}
	if result := p.BooleanEvaluation(context.Background(), "checkout", false, nil); result.Error() != nil {
		t.Fatal("closed provider must detach its diagnostic listener")
	}
}

func TestConcurrentDatafileUpdatesAndEvaluation(t *testing.T) {
	p := newTestProvider()
	defer p.Shutdown()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			p.Featurevisor().SetDatafile("{")
			p.Featurevisor().SetDatafile(testDatafile, true)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			result := p.BooleanEvaluation(context.Background(), "checkout", false, nil)
			if result.Error() != nil && result.ResolutionDetail().ErrorCode != of.ParseErrorCode {
				t.Errorf("unexpected error: %v", result.Error())
			}
		}
	}()
	wg.Wait()
}

func TestShutdownCanReenterFromOwnedModule(t *testing.T) {
	var p *Provider
	p = newTestProvider(func(options *Options) {
		options.FeaturevisorOptions.Modules = []*featurevisor.FeaturevisorModule{{
			Name:  "reentrant-close",
			Close: func() { p.Shutdown() },
		}}
	})
	p.Shutdown()
	p.Shutdown()
}

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
	if result := p.StringEvaluation(context.Background(), "variable:supportEmail", "fallback", ctx); result.Value != "support@example.com" || result.FlagMetadata["featureKey"] != nil {
		t.Fatalf("unexpected global string: %#v", result)
	}
	if result := p.ObjectEvaluation(context.Background(), "variable:settings", map[string]any{}, ctx); result.Value.(map[string]any)["enabled"] != true {
		t.Fatalf("unexpected global object: %#v", result)
	}
	if result := p.StringEvaluation(context.Background(), "checkout:variable", "fallback", ctx); result.ResolutionDetail().ErrorCode != of.FlagNotFoundCode {
		t.Fatalf("feature variable selector collided with global prefix: %#v", result)
	}
}

func TestProviderMapsRequiredVariablesUnmetToDisabledReason(t *testing.T) {
	level := featurevisor.LogLevelFatal
	p := NewProvider(Options{FeaturevisorOptions: featurevisor.FeaturevisorOptions{Datafile: requirementsDatafile, LogLevel: &level}})
	result := p.BooleanEvaluation(context.Background(), "guarded", true, of.FlattenedContext{of.TargetingKey: "user"})
	if result.Value || result.Reason != of.DisabledReason || result.Error() != nil {
		t.Fatalf("unexpected required variable result: %#v", result)
	}
}

func TestProviderMapsRequiredFeatureReasonToDisabled(t *testing.T) {
	if reason := reasonFor(featurevisor.EvaluationReasonConditionsUnmet); reason != of.DisabledReason {
		t.Fatalf("unmet feature requirement mapped to %v", reason)
	}
	if reason := reasonFor(featurevisor.EvaluationReason("required")); reason != of.DefaultReason {
		t.Fatalf("removed reason is still mapped as an alias: %v", reason)
	}
	level := featurevisor.LogLevelFatal
	p := NewProvider(Options{FeaturevisorOptions: featurevisor.FeaturevisorOptions{
		Datafile: `{"schemaVersion":"2","revision":"reason","segments":{},"features":{
			"subject":{"bucketBy":"userId","requiredFeatures":["missing"],"traffic":[]}
		}}`,
		LogLevel: &level,
	}})
	defer p.Shutdown()
	result := p.BooleanEvaluation(context.Background(), "subject", true, of.FlattenedContext{})
	if result.Value || result.Reason != of.DisabledReason || result.Error() != nil || result.FlagMetadata["featurevisorReason"] != "conditions_unmet" {
		t.Fatalf("unexpected unmet feature result: %#v", result)
	}
}

func TestIntegerResolutionRejectsInvalidNumbersAndPreservesMetadata(t *testing.T) {
	values := []struct {
		name  string
		value any
	}{
		{name: "fractional", value: 3.5},
		{name: "positive overflow", value: float64(9223372036854775808)},
		{name: "negative overflow", value: -float64(9223372036854775808) * 2},
		{name: "positive infinity", value: math.Inf(1)},
		{name: "negative infinity", value: math.Inf(-1)},
		{name: "nan", value: math.NaN()},
	}
	for _, test := range values {
		t.Run(test.name, func(t *testing.T) {
			p := newTestProvider(func(options *Options) {
				options.FeaturevisorOptions.Modules = []*featurevisor.FeaturevisorModule{{
					Name: "integer-value",
					AfterEvaluation: func(evaluation featurevisor.Evaluation, options featurevisor.EvaluateOptions) featurevisor.Evaluation {
						if evaluation.VariableKey != nil && *evaluation.VariableKey == "count" {
							evaluation.VariableValue = test.value
						}
						return evaluation
					},
				}}
			})
			result := p.IntEvaluation(context.Background(), "checkout:count", 17, of.FlattenedContext{of.TargetingKey: "forced-user"})
			if result.Value != 17 || result.ResolutionDetail().ErrorCode != of.TypeMismatchCode || result.Reason != of.ErrorReason {
				t.Fatalf("invalid integer did not return a type mismatch fallback: %#v", result)
			}
			if result.FlagMetadata["featureKey"] != "checkout" || result.FlagMetadata["variableKey"] != "count" || result.FlagMetadata["featurevisorReason"] != string(featurevisor.EvaluationReasonAllocated) {
				t.Fatalf("type mismatch lost evaluation metadata: %#v", result.FlagMetadata)
			}
		})
	}
}

func TestIntegerConversionChecksBoundsBeforeCasting(t *testing.T) {
	valid := []struct {
		value any
		want  int64
	}{
		{value: int64(math.MaxInt64), want: math.MaxInt64},
		{value: int64(math.MinInt64), want: math.MinInt64},
		{value: float64(-9223372036854775808), want: math.MinInt64},
		{value: math.Nextafter(float64(9223372036854775808), 0), want: 9223372036854774784},
		{value: json.Number("9223372036854775807"), want: math.MaxInt64},
	}
	for _, test := range valid {
		got, ok := asInt(test.value)
		if !ok || got != test.want {
			t.Fatalf("asInt(%v) = %d, %v; want %d, true", test.value, got, ok, test.want)
		}
	}

	invalid := []any{
		float64(9223372036854775808),
		math.Nextafter(float64(-9223372036854775808), math.Inf(-1)),
		-1.5,
		math.Inf(1),
		math.NaN(),
		json.Number("9223372036854775808"),
	}
	for _, value := range invalid {
		if got, ok := asInt(value); ok {
			t.Fatalf("asInt(%v) = %d, true; want conversion failure", value, got)
		}
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

func TestClientTrackingUpdatesErrorsAndOwnership(t *testing.T) {
	for _, borrowed := range []bool{false, true} {
		t.Run(fmt.Sprint(borrowed), func(t *testing.T) {
			closed := 0
			var tracked TrackingEvent
			level := featurevisor.LogLevelFatal
			options := Options{
				FeaturevisorOptions: featurevisor.FeaturevisorOptions{
					Datafile: testDatafile, LogLevel: &level,
					Modules: []*featurevisor.FeaturevisorModule{{Close: func() { closed++ }}},
				},
				OnTrack: func(event TrackingEvent) { tracked = event },
			}
			if borrowed {
				options.Featurevisor = featurevisor.CreateFeaturevisor(options.FeaturevisorOptions)
			}
			p := NewProvider(options)
			defer p.Featurevisor().Close()
			if err := of.SetProviderAndWait(p); err != nil {
				t.Fatal(err)
			}
			defer of.Shutdown()
			client := of.NewClient("refresh")
			ctx := context.Background()
			evaluationContext := of.NewEvaluationContext("forced-user", map[string]any{"country": "nl"})
			client.Track(ctx, "purchase", evaluationContext, of.NewTrackingEventDetails(12).Add("currency", "EUR"))
			if tracked.Name != "purchase" || tracked.Context.TargetingKey() != "forced-user" || tracked.Context.Attribute("country") != "nl" || tracked.Details.Value() != 12 || tracked.Details.Attribute("currency") != "EUR" {
				t.Fatalf("tracking: %#v", tracked)
			}
			if result, err := client.StringValueDetails(ctx, "checkout:count", "fallback", evaluationContext); err == nil || result.Value != "fallback" || result.ErrorCode != of.TypeMismatchCode {
				t.Fatalf("type mismatch default: %#v %v", result, err)
			}
			if result, err := client.BooleanValueDetails(ctx, "missing", true, evaluationContext); err == nil || !result.Value || result.ErrorCode != of.FlagNotFoundCode {
				t.Fatalf("missing default: %#v %v", result, err)
			}
			updated := strings.ReplaceAll(testDatafile, "support@example.com", "updated@example.com")
			if !p.Featurevisor().SetDatafile(updated, true) {
				t.Fatal("update rejected")
			}
			if value, err := client.StringValue(ctx, "variable:supportEmail", "fallback", evaluationContext); err != nil || value != "updated@example.com" {
				t.Fatalf("updated value: %s %v", value, err)
			}
			of.Shutdown()
			wantClosed := 1
			if borrowed {
				wantClosed = 0
			}
			if closed != wantClosed {
				t.Fatalf("close count: %d, want %d", closed, wantClosed)
			}
		})
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
