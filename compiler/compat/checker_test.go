package compat

import (
	"testing"

	"github.com/helix-rpc/helix/compiler/ast"
)

func TestChecker_CompatibleChanges(t *testing.T) {
	oldAST := &ast.AST{
		Structs: []*ast.StructNode{
			{
				Name: "User",
				Fields: []*ast.FieldNode{
					{Name: "id", ID: 1, Type: ast.TypeNode{Kind: ast.TypeInt32}, Optional: false},
				},
			},
		},
	}

	newAST := &ast.AST{
		Structs: []*ast.StructNode{
			{
				Name: "User",
				Fields: []*ast.FieldNode{
					{Name: "id", ID: 1, Type: ast.TypeNode{Kind: ast.TypeInt32}, Optional: false},
					{Name: "email", ID: 2, Type: ast.TypeNode{Kind: ast.TypeString}, Optional: true},
				},
			},
		},
	}

	report := DiffASTs(oldAST, newAST)
	if report.HasBreaking {
		t.Fatalf("expected compatible change, but got breaking: %v", report.Breaking)
	}
	if len(report.Safe) != 1 {
		t.Fatalf("expected 1 safe change, got %d", len(report.Safe))
	}
}

func TestChecker_BreakingChanges(t *testing.T) {
	oldAST := &ast.AST{
		Structs: []*ast.StructNode{
			{
				Name: "User",
				Fields: []*ast.FieldNode{
					{Name: "id", ID: 1, Type: ast.TypeNode{Kind: ast.TypeInt32}, Optional: false},
				},
			},
		},
	}

	newAST := &ast.AST{
		Structs: []*ast.StructNode{
			{
				Name: "User",
				Fields: []*ast.FieldNode{
					{Name: "id", ID: 1, Type: ast.TypeNode{Kind: ast.TypeString}, Optional: false}, // changed type
				},
			},
		},
	}

	report := DiffASTs(oldAST, newAST)
	if !report.HasBreaking {
		t.Fatal("expected breaking change due to type mismatch")
	}
}
