package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/helix-rpc/helix/compiler/ast"
	"github.com/helix-rpc/helix/compiler/codegen"
	"github.com/helix-rpc/helix/compiler/parser"
)

const version = "0.2.0"

const usageBanner = `
██╗  ██╗███████╗██╗     ██╗██╗  ██╗     ██████╗ ███████╗███╗   ██╗
██║  ██║██╔════╝██║     ██║╚██╗██╔╝    ██╔════╝ ██╔════╝████╗  ██║
███████║█████╗  ██║     ██║ ╚███╔╝     ██║  ███╗█████╗  ██╔██╗ ██║
██╔══██║██╔══╝  ██║     ██║ ██╔██╗     ██║   ██║██╔══╝  ██║╚██╗██║
██║  ██║███████╗███████╗██║██╔╝ ██╗    ╚██████╔╝███████╗██║ ╚████║
╚═╝  ╚═╝╚══════╝╚══════╝╚═╝╚═╝  ╚═╝     ╚═════╝ ╚══════╝╚═╝  ╚═══╝
                                                       v` + version + `

Helix RPC code generator and project scaffolding tool.

SUBCOMMANDS:
  helix-gen generate  Generate code from an IDL schema
  helix-gen init      Scaffold a new Helix RPC service
  helix-gen diff      Compare two schema versions for compatibility

Run 'helix-gen <subcommand> --help' for detailed usage.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usageBanner)
		os.Exit(0)
	}

	switch os.Args[1] {
	case "generate", "gen":
		runGenerate(os.Args[2:])
	case "init":
		runInit(os.Args[2:])
	case "diff":
		runDiff(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Printf("helix-gen %s\n", version)
	case "help", "--help", "-h":
		fmt.Print(usageBanner)
	default:
		// Legacy compatibility: if first arg looks like a flag, treat it as generate
		if len(os.Args[1]) > 0 && os.Args[1][0] == '-' {
			runGenerate(os.Args[1:])
		} else {
			printError(fmt.Sprintf("unknown subcommand %q", os.Args[1]))
			fmt.Print(usageBanner)
			os.Exit(1)
		}
	}
}

// ---------------------------------------------------------------------------
// generate subcommand
// ---------------------------------------------------------------------------

func runGenerate(args []string) {
	fs := flag.NewFlagSet("generate", flag.ExitOnError)
	idlPath := fs.String("idl", "", "Path to the IDL file (.proto or .thrift)")
	lang := fs.String("lang", "go", "Target language (go, rust, python)")
	outPath := fs.String("out", "", "Output filepath for generated code")
	watch := fs.Bool("watch", false, "Watch IDL file and regenerate on change")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: helix-gen generate -idl <schema.proto> -lang <go|rust|python> -out <output-file> [--watch]")
		fs.PrintDefaults()
	}
	fs.Parse(args) //nolint:errcheck

	if *idlPath == "" || *outPath == "" {
		fs.Usage()
		os.Exit(1)
	}

	if err := generate(*idlPath, *lang, *outPath); err != nil {
		os.Exit(1)
	}

	if *watch {
		fmt.Printf("👀 Watching %s for changes (Ctrl+C to stop)...\n", *idlPath)
		last := time.Now()
		for {
			time.Sleep(500 * time.Millisecond)
			info, err := os.Stat(*idlPath)
			if err != nil {
				continue
			}
			if info.ModTime().After(last) {
				last = info.ModTime()
				fmt.Printf("🔄 Schema changed — regenerating %s...\n", *outPath)
				if err := generate(*idlPath, *lang, *outPath); err == nil {
					fmt.Printf("✅ Generated %s\n", *outPath)
				}
			}
		}
	}
}

func generate(idlPath, lang, outPath string) error {
	content, err := ioutil.ReadFile(idlPath)
	if err != nil {
		printError(fmt.Sprintf("cannot read IDL file %q: %v", idlPath, err))
		return err
	}

	ext := filepath.Ext(idlPath)
	var parsed *ast.AST

	switch ext {
	case ".proto":
		parsed, err = parser.ParseProto(string(content))
	case ".thrift":
		parsed, err = parser.ParseThrift(string(content))
	default:
		printError(fmt.Sprintf("unsupported file extension %q", ext))
		fmt.Fprintln(os.Stderr, "  Supported formats: .proto, .thrift")
		return fmt.Errorf("unsupported extension")
	}

	if err != nil {
		// err already contains friendly context from the parser
		fmt.Fprintf(os.Stderr, "\n✗ helix-gen: parse error in %s\n  %v\n\n", filepath.Base(idlPath), err)
		return err
	}

	var generated string
	switch lang {
	case "go":
		generated, err = codegen.GenerateGo(parsed)
	case "rust":
		generated, err = codegen.GenerateRust(parsed)
	case "python":
		generated, err = codegen.GeneratePython(parsed)
	default:
		printError(fmt.Sprintf("unsupported language %q", lang))
		fmt.Fprintln(os.Stderr, "  Supported languages: go, rust, python")
		return fmt.Errorf("unsupported language")
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "\n✗ helix-gen: code generation failed\n  %v\n\n", err)
		return err
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		printError(fmt.Sprintf("cannot create output directory: %v", err))
		return err
	}

	if err := ioutil.WriteFile(outPath, []byte(generated), 0644); err != nil {
		printError(fmt.Sprintf("cannot write %q: %v", outPath, err))
		return err
	}

	fmt.Printf("✅ Generated %s → %s\n", lang, outPath)
	return nil
}

// ---------------------------------------------------------------------------
// init subcommand
// ---------------------------------------------------------------------------

func runInit(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	lang := fs.String("lang", "go", "Primary language (go, rust, python)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: helix-gen init <service-name> [--lang go|rust|python]")
	}
	fs.Parse(args) //nolint:errcheck

	name := fs.Arg(0)
	if name == "" {
		fs.Usage()
		printError("service name is required")
		os.Exit(1)
	}

	if err := scaffold(name, *lang); err != nil {
		log.Fatal(err)
	}
}

func scaffold(name, lang string) error {
	base := name
	if err := os.MkdirAll(base, 0755); err != nil {
		return err
	}

	// schema.proto
	writeFile(filepath.Join(base, "schema.proto"), fmt.Sprintf(`syntax = "proto3";

