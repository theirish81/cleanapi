package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "cleanapi",
	Short: "A tool to clean and prune OpenAPI specifications",
	Long:  `cleanapi is a CLI tool that removes examples, extensions, and non-2xx responses from OpenAPI specs while preserving descriptions and summaries.`,
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Root flags can be defined here
}
