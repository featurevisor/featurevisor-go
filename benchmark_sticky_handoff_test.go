package featurevisor

import (
	"encoding/json"
	"fmt"
	"testing"
)

const stickyHandoffDatafile = `{"schemaVersion":"2","revision":"sticky-handoff","segments":{},"features":{
    "gate":{"bucketBy":"userId","traffic":[{"key":"all","segments":"*","percentage":100000,"enabled":true}]}
  },"variables":{
    "leaf":{"type":"boolean","defaultValue":true},
    "featureRoot":{"type":"boolean","defaultValue":true,"requiredFeatures":["gate","gate"]},
    "variableRoot":{"type":"boolean","defaultValue":true,"conditions":[
      {"variable":"leaf","operator":"equals","value":true},
      {"variable":"leaf","operator":"equals","value":true}
    ]}
  }}`

// BenchmarkStickyHandoff measures a real exported server snapshot passed to a
// primary or child instance. Run serially after builds and tests finish.
func BenchmarkStickyHandoff(b *testing.B) {
	for _, kind := range []string{"features", "variables"} {
		for _, size := range []int{0, 100, 1000, 5000} {
			for _, childMode := range []bool{false, true} {
				targetName := "instance"
				if childMode {
					targetName = "child"
				}
				b.Run(fmt.Sprintf("%s/size=%d/%s", kind, size, targetName), func(b *testing.B) {
					context := Context{"userId": "sticky-bench"}
					features := map[string]interface{}{}
					variables := map[string]interface{}{}
					keys := make([]string, size)
					for index := 0; index < size; index++ {
						key := fmt.Sprintf("unrelated%d", index)
						keys[index] = key
						if kind == "features" {
							features[key] = map[string]interface{}{"bucketBy": "userId", "traffic": []interface{}{
								map[string]interface{}{"key": "all", "segments": "*", "percentage": 100000, "enabled": true},
							}}
						} else {
							variables[key] = map[string]interface{}{"type": "boolean", "defaultValue": true}
						}
					}
					source, err := json.Marshal(map[string]interface{}{
						"schemaVersion": "2", "revision": "sticky-source", "segments": map[string]interface{}{},
						"features": features, "variables": variables,
					})
					if err != nil {
						b.Fatal(err)
					}
					level := LogLevelFatal
					server := CreateFeaturevisor(FeaturevisorOptions{Datafile: string(source), LogLevel: &level})
					stickyFeatures := server.GetFeatureEvaluations(context, keys, OverrideOptions{})
					stickyVariables := StickyVariables(server.GetVariableEvaluations(context, keys, OverrideOptions{}))
					server.Close()
					if kind == "features" && len(stickyFeatures) != size || kind == "variables" && len(stickyVariables) != size {
						b.Fatalf("exported sticky size differs from %d", size)
					}
					// root is added only after exporting, so sticky state cannot
					// bypass the gated global or its duplicate prerequisites.
					config := FeaturevisorOptions{Datafile: stickyHandoffDatafile, LogLevel: &level}
					if !childMode {
						if kind == "features" {
							config.StickyFeatures = &stickyFeatures
						} else {
							config.StickyVariables = &stickyVariables
						}
					}
					instance := CreateFeaturevisor(config)
					var target interface {
						GetGlobalVariable(string, ...interface{}) VariableValue
					}
					target = instance
					if childMode {
						options := SpawnOptions{}
						if kind == "features" {
							options.StickyFeatures = &stickyFeatures
						} else {
							options.StickyVariables = &stickyVariables
						}
						child := instance.Spawn(Context{}, options)
						defer child.Close()
						target = child
					}
					defer instance.Close()
					root := "variableRoot"
					if kind == "features" {
						root = "featureRoot"
					}
					if value := target.GetGlobalVariable(root, context); value != true {
						b.Fatalf("gated global = %#v", value)
					}
					b.ReportAllocs()
					b.ResetTimer()
					for index := 0; index < b.N; index++ {
						if value := target.GetGlobalVariable(root, context); value != true {
							b.Fatalf("gated global changed = %#v", value)
						}
					}
				})
			}
		}
	}
}
