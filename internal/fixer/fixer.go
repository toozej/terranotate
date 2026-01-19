package fixer

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/afero"
	"github.com/toozej/terranotate/internal/parser"
	"github.com/toozej/terranotate/internal/validator"
)

// CommentFixer handles automatic fixing of validation errors
type CommentFixer struct {
	fs     afero.Fs
	schema validator.ValidationSchema
}

// NewCommentFixer creates a new comment fixer
func NewCommentFixer(fs afero.Fs, schema validator.ValidationSchema) *CommentFixer {
	if fs == nil {
		fs = afero.NewOsFs()
	}
	return &CommentFixer{fs: fs, schema: schema}
}

// FixFile attempts to fix validation errors in a Terraform file
func (cf *CommentFixer) FixFile(filename string, resources []parser.TerraformResource, errors []validator.ValidationError) (string, int, error) {
	// #nosec G304 - File provided by user via CLI, using afero abstraction
	f, err := cf.fs.Open(filename)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	content, err := io.ReadAll(f)
	if err != nil {
		return "", 0, err
	}

	lines := strings.Split(string(content), "\n")
	fixCount := 0

	// Group errors by resource
	errorsByResource := cf.groupErrorsByResource(errors)

	// Process each resource
	for _, resource := range resources {
		resourceKey := fmt.Sprintf("%s.%s", resource.Type, resource.Name)
		resourceErrors, hasErrors := errorsByResource[resourceKey]

		if !hasErrors {
			continue
		}

		// Generate fixes for this resource
		fixes := cf.generateFixes(resource, resourceErrors)

		if len(fixes) == 0 {
			continue
		}

		// Insert comment block before the resource
		insertLine := resource.StartLine - 1
		if insertLine < 0 {
			insertLine = 0
		}

		// Build comment block
		commentBlock := cf.buildCommentBlock(fixes)

		// Insert the comment block
		lines = cf.insertLines(lines, insertLine, commentBlock)
		fixCount += len(fixes)
	}

	return strings.Join(lines, "\n"), fixCount, nil
}

// groupErrorsByResource groups validation errors by resource
func (cf *CommentFixer) groupErrorsByResource(errors []validator.ValidationError) map[string][]validator.ValidationError {
	result := make(map[string][]validator.ValidationError)

	for _, err := range errors {
		// Remove filename suffix if present
		resourceType := err.ResourceType
		if idx := strings.Index(resourceType, " ("); idx != -1 {
			resourceType = resourceType[:idx]
		}

		key := fmt.Sprintf("%s.%s", resourceType, err.ResourceName)
		result[key] = append(result[key], err)
	}

	return result
}

// generateFixes generates comment fixes for a resource
func (cf *CommentFixer) generateFixes(resource parser.TerraformResource, errors []validator.ValidationError) []CommentFix {
	var fixes []CommentFix

	// Get applicable schema rules
	rules := cf.getApplicableRules(resource.Type)

	// Track which prefixes we need to add
	missingPrefixes := make(map[string]bool)
	missingFields := make(map[string][]string) // prefix -> []fields

	for _, err := range errors {
		// Check if it's a missing prefix error
		if strings.Contains(err.Message, "Missing required comment prefix:") {
			prefix := strings.TrimSpace(strings.TrimPrefix(err.Message, "Missing required comment prefix:"))
			missingPrefixes[prefix] = true
			continue
		}

		// Check if it's a missing field error
		if strings.Contains(err.Message, "Missing required field") {
			// Extract prefix and field from error message
			// Format: "@metadata: Missing required field 'owner'"
			parts := strings.SplitN(err.Message, ":", 2)
			if len(parts) == 2 {
				prefix := strings.TrimSpace(parts[0])
				fieldMsg := strings.TrimSpace(parts[1])

				// Extract field name from quotes
				if start := strings.Index(fieldMsg, "'"); start != -1 {
					if end := strings.Index(fieldMsg[start+1:], "'"); end != -1 {
						field := fieldMsg[start+1 : start+1+end]
						missingFields[prefix] = append(missingFields[prefix], field)
					}
				}
			}
		}
	}

	// Generate fixes for missing prefixes
	for prefix := range missingPrefixes {
		fix := cf.generatePrefixFix(prefix, rules)
		if fix != nil {
			fixes = append(fixes, *fix)
		}
	}

	// Generate fixes for missing fields
	for prefix, fields := range missingFields {
		fix := cf.generateFieldFix(prefix, fields, rules)
		if fix != nil {
			fixes = append(fixes, *fix)
		}
	}

	return fixes
}

// CommentFix represents a fix to apply
type CommentFix struct {
	Prefix string
	Fields map[string]string
}

// generatePrefixFix generates a fix for a missing prefix with default required fields
func (cf *CommentFixer) generatePrefixFix(prefix string, rules validator.ResourceRules) *CommentFix {
	prefixRule, exists := rules.PrefixRules[prefix]
	if !exists {
		return nil
	}

	fix := &CommentFix{
		Prefix: prefix,
		Fields: make(map[string]string),
	}

	// Add placeholders for all required fields
	for _, field := range prefixRule.RequiredFields {
		fix.Fields[field] = cf.getPlaceholderValue(field)
	}

	// Add placeholders for required nested fields
	for nestedPath, nestedRule := range prefixRule.NestedFields {
		for _, field := range nestedRule.RequiredFields {
			fullPath := nestedPath + "." + field
			fix.Fields[fullPath] = cf.getPlaceholderValue(field)
		}
	}

	return fix
}

