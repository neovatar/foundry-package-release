package main

import (
	"fmt"

	githubactions "github.com/sethvargo/go-githubactions"
)

func main() {
	greeting := githubactions.GetInput("greeting")
	if greeting == "" {
		greeting = "Hello"
	}

	name := githubactions.GetInput("name")
	if name == "" {
		githubactions.Fatalf("missing input 'name'")
	}

	message := fmt.Sprintf("%s, %s!", greeting, name)

	githubactions.Infof(message)
	githubactions.SetOutput("message", message)
}
