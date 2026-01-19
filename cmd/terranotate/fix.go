package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/toozej/terranotate/internal/app"
)

var fixCmd = &cobra.Command{
	Use:   "fix [terraform-file-or-dir] [schema-file]",
	Short: "Auto-fix validation issues by adding missing comments",
	Args:  cobra.ExactArgs(2),
	Run:   runFixCommand,
}

func init() {
	rootCmd.AddCommand(fixCmd)
}

func runFixCommand(cmd *cobra.Command, args []string) {
	path := args[0]
	schemaFile := args[1]

	if err := app.Fix(afero.NewOsFs(), path, schemaFile); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