package %s;

message PredictRequest {
  string prompt = 1;
}

message PredictResponse {
  string completion = 1;
}

service ModelService {
  rpc Predict(PredictRequest) returns (PredictResponse);
}
`, name))

	// Makefile
	writeFile(filepath.Join(base, "Makefile"), fmt.Sprintf(`SERVICE := %s
LANG    := %s

.PHONY: gen build test dev

gen:
	helix-gen generate -idl schema.proto -lang $(LANG) -out generated/generated.$(if $(filter go,$(LANG)),go,$(if $(filter rust,$(LANG)),rs,py))

watch:
	helix-gen generate -idl schema.proto -lang $(LANG) -out generated/generated.$(if $(filter go,$(LANG)),go,$(if $(filter rust,$(LANG)),rs,py)) --watch

build: gen
	$(if $(filter go,$(LANG)),go build ./...,$(if $(filter rust,$(LANG)),cargo build,echo "Python: no build step required"))

test:
	$(if $(filter go,$(LANG)),go test ./...,$(if $(filter rust,$(LANG)),cargo test,pytest tests/))

dev: gen
	@echo "🚀 Starting $(SERVICE) in dev mode..."
	$(if $(filter go,$(LANG)),go run server/main.go,$(if $(filter rust,$(LANG)),cargo run,python server.py))
`, name, lang))

	// README
	writeFile(filepath.Join(base, "README.md"), fmt.Sprintf(`# %s

A Helix RPC service scaffolded with `+"`helix-gen init`"+`.

## Quick Start

`+"```"+`bash
# 1. Generate code from schema
make gen

# 2. Run the server
make dev

# 3. Test it
curl -X POST http://localhost:8080/predict \
     -H 'Content-Type: application/json' \
     -d '{"prompt": "hello world"}'
`+"```"+`

## Watch Mode (Live Reload)
`+"```"+`bash
make watch   # regenerates on every schema.proto save
`+"```"+`

## Schema
Edit [schema.proto](./schema.proto) and run `+"`make gen`"+` to regenerate.
`, name))

	// Language-specific server entrypoint
	switch lang {
	case "go":
		os.MkdirAll(filepath.Join(base, "server"), 0755)     //nolint:errcheck
		os.MkdirAll(filepath.Join(base, "generated"), 0755)  //nolint:errcheck
		writeFile(filepath.Join(base, "go.mod"), fmt.Sprintf("module github.com/example/%s\n\ngo 1.22\n\nrequire github.com/helix-rpc/helix/runtime-go v0.2.0\n", name))
		writeFile(filepath.Join(base, "server", "main.go"), fmt.Sprintf(`package main

