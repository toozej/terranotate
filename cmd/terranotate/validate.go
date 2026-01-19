package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/toozej/terranotate/internal/app"
)

var validateCmd = &cobra.Command{
	Use:   "validate [terraform-file] [schema-file]",
	Short: "Validate single Terraform file against schema",
	Args:  cobra.ExactArgs(2),
	Run:   runValidateCommand,
}

var validateModuleCmd = &cobra.Command{
	Use:   "validate-module [module-dir] [schema-file]",
	Short: "Validate Terraform module (including sub-modules)",
	Args:  cobra.ExactArgs(2),
	Run:   runValidateModuleCommand,
}

var validateWorkspaceCmd = &cobra.Command{
	Use:   "validate-workspace [workspace-dir] [schema-file]",
	Short: "Validate entire Terraform workspace",
	Args:  cobra.ExactArgs(2),
	Run:   runValidateWorkspaceCommand,
}

func init() {
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(validateModuleCmd)
	rootCmd.AddCommand(validateWorkspaceCmd)
}

func runValidateCommand(cmd *cobra.Command, args []string) {
	terraformFile := args[0]
	schemaFile := args[1]

	if err := app.Validate(afero.NewOsFs(), terraformFile, schemaFile); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func runValidateModuleCommand(cmd *cobra.Command, args []string) {
	moduleDir := args[0]
	schemaFile := args[1]

	if err := app.ValidateModule(afero.NewOsFs(), moduleDir, schemaFile); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func runValidateWorkspaceCommand(cmd *cobra.Command, args []string) {
	workspaceDir := args[0]
	schemaFile := args[1]

	if err := app.ValidateWorkspace(afero.NewOsFs(), workspaceDir, schemaFile); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
