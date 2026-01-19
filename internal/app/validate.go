package app

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"
	"github.com/toozej/terranotate/internal/parser"
	"github.com/toozej/terranotate/internal/validator"
)

// Validate implements the validate command logic
func Validate(fs afero.Fs, terraformFile, schemaFile string) error {
	fmt.Println("=================================================")
	fmt.Println("Terranotate - Schema Validation")
	fmt.Println("=================================================")
	fmt.Printf("Terraform file: %s\n", terraformFile)
	fmt.Printf("Schema file: %s\n\n", schemaFile)

	// Parse the Terraform file
	prefixes := []string{"@metadata", "@docs", "@validation", "@config"}
	p := parser.NewCommentParser(fs, prefixes)

	resources, err := p.ParseFile(terraformFile)
	if err != nil {
		return fmt.Errorf("failed to parse Terraform file: %w", err)
	}

	fmt.Printf("Parsed %d resources\n", len(resources))

	// Load and validate against schema
	v, err := validator.NewSchemaValidator(fs, schemaFile)
	if err != nil {
		return fmt.Errorf("failed to load schema: %w", err)
	}

	fmt.Println("Validating against schema...")

	result := v.ValidateResources(resources)

	validator.PrintValidationResults(result)

	if !result.Passed {
		return fmt.Errorf("\n💡 Tip: Run 'terranotate fix %s %s' to auto-fix some issues", terraformFile, schemaFile)
	}

	return nil
}

// ValidateModule implements the validate-module command logic
func ValidateModule(fs afero.Fs, moduleDir, schemaFile string) error {
	fmt.Println("=======================================================")
	fmt.Println("Terranotate - Module Validation (with Sub-modules)")
	fmt.Println("=======================================================")
	fmt.Printf("Module directory: %s\n", moduleDir)
	fmt.Printf("Schema file: %s\n\n", schemaFile)

	// Validate the module structure
	if err := validateModuleStructure(moduleDir); err != nil {
		return fmt.Errorf("invalid module structure: %w", err)
	}

	// Find all Terraform files in the module and sub-modules
	tfFiles, err := findModuleTerraformFiles(moduleDir)
	if err != nil {
		return fmt.Errorf("failed to scan module directory: %w", err)
	}

	if len(tfFiles) == 0 {
		return fmt.Errorf("no Terraform files found in module: %s", moduleDir)
	}

	fmt.Printf("Found %d Terraform files across module and sub-modules:\n", len(tfFiles))
	for _, file := range tfFiles {
		relPath, _ := filepath.Rel(moduleDir, file)
		fmt.Printf("  - %s\n", relPath)
	}
	fmt.Println()

	// Validate all files
	result := validateTerraformFiles(fs, tfFiles, schemaFile)

	printModuleValidationResults(result, moduleDir)

	if !result.Passed {
		return fmt.Errorf("module validation failed")
	}

	return nil
}

// ValidateWorkspace implements the validate-workspace command logic
func ValidateWorkspace(fs afero.Fs, workspaceDir, schemaFile string) error {
	fmt.Println("=========================================================")
	fmt.Println("Terranotate - Workspace Validation (Recursive)")
	fmt.Println("=========================================================")
	fmt.Printf("Workspace directory: %s\n", workspaceDir)
	fmt.Printf("Schema file: %s\n\n", schemaFile)

	// Find all Terraform files in the workspace
	tfFiles, err := findWorkspaceTerraformFiles(workspaceDir)
	if err != nil {
		return fmt.Errorf("failed to scan workspace directory: %w", err)
	}

	if len(tfFiles) == 0 {
		return fmt.Errorf("no Terraform files found in workspace: %s", workspaceDir)
	}

	// Group files by directory for better reporting
	filesByDir := groupFilesByDirectory(tfFiles, workspaceDir)

	fmt.Printf("Found %d Terraform files in %d directories:\n", len(tfFiles), len(filesByDir))
	for dir, files := range filesByDir {
		fmt.Printf("\n  📁 %s (%d files)\n", dir, len(files))
		for _, file := range files {
			fmt.Printf("    - %s\n", filepath.Base(file))
		}
	}
	fmt.Println()

	// Validate all files
	result := validateTerraformFiles(fs, tfFiles, schemaFile)

	printWorkspaceValidationResults(result, workspaceDir, filesByDir)

	if !result.Passed {
		return fmt.Errorf("workspace validation failed")
	}

	return nil
}

// Helper functions

func validateModuleStructure(moduleDir string) error {
	info, err := os.Stat(moduleDir)
	if err != nil {
		return fmt.Errorf("directory does not exist: %s", moduleDir)
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory: %s", moduleDir)
	}

	entries, err := os.ReadDir(moduleDir)
	if err != nil {
		return err
	}

	hasTfFile := false
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".tf") {
			hasTfFile = true
			break
		}
	}

	if !hasTfFile {
		return fmt.Errorf("no .tf files found in module root: %s", moduleDir)
	}

	return nil
}

