package go_go

import (
	"bufio"
	"net"
	"os/exec"
	"regexp"
	"testing"
	"time"

	generated "github.com/helix-rpc/helix/integration-tests/go-go/generated"
	"github.com/helix-rpc/helix/runtime-go"
)

func TestCrossLangGoClientRustServer(t *testing.T) {
	// 1. Build the Rust server
	buildCmd := exec.Command("cargo", "build")
	buildCmd.Dir = "../rust-rust"
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build Rust server: %v", err)
	}

	// 2. Start the Rust server
	runCmd := exec.Command("cargo", "run", "--", "--server")
	runCmd.Dir = "../rust-rust"
	stdout, err := runCmd.StdoutPipe()
	if err != nil {
		t.Fatalf("failed to get stdout pipe: %v", err)
	}

	if err := runCmd.Start(); err != nil {
		t.Fatalf("failed to start Rust server: %v", err)
	}
	defer func() {
		_ = runCmd.Process.Kill()
	}()

	// 3. Read the port from stdout
	reader := bufio.NewReader(stdout)
	var addr string
	portRegex := regexp.MustCompile(`listening on (127.0.0.1:\d+)`)

	// Give it up to 5 seconds to print the address
	lineChan := make(chan string, 1)
	go func() {
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				break
			}
			if portRegex.MatchString(line) {
				matches := portRegex.FindStringSubmatch(line)
				if len(matches) > 1 {
					lineChan <- matches[1]
					break
				}
			}
		}
	}()

	select {
	case addr = <-lineChan:
		t.Logf("Found Rust server address: %s", addr)
	case <-time.After(5 * time.Second):
		t.Fatalf("timeout waiting for Rust server to print port")
	}

	// 4. Run Go Thrift Client calls against Rust server
	t.Run("Thrift-Compact", func(t *testing.T) {
		req := &generated.UserProfile{
			UserID:   777,
			Username: "go-caller",
			Email:    "caller@go.com",
		}
		resp, err := callThrift(addr, true, req)
		if err != nil {
			t.Fatalf("thrift compact cross-lang call failed: %v", err)
		}
		if resp.UserID != 777 || resp.Username != "go-caller-response" || resp.Email != "caller@go.com-verified" {
			t.Errorf("unexpected response from Rust server: %+v", resp)
		}
	})

	t.Run("Thrift-Binary", func(t *testing.T) {
		req := &generated.UserProfile{
			UserID:   888,
			Username: "go-caller-bin",
			Email:    "caller-bin@go.com",
		}
		resp, err := callThrift(addr, false, req)
		if err != nil {
			t.Fatalf("thrift binary cross-lang call failed: %v", err)
		}
		if resp.UserID != 888 || resp.Username != "go-caller-bin-response" || resp.Email != "caller-bin@go.com-verified" {
			t.Errorf("unexpected response from Rust server: %+v", resp)
		}
	})
}

func TestCrossLangRustClientGoServer(t *testing.T) {
	// Start Go server on dynamic port
	server := runtime.NewServer("127.0.0.1:0")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	server.Addr = addr
	serviceImpl := &myUserProfileService{}
	server.RegisterService("helix_example.UserProfileService", serviceImpl)
	server.RegisterThriftProcessor(generated.NewUserProfileServiceProcessor(serviceImpl))

	go func() {
		if err := server.Start(); err != nil {
			panic(err)
		}
	}()

	// Wait for server to start
	time.Sleep(200 * time.Millisecond)

	// Execute Rust client against the Go server
	cmd := exec.Command("cargo", "run", "--", "--client", addr)
	cmd.Dir = "../rust-rust"
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Rust client E2E test failed: %v\nOutput: %s", err, string(output))
	}
	t.Logf("Rust client output:\n%s", string(output))
}
