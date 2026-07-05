package compat

import (
	"fmt"
	"io/ioutil"
	"regexp"
	"strings"
)

// TranspileThriftToProto converts a legacy Thrift IDL file into a Protobuf v3 schema.
func TranspileThriftToProto(thriftContent string) (string, error) {
	var sb strings.Builder
	sb.WriteString("// Automatically transpiled from Thrift by Helix RPC. DO NOT EDIT.\n")
	sb.WriteString("syntax = \"proto3\";\n\n")
	sb.WriteString("package helix.migrated;\n\n")

	lines := strings.Split(thriftContent, "\n")
	inStruct := false
	structName := ""

	// Regex patterns
	structStartRx := regexp.MustCompile(`(?i)^\s*struct\s+(\w+)\s*\{`)
	structEndRx := regexp.MustCompile(`^\s*\}\s*`)
	fieldRx := regexp.MustCompile(`^\s*(\d+)\s*:\s*(\w+)\s+(\w+)(?:\s*,|;)?`)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
			continue
		}

		if !inStruct {
			if matches := structStartRx.FindStringSubmatch(line); len(matches) > 1 {
				inStruct = true
				structName = matches[1]
				sb.WriteString(fmt.Sprintf("message %s {\n", structName))
			}
		} else {
			if structEndRx.MatchString(line) {
				inStruct = false
				sb.WriteString("}\n\n")
			} else if matches := fieldRx.FindStringSubmatch(line); len(matches) >= 4 {
				fieldID := matches[1]
				fieldType := matches[2]
				fieldName := matches[3]

				protoType := mapThriftTypeToProto(fieldType)
				sb.WriteString(fmt.Sprintf("    %s %s = %s;\n", protoType, fieldName, fieldID))
			}
		}
	}

	return sb.String(), nil
}

func mapThriftTypeToProto(t string) string {
	switch strings.ToLower(t) {
	case "i16", "i32":
		return "int32"
	case "i64":
		return "int64"
	case "string":
		return "string"
	case "double":
		return "double"
	case "bool":
		return "bool"
	case "binary":
		return "bytes"
	default:
		return "string"
	}
}

// TranspileFile reads a thrift schema file and writes a proto schema file.
func TranspileFile(inPath, outPath string) error {
	data, err := ioutil.ReadFile(inPath)
	if err != nil {
		return fmt.Errorf("failed to read thrift file: %w", err)
	}

	protoContent, err := TranspileThriftToProto(string(data))
	if err != nil {
		return fmt.Errorf("failed to transpile schema: %w", err)
	}

	err = ioutil.WriteFile(outPath, []byte(protoContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to write proto file: %w", err)
	}

	return nil
}
