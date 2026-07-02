package compat

import (
	"fmt"

	"github.com/helix-rpc/helix/compiler/ast"
)

type CompatReport struct {
	Breaking    []string
	Safe        []string
	HasBreaking bool
}

func DiffASTs(oldAST, newAST *ast.AST) CompatReport {
	r := CompatReport{}

	// 1. Index Structs
	oldStructs := make(map[string]*ast.StructNode)
	for _, s := range oldAST.Structs {
		oldStructs[s.Name] = s
	}
	newStructs := make(map[string]*ast.StructNode)
	for _, s := range newAST.Structs {
		newStructs[s.Name] = s
	}

	// Compare Structs
	for name, oldStruct := range oldStructs {
		newStruct, exists := newStructs[name]
		if !exists {
			r.Breaking = append(r.Breaking, fmt.Sprintf("REMOVED struct %q", name))
			r.HasBreaking = true
			continue
		}

		// Index fields
		oldFields := make(map[string]*ast.FieldNode)
		for _, f := range oldStruct.Fields {
			oldFields[f.Name] = f
		}
		newFields := make(map[string]*ast.FieldNode)
		for _, f := range newStruct.Fields {
			newFields[f.Name] = f
		}

		// Check deleted or modified fields
		for fName, oldField := range oldFields {
			newField, exists := newFields[fName]
			if !exists {
				r.Breaking = append(r.Breaking, fmt.Sprintf("REMOVED field %q.%q", name, fName))
				r.HasBreaking = true
				continue
			}

			// Check ID/Tag change
			if oldField.ID != newField.ID {
				r.Breaking = append(r.Breaking, fmt.Sprintf("CHANGED tag ID of field %q.%q: %d → %d", name, fName, oldField.ID, newField.ID))
				r.HasBreaking = true
			}

			// Check Type change
			if !typesEqual(oldField.Type, newField.Type) {
				r.Breaking = append(r.Breaking, fmt.Sprintf("CHANGED type of field %q.%q: %s → %s", name, fName, formatType(oldField.Type), formatType(newField.Type)))
				r.HasBreaking = true
			}

			// Check Optionality change (optional -> required is breaking)
			if oldField.Optional && !newField.Optional {
				r.Breaking = append(r.Breaking, fmt.Sprintf("CHANGED optional field %q.%q to REQUIRED", name, fName))
				r.HasBreaking = true
			}
		}

		// Check added fields
		for fName, newField := range newFields {
			if _, exists := oldFields[fName]; !exists {
				if !newField.Optional {
					r.Breaking = append(r.Breaking, fmt.Sprintf("ADDED REQUIRED field %q.%q", name, fName))
					r.HasBreaking = true
				} else {
					r.Safe = append(r.Safe, fmt.Sprintf("ADDED optional field %q.%q (backward compatible)", name, fName))
				}
			}
		}
	}

	// New Structs
	for name := range newStructs {
		if _, exists := oldStructs[name]; !exists {
			r.Safe = append(r.Safe, fmt.Sprintf("ADDED struct %q (backward compatible)", name))
		}
	}

	// 2. Index Enums
	oldEnums := make(map[string]*ast.EnumNode)
	for _, e := range oldAST.Enums {
		oldEnums[e.Name] = e
	}
	newEnums := make(map[string]*ast.EnumNode)
	for _, e := range newAST.Enums {
		newEnums[e.Name] = e
	}

	// Compare Enums
	for name, oldEnum := range oldEnums {
		newEnum, exists := newEnums[name]
		if !exists {
			r.Breaking = append(r.Breaking, fmt.Sprintf("REMOVED enum %q", name))
			r.HasBreaking = true
			continue
		}

		// Compare values
		for valName, oldVal := range oldEnum.Values {
			newVal, exists := newEnum.Values[valName]
			if !exists {
				r.Breaking = append(r.Breaking, fmt.Sprintf("REMOVED enum value %q.%q", name, valName))
				r.HasBreaking = true
			} else if oldVal != newVal {
				r.Breaking = append(r.Breaking, fmt.Sprintf("CHANGED enum integer mapping of %q.%q: %d → %d", name, valName, oldVal, newVal))
				r.HasBreaking = true
			}
		}

		for valName := range newEnum.Values {
			if _, exists := oldEnum.Values[valName]; !exists {
				r.Safe = append(r.Safe, fmt.Sprintf("ADDED enum value %q.%q (backward compatible)", name, valName))
			}
		}
	}

	// New Enums
	for name := range newEnums {
		if _, exists := oldEnums[name]; !exists {
			r.Safe = append(r.Safe, fmt.Sprintf("ADDED enum %q (backward compatible)", name))
		}
	}

	// 3. Index Services
	oldServices := make(map[string]*ast.ServiceNode)
	for _, s := range oldAST.Services {
		oldServices[s.Name] = s
	}
	newServices := make(map[string]*ast.ServiceNode)
	for _, s := range newAST.Services {
		newServices[s.Name] = s
	}

	// Compare Services
	for name, oldSvc := range oldServices {
		newSvc, exists := newServices[name]
		if !exists {
			r.Breaking = append(r.Breaking, fmt.Sprintf("REMOVED service %q", name))
			r.HasBreaking = true
			continue
		}

		// Index methods
		oldMethods := make(map[string]*ast.MethodNode)
		for _, m := range oldSvc.Methods {
			oldMethods[m.Name] = m
		}
		newMethods := make(map[string]*ast.MethodNode)
		for _, m := range newSvc.Methods {
			newMethods[m.Name] = m
		}

		// Compare methods
		for mName, oldMethod := range oldMethods {
			newMethod, exists := newMethods[mName]
			if !exists {
				r.Breaking = append(r.Breaking, fmt.Sprintf("REMOVED method %q.%q", name, mName))
				r.HasBreaking = true
				continue
			}

			// Check input/output types
			if oldMethod.InputType != newMethod.InputType {
				r.Breaking = append(r.Breaking, fmt.Sprintf("CHANGED input type of method %q.%q: %q → %q", name, mName, oldMethod.InputType, newMethod.InputType))
				r.HasBreaking = true
			}
			if oldMethod.OutputType != newMethod.OutputType {
				r.Breaking = append(r.Breaking, fmt.Sprintf("CHANGED output type of method %q.%q: %q → %q", name, mName, oldMethod.OutputType, newMethod.OutputType))
				r.HasBreaking = true
			}

			// Check streaming characteristics
			if oldMethod.ClientStreaming != newMethod.ClientStreaming {
				r.Breaking = append(r.Breaking, fmt.Sprintf("CHANGED client streaming of method %q.%q", name, mName))
				r.HasBreaking = true
			}
			if oldMethod.ServerStreaming != newMethod.ServerStreaming {
				r.Breaking = append(r.Breaking, fmt.Sprintf("CHANGED server streaming of method %q.%q", name, mName))
				r.HasBreaking = true
			}
		}

		// Check added methods
		for mName := range newMethods {
			if _, exists := oldMethods[mName]; !exists {
				r.Safe = append(r.Safe, fmt.Sprintf("ADDED method %q.%q (backward compatible)", name, mName))
			}
		}
	}

	// New Services
	for name := range newServices {
		if _, exists := oldServices[name]; !exists {
			r.Safe = append(r.Safe, fmt.Sprintf("ADDED service %q (backward compatible)", name))
		}
	}

	return r
}

func typesEqual(t1, t2 ast.TypeNode) bool {
	if t1.Kind != t2.Kind {
		return false
	}
	if t1.Name != t2.Name {
		return false
	}
	if (t1.KeyType == nil) != (t2.KeyType == nil) {
		return false
	}
	if t1.KeyType != nil && !typesEqual(*t1.KeyType, *t2.KeyType) {
		return false
	}
	if (t1.ValueType == nil) != (t2.ValueType == nil) {
		return false
	}
	if t1.ValueType != nil && !typesEqual(*t1.ValueType, *t2.ValueType) {
		return false
	}
	return true
}

func formatType(t ast.TypeNode) string {
	switch t.Kind {
	case ast.TypeStruct, ast.TypeEnum:
		return t.Name
	case ast.TypeMap:
		return fmt.Sprintf("map<%s, %s>", formatType(*t.KeyType), formatType(*t.ValueType))
	case ast.TypeList:
		return fmt.Sprintf("list<%s>", formatType(*t.ValueType))
	default:
		return string(t.Kind)
	}
}
