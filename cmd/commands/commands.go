package commands

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// CLIOptions represents all CLI options
type CLIOptions struct {
	Command              string
	AssertionPattern     string
	Case                 string
	Context              string
	Coverage             bool
	Environment          string
	EntityType           string
	Feature              string
	KeyPattern           string
	N                    int
	OnlyFailures         bool
	Quiet                bool
	Variable             string
	Variation            bool
	Verbose              bool
	Inflate              int
	JSON                 bool
	Pretty               bool
	WithScopes           bool
	WithTags             bool
	ShowDatafile         bool
	SchemaVersion        string
	Set                  string
	SetProvided          bool
	ProjectDirectoryPath string
	PopulateUuid         []string
	Targets              []string
	Uncovered            bool
}

// ParseCLIOptions parses command line arguments into CLIOptions
func ParseCLIOptions(args []string) CLIOptions {
	opts, err := parseCLIOptions(args, os.Stderr)
	if err == flag.ErrHelp {
		os.Exit(0)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	return opts
}

func parseCLIOptions(args []string, output io.Writer) (CLIOptions, error) {
	opts := CLIOptions{
		ProjectDirectoryPath: getCurrentDir(),
		N:                    1000, // default value
	}

	// Parse flags
	fs := flag.NewFlagSet("featurevisor", flag.ContinueOnError)
	fs.SetOutput(output)
	fs.Usage = func() {
		fmt.Fprintln(output, "Usage: featurevisor <test|benchmark|assess-distribution> [options]")
		fmt.Fprintln(output, "Node.js handles project parsing and datafiles. This CLI evaluates with the Go SDK.")
		fs.PrintDefaults()
	}
	fs.StringVar(&opts.AssertionPattern, "assertionPattern", "", "Assertion pattern")
	fs.StringVar(&opts.Case, "case", "", "Exact expanded test case ID")
	fs.StringVar(&opts.Context, "context", "", "Context JSON")
	fs.BoolVar(&opts.Coverage, "coverage", false, "Report observed rule and override selections")
	fs.StringVar(&opts.Environment, "environment", "", "Environment")
	fs.StringVar(&opts.EntityType, "entityType", "", "Entity type")
	fs.StringVar(&opts.Feature, "feature", "", "Feature key")
	fs.StringVar(&opts.KeyPattern, "keyPattern", "", "Key pattern")
	fs.Func("n", "Number of iterations (default 1000, also -n)", func(value string) error {
		n, err := strconv.Atoi(value)
		if err != nil || n <= 0 {
			return fmt.Errorf("--n must be a positive integer")
		}
		opts.N = n
		return nil
	})
	fs.Func("populateUuid", "Context attribute to populate with a UUID (repeatable)", func(value string) error {
		if value == "" {
			return fmt.Errorf("populate-uuid must not be empty")
		}
		opts.PopulateUuid = append(opts.PopulateUuid, value)
		return nil
	})
	fs.Func("target", "Target key (repeatable)", func(value string) error {
		if value == "" {
			return fmt.Errorf("target must not be empty")
		}
		if !containsString(opts.Targets, value) {
			opts.Targets = append(opts.Targets, value)
		}
		return nil
	})
	fs.BoolVar(&opts.OnlyFailures, "onlyFailures", false, "Only show failures")
	fs.BoolVar(&opts.Quiet, "quiet", false, "Quiet mode")
	fs.StringVar(&opts.Variable, "variable", "", "Variable key")
	fs.BoolVar(&opts.Variation, "variation", false, "Variation mode")
	fs.BoolVar(&opts.Verbose, "verbose", false, "Verbose mode")
	// Zero means omitted; validate supplied values before storing them.
	fs.Func("inflate", "Inflation multiplier", func(value string) error {
		inflate, err := strconv.Atoi(value)
		if err != nil || inflate <= 0 {
			return fmt.Errorf("--inflate must be a positive integer")
		}
		opts.Inflate = inflate
		return nil
	})
	fs.BoolVar(&opts.JSON, "json", false, "Output a versioned JSON report")
	fs.BoolVar(&opts.Pretty, "pretty", false, "Pretty print JSON output")
	fs.BoolVar(&opts.WithScopes, "with-scopes", false, "Legacy option accepted for compatibility and ignored")
	fs.BoolVar(&opts.WithTags, "with-tags", false, "Legacy option accepted for compatibility and ignored")
	fs.BoolVar(&opts.ShowDatafile, "showDatafile", false, "Show datafile")
	fs.StringVar(&opts.SchemaVersion, "schemaVersion", "", "Legacy option accepted for compatibility and ignored")
	fs.StringVar(&opts.SchemaVersion, "schema-version", "", "Legacy option accepted for compatibility and ignored")
	fs.Func("set", "Project set", func(value string) error {
		opts.Set = value
		opts.SetProvided = true
		return nil
	})
	fs.StringVar(&opts.ProjectDirectoryPath, "projectDirectoryPath", opts.ProjectDirectoryPath, "Project directory path")
	fs.BoolVar(&opts.Uncovered, "uncovered", false, "Report definitions without associated specs")
	for alias, name := range map[string]string{
		"assertion-pattern": "assertionPattern", "key-pattern": "keyPattern",
		"only-failures": "onlyFailures", "entity-type": "entityType",
		"show-datafile": "showDatafile", "populate-uuid": "populateUuid",
		"project-directory-path": "projectDirectoryPath", "root-directory-path": "projectDirectoryPath",
		"rootDirectoryPath": "projectDirectoryPath",
	} {
		original := fs.Lookup(name)
		fs.Var(original.Value, alias, "Alias for --"+name)
	}

	// Reject a following option where a value is required. Keep argv items intact.
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--" {
			break
		}
		if !strings.HasPrefix(arg, "-") || strings.Contains(arg, "=") {
			continue
		}
		name := strings.TrimLeft(arg, "-")
		option := fs.Lookup(name)
		if option == nil {
			continue
		}
		if boolean, ok := option.Value.(interface{ IsBoolFlag() bool }); ok && boolean.IsBoolFlag() {
			continue
		}
		if index+1 >= len(args) || (strings.HasPrefix(args[index+1], "-") && name != "n" && name != "inflate") {
			return opts, fmt.Errorf("--%s requires a value", name)
		}
		index++
	}
	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	if fs.NArg() != 0 {
		return opts, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	var invalid error
	fs.Visit(func(option *flag.Flag) {
		switch option.Name {
		case "projectDirectoryPath", "project-directory-path", "rootDirectoryPath", "root-directory-path", "feature", "variable", "environment", "case", "entityType", "entity-type", "context":
			if option.Value.String() == "" {
				invalid = fmt.Errorf("--%s must not be empty", option.Name)
			}
		}
	})
	if invalid != nil {
		return opts, invalid
	}
	if opts.EntityType != "" && !containsString([]string{"feature", "segment", "variable", "scenario"}, opts.EntityType) {
		return opts, fmt.Errorf("--entity-type must be feature, segment, variable or scenario")
	}
	if _, err := evaluationContext(opts); err != nil {
		return opts, err
	}
	return opts, nil
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
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
	if err := runBenchmark(opts); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

// RunAssessDistribution runs the assess-distribution command
func RunAssessDistribution(args []string) {
	opts := ParseCLIOptions(args)
	if err := runAssessDistribution(opts); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
