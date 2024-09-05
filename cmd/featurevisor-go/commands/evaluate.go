package commands

import "fmt"

func Evaluate(args []string) {
	parsedArgs := ParseArgs(args)

	fmt.Println("Hello World from the evaluate command!")
	fmt.Println("Named arguments:")
	for key, value := range parsedArgs.Named {
		fmt.Printf("  %s: %s\n", key, value)
	}
	fmt.Println("Positional arguments:", parsedArgs.Positional)
}