import (
	"context"
	"fmt"
	"log"

	runtime "github.com/helix-rpc/helix/runtime-go"
)

func main() {
	server := runtime.NewServer(":8080")

	// Register your service methods here
	server.RegisterMethod("/v1.ModelService/Predict", runtime.MethodInfo{
		Handler: func(ctx context.Context, req interface{}) (interface{}, error) {
			return map[string]string{"completion": "Hello from %s!"}, nil
		},
	})
	server.RegisterRESTRoute("POST", "/predict", "/v1.ModelService/Predict")

	log.Println("🚀 %s listening on :8080")
	log.Fatal(server.Start())
}
`, name, name))

	case "python":
		os.MkdirAll(filepath.Join(base, "generated"), 0755) //nolint:errcheck
		writeFile(filepath.Join(base, "server.py"), fmt.Sprintf(`import sys
sys.path.insert(0, ".")

from helix_rt.server import HelixServer

server = HelixServer(host="127.0.0.1", port=8080)

async def predict(body: dict) -> dict:
    prompt = body.get("prompt", "")
    return {"completion": f"Hello from %s! You said: {prompt}"}

server.register_route("POST", "/predict", predict)
server.start()
`, name))

	case "rust":
		os.MkdirAll(filepath.Join(base, "src"), 0755)       //nolint:errcheck
		os.MkdirAll(filepath.Join(base, "generated"), 0755) //nolint:errcheck
		writeFile(filepath.Join(base, "Cargo.toml"), fmt.Sprintf(`[package]
name = "%s"
version = "0.1.0"
edition = "2021"

[dependencies]
helix-rt = "0.1.0"
tokio = { version = "1.0", features = ["full"] }
serde_json = "1.0"
`, name))
		writeFile(filepath.Join(base, "src", "main.rs"), fmt.Sprintf(`use helix_rt::server::{HttpServiceHandler, RestRoute};
use std::sync::Arc;

