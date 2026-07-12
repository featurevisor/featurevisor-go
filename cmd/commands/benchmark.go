package commands

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	featurevisor "github.com/featurevisor/featurevisor-go"
)

// BenchmarkOutput represents the result of a benchmark operation
type BenchmarkOutput struct {
	Value           interface{}
	Duration        time.Duration
	MinDuration     time.Duration
	AverageDuration time.Duration
	MaxDuration     time.Duration
}

func benchmarkEvaluation(n int, evaluate func() interface{}) BenchmarkOutput {
	var value interface{}
	var totalDuration time.Duration
	var minDuration time.Duration
	var maxDuration time.Duration

	for i := 0; i < n; i++ {
		start := time.Now()
		value = evaluate()
		duration := time.Since(start)

		totalDuration += duration
		if i == 0 || duration < minDuration {
			minDuration = duration
		}
		if duration > maxDuration {
			maxDuration = duration
		}
	}

	return BenchmarkOutput{
		Value:           value,
		Duration:        totalDuration,
		MinDuration:     minDuration,
		AverageDuration: totalDuration / time.Duration(n),
		MaxDuration:     maxDuration,
	}
}

// benchmarkFeatureFlag benchmarks the feature flag evaluation
func benchmarkFeatureFlag(
	instance *featurevisor.Featurevisor,
	featureKey string,
	context featurevisor.Context,
	n int,
) BenchmarkOutput {
	return benchmarkEvaluation(n, func() interface{} {
		return instance.IsEnabled(featureKey, context, featurevisor.OverrideOptions{})
	})
}

// benchmarkFeatureVariation benchmarks the feature variation evaluation
func benchmarkFeatureVariation(
	instance *featurevisor.Featurevisor,
	featureKey string,
	context featurevisor.Context,
	n int,
) BenchmarkOutput {
	return benchmarkEvaluation(n, func() interface{} {
		return instance.GetVariation(featureKey, context, featurevisor.OverrideOptions{})
	})
}

// benchmarkFeatureVariable benchmarks the feature variable evaluation
func benchmarkFeatureVariable(
	instance *featurevisor.Featurevisor,
	featureKey string,
	variableKey string,
	context featurevisor.Context,
	n int,
) BenchmarkOutput {
	return benchmarkEvaluation(n, func() interface{} {
		return instance.GetVariable(featureKey, variableKey, context, featurevisor.OverrideOptions{})
	})
}

func formatDurationMs(duration time.Duration) string {
	return fmt.Sprintf("%.6fms", float64(duration.Nanoseconds())/1_000_000.0)
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

	if len(opts.Targets) > 1 {
		for _, target := range opts.Targets {
			selected := opts
			selected.Targets = []string{target}
			runBenchmark(selected)
		}
		return
	}

	var context featurevisor.Context
	if opts.Context != "" {
		json.Unmarshal([]byte(opts.Context), &context)
	} else {
		context = make(featurevisor.Context)
	}

	levelStr := getLoggerLevel(opts)
	level := featurevisor.LogLevel(levelStr)

	fmt.Println("")
	fmt.Printf("Running benchmark for feature \"%s\"...\n", opts.Feature)
	fmt.Println("")

	datafileBuildStart := time.Now()
	var target *string
	if len(opts.Targets) == 1 {
		target = &opts.Targets[0]
	}
	datafile := buildDatafileJSON(featurevisorProjectPath, &opts.Environment, opts.Inflate, target)
	datafileBuildDuration := time.Since(datafileBuildStart)
	// Convert to milliseconds to match TypeScript behavior
	datafileBuildDurationMs := datafileBuildDuration.Milliseconds()
	fmt.Printf("Datafile build duration: %dms\n", datafileBuildDurationMs)

	// Convert datafile to proper format
	var datafileContent featurevisor.DatafileContent
	var datafileBytes []byte
	var err error
	if datafileBytes, err = json.Marshal(datafile); err == nil {
		json.Unmarshal(datafileBytes, &datafileContent)
	}

	// Calculate datafile size
	datafileSize := len(datafileBytes)
	if target != nil {
		fmt.Printf("Target: %s\n", *target)
	}
	fmt.Printf("Datafile size: %.2f kB\n", float64(datafileSize)/1024.0)

	instance := featurevisor.NewFeaturevisor(featurevisor.Options{
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
	fmt.Printf("Minimum duration: %s\n", formatDurationMs(output.MinDuration))
	fmt.Printf("Average duration: %s\n", formatDurationMs(output.AverageDuration))
	fmt.Printf("Maximum duration: %s\n", formatDurationMs(output.MaxDuration))
}
