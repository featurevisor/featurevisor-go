package main

import (
	"fmt"
	"github.com/featurevisor/featurevisor-go"
)

func main() {
	datafileURL := "https://featurevisor.com/datafile.yml"

	instance, err := featurevisor.NewInstance(datafileURL)
	if err != nil {
		fmt.Printf("Error creating Featurevisor: %s\n", err)
		return
	}

	revision := instance.GetRevision()
	fmt.Printf("Featurevisor datafile revision: %s\n", revision)
}
