package commands

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/featurevisor/featurevisor-go/sdk"
)

// BenchmarkOutput represents the result of a benchmark operation
type BenchmarkOutput struct {
	Value    interface{}
	Duration time.Duration
}

// benchmarkFeatureFlag benchmarks the feature flag evaluation
func benchmarkFeatureFlag(
	instance *sdk.Featurevisor,
	featureKey string,
	context sdk.Context,
	n int,
) BenchmarkOutput {
	start := time.Now()
	var value interface{}

	for i := 0; i < n; i++ {
		value = instance.IsEnabled(featureKey, context, sdk.OverrideOptions{})
	}

	duration := time.Since(start)

	return BenchmarkOutput{
		Value:    value,
		Duration: duration,
	}
}

// benchmarkFeatureVariation benchmarks the feature variation evaluation
func benchmarkFeatureVariation(
	instance *sdk.Featurevisor,
	featureKey string,
	context sdk.Context,
	n int,
) BenchmarkOutput {
	start := time.Now()
	var value interface{}

	for i := 0; i < n; i++ {
		value = instance.GetVariation(featureKey, context, sdk.OverrideOptions{})
	}

	duration := time.Since(start)

	return BenchmarkOutput{
		Value:    value,
		Duration: duration,
	}
}

// benchmarkFeatureVariable benchmarks the feature variable evaluation
func benchmarkFeatureVariable(
	instance *sdk.Featurevisor,
	featureKey string,
	variableKey string,
	context sdk.Context,
	n int,
) BenchmarkOutput {
	start := time.Now()
	var value interface{}

	for i := 0; i < n; i++ {
		value = instance.GetVariable(featureKey, variableKey, context, sdk.OverrideOptions{})
	}

	duration := time.Since(start)

	return BenchmarkOutput{
		Value:    value,
		Duration: duration,
	}
}

// prettyDuration formats duration in a human-readable format matching TypeScript implementation
func prettyDuration(duration time.Duration) string {
	duration = duration.Abs()

	if duration == 0 {
		return "0ms"
	}

	// Convert to milliseconds for consistency with TypeScript
	ms := duration.Milliseconds()
	remaining := duration - time.Duration(ms)*time.Millisecond

	// Handle sub-millisecond precision
	if ms == 0 && remaining > 0 {
		return fmt.Sprintf("%dμs", remaining.Microseconds())
	}

	// Format like TypeScript: hours, minutes, seconds, milliseconds
	var result strings.Builder

	hours := ms / 3600000
	ms = ms % 3600000
	minutes := ms / 60000
	ms = ms % 60000
	seconds := ms / 1000
	ms = ms % 1000

	if hours > 0 {
		result.WriteString(fmt.Sprintf(" %dh", hours))
	}
	if minutes > 0 {
		result.WriteString(fmt.Sprintf(" %dm", minutes))
	}
	if seconds > 0 {
		result.WriteString(fmt.Sprintf(" %ds", seconds))
	}
	if ms > 0 {
		result.WriteString(fmt.Sprintf(" %dms", ms))
	}

	return strings.TrimSpace(result.String())
}

// runBenchmark runs the benchmark command
func runBenchmark(opts CLIOptions) {
	featurevisorProjectPath := opts.ProjectDirectoryPath

	if opts.Environment == "" {
		fmt.Println("Environment is required")
		return
	}

	if opts.Feature == "" {
		fmt.Println("Feature is required")
		return
	}

	var context sdk.Context
	if opts.Context != "" {
		json.Unmarshal([]byte(opts.Context), &context)
	} else {
		context = make(sdk.Context)
	}

	levelStr := getLoggerLevel(opts)
	level := sdk.LogLevel(levelStr)

	fmt.Println("")
	fmt.Printf("Running benchmark for feature \"%s\"...\n", opts.Feature)
	fmt.Println("")

	fmt.Printf("Building datafile containing all features for \"%s\"...\n", opts.Environment)
	datafileBuildStart := time.Now()
	datafilesByEnvironment := buildDatafiles(featurevisorProjectPath, []string{opts.Environment}, "", 0)
	datafileBuildDuration := time.Since(datafileBuildStart)
	// Convert to milliseconds to match TypeScript behavior
	datafileBuildDurationMs := datafileBuildDuration.Milliseconds()
	fmt.Printf("Datafile build duration: %dms\n", datafileBuildDurationMs)

	// Create SDK instance
	datafile := datafilesByEnvironment[opts.Environment]

	// Convert datafile to proper format
	var datafileContent sdk.DatafileContent
	var datafileBytes []byte
	var err error
	if datafileBytes, err = json.Marshal(datafile); err == nil {
		json.Unmarshal(datafileBytes, &datafileContent)
	}

	// Calculate datafile size
	datafileSize := len(datafileBytes)
	fmt.Printf("Datafile size: %.2f kB\n", float64(datafileSize)/1024.0)

	instance := sdk.CreateInstance(sdk.InstanceOptions{
		Datafile: datafileContent,
		LogLevel: &level,
	})
	fmt.Println("...SDK initialized")

	fmt.Println("")
	// Format context to match TypeScript JSON.stringify behavior
	contextJSON, _ := json.Marshal(context)
	fmt.Printf("Against context: %s\n", string(contextJSON))

	var output BenchmarkOutput
	if opts.Variation {
		// variation
		fmt.Printf("Evaluating variation %d times...\n", opts.N)
		output = benchmarkFeatureVariation(instance, opts.Feature, context, opts.N)
	} else if opts.Variable != "" {
		// variable
		fmt.Printf("Evaluating variable \"%s\" %d times...\n", opts.Variable, opts.N)
		output = benchmarkFeatureVariable(instance, opts.Feature, opts.Variable, context, opts.N)
	} else {
		// flag
		fmt.Printf("Evaluating flag %d times...\n", opts.N)
		output = benchmarkFeatureFlag(instance, opts.Feature, context, opts.N)
	}

	fmt.Println("")

	// Format the value output to match TypeScript behavior
	var valueOutput string
	if output.Value == nil {
		valueOutput = "null"
	} else {
		if valueBytes, err := json.Marshal(output.Value); err == nil {
			valueOutput = string(valueBytes)
		} else {
			valueOutput = fmt.Sprintf("%v", output.Value)
		}
	}

	fmt.Printf("Evaluated value : %s\n", valueOutput)
	fmt.Printf("Total duration  : %s\n", prettyDuration(output.Duration))
	fmt.Printf("Average duration: %s\n", prettyDuration(output.Duration/time.Duration(opts.N)))
}
