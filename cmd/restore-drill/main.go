// Command restore-drill is the CLI entrypoint; all wiring lives in internal/cli.
package main

import (
	"os"

	"github.com/RamazanKara/restore-drill/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
