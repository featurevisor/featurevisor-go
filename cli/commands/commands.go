package commands

import (
	"flag"
	"os"
	"strings"
)

// CLIOptions represents all CLI options
type CLIOptions struct {
	Command              string
	AssertionPattern     string
	Context              string
	Environment          string
	Feature              string
	KeyPattern           string
	N                    int
	OnlyFailures         bool
	Quiet                bool
	Variable             string
	Variation            bool
	Verbose              bool
	Inflate              int
	ShowDatafile         bool
	SchemaVersion        string
	ProjectDirectoryPath string
	PopulateUuid         []string
}

// ParseCLIOptions parses command line arguments into CLIOptions
func ParseCLIOptions(args []string) CLIOptions {
	opts := CLIOptions{
		ProjectDirectoryPath: getCurrentDir(),
		N:                    1000, // default value
	}

	// Handle populateUuid flags (can be multiple) before main flag parsing
	var filteredArgs []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "--populateUuid=") {
			value := strings.TrimPrefix(arg, "--populateUuid=")
			opts.PopulateUuid = append(opts.PopulateUuid, value)
		} else {
			filteredArgs = append(filteredArgs, arg)
		}
	}

	// Parse flags
	fs := flag.NewFlagSet("featurevisor", flag.ExitOnError)
	fs.StringVar(&opts.AssertionPattern, "assertionPattern", "", "Assertion pattern")
	fs.StringVar(&opts.Context, "context", "", "Context JSON")
	fs.StringVar(&opts.Environment, "environment", "", "Environment")
	fs.StringVar(&opts.Feature, "feature", "", "Feature key")
	fs.StringVar(&opts.KeyPattern, "keyPattern", "", "Key pattern")
	fs.IntVar(&opts.N, "n", 1000, "Number of iterations")
	fs.BoolVar(&opts.OnlyFailures, "onlyFailures", false, "Only show failures")
	fs.BoolVar(&opts.Quiet, "quiet", false, "Quiet mode")
	fs.StringVar(&opts.Variable, "variable", "", "Variable key")
	fs.BoolVar(&opts.Variation, "variation", false, "Variation mode")
	fs.BoolVar(&opts.Verbose, "verbose", false, "Verbose mode")
	fs.IntVar(&opts.Inflate, "inflate", 0, "Inflate mode")
	fs.BoolVar(&opts.ShowDatafile, "showDatafile", false, "Show datafile")
	fs.StringVar(&opts.SchemaVersion, "schemaVersion", "", "Schema version")
	fs.StringVar(&opts.ProjectDirectoryPath, "projectDirectoryPath", "", "Project directory path")

	// Parse the filtered flags
	fs.Parse(filteredArgs)

	return opts
}

// getCurrentDir returns the current working directory
func getCurrentDir() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	return dir
}

// RunTest runs the test command
func RunTest(args []string) {
	opts := ParseCLIOptions(args)
	runTest(opts)
}

// Examples for the test command (matching TypeScript implementation)
// test - run all tests
// test --keyPattern=pattern - run tests matching key pattern
// test --assertionPattern=pattern - run tests matching assertion pattern
// test --onlyFailures - run only failed tests
// test --showDatafile - show datafile content for each test
// test --verbose - show all test results

// RunBenchmark runs the benchmark command
func RunBenchmark(args []string) {
	opts := ParseCLIOptions(args)
	runBenchmark(opts)
}

// RunAssessDistribution runs the assess-distribution command
func RunAssessDistribution(args []string) {
	opts := ParseCLIOptions(args)
	runAssessDistribution(opts)
}
