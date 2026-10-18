package featurevisor

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"sync"
	"testing"
)

type statefulFixture struct {
	Kind          string         `json:"kind"`
	SchemaVersion int            `json:"schemaVersion"`
	Cases         []statefulCase `json:"cases"`
}

type statefulCase struct {
	Name    string `json:"name"`
	Initial struct {
		Datafile        json.RawMessage  `json:"datafile"`
		Context         Context          `json:"context"`
		StickyFeatures  *StickyFeatures  `json:"stickyFeatures"`
		StickyVariables *StickyVariables `json:"stickyVariables"`
	} `json:"initial"`
	Operations []statefulOperation `json:"operations"`
}

type statefulOperation struct {
	Action          string               `json:"action"`
	Target          string               `json:"target"`
	Parent          string               `json:"parent"`
	Context         Context              `json:"context"`
	Options         statefulSpawnOptions `json:"options"`
	Datafile        json.RawMessage      `json:"datafile"`
	StickyFeatures  StickyFeatures       `json:"stickyFeatures"`
	StickyVariables StickyVariables      `json:"stickyVariables"`
	Replace         bool                 `json:"replace"`
	Expect          statefulExpectation  `json:"expect"`
}

type statefulSpawnOptions struct {
	StickyFeatures  *StickyFeatures  `json:"stickyFeatures"`
	StickyVariables *StickyVariables `json:"stickyVariables"`
}

type statefulExpectation struct {
	Error   *statefulError   `json:"error"`
	Events  []statefulEvent  `json:"events"`
	Results []statefulResult `json:"results"`
}

type statefulError struct {
	Code string `json:"code"`
}

type statefulEvent struct {
	Target  string                 `json:"target"`
	Name    string                 `json:"name"`
	Details map[string]interface{} `json:"details"`
}

type statefulResult struct {
	Target string        `json:"target"`
	Method string        `json:"method"`
	Args   []interface{} `json:"args"`
	Value  interface{}   `json:"value"`
}

type statefulTarget struct {
	root  *Featurevisor
	child *FeaturevisorChild
}

