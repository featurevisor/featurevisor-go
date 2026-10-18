package commands

import (
	"encoding/json"
	"fmt"

	featurevisor "github.com/featurevisor/featurevisor-go/v4"
)

// resolveEvaluationSelections reads configuration through the Node CLI.
// Each build delegates set and target validation to that same CLI.
func resolveEvaluationSelections(opts CLIOptions) ([]CLIOptions, error) {
	config, err := getConfigForReport(opts.ProjectDirectoryPath, "")
	if err != nil {
		return nil, err
	}
	if raw, configured := config["environments"]; configured {
		environments, ok := raw.([]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid configured environments")
		}
		if opts.Environment == "" {
			return nil, fmt.Errorf("--environment is required for a project with configured environments")
		}
		found := false
		for _, environment := range environments {
			found = found || environment == opts.Environment
		}
		if !found {
			return nil, fmt.Errorf("unknown environment %q", opts.Environment)
		}
	} else if opts.Environment != "" {
		return nil, fmt.Errorf("--environment cannot be used in a project without environments")
	}
	sets, err := discoverTestSets(opts.ProjectDirectoryPath, config, opts.Set, opts.SetProvided)
	if err != nil {
		return nil, err
	}
	targets := []string{}
	for _, target := range opts.Targets {
		if !containsString(targets, target) {
			targets = append(targets, target)
		}
	}
	if len(targets) == 0 {
		targets = []string{""}
	}
	selections := []CLIOptions{}
	for _, set := range sets {
		for _, target := range targets {
			selected := opts
			selected.Set = set
			selected.Targets = nil
			if target != "" {
				selected.Targets = []string{target}
			}
			selections = append(selections, selected)
		}
	}
	return selections, nil
}

func evaluationContext(opts CLIOptions) (featurevisor.Context, error) {
	if opts.N <= 0 {
		return nil, fmt.Errorf("--n must be a positive integer")
	}
	if opts.Inflate < 0 {
		return nil, fmt.Errorf("--inflate must be a positive integer")
	}
	context := make(featurevisor.Context)
	if opts.Context != "" {
		if err := json.Unmarshal([]byte(opts.Context), &context); err != nil || context == nil {
			return nil, fmt.Errorf("--context must be a JSON object")
		}
	}
	return context, nil
}

func buildEvaluationDatafile(opts CLIOptions) (featurevisor.DatafileContent, int, error) {
	var environment, target *string
	if opts.Environment != "" {
		environment = &opts.Environment
	}
	if len(opts.Targets) == 1 {
		target = &opts.Targets[0]
	}
	datafile, err := buildDatafileForReport(opts.ProjectDirectoryPath, environment, target, opts.Inflate, opts.Set)
	if err != nil {
		return featurevisor.DatafileContent{}, 0, err
	}
	bytes, err := json.Marshal(datafile)
	if err != nil {
		return featurevisor.DatafileContent{}, 0, err
	}
	var content featurevisor.DatafileContent
	if err := json.Unmarshal(bytes, &content); err != nil {
		return content, 0, fmt.Errorf("could not parse generated datafile: %w", err)
	}
	return content, len(bytes), nil
}
