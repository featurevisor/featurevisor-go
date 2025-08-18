package commands

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/featurevisor/featurevisor-go"
)

// UUID_LENGTHS matches the TypeScript implementation
var UUID_LENGTHS = []int{4, 2, 2, 2, 6}

// generateUuid generates a UUID string matching the TypeScript format
func generateUuid() string {
	parts := make([]string, len(UUID_LENGTHS))
	for i, length := range UUID_LENGTHS {
		bytes := make([]byte, length)
		rand.Read(bytes)
		parts[i] = hex.EncodeToString(bytes)
	}
	return strings.Join(parts, "-")
}

// prettyNumber formats numbers with commas for thousands
func prettyNumber(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%d", n) // Go doesn't have built-in comma formatting, but we can add it later if needed
}

// prettyPercentage formats percentage with 2 decimal places
func prettyPercentage(count, total int) string {
	if total == 0 {
		return "0.00%"
	}
	percentage := float64(count) / float64(total) * 100
	return fmt.Sprintf("%.2f%%", percentage)
}

// printCounts prints the evaluation counts in the same format as TypeScript
func printCounts(evaluations map[interface{}]int, n int, sortResults bool) {
	// Convert to entries for sorting
	type entry struct {
		value interface{}
		count int
	}

	var entries []entry
	for value, count := range evaluations {
		entries = append(entries, entry{value, count})
	}

	// Sort by count descending if requested
	if sortResults {
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].count > entries[j].count
		})
	}

	// Find longest value string for alignment
	longestValueLength := 0
	highestCount := 0
	for _, entry := range entries {
		valueStr := fmt.Sprintf("%v", entry.value)
		if len(valueStr) > longestValueLength {
			longestValueLength = len(valueStr)
		}
		if entry.count > highestCount {
			highestCount = entry.count
		}
	}

	// Print each entry with proper alignment
	for _, entry := range entries {
		valueStr := fmt.Sprintf("%v", entry.value)
		fmt.Print("  - ", valueStr, ": ", prettyNumber(entry.count), " ", prettyPercentage(entry.count, n), "\n")
	}
}

// runAssessDistribution runs the assess-distribution command
func runAssessDistribution(opts CLIOptions) {
	featurevisorProjectPath := opts.ProjectDirectoryPath

	if opts.Environment == "" {
		fmt.Println("Environment is required")
		return
	}

	if opts.Feature == "" {
		fmt.Println("Feature is required")
		return
	}

	var context featurevisor.Context
	if opts.Context != "" {
		json.Unmarshal([]byte(opts.Context), &context)
	} else {
		context = make(featurevisor.Context)
	}
	populateUuid := opts.PopulateUuid

	levelStr := getLoggerLevel(opts)
	level := featurevisor.LogLevel(levelStr)
	datafilesByEnvironment := buildDatafiles(featurevisorProjectPath, []string{opts.Environment}, "", 0)

	// Create SDK instance
	datafile := datafilesByEnvironment[opts.Environment]

	// Convert datafile to proper format
	var datafileContent featurevisor.DatafileContent
	if datafileBytes, err := json.Marshal(datafile); err == nil {
		json.Unmarshal(datafileBytes, &datafileContent)
	}

	instance := featurevisor.CreateInstance(featurevisor.Options{
		Datafile: datafileContent,
		LogLevel: &level,
	})

	// Check if feature has variations
	feature := instance.GetFeature(opts.Feature)
	hasVariations := feature != nil && len(feature.Variations) > 0

	// Initialize evaluation counters
	flagEvaluations := map[interface{}]int{
		"enabled":  0,
		"disabled": 0,
	}
	variationEvaluations := make(map[interface{}]int)

	// Print header matching TypeScript format
	fmt.Println("\nAssessing distribution for feature:", opts.Feature, "...")

	// Print context information
	if opts.Context != "" {
		fmt.Printf("Against context: %s\n", opts.Context)
	} else {
		fmt.Printf("Against context: {}\n")
	}

	fmt.Printf("Running %d times...\n", opts.N)

	// Run evaluations
	for i := 0; i < opts.N; i++ {
		// Create a copy of context for this iteration
		contextCopy := make(featurevisor.Context)
		for k, v := range context {
			contextCopy[k] = v
		}

		// Populate UUIDs if requested
		if len(populateUuid) > 0 {
			for _, key := range populateUuid {
				uuid := generateUuid()
				contextCopy[key] = uuid
			}
		}

		// Evaluate flag
		flagEvaluation := instance.IsEnabled(opts.Feature, contextCopy, featurevisor.OverrideOptions{})
		if flagEvaluation {
			flagEvaluations["enabled"]++
		} else {
			flagEvaluations["disabled"]++
		}

		// Evaluate variation if feature has variations
		if hasVariations {
			variationEvaluation := instance.GetVariation(opts.Feature, contextCopy, featurevisor.OverrideOptions{})
			if variationEvaluation != nil {
				// Dereference the pointer to get the actual string value
				variationValue := *variationEvaluation
				variationEvaluations[variationValue]++
			}
		}
	}

	// Print results in the same format as TypeScript
	fmt.Println("\n\nFlag evaluations:")
	printCounts(flagEvaluations, opts.N, true)

	if hasVariations {
		fmt.Println("\n\nVariation evaluations:")
		printCounts(variationEvaluations, opts.N, true)
	}
}
