package commands

import (
	"strings"
)

type Args struct {
	Named      map[string]string
	Positional []string
}

func ParseArgs(args []string) Args {
	parsed := Args{
		Named:      make(map[string]string),
		Positional: []string{},
	}

	for _, arg := range args {
		if strings.HasPrefix(arg, "--") {
			parts := strings.SplitN(arg[2:], "=", 2)
			if len(parts) == 2 {
				parsed.Named[parts[0]] = parts[1]
			} else {
				parsed.Named[parts[0]] = "true"
			}
		} else {
			parsed.Positional = append(parsed.Positional, arg)
		}
	}

	return parsed
}