// generateFieldFix generates a fix for missing fields in an existing prefix
func (cf *CommentFixer) generateFieldFix(prefix string, fields []string, rules validator.ResourceRules) *CommentFix {
	fix := &CommentFix{
		Prefix: prefix,
		Fields: make(map[string]string),
	}

	for _, field := range fields {
		fix.Fields[field] = cf.getPlaceholderValue(field)
	}

	return fix
}

// getPlaceholderValue returns a placeholder value for a field
func (cf *CommentFixer) getPlaceholderValue(field string) string {
	// Remove nested path if present
	parts := strings.Split(field, ".")
	fieldName := parts[len(parts)-1]

	// Common field placeholders
	placeholders := map[string]string{
		"owner":             "CHANGEME",
		"team":              "CHANGEME",
		"priority":          "medium",
		"environment":       "production",
		"email":             "changeme@example.com",
		"slack":             "@changeme",
		"phone":             "555-0000",
		"description":       "CHANGEME: Add description",
		"required":          "true",
		"enabled":           "true",
		"backup":            "true",
		"encrypted":         "true",
		"cost_center":       "CHANGEME",
		"department":        "CHANGEME",
		"emergency_contact": "oncall@example.com",
		"uptime":            "99.9",
		"replicas":          "3",
		"backup_required":   "true",
		"mfa_required":      "true",
		"password_policy":   "strict",
	}

	if val, exists := placeholders[fieldName]; exists {
		return val
	}

	// Check field validation for type hints
	if validation, exists := cf.schema.FieldValidations[fieldName]; exists {
		if len(validation.AllowedValues) > 0 {
			return validation.AllowedValues[0]
		}

		switch validation.Type {
		case "boolean":
			return "true"
		case "integer":
			if validation.Min > 0 {
				return fmt.Sprintf("%d", int(validation.Min))
			}
			return "1"
		case "float":
			if validation.Min > 0 {
				return fmt.Sprintf("%.1f", validation.Min)
			}
			return "1.0"
		case "array":
			return "[CHANGEME]"
		}
	}

	return "CHANGEME"
}

// buildCommentBlock builds a comment block from fixes
func (cf *CommentFixer) buildCommentBlock(fixes []CommentFix) []string {
	var lines []string

	for _, fix := range fixes {
		// Group fields by prefix (for nested fields)
		rootFields := make(map[string]string)
		nestedFields := make(map[string]map[string]string)

		for field, value := range fix.Fields {
			if strings.Contains(field, ".") {
				// Nested field
				parts := strings.SplitN(field, ".", 2)
				prefix := parts[0]
				rest := parts[1]

				if nestedFields[prefix] == nil {
					nestedFields[prefix] = make(map[string]string)
				}
				nestedFields[prefix][rest] = value
			} else {
				// Root field
				rootFields[field] = value
			}
		}

		// Build comment lines
		commentLine := "# " + fix.Prefix

		// Add root fields
		for field, value := range rootFields {
			commentLine += fmt.Sprintf(" %s:%s", field, value)
		}

		lines = append(lines, commentLine)

		// Add nested fields on separate lines
		for prefix, fields := range nestedFields {
			nestedLine := "#"
			for field, value := range fields {
				nestedLine += fmt.Sprintf(" %s.%s:%s", prefix, field, value)
			}
			lines = append(lines, nestedLine)
		}
	}

	return lines
}

// insertLines inserts new lines at the specified position
func (cf *CommentFixer) insertLines(lines []string, position int, newLines []string) []string {
	// Ensure position is valid
	if position < 0 {
		position = 0
	}
	if position > len(lines) {
		position = len(lines)
	}

	// Insert new lines
	result := make([]string, 0, len(lines)+len(newLines))
	result = append(result, lines[:position]...)
	result = append(result, newLines...)
	result = append(result, lines[position:]...)

	return result
}

// getApplicableRules returns applicable rules for a resource type
func (cf *CommentFixer) getApplicableRules(resourceType string) validator.ResourceRules {
	if rules, exists := cf.schema.ResourceTypes[resourceType]; exists {
		return rules
	}

	return validator.ResourceRules{
		RequiredPrefixes: cf.schema.Global.RequiredPrefixes,
		PrefixRules:      cf.schema.Global.PrefixRules,
	}
}

// CopyFile copies a file from src to dst. Exported for utility use.
func CopyFile(fs afero.Fs, src, dst string) error {
	// #nosec G304 - Source path provided by user
	sourceFile, err := fs.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	// #nosec G304 - Destination path derived from user input
	destFile, err := fs.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	/*
		if err != nil {
			return err
		}
		// Afero files might not implement Sync, so checking interface
		if syncer, ok := destFile.(interface{ Sync() error }); ok {
			return syncer.Sync()
		}
	*/
	return err
}
