package main

import (
	"fmt"
	"os"

	"github.com/featurevisor/featurevisor-go/cli/commands"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Learn more at https://featurevisor.com/docs/sdks/go/")
		os.Exit(0)
	}

	command := os.Args[1]
	args := os.Args[2:]

	switch command {
	case "test":
		commands.RunTest(args)
	case "benchmark":
		commands.RunBenchmark(args)
	case "assess-distribution":
		commands.RunAssessDistribution(args)
	default:
		fmt.Println("Learn more at https://featurevisor.com/docs/sdks/go/")
		os.Exit(0)
	}
}
