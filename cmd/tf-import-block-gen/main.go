package main

import (
	"os"

	"github.com/moveeeax/tf-import-block-gen/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
