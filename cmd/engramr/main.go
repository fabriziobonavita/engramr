package main

import (
	"os"

	"github.com/fabriziobonavita/engramr/internal/app"
)

func main() {
	if err := app.NewApp().Execute(); err != nil {
		os.Exit(1)
	}
}
