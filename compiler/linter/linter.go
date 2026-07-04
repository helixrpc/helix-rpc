package linter

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/helix-rpc/helix/compiler/ast"
)

var (
	pascalCaseRegex = regexp.MustCompile(`^[A-Z][a-zA-Z0-9]*$`)
	snakeCaseRegex  = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
)

func isPascalCase(s string) bool {
	return pascalCaseRegex.MatchString(s)
}

func isSnakeCase(s string) bool {
	return snakeCaseRegex.MatchString(s)
}

func Lint(tree *ast.AST) []string {
	var errors []string

	// Enums
	for _, enum := range tree.Enums {
		if !isPascalCase(enum.Name) {
			errors = append(errors, fmt.Sprintf("Enum %q should be PascalCase", enum.Name))
		}
		for key := range enum.Values {
			if !isPascalCase(key) && !isSnakeCase(key) {
				errors = append(errors, fmt.Sprintf("Enum value %q in %q should be PascalCase or snake_case", key, enum.Name))
			}
		}
	}

	// Structs
	for _, s := range tree.Structs {
		if !isPascalCase(s.Name) {
			errors = append(errors, fmt.Sprintf("Struct %q should be PascalCase", s.Name))
		}
		for _, f := range s.Fields {
			if !isSnakeCase(f.Name) {
				errors = append(errors, fmt.Sprintf("Field %q in Struct %q should be snake_case", f.Name, s.Name))
			}
			if f.ID <= 0 {
				errors = append(errors, fmt.Sprintf("Field %q in Struct %q has invalid ID %d (must be > 0)", f.Name, s.Name, f.ID))
			}
		}
	}

	// Services
	for _, srv := range tree.Services {
		if !isPascalCase(srv.Name) {
			errors = append(errors, fmt.Sprintf("Service %q should be PascalCase", srv.Name))
		}
		if !strings.HasSuffix(srv.Name, "Service") {
			errors = append(errors, fmt.Sprintf("Service %q should end with 'Service' suffix", srv.Name))
		}
		for _, m := range srv.Methods {
			if !isPascalCase(m.Name) {
				errors = append(errors, fmt.Sprintf("Method %q in Service %q should be PascalCase", m.Name, srv.Name))
			}
		}
	}

	return errors
}
