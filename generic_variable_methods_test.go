package featurevisor

import "testing"

type typedConfig struct {
	Theme string `json:"theme"`
}

type nestedPalette struct {
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

type nestedLayout struct {
	Columns int            `json:"columns"`
	Meta    map[string]any `json:"meta"`
}

type complexConfig struct {
	Theme     string          `json:"theme"`
	Layout    nestedLayout    `json:"layout"`
	Palettes  []nestedPalette `json:"palettes"`
	Flags     []string        `json:"flags"`
	Threshold float64         `json:"threshold"`
}

type rolloutStep struct {
	Name       string  `json:"name"`
	Percentage float64 `json:"percentage"`
}

func TestGenericVariableMethods(t *testing.T) {
	jsonDatafile := `{
		"schemaVersion": "2",
		"revision": "1.0",
		"segments": {},
		"features": {
			"typed": {
				"key": "typed",
				"bucketBy": "userId",
				"variablesSchema": {
					"items": {
						"key": "items",
						"type": "array",
						"defaultValue": ["a", "b", "c"]
					},
					"config": {
						"key": "config",
						"type": "object",
						"defaultValue": {"theme": "dark"}
					},
					"complexConfig": {
						"key": "complexConfig",
						"type": "object",
						"defaultValue": {
							"theme": "light",
							"layout": {
								"columns": 3,
								"meta": {
									"region": "eu",
									"version": 2
								}
							},
							"palettes": [
								{"name": "primary", "active": true},
								{"name": "secondary", "active": false}
							],
							"flags": ["beta", "edge"],
							"threshold": 0.85
						}
					},
					"rolloutPlan": {
						"key": "rolloutPlan",
						"type": "array",
						"defaultValue": [
							{"name": "phase-1", "percentage": 10},
							{"name": "phase-2", "percentage": 55.5},
							{"name": "phase-3", "percentage": 100}
						]
					}
				},
				"traffic": [
					{
						"key": "all",
						"segments": "*",
						"percentage": 100000,
						"enabled": true,
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
	context := Context{"userId": "123"}

	var arr []string
	if err := sdk.GetVariableArrayInto("typed", "items", context, &arr); err != nil {
		t.Fatalf("expected array decode to succeed, got error: %v", err)
	}
	if len(arr) != 3 || arr[0] != "a" || arr[2] != "c" {
		t.Fatalf("expected typed array values, got %#v", arr)
	}

	var config typedConfig
	if err := sdk.GetVariableObjectInto("typed", "config", context, &config); err != nil {
		t.Fatalf("expected object decode to succeed, got error: %v", err)
	}
	if config.Theme != "dark" {
		t.Fatalf("expected typed object with theme=dark, got %#v", config)
	}

	var complex complexConfig
	if err := sdk.GetVariableObjectInto("typed", "complexConfig", context, OverrideOptions{}, &complex); err != nil {
		t.Fatalf("expected complex object decode to succeed, got error: %v", err)
	}
	if complex.Theme != "light" || complex.Layout.Columns != 3 || complex.Layout.Meta["region"] != "eu" {
		t.Fatalf("unexpected complex config core fields: %#v", complex)
	}
	if len(complex.Palettes) != 2 || complex.Palettes[0].Name != "primary" || !complex.Palettes[0].Active {
		t.Fatalf("unexpected complex config palettes: %#v", complex.Palettes)
	}
	if len(complex.Flags) != 2 || complex.Flags[1] != "edge" || complex.Threshold != 0.85 {
		t.Fatalf("unexpected complex config flags/threshold: %#v", complex)
	}

	var rollout []rolloutStep
	if err := sdk.GetVariableArrayInto("typed", "rolloutPlan", context, OverrideOptions{}, &rollout); err != nil {
		t.Fatalf("expected rollout decode to succeed, got error: %v", err)
	}
	if len(rollout) != 3 {
		t.Fatalf("expected 3 rollout entries, got %#v", rollout)
	}
	if rollout[0].Name != "phase-1" || rollout[1].Percentage != 55.5 || rollout[2].Percentage != 100 {
		t.Fatalf("unexpected rollout values: %#v", rollout)
	}

	child := sdk.Spawn(Context{"country": "nl"})
	var childArr []string
	if err := child.GetVariableArrayInto("typed", "items", &childArr); err != nil {
		t.Fatalf("expected child array decode to succeed, got error: %v", err)
	}
	if len(childArr) != 3 || childArr[1] != "b" {
		t.Fatalf("expected child typed array values, got %#v", childArr)
	}

	var childConfig typedConfig
	if err := child.GetVariableObjectInto("typed", "config", &childConfig); err != nil {
		t.Fatalf("expected child object decode to succeed, got error: %v", err)
	}
	if childConfig.Theme != "dark" {
		t.Fatalf("expected child typed object with theme=dark, got %#v", childConfig)
	}

	var childComplex complexConfig
	if err := child.GetVariableObjectInto("typed", "complexConfig", Context{}, &childComplex); err != nil {
		t.Fatalf("expected child complex object decode to succeed, got error: %v", err)
	}
	if childComplex.Layout.Meta["region"] != "eu" || childComplex.Palettes[1].Name != "secondary" {
		t.Fatalf("unexpected child complex config: %#v", childComplex)
	}

	var childRollout []rolloutStep
	if err := child.GetVariableArrayInto("typed", "rolloutPlan", Context{}, &childRollout); err != nil {
		t.Fatalf("expected child rollout decode to succeed, got error: %v", err)
	}
	if len(childRollout) != 3 || childRollout[2].Name != "phase-3" {
		t.Fatalf("unexpected child rollout values: %#v", childRollout)
	}
}
