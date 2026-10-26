package cli

import (
"fmt"
"os"
"runtime"

"github.com/spf13/cobra"

"github.com/eddievibe4tech-byte/termux-ops/internal/termux"
)

var rootCmd = &cobra.Command{
Use:   "hlops",
Short: "Termux-optimised ops helper (backup, cleanup, analyze)",
Long:  "hlops is a single-binary operations helper optimised for Termux on Android,\nalso usable on regular Linux.",
}

// Execute runs the CLI.
func Execute() {
if err := rootCmd.Execute(); err != nil {
os.Exit(1)
}
}

func init() {
rootCmd.AddCommand(&cobra.Command{
Use:   "version",
Short: "Show version, OS, arch and Termux detection",
Run: func(cmd *cobra.Command, args []string) {
env := termux.Detect()
fmt.Printf("hlops %s\n", Version)
fmt.Printf("goos: %s\n", runtime.GOOS)
fmt.Printf("goarch: %s\n", runtime.GOARCH)
fmt.Printf("termux: %t\n", env.IsTermux)
fmt.Printf("home: %s\n", env.Home)
if env.Prefix != "" {
fmt.Printf("prefix: %s\n", env.Prefix)
}
},
})
}

// Version is set via -ldflags at build time.
var Version = "dev"
