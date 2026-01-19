# CI/CD Integration

## GitHub Actions Example

```yaml
name: Terraform Comment Validation

on: [push, pull_request]

jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2
      
      - name: Set up Go
        uses: actions/setup-go@v2
        with:
          go-version: 1.25
      
      - name: Install dependencies
        run: |
          go get github.com/hashicorp/hcl/v2
          go get gopkg.in/yaml.v3
      
      - name: Build validator
        run: go build -o terranotate
      
      - name: Validate Terraform files
        run: |
          for file in $(find . -name "*.tf"); do
            echo "Validating $file"
            ./terranotate validate "$file" schema.yaml || exit 1
          done
```

## Pre-commit Hook

Create `.git/hooks/pre-commit`:

```bash
#!/bin/bash

echo "Validating Terraform files..."

# Validate all changed modules
for module_dir in $(find . -name "*.tf" -exec dirname {} \; | sort -u); do
    ./terranotate validate-module "$module_dir" examples/schema.yaml || exit 1
done
```
