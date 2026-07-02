package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/helix-rpc/helix/compiler/ast"
	"github.com/helix-rpc/helix/compiler/codegen"
	"github.com/helix-rpc/helix/compiler/compat"
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
	disableMetrics := fs.Bool("disable-metrics", false, "Disable Prometheus metrics endpoint")
	disableHealth := fs.Bool("disable-health", false, "Disable built-in health checking")
	disableGzip := fs.Bool("disable-gzip", false, "Disable response gzip compression")
	disableDeadline := fs.Bool("disable-deadline", false, "Disable deadline propagation")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: helix-gen init <service-name> [--lang go|rust|python] [--disable-metrics] [--disable-health] [--disable-gzip] [--disable-deadline]")
	}

	var serviceName string
	var flagArgs []string
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "-") {
			flagArgs = append(flagArgs, args[i])
			if (args[i] == "-lang" || args[i] == "--lang") && i+1 < len(args) {
				flagArgs = append(flagArgs, args[i+1])
				i++
			}
		} else {
			if serviceName == "" {
				serviceName = args[i]
			} else {
				flagArgs = append(flagArgs, args[i])
			}
		}
	}

	fs.Parse(flagArgs) //nolint:errcheck

	if serviceName == "" {
		fs.Usage()
		printError("service name is required")
		os.Exit(1)
	}

	if err := scaffold(serviceName, *lang, *disableMetrics, *disableHealth, *disableGzip, *disableDeadline); err != nil {
		log.Fatal(err)
	}
}

func pyBool(val bool) string {
	if val {
		return "True"
	}
	return "False"
}

func scaffold(name, lang string, disableMetrics, disableHealth, disableGzip, disableDeadline bool) error {
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

.PHONY: gen build test dev docker-build deploy-aws deploy-gcp deploy-azure

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

docker-build:
	@echo "🐳 Building optimized Docker container..."
	docker build -t $(SERVICE):latest -f containers/Dockerfile.$(LANG) .

deploy-aws:
	@echo "☁️ Deploying to AWS Fargate..."
	cd deployments && terraform init && terraform apply -target=aws_ecs_service.helix_service -auto-approve

deploy-gcp:
	@echo "☁️ Deploying to GCP Cloud Run..."
	cd deployments && terraform init && terraform apply -target=google_cloud_run_service.helix_service -auto-approve

deploy-azure:
	@echo "☁️ Deploying to Azure Container Apps..."
	cd deployments && terraform init && terraform apply -target=azurerm_container_app.helix_app -auto-approve
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

	// helix.json
	writeFile(filepath.Join(base, "helix.json"), fmt.Sprintf(`{
  "host": "127.0.0.1",
  "port": 8080,
  "disable_metrics": %t,
  "disable_health": %t,
  "disable_gzip": %t,
  "disable_deadline": %t,
  "rate_limit_rate": 100.0,
  "rate_limit_burst": 10
}
`, disableMetrics, disableHealth, disableGzip, disableDeadline))

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
	cfg, err := runtime.LoadConfig("helix.json")
	if err != nil {
		log.Printf("⚠️  Could not load helix.json (using defaults): %%v", err)
		cfg = &runtime.Config{
			Host: "127.0.0.1",
			Port: 8080,
		}
	}

	server := runtime.NewServerWithConfig(fmt.Sprintf("%%s:%%d", cfg.Host, cfg.Port), runtime.ServerConfig{
		DisableMetrics: cfg.DisableMetrics,
		DisableHealth:  cfg.DisableHealth,
	})

	// Register your service methods here
	server.RegisterMethod("/v1.ModelService/Predict", runtime.MethodInfo{
		Handler: func(ctx context.Context, req interface{}) (interface{}, error) {
			return map[string]string{"completion": "Hello from %s!"}, nil
		},
	})
	server.RegisterRESTRoute("POST", "/predict", "/v1.ModelService/Predict")

	runtime.WatchConfig("helix.json", func(newCfg *runtime.Config) {
		log.Printf("🔄 [Helix] Loaded new configs: metrics_disabled=%%t", newCfg.DisableMetrics)
	})

	log.Printf("🚀 %s listening on %%s:%%d", cfg.Host, cfg.Port)
	log.Fatal(server.Start())
}
`, name, name))

	case "python":
		os.MkdirAll(filepath.Join(base, "generated"), 0755) //nolint:errcheck
		writeFile(filepath.Join(base, "server.py"), fmt.Sprintf(`import sys
sys.path.insert(0, ".")

import logging
from helix_rt.server import HelixServer
from helix_rt.config import load_config, watch_config

try:
    cfg = load_config("helix.json")
except Exception:
    from helix_rt.config import Config
    cfg = Config()

server = HelixServer(
    host=cfg.host,
    port=cfg.port,
    disable_metrics=cfg.disable_metrics,
    disable_health=cfg.disable_health,
    disable_gzip=cfg.disable_gzip,
    disable_deadline=cfg.disable_deadline,
)

async def predict(body: dict) -> dict:
    prompt = body.get("prompt", "")
    return {"completion": f"Hello from %s! You said: {prompt}"}

server.register_route("POST", "/predict", predict)

def handle_reload(new_cfg):
    logging.info("🧬 [Helix] Config reloaded dynamically.")

watch_config("helix.json", handle_reload)

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
serde = { version = "1.0", features = ["derive"] }
`, name))
		writeFile(filepath.Join(base, "src", "main.rs"), fmt.Sprintf(`use helix_rt::server::{HttpServiceHandler, RestRoute, HelixServer, ServerConfig};
use helix_rt::config::{load_config, watch_config};
use std::sync::Arc;

#[tokio::main]
async fn main() {
    let cfg = load_config("helix.json").unwrap_or_default();
    println!("🚀 %s listening on {}:{}", cfg.host, cfg.port);
    
    // Dynamic config reloader registration
    watch_config("helix.json".to_string(), |new_cfg| {
        println!("🧬 [Helix] Dynamic config changed: metrics_disabled={}", new_cfg.disable_metrics);
    });

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

	report := compat.DiffASTs(oldAST, newAST)
	printCompatReport(report, oldPath, newPath)
	if report.HasBreaking {
		os.Exit(2) // non-zero so CI pipelines fail on breaking changes
	}
}

func printCompatReport(r compat.CompatReport, oldPath, newPath string) {
	fmt.Printf("\n📋 Helix Schema Compatibility Report\n")
	fmt.Printf("   %s → %s\n\n", filepath.Base(oldPath), filepath.Base(newPath))

	if len(r.Safe) == 0 && len(r.Breaking) == 0 {
		fmt.Println("  ✅ No schema changes detected.")
		return
	}

	for _, s := range r.Safe {
		fmt.Printf("  ✅ %s\n", s)
	}
	for _, b := range r.Breaking {
		fmt.Printf("  ✗  BREAKING: %s\n", b)
	}
	fmt.Println()

	if r.HasBreaking {
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
