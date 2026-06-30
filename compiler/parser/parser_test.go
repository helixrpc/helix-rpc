package parser

import (
	"testing"

	"github.com/helix-rpc/helix/compiler/ast"
)

func TestProtoParser(t *testing.T) {
	content := `
	syntax = "proto3";
	package helix.example;

	message UserProfile {
		int64 user_id = 1;
		string username = 2;
		optional string email = 3;
	}

	service UserProfileService {
		rpc GetUserProfile (UserProfile) returns (UserProfile);
	}
	`

	parsed, err := ParseProto(content)
	if err != nil {
		t.Fatalf("failed to parse proto: %v", err)
	}

	if parsed.Namespace != "helix.example" {
		t.Errorf("expected namespace 'helix.example', got '%s'", parsed.Namespace)
	}

	if len(parsed.Structs) != 1 {
		t.Fatalf("expected 1 struct, got %d", len(parsed.Structs))
	}

	str := parsed.Structs[0]
	if str.Name != "UserProfile" {
		t.Errorf("expected struct name 'UserProfile', got '%s'", str.Name)
	}

	if len(str.Fields) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(str.Fields))
	}

	f0 := str.Fields[0]
	if f0.Name != "user_id" || f0.ID != 1 || f0.Type.Kind != ast.TypeInt64 {
		t.Errorf("field 0 mismatch: %+v", f0)
	}

	f2 := str.Fields[2]
	if f2.Name != "email" || f2.ID != 3 || !f2.Optional || f2.Type.Kind != ast.TypeString {
		t.Errorf("field 2 mismatch: %+v", f2)
	}

	if len(parsed.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(parsed.Services))
	}

	srv := parsed.Services[0]
	if srv.Name != "UserProfileService" {
		t.Errorf("expected service name 'UserProfileService', got '%s'", srv.Name)
	}

	if len(srv.Methods) != 1 {
		t.Fatalf("expected 1 method, got %d", len(srv.Methods))
	}

	m0 := srv.Methods[0]
	if m0.Name != "GetUserProfile" || m0.InputType != "UserProfile" || m0.OutputType != "UserProfile" {
		t.Errorf("method mismatch: %+v", m0)
	}
}

func TestThriftParser(t *testing.T) {
	content := `
	namespace go helix.example
	namespace py helix.example.py

	struct UserProfile {
		1: required i64 user_id
		2: string username
		3: optional string email
	}

	service UserProfileService {
		UserProfile GetUserProfile(1: UserProfile request)
	}
	`

	parsed, err := ParseThrift(content)
	if err != nil {
		t.Fatalf("failed to parse thrift: %v", err)
	}

	if parsed.Namespace != "helix.example" {
		t.Errorf("expected namespace 'helix.example', got '%s'", parsed.Namespace)
	}

	if len(parsed.Structs) != 1 {
		t.Fatalf("expected 1 struct, got %d", len(parsed.Structs))
	}

	str := parsed.Structs[0]
	if str.Name != "UserProfile" {
		t.Errorf("expected struct name 'UserProfile', got '%s'", str.Name)
	}

	if len(str.Fields) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(str.Fields))
	}

	f0 := str.Fields[0]
	if f0.Name != "user_id" || f0.ID != 1 || f0.Type.Kind != ast.TypeInt64 {
		t.Errorf("field 0 mismatch: %+v", f0)
	}

	f2 := str.Fields[2]
	if f2.Name != "email" || f2.ID != 3 || !f2.Optional || f2.Type.Kind != ast.TypeString {
		t.Errorf("field 2 mismatch: %+v", f2)
	}

	if len(parsed.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(parsed.Services))
	}

	srv := parsed.Services[0]
	if srv.Name != "UserProfileService" {
		t.Errorf("expected service name 'UserProfileService', got '%s'", srv.Name)
	}

	if len(srv.Methods) != 1 {
		t.Fatalf("expected 1 method, got %d", len(srv.Methods))
	}

	m0 := srv.Methods[0]
	if m0.Name != "GetUserProfile" || m0.InputType != "UserProfile" || m0.OutputType != "UserProfile" {
		t.Errorf("method mismatch: %+v", m0)
	}
}
