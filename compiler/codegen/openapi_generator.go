package codegen

import (
	"encoding/json"
	"fmt"

	"github.com/helix-rpc/helix/compiler/ast"
)

func GenerateOpenAPI(parsed *ast.AST) (string, error) {
	// OpenAPI 3.0 Document Structure
	openapi := map[string]interface{}{
		"openapi": "3.0.0",
		"info": map[string]interface{}{
			"title":   "Helix RPC Unified Service",
			"version": "1.0.0",
			"description": "Auto-generated OpenAPI specification from Helix IDL.",
		},
		"paths": make(map[string]interface{}),
		"components": map[string]interface{}{
			"schemas": make(map[string]interface{}),
		},
	}

	paths := openapi["paths"].(map[string]interface{})
	schemas := openapi["components"].(map[string]interface{})["schemas"].(map[string]interface{})

	// 1. Generate Schemas (Structs)
	for _, str := range parsed.Structs {
		schema := map[string]interface{}{
			"type": "object",
			"properties": make(map[string]interface{}),
		}
		props := schema["properties"].(map[string]interface{})

		for _, f := range str.Fields {
			props[f.Name] = mapAstTypeToOpenAPI(f.Type)
		}
		schemas[str.Name] = schema
	}

	// 2. Generate Schemas (Enums)
	for _, e := range parsed.Enums {
		schema := map[string]interface{}{
			"type": "integer",
			"description": fmt.Sprintf("Enum %s", e.Name),
		}
		schemas[e.Name] = schema
	}

	// 3. Generate Paths (Services and Methods)
	for _, svc := range parsed.Services {
		for _, method := range svc.Methods {
			path := method.RESTPath
			if path == "" {
				path = fmt.Sprintf("/v1.%s/%s", svc.Name, method.Name)
			}
			
			httpMethod := "post"
			if method.RESTMethod != "" {
				httpMethod = method.RESTMethod
			}

			operation := map[string]interface{}{
				"operationId": method.Name,
				"tags": []string{svc.Name},
				"requestBody": map[string]interface{}{
					"required": true,
					"content": map[string]interface{}{
						"application/json": map[string]interface{}{
							"schema": map[string]interface{}{
								"$ref": fmt.Sprintf("#/components/schemas/%s", method.InputType),
							},
						},
					},
				},
				"responses": map[string]interface{}{
					"200": map[string]interface{}{
						"description": "Successful response",
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"$ref": fmt.Sprintf("#/components/schemas/%s", method.OutputType),
								},
							},
						},
					},
				},
			}

			// If server streaming, adjust description
			if method.ServerStreaming {
				operation["description"] = "Server Streaming (SSE) endpoint"
				operation["responses"].(map[string]interface{})["200"].(map[string]interface{})["content"] = map[string]interface{}{
					"text/event-stream": map[string]interface{}{
						"schema": map[string]interface{}{
							"$ref": fmt.Sprintf("#/components/schemas/%s", method.OutputType),
						},
					},
				}
			}

			// Add to paths
			if paths[path] == nil {
				paths[path] = make(map[string]interface{})
			}
			paths[path].(map[string]interface{})[httpMethod] = operation
		}
	}

	// Serialize to JSON
	bytes, err := json.MarshalIndent(openapi, "", "  ")
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func mapAstTypeToOpenAPI(t ast.TypeNode) map[string]interface{} {
	switch t.Kind {
	case ast.TypeInt32:
		return map[string]interface{}{"type": "integer", "format": "int32"}
	case ast.TypeInt64:
		return map[string]interface{}{"type": "integer", "format": "int64"}
	case ast.TypeDouble:
		return map[string]interface{}{"type": "number", "format": "double"}
	case ast.TypeBool:
		return map[string]interface{}{"type": "boolean"}
	case ast.TypeString:
		return map[string]interface{}{"type": "string"}
	case ast.TypeBinary:
		return map[string]interface{}{"type": "string", "format": "byte"}
	case ast.TypeStruct, ast.TypeEnum:
		return map[string]interface{}{"$ref": fmt.Sprintf("#/components/schemas/%s", t.Name)}
	case ast.TypeList:
		return map[string]interface{}{
			"type": "array",
			"items": mapAstTypeToOpenAPI(*t.ValueType),
		}
	case ast.TypeMap:
		return map[string]interface{}{
			"type": "object",
			"additionalProperties": mapAstTypeToOpenAPI(*t.ValueType),
		}
	default:
		return map[string]interface{}{"type": "object"}
	}
}
