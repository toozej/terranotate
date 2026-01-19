package app

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"
	"github.com/toozej/terranotate/internal/fixer"
	"github.com/toozej/terranotate/internal/parser"
	"github.com/toozej/terranotate/internal/validator"
	"gopkg.in/yaml.v3"
)

// Fix implements the fix command logic
func Fix(fs afero.Fs, path, schemaFile string) error {
	fmt.Println("=================================================")
	fmt.Println("Terranotate - Auto-Fix Validation Issues")
	fmt.Println("=================================================")
	fmt.Printf("Path: %s\n", path)
	fmt.Printf("Schema file: %s\n\n", schemaFile)

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to stat path: %w", err)
	}

	var files []string
	if info.IsDir() {
		files, err = findTerraformFiles(path)
		if err != nil {
			return fmt.Errorf("failed to find terraform files: %w", err)
		}
	} else {
		files = []string{path}
	}

	if len(files) == 0 {
		return fmt.Errorf("no Terraform files found in: %s", path)
	}

	totalFixed := 0
	totalFilesFixed := 0

	for _, file := range files {
		fmt.Printf("\nProcessing: %s\n", file)
		fixed, count, err := fixSingleFile(fs, file, schemaFile)
		if err != nil {
			log.Printf("Warning: Failed to fix %s: %v", file, err)
			continue
		}
		if fixed {
			totalFixed += count
			totalFilesFixed++
		}
	}

	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Printf("Fix Summary: %d files processed, %d files fixed, %d total fixes applied\n", len(files), totalFilesFixed, totalFixed)
	fmt.Println(strings.Repeat("=", 50))

	return nil
}

func fixSingleFile(fs afero.Fs, terraformFile, schemaFile string) (bool, int, error) {
	// Parse the Terraform file
	prefixes := []string{"@metadata", "@docs", "@validation", "@config"}
	p := parser.NewCommentParser(fs, prefixes)

	resources, err := p.ParseFile(terraformFile)
	if err != nil {
		return false, 0, fmt.Errorf("failed to parse Terraform file: %w", err)
	}

	// Load and validate against schema
	v, err := validator.NewSchemaValidator(fs, schemaFile)
	if err != nil {
		return false, 0, fmt.Errorf("failed to load schema: %w", err)
	}

	fmt.Println("  Analyzing validation errors...")
	result := v.ValidateResources(resources)

	if result.Passed {
		fmt.Println("  ✅ No issues found - file already passes validation!")
		return false, 0, nil
	}

	fmt.Printf("  Found %d validation errors\n", len(result.Errors))
	fmt.Println("  Attempting to fix issues...")

	// Create backup
	backupFile := terraformFile + ".bak"
	if err := fixer.CopyFile(fs, terraformFile, backupFile); err != nil {
		return false, 0, fmt.Errorf("failed to create backup: %w", err)
	}
	fmt.Printf("  ✅ Created backup: %s\n", backupFile)

	// Load schema for fixer
	schema, err := loadSchema(schemaFile)
	if err != nil {
		return false, 0, fmt.Errorf("failed to parse schema for fixer: %w", err)
	}

	// Fix the file
	f := fixer.NewCommentFixer(fs, schema)
	fixedContent, fixCount, err := f.FixFile(terraformFile, resources, result.Errors)
	if err != nil {
		return false, 0, fmt.Errorf("failed to fix file: %w", err)
	}

	// Write fixed content
	// #nosec G306 - Writing source code (Terraform), 0644 is appropriate
	// Using afero abstraction
	if err := afero.WriteFile(fs, terraformFile, []byte(fixedContent), 0644); err != nil {
		return false, 0, fmt.Errorf("failed to write fixed file: %w", err)
	}

	fmt.Printf("  ✅ Applied %d fixes to %s\n", fixCount, terraformFile)
	fmt.Println("  Re-validating fixed file...")

	// Re-validate
	resources, _ = p.ParseFile(terraformFile)
	newResult := v.ValidateResources(resources)

	if newResult.Passed {
		fmt.Println("  ✅ All fixable issues resolved! File now passes validation.")
	} else {
		fmt.Printf("  ⚠️  %d issues remain (may require manual intervention)\n", len(newResult.Errors))
		// Optional: print detailed remaining errors
	}

	fmt.Printf("  💡 Backup saved as: %s\n", backupFile)
	return true, fixCount, nil
}

func loadSchema(schemaFile string) (validator.ValidationSchema, error) {
	var schema validator.ValidationSchema
	// #nosec G304 - Schema file provided by user
	data, err := os.ReadFile(schemaFile)
	if err != nil {
		return schema, err
	}

	if err := yaml.Unmarshal(data, &schema); err != nil {
		return schema, err
	}

	return schema, nil
}

func findTerraformFiles(root string) ([]string, error) {
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == ".terraform" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(info.Name(), ".tf") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}