func findModuleTerraformFiles(moduleDir string) ([]string, error) {
	var tfFiles []string

	// Get files in the root module
	rootFiles, err := filepath.Glob(filepath.Join(moduleDir, "*.tf"))
	if err != nil {
		return nil, err
	}
	tfFiles = append(tfFiles, rootFiles...)

	// Check for modules subdirectory
	modulesDir := filepath.Join(moduleDir, "modules")
	if info, err := os.Stat(modulesDir); err == nil && info.IsDir() {
		err := filepath.Walk(modulesDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && strings.HasSuffix(info.Name(), ".tf") {
				tfFiles = append(tfFiles, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	return tfFiles, nil
}

func findWorkspaceTerraformFiles(workspaceDir string) ([]string, error) {
	var tfFiles []string

	err := filepath.Walk(workspaceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") ||
				name == "node_modules" ||
				name == ".terraform" ||
				name == "terraform.tfstate.d" {
				return filepath.SkipDir
			}
		}

		if !info.IsDir() && strings.HasSuffix(info.Name(), ".tf") {
			tfFiles = append(tfFiles, path)
		}
		return nil
	})

	return tfFiles, err
}

func groupFilesByDirectory(files []string, baseDir string) map[string][]string {
	result := make(map[string][]string)

	for _, file := range files {
		dir := filepath.Dir(file)
		relDir, _ := filepath.Rel(baseDir, dir)
		if relDir == "." {
			relDir = "root"
		}
		result[relDir] = append(result[relDir], file)
	}

	return result
}

func validateTerraformFiles(fs afero.Fs, files []string, schemaFile string) validator.ValidationResult {
	aggregatedResult := validator.ValidationResult{Passed: true}

	v, err := validator.NewSchemaValidator(fs, schemaFile)
	if err != nil {
		log.Fatalf("Failed to load schema: %v", err)
	}

	prefixes := []string{"@metadata", "@docs", "@validation", "@config"}
	p := parser.NewCommentParser(fs, prefixes)

	for _, file := range files {
		resources, err := p.ParseFile(file)
		if err != nil {
			log.Printf("Warning: Failed to parse %s: %v", file, err)
			continue
		}

		if len(resources) == 0 {
			continue // Skip files with no resources
		}

		result := v.ValidateResources(resources)

		// Add file context to errors
		for i := range result.Errors {
			result.Errors[i].ResourceType = fmt.Sprintf("%s (%s)",
				result.Errors[i].ResourceType,
				file)
		}

		aggregatedResult.Errors = append(aggregatedResult.Errors, result.Errors...)
		if !result.Passed {
			aggregatedResult.Passed = false
		}
	}

	return aggregatedResult
}

func printModuleValidationResults(result validator.ValidationResult, moduleDir string) {
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("MODULE VALIDATION RESULTS")
	fmt.Println(strings.Repeat("=", 80))

	if result.Passed {
		fmt.Println("\n✅ Module validation passed!")
		fmt.Printf("   All files in %s meet schema requirements\n", moduleDir)
		return
	}

	fmt.Printf("\n❌ Module validation failed for: %s\n", moduleDir)
	validator.PrintValidationResults(result)
}

func printWorkspaceValidationResults(result validator.ValidationResult, workspaceDir string, filesByDir map[string][]string) {
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("WORKSPACE VALIDATION RESULTS")
	fmt.Println(strings.Repeat("=", 80))

	if result.Passed {
		fmt.Println("\n✅ Workspace validation passed!")
		fmt.Printf("   All %d directories in %s meet schema requirements\n",
			len(filesByDir), workspaceDir)

		fmt.Println("\n📊 Validated directories:")
		for dir := range filesByDir {
			fmt.Printf("   ✓ %s\n", dir)
		}
		return
	}

	fmt.Printf("\n❌ Workspace validation failed for: %s\n", workspaceDir)

	errorsByDir := make(map[string][]validator.ValidationError)
	for _, err := range result.Errors {
		parts := strings.Split(err.ResourceType, " (")
		if len(parts) == 2 {
			filePath := strings.TrimSuffix(parts[1], ")")
			for dir, files := range filesByDir {
				for _, file := range files {
					if file == filePath {
						// Clean up ResourceType for display
						err.ResourceType = parts[0] + " (" + filepath.Base(filePath) + ")"
						errorsByDir[dir] = append(errorsByDir[dir], err)
						break
					}
				}
			}
		}
	}

	fmt.Println("\n" + strings.Repeat("=", 80))
	for dir, errors := range errorsByDir {
		fmt.Printf("\n📁 Directory: %s (%d errors)\n", dir, len(errors))
		fmt.Println(strings.Repeat("-", 80))

		for _, err := range errors {
			severity := "ERROR"
			icon := "❌"
			if err.Severity == "warning" {
				severity = "WARNING"
				icon = "⚠️"
			}

			fmt.Printf("  %s [%s] %s - Line %d\n", icon, severity, err.ResourceType, err.Line)
			fmt.Printf("     %s\n\n", err.Message)
		}
	}

	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("\nTotal errors: %d across %d directories\n", len(result.Errors), len(errorsByDir))
}
