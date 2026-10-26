// Command hlops is a Termux-friendly operations tool: analyze, multi-task
// backup, cleanup, scheduled runs and environment doctor checks.
package main

import (
"os"

"github.com/eddievibe4tech-byte/termux-ops/internal/cli"
)

func main() {
os.Exit(cli.Execute())
}
