package commands

import (
	"encoding/json"
	"fmt"

	"github.com/featurevisor/featurevisor-go/sdk"
	"github.com/featurevisor/featurevisor-go/types"
)

func Evaluate(args []string) {
	parsedArgs := ParseArgs(args)

	// Assume datafile content exists as a string in a variable
	datafileContent := `
	{
		"schemaVersion": "1",
		"revision": "1.0",
		"features": [],
		"attributes": [],
		"segments": []
	}
	`

	// Create a new instance of the SDK
	datafile, datafileErr := sdk.NewDatafileContent(datafileContent)
	if datafileErr != nil {
		fmt.Printf("Error creating datafile content: %v\n", datafileErr)
		return
	}
	instance, err := sdk.CreateInstance(sdk.InstanceOptions{
		Datafile: datafile,
	})
	if err != nil {
		fmt.Printf("Error creating SDK instance: %v\n", err)
		return
	}

	// Get the feature key from the --feature option
	featureKey, ok := parsedArgs.Named["feature"]
	if !ok {
		fmt.Println("Error: --feature option is required")
		return
	}

	// Parse the context from the --context option
	var context types.Context
	contextJSON, ok := parsedArgs.Named["context"]
	if ok {
		err := json.Unmarshal([]byte(contextJSON), &context)
		if err != nil {
			fmt.Printf("Error parsing context JSON: %v\n", err)
			return
		}
	}

	// Evaluate the feature
	evaluation := instance.EvaluateFlag(types.FeatureKey(featureKey), context)

	// Print the evaluation details
	fmt.Printf("Feature: %s\n", featureKey)
	fmt.Printf("Enabled: %v\n", evaluation.Enabled)
	fmt.Printf("Reason: %s\n", evaluation.Reason)
}
