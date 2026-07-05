package k8s

import (
	"context"
	"sync"
	"testing"
	"time"
)

type mockCompiler struct {
	mu       sync.Mutex
	compiled []compileCall
}

type compileCall struct {
	idlPath  string
	language string
	outDir   string
}

func (m *mockCompiler) Compile(idlPath, language, outDir string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.compiled = append(m.compiled, compileCall{
		idlPath:  idlPath,
		language: language,
		outDir:   outDir,
	})
	return nil
}

func TestSchemaControllerReconciliation(t *testing.T) {
	mc := &mockCompiler{}
	controller := NewSchemaController(mc)

	logChan := make(chan string, 10)
	controller.Logger = func(format string, args ...interface{}) {
		logChan <- "logged"
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	controller.Start(ctx)
	defer controller.Stop()

	// Publish ADDED event
	schema := HelixSchema{
		Name:      "test-schema",
		Namespace: "default",
		Spec: HelixSchemaSpec{
			IDLContent:      "syntax = \"proto3\"; message Ping {}",
			Language:        "rust",
			OutputDirectory: "/tmp/gen-rust",
		},
	}

	controller.PublishEvent(SchemaEvent{
		Type:   EventAdded,
		Object: schema,
	})

	// Wait for reconciliation to complete
	select {
	case <-logChan:
		// First log: reconciliation started
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for start log")
	}

	select {
	case <-logChan:
		// Second log: successfully compiled!
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for compile log")
	}

	mc.mu.Lock()
	defer mc.mu.Unlock()
	if len(mc.compiled) != 1 {
		t.Fatalf("expected 1 compile call, got %d", len(mc.compiled))
	}

	call := mc.compiled[0]
	if call.language != "rust" {
		t.Errorf("expected language 'rust', got '%s'", call.language)
	}
	if call.outDir != "/tmp/gen-rust" {
		t.Errorf("expected outDir '/tmp/gen-rust', got '%s'", call.outDir)
	}
}
