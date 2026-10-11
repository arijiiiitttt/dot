// Dot - a VS Code-style code editor for the terminal.
// All of the code lives in ./src; this file only starts it.
package main

import (
	"os"

	"dot/src"
)

func main() {
	os.Exit(src.Run(os.Args[1:]))
}
