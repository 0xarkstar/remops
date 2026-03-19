package main

import (
	"os"

	"github.com/0xarkstar/remops/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