#[tokio::main]
async fn main() {
    println!("🚀 %s listening on :8080");
    // Add your service handler here
    todo!("wire up generated handler")
}
`, name))
	}

	fmt.Printf("\n✅ Scaffolded %q (%s)\n\n", name, lang)
	fmt.Printf("  Next steps:\n")
	fmt.Printf("    cd %s\n", name)
	fmt.Printf("    make gen    # generate code from schema.proto\n")
	fmt.Printf("    make dev    # start the server\n\n")
	return nil
}

// ---------------------------------------------------------------------------
// diff subcommand
// ---------------------------------------------------------------------------

func runDiff(args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: helix-gen diff <old-schema> <new-schema>")
		os.Exit(1)
	}
	oldPath, newPath := args[0], args[1]

	oldAST, err := parseIDL(oldPath)
	if err != nil {
		printError(fmt.Sprintf("cannot parse %s: %v", oldPath, err))
		os.Exit(1)
	}
	newAST, err := parseIDL(newPath)
	if err != nil {
		printError(fmt.Sprintf("cannot parse %s: %v", newPath, err))
		os.Exit(1)
	}

	report := diffASTs(oldAST, newAST)
	printCompatReport(report, oldPath, newPath)
	if report.hasBreaking {
		os.Exit(2) // non-zero so CI pipelines fail on breaking changes
	}
}

type compatReport struct {
	breaking    []string
	safe        []string
	hasBreaking bool
}

func diffASTs(oldAST, newAST *ast.AST) compatReport {
	r := compatReport{}

	// Index old messages
	oldMsgs := map[string]*ast.Message{}
	for i := range oldAST.Messages {
		oldMsgs[oldAST.Messages[i].Name] = &oldAST.Messages[i]
	}
	// Index new messages
	newMsgs := map[string]*ast.Message{}
	for i := range newAST.Messages {
		newMsgs[newAST.Messages[i].Name] = &newAST.Messages[i]
	}

	// Check for removed or modified messages
	for name, oldMsg := range oldMsgs {
		newMsg, exists := newMsgs[name]
		if !exists {
			r.breaking = append(r.breaking, fmt.Sprintf("REMOVED message %q", name))
			r.hasBreaking = true
			continue
		}
		// Check removed fields
		oldFields := map[string]ast.Field{}
		for _, f := range oldMsg.Fields {
			oldFields[f.Name] = f
		}
		for _, f := range newMsg.Fields {
			if _, ok := oldFields[f.Name]; !ok {
				r.safe = append(r.safe, fmt.Sprintf("ADDED field %q.%q (backward compatible)", name, f.Name))
			}
		}
		newFields := map[string]ast.Field{}
		for _, f := range newMsg.Fields {
			newFields[f.Name] = f
		}
		for _, f := range oldMsg.Fields {
			if _, ok := newFields[f.Name]; !ok {
				r.breaking = append(r.breaking, fmt.Sprintf("REMOVED field %q.%q", name, f.Name))
				r.hasBreaking = true
			} else if newFields[f.Name].Type != f.Type {
				r.breaking = append(r.breaking, fmt.Sprintf("CHANGED type of %q.%q: %q → %q", name, f.Name, f.Type, newFields[f.Name].Type))
				r.hasBreaking = true
			}
		}
	}
	// New messages
	for name := range newMsgs {
		if _, ok := oldMsgs[name]; !ok {
			r.safe = append(r.safe, fmt.Sprintf("ADDED message %q (backward compatible)", name))
		}
	}

	// Check services / methods
	oldSvcs := map[string]*ast.Service{}
	for i := range oldAST.Services {
		oldSvcs[oldAST.Services[i].Name] = &oldAST.Services[i]
	}
	newSvcs := map[string]*ast.Service{}
	for i := range newAST.Services {
		newSvcs[newAST.Services[i].Name] = &newAST.Services[i]
	}
	for name, oldSvc := range oldSvcs {
		newSvc, exists := newSvcs[name]
		if !exists {
			r.breaking = append(r.breaking, fmt.Sprintf("REMOVED service %q", name))
			r.hasBreaking = true
			continue
		}
		oldMethods := map[string]ast.Method{}
		for _, m := range oldSvc.Methods {
			oldMethods[m.Name] = m
		}
		newMethods := map[string]ast.Method{}
		for _, m := range newSvc.Methods {
			newMethods[m.Name] = m
		}
		for mName := range oldMethods {
			if _, ok := newMethods[mName]; !ok {
				r.breaking = append(r.breaking, fmt.Sprintf("REMOVED method %q.%q", name, mName))
				r.hasBreaking = true
			}
		}
		for mName := range newMethods {
			if _, ok := oldMethods[mName]; !ok {
				r.safe = append(r.safe, fmt.Sprintf("ADDED method %q.%q (backward compatible)", name, mName))
			}
		}
	}
	return r
}

func printCompatReport(r compatReport, oldPath, newPath string) {
	fmt.Printf("\n📋 Helix Schema Compatibility Report\n")
	fmt.Printf("   %s → %s\n\n", filepath.Base(oldPath), filepath.Base(newPath))

	if len(r.safe) == 0 && len(r.breaking) == 0 {
		fmt.Println("  ✅ No schema changes detected.")
		return
	}

	for _, s := range r.safe {
		fmt.Printf("  ✅ %s\n", s)
	}
	for _, b := range r.breaking {
		fmt.Printf("  ✗  BREAKING: %s\n", b)
	}
	fmt.Println()

	if r.hasBreaking {
		fmt.Println("  ⚠️  Schema has BREAKING changes. Clients must be updated before deploying.")
	} else {
		fmt.Println("  ✅ Schema is BACKWARD COMPATIBLE. Safe to deploy.")
	}
	fmt.Println()
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func parseIDL(path string) (*ast.AST, error) {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	switch filepath.Ext(path) {
	case ".proto":
		return parser.ParseProto(string(content))
	case ".thrift":
		return parser.ParseThrift(string(content))
	default:
		return nil, fmt.Errorf("unsupported extension %q", filepath.Ext(path))
	}
}

func writeFile(path, content string) {
	if err := ioutil.WriteFile(path, []byte(content), 0644); err != nil {
		log.Fatalf("cannot write %s: %v", path, err)
	}
}

func printError(msg string) {
	fmt.Fprintf(os.Stderr, "\n✗ helix-gen: %s\n\n", msg)
}
