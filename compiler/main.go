package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/helix-rpc/helix/compiler/ast"
	"github.com/helix-rpc/helix/compiler/codegen"
	"github.com/helix-rpc/helix/compiler/compat"
	"github.com/helix-rpc/helix/compiler/linter"
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
  helix-gen lint      Lint a schema for Helix style compliance

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
	case "lint":
		runLint(os.Args[2:])
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
	case "node", "typescript":
		generated, err = codegen.GenerateNode(parsed)
	default:
		printError(fmt.Sprintf("unsupported language %q", lang))
		fmt.Fprintln(os.Stderr, "  Supported languages: go, rust, python, node")
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
	lang := fs.String("lang", "go", "Primary language (go, rust, python, node)")
	db := fs.String("db", "none", "Scaffold database configuration (mysql, nosql, none)")
	disableMetrics := fs.Bool("disable-metrics", false, "Disable Prometheus metrics endpoint")
	disableHealth := fs.Bool("disable-health", false, "Disable built-in health checking")
	disableGzip := fs.Bool("disable-gzip", false, "Disable response gzip compression")
	disableDeadline := fs.Bool("disable-deadline", false, "Disable deadline propagation")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: helix-gen init <service-name> [--lang go|rust|python|node] [--db mysql|nosql|none] [--disable-metrics] [--disable-health] [--disable-gzip] [--disable-deadline]")
	}

	var serviceName string
	var flagArgs []string
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "-") {
			flagArgs = append(flagArgs, args[i])
			if (args[i] == "-lang" || args[i] == "--lang" || args[i] == "-db" || args[i] == "--db") && i+1 < len(args) {
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

	if serviceName == "" && len(flagArgs) == 0 {
		fmt.Printf("\n✨ Welcome to the Helix RPC Project Scaffolding Wizard!\n\n")
		fmt.Print("? Enter service name: ")
		fmt.Scanln(&serviceName)
		serviceName = strings.TrimSpace(serviceName)
		if serviceName == "" {
			printError("service name is required")
			os.Exit(1)
		}

		fmt.Print("? Choose target language (go, rust, python, node) [go]: ")
		var l string
		fmt.Scanln(&l)
		l = strings.TrimSpace(l)
		if l != "" {
			*lang = l
		}

		fmt.Print("? Choose database configuration (mysql, nosql, none) [none]: ")
		var d string
		fmt.Scanln(&d)
		d = strings.TrimSpace(d)
		if d != "" {
			*db = d
		}
	}

	if serviceName == "" {
		fs.Usage()
		printError("service name is required")
		os.Exit(1)
	}

	if err := scaffold(serviceName, *lang, *db, *disableMetrics, *disableHealth, *disableGzip, *disableDeadline); err != nil {
		log.Fatal(err)
	}
}

func pyBool(val bool) string {
	if val {
		return "True"
	}
	return "False"
}

func scaffold(name, lang, dbType string, disableMetrics, disableHealth, disableGzip, disableDeadline bool) error {
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
	helix-gen generate -idl schema.proto -lang $(LANG) -out generated/generated.$(if $(filter go,$(LANG)),go,$(if $(filter rust,$(LANG)),rs,$(if $(filter node,$(LANG)),ts,py)))

watch:
	helix-gen generate -idl schema.proto -lang $(LANG) -out generated/generated.$(if $(filter go,$(LANG)),go,$(if $(filter rust,$(LANG)),rs,$(if $(filter node,$(LANG)),ts,py))) --watch

build: gen
	$(if $(filter go,$(LANG)),go build ./...,$(if $(filter rust,$(LANG)),cargo build,$(if $(filter node,$(LANG)),npm run build,echo "Python: no build step required")))

test:
	$(if $(filter go,$(LANG)),go test ./...,$(if $(filter rust,$(LANG)),cargo test,$(if $(filter node,$(LANG)),npm test,pytest tests/)))

dev: gen
	@echo "🚀 Starting $(SERVICE) in dev mode..."
	$(if $(filter go,$(LANG)),go run server/main.go,$(if $(filter rust,$(LANG)),cargo run,$(if $(filter node,$(LANG)),npm run dev,python server.py)))

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
  "rate_limit_burst": 10,
  "enable_jwt_auth": false,
  "jwt_secret": "default-secret-key",
  "enable_api_key_auth": false,
  "api_key": "default-api-key"
}
`, disableMetrics, disableHealth, disableGzip, disableDeadline))

	// Language-specific server entrypoint
	switch lang {
	case "go":
		os.MkdirAll(filepath.Join(base, "server"), 0755)    //nolint:errcheck
		os.MkdirAll(filepath.Join(base, "generated"), 0755) //nolint:errcheck
		goModDeps := "require github.com/helix-rpc/helix/runtime-go v0.2.0\n"
		if dbType == "mysql" {
			goModDeps += "require github.com/go-sql-driver/mysql v1.8.1\n"
			writeFile(filepath.Join(base, "server", "database.go"), `package main
import (
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
)
func ConnectDB() (*sql.DB, error) {
	return sql.Open("mysql", "root:secret@tcp(127.0.0.1:3306)/dbname")
}
`)
		} else if dbType == "nosql" {
			goModDeps += "require github.com/redis/go-redis/v9 v9.7.0\n"
			writeFile(filepath.Join(base, "server", "database.go"), `package main
import "github.com/redis/go-redis/v9"
func ConnectNoSQL() *redis.Client {
	return redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"})
}
`)
		}
		writeFile(filepath.Join(base, "go.mod"), fmt.Sprintf("module github.com/example/%s\n\ngo 1.24\n\n%s", name, goModDeps))

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
		if dbType == "mysql" {
			writeFile(filepath.Join(base, "database.py"), `import aiomysql
async def connect_db():
    return await aiomysql.create_pool(host='127.0.0.1', port=3306, user='root', password='', db='dbname')
`)
		} else if dbType == "nosql" {
			writeFile(filepath.Join(base, "database.py"), `import redis.asyncio as redis
def connect_nosql():
    return redis.Redis(host='127.0.0.1', port=6379)
`)
		}
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
		rustDeps := "helix-rt = \"0.1.0\"\ntokio = { version = \"1.0\", features = [\"full\"] }\nserde_json = \"1.0\"\nserde = { version = \"1.0\", features = [\"derive\"] }\n"
		if dbType == "mysql" {
			rustDeps += "sqlx = { version = \"0.7\", features = [\"runtime-tokio-native-tls\", \"mysql\"] }\n"
			writeFile(filepath.Join(base, "src", "database.rs"), `use sqlx::mysql::MySqlPool;
pub async fn connect_db() -> Result<MySqlPool, sqlx::Error> {
	MySqlPool::connect("mysql://root:secret@127.0.0.1:3306/dbname").await
}
`)
		} else if dbType == "nosql" {
			rustDeps += "redis = \"0.25\"\n"
			writeFile(filepath.Join(base, "src", "database.rs"), `use redis::Client;
pub fn connect_nosql() -> Result<Client, redis::RedisError> {
	Client::open("redis://127.0.0.1:6379")
}
`)
		}
		writeFile(filepath.Join(base, "Cargo.toml"), fmt.Sprintf(`[package]
name = "%s"
version = "0.1.0"
edition = "2021"

[dependencies]
%s`, name, rustDeps))

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

	case "node", "typescript":
		os.MkdirAll(filepath.Join(base, "src"), 0755)       //nolint:errcheck
		os.MkdirAll(filepath.Join(base, "generated"), 0755) //nolint:errcheck
		nodeDeps := `"helix-rt-node": "file:../../runtimes/node"`
		if dbType == "mysql" {
			nodeDeps += ",\n    \"mysql2\": \"^3.9.0\""
			writeFile(filepath.Join(base, "src", "database.ts"), `import mysql from 'mysql2/promise';
export async function connectDB() {
    return mysql.createPool({ host: '127.0.0.1', user: 'root', database: 'dbname' });
}
`)
		} else if dbType == "nosql" {
			nodeDeps += ",\n    \"ioredis\": \"^5.3.2\""
			writeFile(filepath.Join(base, "src", "database.ts"), `import Redis from 'ioredis';
export function connectNoSQL() {
    return new Redis('redis://127.0.0.1:6379');
}
`)
		}

		writeFile(filepath.Join(base, "package.json"), fmt.Sprintf(`{
  "name": "%s",
  "version": "1.0.0",
  "type": "module",
  "scripts": {
    "test": "tsc && node dist/test.js",
    "dev": "tsc && node dist/server.js"
  },
  "devDependencies": {
    "typescript": "^5.3.3",
    "@types/node": "^20.0.0"
  },
  "dependencies": {
    %s
  }
}
`, name, nodeDeps))

		writeFile(filepath.Join(base, "tsconfig.json"), `{
  "compilerOptions": {
    "target": "ES2022",
    "module": "NodeNext",
    "moduleResolution": "NodeNext",
    "declaration": true,
    "outDir": "./dist",
    "strict": true,
    "skipLibCheck": true
  },
  "include": ["src/**/*"]
}
`)

		writeFile(filepath.Join(base, "src", "server.ts"), `import { HelixServer } from 'helix-rt-node';
const server = new HelixServer('127.0.0.1:8080');
console.log("Starting Node.js Helix server...");
await server.start();
`)
	}

	fmt.Printf("\n✅ Scaffolded %q (%s)\n", name, lang)

	// Run auto-dependency tidy post-scaffolding
	fmt.Printf("📦 Running automatic package configuration and dependency setup...\n")
	if lang == "go" {
		cmd := exec.Command("go", "mod", "tidy")
		cmd.Dir = base
		_ = cmd.Run()
	} else if lang == "rust" {
		cmd := exec.Command("cargo", "check")
		cmd.Dir = base
		_ = cmd.Run()
	} else if lang == "node" {
		cmd := exec.Command("npm", "install")
		cmd.Dir = base
		_ = cmd.Run()
	}

	fmt.Printf("\n  Next steps:\n")
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

// ---------------------------------------------------------------------------
// lint subcommand
// ---------------------------------------------------------------------------

func runLint(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: helix-gen lint <schema.proto|schema.thrift>")
		os.Exit(1)
	}
	path := args[0]

	parsed, err := parseIDL(path)
	if err != nil {
		printError(fmt.Sprintf("cannot parse %s: %v", path, err))
		os.Exit(1)
	}

	errors := linter.Lint(parsed)
	if len(errors) > 0 {
		fmt.Printf("\n📋 Helix Linter Style Violations in %s:\n", filepath.Base(path))
		for _, e := range errors {
			fmt.Printf("  ✗  %s\n", e)
		}
		fmt.Println()
		os.Exit(1)
	}

	fmt.Printf("✅ Schema %s matches Helix style guidelines!\n", filepath.Base(path))
}
