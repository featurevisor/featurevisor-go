package main

import (
	"fmt"
	"os"

	"github.com/featurevisor/featurevisor-go/cmd/featurevisor-go/commands"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: featurevisor-go <command> [arguments]")
		os.Exit(1)
	}

	command := os.Args[1]
	args := os.Args[2:]

	switch command {
	case "test":
		commands.Test(args)
	case "evaluate":
		commands.Evaluate(args)
	default:
		fmt.Printf("Unknown command: %s\n", command)
		os.Exit(1)
	}
}
