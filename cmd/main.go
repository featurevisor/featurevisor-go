package main

import (
	"fmt"
	"os"

	"github.com/featurevisor/featurevisor-go/v4/cmd/commands"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(0)
	}

	command := os.Args[1]
	args := os.Args[2:]

	switch command {
	case "--help", "-h", "help":
		if len(args) != 0 {
			if command == "help" && len(args) == 1 && (args[0] == "test" || args[0] == "benchmark" || args[0] == "assess-distribution") {
				commands.ParseCLIOptions([]string{"--help"})
				return
			}
			fmt.Fprintln(os.Stderr, "Unexpected help arguments")
			os.Exit(2)
		}
		printUsage()
	case "test":
		commands.RunTest(args)
	case "benchmark":
		commands.RunBenchmark(args)
	case "assess-distribution":
		commands.RunAssessDistribution(args)
	default:
		fmt.Fprintln(os.Stderr, "Unknown command:", command)
		os.Exit(2)
	}
}

func printUsage() {
	fmt.Println("Usage: featurevisor <test|benchmark|assess-distribution> [options]")
	fmt.Println("Use <command> --help for options. Learn more at https://featurevisor.com/docs/sdks/go/")
}
