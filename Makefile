.PHONY: build test fmt clean

build:
	@echo "🛠 Building Helix IDL Compiler..."
	cd compiler && go build -o helix-gen . && cp helix-gen ../helix-gen
	@echo "🛠 Building Envoy Wasm Filter..."
	cd extensions/envoy-filter && cargo build --target wasm32-unknown-unknown --release

test:
	@chmod +x ./test.sh
	./test.sh

fmt:
	@echo "🧹 Formatting Go files..."
	for d in compiler runtimes/go tests/go-go tests/go-python-ai examples/go-dynamic-batcher examples/go-resilience; do (cd $$d && go fmt ./...); done
	@echo "🧹 Formatting Rust files..."
	for d in runtimes/rust tests/rust-rust extensions/envoy-filter; do (cd $$d && cargo fmt); done

clean:
	@echo "🧼 Cleaning compilation outputs and Rust target directories..."
	rm -f helix-gen compiler/helix-gen
	rm -rf runtimes/rust/target tests/rust-rust/target extensions/envoy-filter/target
