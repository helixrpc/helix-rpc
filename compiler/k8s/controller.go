package k8s

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// HelixSchemaSpec holds the target language and IDL content for the controller to compile.
type HelixSchemaSpec struct {
	IDLContent      string `json:"idlContent"`
	Language        string `json:"language"`
	OutputDirectory string `json:"outputDirectory"`
}

// HelixSchema represents the Custom Resource object.
type HelixSchema struct {
	Name      string          `json:"name"`
	Namespace string          `json:"namespace"`
	Spec      HelixSchemaSpec `json:"spec"`
}

// EventType represents the type of Kubernetes resource watch event.
type EventType string

const (
	EventAdded   EventType = "ADDED"
	EventUpdated EventType = "UPDATED"
	EventDeleted EventType = "DELETED"
)

// SchemaEvent is a watch event returned by Kubernetes.
type SchemaEvent struct {
	Type   EventType
	Object HelixSchema
}

// CompilerInterface abstraction to invoke the actual helix-gen compilation loop.
type CompilerInterface interface {
	Compile(idlPath, language, outDir string) error
}

// SchemaController manages the watch controller loop for HelixSchema resources.
type SchemaController struct {
	mu         sync.Mutex
	compiler   CompilerInterface
	eventsChan chan SchemaEvent
	cancel     context.CancelFunc
	running    bool
	Logger     func(string, ...interface{})
}

// NewSchemaController creates a new instance of the K8s watch controller.
func NewSchemaController(compiler CompilerInterface) *SchemaController {
	return &SchemaController{
		compiler:   compiler,
		eventsChan: make(chan SchemaEvent, 100),
		Logger:     func(format string, args ...interface{}) {},
	}
}

// PublishEvent pushes a mock Kubernetes resource event into the controller's watch stream.
func (c *SchemaController) PublishEvent(event SchemaEvent) {
	c.eventsChan <- event
}

// Start starts the background controller processing loop.
func (c *SchemaController) Start(ctx context.Context) {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return
	}
	c.running = true
	runCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	c.mu.Unlock()

	go c.runLoop(runCtx)
}

// Stop terminates the controller processing loop.
func (c *SchemaController) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.running {
		return
	}
	c.running = false
	if c.cancel != nil {
		c.cancel()
	}
}

func (c *SchemaController) runLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-c.eventsChan:
			if !ok {
				return
			}
			c.processEvent(event)
		}
	}
}

func (c *SchemaController) processEvent(event SchemaEvent) {
	switch event.Type {
	case EventAdded, EventUpdated:
		c.Logger("HelixSchema %s/%s reconciliation started...", event.Object.Namespace, event.Object.Name)
		err := c.reconcile(event.Object)
		if err != nil {
			c.Logger("Error reconciling HelixSchema %s: %v", event.Object.Name, err)
		} else {
			c.Logger("HelixSchema %s/%s successfully compiled!", event.Object.Namespace, event.Object.Name)
		}
	case EventDeleted:
		c.Logger("HelixSchema %s/%s deleted. Cleaning up generated targets...", event.Object.Namespace, event.Object.Name)
	}
}

func (c *SchemaController) reconcile(schema HelixSchema) error {
	// Create temporary workspace to dump IDL content
	tempDir, err := os.MkdirTemp("", "helix-k8s-")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	idlFile := filepath.Join(tempDir, "schema.proto")
	if err := os.WriteFile(idlFile, []byte(schema.Spec.IDLContent), 0644); err != nil {
		return fmt.Errorf("failed to write IDL temp file: %w", err)
	}

	// Invoke compile
	err = c.compiler.Compile(idlFile, schema.Spec.Language, schema.Spec.OutputDirectory)
	if err != nil {
		return fmt.Errorf("compiler invocation failed: %w", err)
	}

	return nil
}
