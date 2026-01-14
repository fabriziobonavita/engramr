package main

import (
	"os"

	"github.com/fabriziobonavita/engramr/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
