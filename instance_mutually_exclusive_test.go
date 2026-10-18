package featurevisor

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestLegacyFeatureRangesAreIgnored(t *testing.T) {
	for _, ranges := range []string{`[[0, 10000]]`, `[[0, 100000]]`, `[]`, `null`, `{"removed": true}`, `"ignored"`} {
		for _, percentage := range []int{0, 25000, 50000, 100000} {
			t.Run(fmt.Sprintf("%s/%d", ranges, percentage), func(t *testing.T) {
				var datafile DatafileContent
				err := datafile.FromJSON(fmt.Sprintf(`{
					"schemaVersion": "2", "revision": "legacy", "segments": {},
					"features": {"experiment": {
						"bucketBy": "userId", "ranges": %s,
						"variations": [{"value": "a"}, {"value": "b"}],
						"traffic": [{"key": "all", "segments": "*", "percentage": %d,
							"allocation": [{"variation": "a", "range": [0, 50000]},
								{"variation": "b", "range": [50000, 100000]}]}]
					}}
				}`, ranges, percentage))
				if err != nil {
					t.Fatal(err)
				}
				encoded, err := json.Marshal(datafile.Features["experiment"])
				if err != nil {
					t.Fatal(err)
				}
				var feature map[string]interface{}
				if err := json.Unmarshal(encoded, &feature); err != nil {
					t.Fatal(err)
				}
				if _, exists := feature["ranges"]; exists {
					t.Fatal("Legacy ranges must not be retained in the datafile model")
				}
				bucketValue := 40000
				logLevel := LogLevelWarn
				sdk := CreateFeaturevisor(FeaturevisorOptions{
					Datafile: datafile,
					LogLevel: &logLevel,
					OnDiagnostic: func(diagnostic FeaturevisorDiagnostic) {
						t.Errorf("Unexpected warning or error for ignored ranges: %+v", diagnostic)
					},
					Modules: []*FeaturevisorModule{{Name: "bucket", BucketValue: func(ConfigureBucketValueOptions) int { return bucketValue }}},
				})
				defer sdk.Close()
				for _, bucket := range []int{10000, 40000, 60000} {
					bucketValue = bucket
					enabled := percentage > 0 && bucket <= percentage
					evaluation := sdk.EvaluateFlag("experiment", Context{"userId": "123"})
					if evaluation.Enabled == nil || *evaluation.Enabled != enabled {
						t.Fatalf("bucket %d: expected enabled %v, got %+v", bucket, enabled, evaluation)
					}
					reason := EvaluationReasonRule
					if !enabled && percentage != 0 {
						reason = EvaluationReasonNoMatch
					}
					if evaluation.Reason != reason {
						t.Fatalf("bucket %d: expected reason %s, got %s", bucket, reason, evaluation.Reason)
					}
					variation := sdk.GetVariation("experiment", Context{"userId": "123"})
					if !enabled {
						if variation != nil {
							t.Fatalf("Disabled experiment returned variation %s", *variation)
						}
						continue
					}
					want := "a"
					if bucket >= 50000 {
						want = "b"
					}
					if variation == nil || *variation != want {
						t.Fatalf("bucket %d: expected ordinary allocation %s, got %v", bucket, want, variation)
					}
				}
			})
		}
	}
}

func TestRequiredFeaturesMutualExclusionWithNormalHashing(t *testing.T) {
	var datafile DatafileContent
	if err := datafile.FromJSON(`{
		"schemaVersion": "2", "revision": "requirements", "segments": {},
		"features": {
			"allocation": {"bucketBy": "userId", "variations": [{"value": "a"}, {"value": "b"}, {"value": "holdout"}],
				"traffic": [{"key": "all", "segments": "*", "percentage": 100000,
					"allocation": [{"variation": "a", "range": [0, 40000]}, {"variation": "b", "range": [40000, 80000]}, {"variation": "holdout", "range": [80000, 100000]}]}]},
			"experimentA": {"bucketBy": "userId", "requiredFeatures": [{"feature": "allocation", "variation": "a"}],
				"traffic": [{"key": "all", "segments": "*", "percentage": 100000}]},
			"experimentB": {"bucketBy": "userId", "requiredFeatures": [{"feature": "allocation", "variation": "b"}],
				"traffic": [{"key": "all", "segments": "*", "percentage": 100000}]},
			"primary": {"bucketBy": "userId", "traffic": [{"key": "all", "segments": "*", "percentage": 50000}]},
			"secondary": {"bucketBy": "userId", "requiredFeatures": [{"feature": "primary", "enabled": false}],
				"traffic": [{"key": "all", "segments": "*", "percentage": 100000}]}
		}
	}`); err != nil {
		t.Fatal(err)
	}
	sdk := CreateFeaturevisor(FeaturevisorOptions{Datafile: datafile})
	defer sdk.Close()
	counts := map[string]int{}
	for user := 0; user < 20000; user++ {
		context := Context{"userId": fmt.Sprintf("user-%d", user)}
		a, b := sdk.IsEnabled("experimentA", context), sdk.IsEnabled("experimentB", context)
		primary, secondary := sdk.IsEnabled("primary", context), sdk.IsEnabled("secondary", context)
		if (a && b) || primary == secondary {
			t.Fatalf("user %d: shared allocation or priority exclusion failed", user)
		}
		variation := sdk.GetVariation("allocation", context)
		if variation == nil || a != (*variation == "a") || b != (*variation == "b") {
			t.Fatalf("user %d: dependents must follow the shared variation", user)
		}
		if !a && !b && *variation != "holdout" {
			t.Fatalf("user %d: only the holdout may disable both experiments", user)
		}
		bReversed := sdk.IsEnabled("experimentB", context)
		aReversed := sdk.IsEnabled("experimentA", context)
		secondaryReversed := sdk.IsEnabled("secondary", context)
		primaryReversed := sdk.IsEnabled("primary", context)
		if bReversed != b || aReversed != a || secondaryReversed != secondary || primaryReversed != primary {
			t.Fatalf("user %d: reversing evaluation order changed allocation", user)
		}
		if sdk.EvaluateVariation("allocation", context).Reason != EvaluationReasonAllocated {
			t.Fatal("Shared variation must use ordinary allocation")
		}
		counts[*variation]++
		if primary {
			counts["primary"]++
		}
	}
	for _, key := range []string{"a", "b"} {
		if counts[key] < 7000 || counts[key] > 9000 {
			t.Errorf("Expected roughly 40 percent of the population for %s, got %d", key, counts[key])
		}
	}
	if counts["holdout"] < 3000 || counts["holdout"] > 5000 {
		t.Errorf("Expected roughly 20 percent holdout, got %d", counts["holdout"])
	}
	if counts["primary"] < 9000 || counts["primary"] > 11000 {
		t.Errorf("Expected roughly half the population for primary, got %d", counts["primary"])
	}
	t.Logf("20,000 users: allocation=%v; zero overlap in both evaluation orders", counts)
}
