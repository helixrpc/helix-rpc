#!/bin/bash
set -e

# setup_microvm.sh
# Statically compiles a Helix service and packages it into a minimal Firecracker rootfs.
# The service runs directly as the kernel's PID 1 (init) for maximum speed and isolation.

SERVICE_DIR=$1
LANG=${2:-go}

if [ -z "$SERVICE_DIR" ]; then
    echo "Usage: ./setup_microvm.sh <service-directory> [go|rust]"
    exit 1
fi

echo "📦 Packaging $SERVICE_DIR into a Firecracker microVM..."

# 1. Compile static binary
cd "$SERVICE_DIR"
if [ "$LANG" = "go" ]; then
    echo "🔨 Compiling static Go binary..."
    CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags "-s -w" -o server server/main.go
elif [ "$LANG" = "rust" ]; then
    echo "🔨 Compiling static Rust binary (musl)..."
    rustup target add x86_64-unknown-linux-musl || true
    cargo build --target x86_64-unknown-linux-musl --release
    cp target/x86_64-unknown-linux-musl/release/*-service server
else
    echo "Unsupported language for microVM static compilation."
    exit 1
fi
cd -

# 2. Create minimal ext4 disk image (50MB)
echo "💾 Creating rootfs.ext4 disk image..."
dd if=/dev/zero of=firecracker/rootfs.ext4 bs=1M count=50
mkfs.ext4 firecracker/rootfs.ext4

# 3. Mount disk image and copy binaries/configs
echo "🔌 Mounting and copying binaries..."
mkdir -p firecracker/mnt
sudo mount firecracker/rootfs.ext4 firecracker/mnt

# Copy compiled static server as /server (init process)
sudo cp "$SERVICE_DIR/server" firecracker/mnt/server
# Copy configuration file
sudo cp "$SERVICE_DIR/helix.json" firecracker/mnt/helix.json

# Clean up mount
sudo umount firecracker/mnt
rm -rf firecracker/mnt

echo "✅ Created firecracker/rootfs.ext4 successfully."
echo "🚀 To run the microVM:"
echo "   1. Download kernel: curl -fsSL -o firecracker/vmlinux https://s3.amazonaws.com/spec.ccfc.min/firecracker-kernels/vmlinux-5.10.0"
echo "   2. Start Firecracker: firecracker --config-file firecracker/config.json"