func loadStatefulFixture(t *testing.T) statefulFixture {
	t.Helper()
	content, err := os.ReadFile("conformance/sdk-stateful.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture statefulFixture
	if err := json.Unmarshal(content, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Kind != "featurevisor.sdk-stateful" || fixture.SchemaVersion != 1 {
		t.Fatalf("unsupported stateful fixture %q version %d", fixture.Kind, fixture.SchemaVersion)
	}
	if len(fixture.Cases) != 8 {
		t.Fatalf("expected eight stateful cases, got %d", len(fixture.Cases))
	}
	return fixture
}

func statefulDatafileInput(t *testing.T, raw json.RawMessage) interface{} {
	t.Helper()
	var text string
	if len(raw) > 0 && raw[0] == '"' {
		if err := json.Unmarshal(raw, &text); err != nil {
			t.Fatal(err)
		}
		return text
	}
	return string(raw)
}

func normalizeStatefulDetails(details map[string]interface{}) map[string]interface{} {
	normalized := make(map[string]interface{}, len(details))
	for key, value := range details {
		if key != "features" && key != "variables" {
			normalized[key] = value
			continue
		}
		var keys []string
		switch values := value.(type) {
		case []string:
			keys = append(keys, values...)
		case []interface{}:
			for _, item := range values {
				if item, ok := item.(string); ok {
					keys = append(keys, item)
				}
			}
		default:
			normalized[key] = value
			continue
		}
		sort.Strings(keys)
		normalized[key] = keys
	}
	return normalized
}

func recordStatefulEvent(events *[]statefulEvent, target string, name EventName, details EventDetails) {
	payload := make(map[string]interface{}, len(details))
	for key, value := range details {
		payload[key] = value
	}
	*events = append(*events, statefulEvent{
		Target:  target,
		Name:    string(name),
		Details: normalizeStatefulDetails(payload),
	})
}

func subscribeStatefulRoot(root *Featurevisor, target string, events *[]statefulEvent) {
	for _, name := range []EventName{
		EventNameDatafileSet,
		EventNameContextSet,
		EventNameStickyFeaturesSet,
		EventNameStickyVariablesSet,
	} {
		eventName := name
		root.On(eventName, func(details EventDetails) {
			recordStatefulEvent(events, target, eventName, details)
		})
	}
}

func subscribeStatefulChild(child *FeaturevisorChild, target string, events *[]statefulEvent) {
	for _, name := range []EventName{
		EventNameDatafileSet,
		EventNameContextSet,
		EventNameStickyFeaturesSet,
		EventNameStickyVariablesSet,
	} {
		eventName := name
		child.On(eventName, func(details EventDetails) {
			recordStatefulEvent(events, target, eventName, details)
		})
	}
}

func statefulStringArg(t *testing.T, result statefulResult, index int) string {
	t.Helper()
	if index >= len(result.Args) {
		t.Fatalf("%s requires argument %d", result.Method, index)
	}
	value, ok := result.Args[index].(string)
	if !ok {
		t.Fatalf("%s argument %d must be a string", result.Method, index)
	}
	return value
}

func evaluateStatefulResult(t *testing.T, target statefulTarget, result statefulResult) interface{} {
	t.Helper()
	switch result.Method {
	case "getRevision":
		if target.root == nil {
			t.Fatal("getRevision requires a root target")
		}
		return target.root.GetRevision()
	case "isEnabled":
		feature := statefulStringArg(t, result, 0)
		context := statefulContextArg(t, result, 1)
		if target.root != nil {
			return target.root.IsEnabled(feature, context)
		}
		return target.child.IsEnabled(feature, context)
	case "getVariable":
		featureVariable := false
		if len(result.Args) > 1 {
			_, featureVariable = result.Args[1].(string)
		}
		if !featureVariable {
			variable := statefulStringArg(t, result, 0)
			context := statefulContextArg(t, result, 1)
			if target.root != nil {
				return target.root.GetGlobalVariable(variable, context)
			}
			return target.child.GetGlobalVariable(variable, context)
		}
		feature := statefulStringArg(t, result, 0)
		variable := statefulStringArg(t, result, 1)
		context := statefulContextArg(t, result, 2)
		if target.root != nil {
			return target.root.GetVariable(feature, variable, context)
		}
		return target.child.GetVariable(feature, variable, context)
	default:
		t.Fatalf("unknown stateful result method %q", result.Method)
	}
	return nil
}

func statefulContextArg(t *testing.T, result statefulResult, index int) Context {
	t.Helper()
	if len(result.Args) <= index {
		return nil
	}
	if len(result.Args) != index+1 {
		t.Fatalf("%s has unexpected extra arguments", result.Method)
	}
	if result.Args[index] == nil {
		return nil
	}
	context, ok := result.Args[index].(map[string]interface{})
	if !ok {
		t.Fatalf("%s argument %d must be a context object", result.Method, index)
	}
	return Context(context)
}

func normalizeStatefulValue(t *testing.T, value interface{}) interface{} {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var normalized interface{}
	if err := json.Unmarshal(encoded, &normalized); err != nil {
		t.Fatal(err)
	}
	return normalized
}

func TestStatefulSDKConformance(t *testing.T) {
	fixture := loadStatefulFixture(t)
	for _, testCase := range fixture.Cases {
		t.Run(testCase.Name, func(t *testing.T) {
			var diagnosticsMu sync.Mutex
			diagnosticCodes := []string{}
			root := CreateFeaturevisor(FeaturevisorOptions{
				Datafile:        statefulDatafileInput(t, testCase.Initial.Datafile),
				Context:         testCase.Initial.Context,
				StickyFeatures:  testCase.Initial.StickyFeatures,
				StickyVariables: testCase.Initial.StickyVariables,
				OnDiagnostic: func(diagnostic FeaturevisorDiagnostic) {
					if diagnostic.Level == LogLevelError {
						diagnosticsMu.Lock()
						diagnosticCodes = append(diagnosticCodes, diagnostic.Code)
						diagnosticsMu.Unlock()
					}
				},
			})
			defer root.Close()
			targets := map[string]statefulTarget{"root": {root: root}}
			events := []statefulEvent{}
			subscribeStatefulRoot(root, "root", &events)

			for _, operation := range testCase.Operations {
				switch operation.Action {
				case "spawn":
					parentName := operation.Parent
					if parentName == "" {
						parentName = "root"
					}
					parent, ok := targets[parentName]
					if !ok || parent.root == nil {
						t.Fatalf("spawn parent %q is not a root", parentName)
					}
					child := parent.root.Spawn(operation.Context, SpawnOptions{
						StickyFeatures:  operation.Options.StickyFeatures,
						StickyVariables: operation.Options.StickyVariables,
					})
					defer child.Close()
					targets[operation.Target] = statefulTarget{child: child}
					subscribeStatefulChild(child, operation.Target, &events)
				case "setDatafile":
					target, ok := targets[operation.Target]
					if !ok || target.root == nil {
						t.Fatalf("setDatafile target %q is not a root", operation.Target)
					}
					target.root.SetDatafile(statefulDatafileInput(t, operation.Datafile), operation.Replace)
				case "setStickyFeatures":
					target, ok := targets[operation.Target]
					if !ok {
						t.Fatalf("unknown sticky feature target %q", operation.Target)
					}
					if target.root != nil {
						target.root.SetStickyFeatures(operation.StickyFeatures, operation.Replace)
					} else {
						target.child.SetStickyFeatures(operation.StickyFeatures, operation.Replace)
					}
				case "setStickyVariables":
					target, ok := targets[operation.Target]
					if !ok {
						t.Fatalf("unknown sticky variable target %q", operation.Target)
					}
					if target.root != nil {
						target.root.SetStickyVariables(operation.StickyVariables, operation.Replace)
					} else {
						target.child.SetStickyVariables(operation.StickyVariables, operation.Replace)
					}
				default:
					t.Fatalf("unknown stateful action %q", operation.Action)
				}

				expectedEvents := make([]statefulEvent, len(operation.Expect.Events))
				copy(expectedEvents, operation.Expect.Events)
				for index := range expectedEvents {
					expectedEvents[index].Details = normalizeStatefulDetails(expectedEvents[index].Details)
				}
				if !reflect.DeepEqual(events, expectedEvents) {
					t.Fatalf("events mismatch\nactual: %#v\nexpected: %#v", events, expectedEvents)
				}
				events = events[:0]

				diagnosticsMu.Lock()
				actualCodes := append([]string(nil), diagnosticCodes...)
				diagnosticCodes = nil
				diagnosticsMu.Unlock()
				var expectedCodes []string
				if operation.Expect.Error != nil {
					expectedCodes = append(expectedCodes, operation.Expect.Error.Code)
				}
				if !reflect.DeepEqual(actualCodes, expectedCodes) {
					t.Fatalf("diagnostic codes mismatch: got %v, want %v", actualCodes, expectedCodes)
				}

				for _, result := range operation.Expect.Results {
					target, ok := targets[result.Target]
					if !ok {
						t.Fatalf("unknown result target %q", result.Target)
					}
					actual := normalizeStatefulValue(t, evaluateStatefulResult(t, target, result))
					if !reflect.DeepEqual(actual, result.Value) {
						t.Fatalf("%s result mismatch: got %#v, want %#v", result.Method, actual, result.Value)
					}
				}
			}
		})
	}
}
