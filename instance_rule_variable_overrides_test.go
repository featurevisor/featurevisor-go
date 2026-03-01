package featurevisor

import "testing"

func TestRuleVariableOverridesParity(t *testing.T) {
	jsonDatafile := `{
		"schemaVersion": "2",
		"revision": "1.0",
		"segments": {
			"germany": {
				"key": "germany",
				"conditions": "[{\"attribute\":\"country\",\"operator\":\"equals\",\"value\":\"de\"}]"
			},
			"mobile": {
				"key": "mobile",
				"conditions": "[{\"attribute\":\"device\",\"operator\":\"equals\",\"value\":\"mobile\"}]"
			}
		},
		"features": {
			"test": {
				"key": "test",
				"bucketBy": "userId",
				"variablesSchema": {
					"config": {
						"key": "config",
						"type": "object",
						"defaultValue": {"source":"default","nested":{"value":0}}
					},
					"banner": {
						"key": "banner",
						"type": "string",
						"defaultValue": "default-banner"
					}
				},
				"traffic": [
					{
						"key": "germany",
						"segments": "germany",
						"percentage": 100000,
						"variables": {
							"config": {"source":"rule","nested":{"value":10},"flag":true},
							"banner": "rule-banner"
						},
						"variableOverrides": {
							"config": [
								{
									"segments": "mobile",
									"value": {"source":"rule","nested":{"value":20},"flag":true}
								},
								{
									"conditions": "[{\"attribute\":\"country\",\"operator\":\"equals\",\"value\":\"de\"}]",
									"value": {"source":"rule","nested":{"value":30},"flag":true}
								}
							],
							"banner": [
								{
									"conditions": [
										{"attribute":"country","operator":"equals","value":"de"}
									],
									"value": "rule-banner-structured"
								}
							]
						},
						"allocation": []
					},
					{
						"key": "everyone",
						"segments": "*",
						"percentage": 100000,
						"variables": {
							"config": {"source":"everyone","nested":{"value":1}}
						},
						"allocation": []
					}
				]
			}
		}
	}`

	var datafile DatafileContent
	if err := datafile.FromJSON(jsonDatafile); err != nil {
		t.Fatalf("failed to parse datafile: %v", err)
	}

	sdk := CreateInstance(Options{Datafile: datafile})

	// first matching rule override by segments should win (index 0)
	evaluation := sdk.EvaluateVariable("test", "config", Context{
		"userId":  "user-1",
		"country": "de",
		"device":  "mobile",
	}, OverrideOptions{})
	if evaluation.Reason != EvaluationReasonVariableOverrideRule {
		t.Fatalf("expected reason %q, got %q", EvaluationReasonVariableOverrideRule, evaluation.Reason)
	}
	if evaluation.VariableOverrideIndex == nil || *evaluation.VariableOverrideIndex != 0 {
		t.Fatalf("expected variableOverrideIndex 0, got %#v", evaluation.VariableOverrideIndex)
	}

	config, ok := evaluation.VariableValue.(map[string]interface{})
	if !ok || config["source"] != "rule" {
		t.Fatalf("expected rule override config value, got %#v", evaluation.VariableValue)
	}
	nested, ok := config["nested"].(map[string]interface{})
	if !ok || nested["value"] != float64(20) {
		t.Fatalf("expected nested.value 20 from first override, got %#v", config)
	}

	// stringified conditions override should match as second entry (index 1)
	evaluation = sdk.EvaluateVariable("test", "config", Context{
		"userId":  "user-1",
		"country": "de",
	}, OverrideOptions{})
	if evaluation.Reason != EvaluationReasonVariableOverrideRule {
		t.Fatalf("expected reason %q, got %q", EvaluationReasonVariableOverrideRule, evaluation.Reason)
	}
	if evaluation.VariableOverrideIndex == nil || *evaluation.VariableOverrideIndex != 1 {
		t.Fatalf("expected variableOverrideIndex 1, got %#v", evaluation.VariableOverrideIndex)
	}

	config, ok = evaluation.VariableValue.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map config, got %#v", evaluation.VariableValue)
	}
	nested, ok = config["nested"].(map[string]interface{})
	if !ok || nested["value"] != float64(30) {
		t.Fatalf("expected nested.value 30 from second override, got %#v", config)
	}

	// structured conditions override should match for banner
	evaluation = sdk.EvaluateVariable("test", "banner", Context{
		"userId":  "user-1",
		"country": "de",
	}, OverrideOptions{})
	if evaluation.Reason != EvaluationReasonVariableOverrideRule {
		t.Fatalf("expected reason %q, got %q", EvaluationReasonVariableOverrideRule, evaluation.Reason)
	}
	if evaluation.VariableOverrideIndex == nil || *evaluation.VariableOverrideIndex != 0 {
		t.Fatalf("expected variableOverrideIndex 0 for banner, got %#v", evaluation.VariableOverrideIndex)
	}
	if evaluation.VariableValue != "rule-banner-structured" {
		t.Fatalf("expected banner override value, got %#v", evaluation.VariableValue)
	}

	// no rule override match should fall back to matched rule variables
	evaluation = sdk.EvaluateVariable("test", "config", Context{
		"userId":  "user-1",
		"country": "nl",
	}, OverrideOptions{})
	if evaluation.Reason != EvaluationReasonRule {
		t.Fatalf("expected rule reason fallback, got %q", evaluation.Reason)
	}
	config, ok = evaluation.VariableValue.(map[string]interface{})
	if !ok || config["source"] != "everyone" {
		t.Fatalf("expected fallback to everyone rule variable, got %#v", evaluation.VariableValue)
	}
}

func TestVariationVariableOverrideReasonSplit(t *testing.T) {
	jsonDatafile := `{
		"schemaVersion": "2",
		"revision": "1.0",
		"segments": {
			"germany": {
				"key": "germany",
				"conditions": "[{\"attribute\":\"country\",\"operator\":\"equals\",\"value\":\"de\"}]"
			}
		},
		"features": {
			"test": {
				"key": "test",
				"bucketBy": "userId",
				"variablesSchema": {
					"color": {
						"key": "color",
						"type": "string",
						"defaultValue": "default-color"
					}
				},
				"variations": [
					{"value":"control"},
					{
						"value":"treatment",
						"variables": {"color":"blue"},
						"variableOverrides": {
							"color": [
								{"segments":"germany","value":"yellow"}
							]
						}
					}
				],
				"traffic": [
					{
						"key":"everyone",
						"segments":"*",
						"percentage":100000,
						"allocation":[
							{"variation":"treatment","range":[0,100000]}
						]
					}
				]
			}
		}
	}`

	var datafile DatafileContent
	if err := datafile.FromJSON(jsonDatafile); err != nil {
		t.Fatalf("failed to parse datafile: %v", err)
	}

	sdk := CreateInstance(Options{Datafile: datafile})
	evaluation := sdk.EvaluateVariable("test", "color", Context{
		"userId":  "user-1",
		"country": "de",
	}, OverrideOptions{})
	if evaluation.Reason != EvaluationReasonVariableOverrideVariation {
		t.Fatalf("expected reason %q, got %q", EvaluationReasonVariableOverrideVariation, evaluation.Reason)
	}
	if evaluation.VariableOverrideIndex == nil || *evaluation.VariableOverrideIndex != 0 {
		t.Fatalf("expected variation variableOverrideIndex 0, got %#v", evaluation.VariableOverrideIndex)
	}
	if evaluation.VariableValue != "yellow" {
		t.Fatalf("expected yellow from variation override, got %#v", evaluation.VariableValue)
	}
}
