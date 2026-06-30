package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/helix-rpc/helix/compiler/ast"
	"github.com/helix-rpc/helix/compiler/codegen"
	"github.com/helix-rpc/helix/compiler/parser"
)

func main() {
	idlPath := flag.String("idl", "", "Path to the IDL file (.proto or .thrift)")
	lang := flag.String("lang", "go", "Target language (go, rust)")
	outPath := flag.String("out", "", "Output filepath for generated code")
	flag.Parse()

	if *idlPath == "" || *outPath == "" {
		fmt.Println("Usage: helix -idl <path> -lang <go|rust> -out <output-file>")
		flag.PrintDefaults()
		os.Exit(1)
	}

	content, err := ioutil.ReadFile(*idlPath)
	if err != nil {
		log.Fatalf("failed to read IDL file: %v", err)
	}

	ext := filepath.Ext(*idlPath)
	var parsed *ast.AST

	switch ext {
	case ".proto":
		parsed, err = parser.ParseProto(string(content))
	case ".thrift":
		parsed, err = parser.ParseThrift(string(content))
	default:
		log.Fatalf("unsupported IDL file extension: %s (must be .proto or .thrift)", ext)
	}

	if err != nil {
		log.Fatalf("failed to parse IDL: %v", err)
	}

	var generated string
	switch *lang {
	case "go":
		generated, err = codegen.GenerateGo(parsed)
	case "rust":
		generated, err = codegen.GenerateRust(parsed)
	default:
		log.Fatalf("unsupported language target: %s", *lang)
	}

	if err != nil {
		log.Fatalf("failed to generate code: %v", err)
	}

	// Ensure output directory exists
	err = os.MkdirAll(filepath.Dir(*outPath), 0755)
	if err != nil {
		log.Fatalf("failed to create output directory: %v", err)
	}

	err = ioutil.WriteFile(*outPath, []byte(generated), 0644)
	if err != nil {
		log.Fatalf("failed to write generated file: %v", err)
	}

	fmt.Printf("Successfully generated %s code at %s\n", *lang, *outPath)
}
