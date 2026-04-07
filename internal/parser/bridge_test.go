package parser

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cheryl-chun/confgen/internal/tree"
)

func TestParseToTree_BasicYAML(t *testing.T) {
	// Create a temporary YAML file
	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "config.yaml")

	yamlContent := `
server:
  host: localhost
  port: 8080
  timeout: 30

database:
  host: db.example.com
  port: 5432

debug: true
`

	if err := os.WriteFile(yamlFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test: Parse YAML to tree
	configTree, err := ParseToTree(yamlFile, tree.SourceFile)
	if err != nil {
		t.Fatalf("ParseToTree() error = %v", err)
	}

	// Verify the tree structure
	cases := []struct {
		path          string
		expectedValue any
		expectedType  tree.ValueType
	}{
		{"server.host", "localhost", tree.TypeString},
		{"server.port", 8080, tree.TypeInt},
		{"server.timeout", 30, tree.TypeInt},
		{"database.host", "db.example.com", tree.TypeString},
		{"database.port", 5432, tree.TypeInt},
		{"debug", true, tree.TypeBool},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			// Get value
			value, exists := configTree.GetValue(tc.path)
			if !exists {
				t.Fatalf("Value for %q should exist", tc.path)
			}

			if value != tc.expectedValue {
				t.Errorf("Value = %v (%T), want %v (%T)", value, value, tc.expectedValue, tc.expectedValue)
			}

			// Get node to check type
			node := configTree.Get(tc.path)
			if node == nil {
				t.Fatalf("Node for %q should exist", tc.path)
			}

			if node.Type != tc.expectedType {
				t.Errorf("Type = %v, want %v", node.Type, tc.expectedType)
			}

			// Verify source is set correctly
			values := node.GetAllValues()
			if len(values) == 0 {
				t.Fatal("Node should have at least one value")
			}

			if values[0].Source != tree.SourceFile {
				t.Errorf("Source = %v, want %v", values[0].Source, tree.SourceFile)
			}
		})
	}
}

func TestParseToTree_JSON(t *testing.T) {
	tmpDir := t.TempDir()
	jsonFile := filepath.Join(tmpDir, "config.json")

	jsonContent := `{
		"server": {
			"host": "localhost",
			"port": 8080
		},
		"enabled": true
	}`

	if err := os.WriteFile(jsonFile, []byte(jsonContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	configTree, err := ParseToTree(jsonFile, tree.SourceFile)
	if err != nil {
		t.Fatalf("ParseToTree() error = %v", err)
	}

	// Verify
	host, exists := configTree.GetValue("server.host")
	if !exists || host != "localhost" {
		t.Errorf("server.host = %v, want 'localhost'", host)
	}

	port, exists := configTree.GetValue("server.port")
	if !exists {
		t.Fatal("server.port should exist")
	}

	// JSON numbers are float64
	portFloat, ok := port.(float64)
	if !ok || portFloat != 8080 {
		t.Errorf("server.port = %v (%T), want 8080", port, port)
	}
}

func TestParseToTree_Array(t *testing.T) {
	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "config.yaml")

	yamlContent := `
features:
  - ssl
  - cache
  - compression

servers:
  - host: server1.com
    port: 8001
  - host: server2.com
    port: 8002
`

	if err := os.WriteFile(yamlFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	configTree, err := ParseToTree(yamlFile, tree.SourceFile)
	if err != nil {
		t.Fatalf("ParseToTree() error = %v", err)
	}

	// Test string array
	features, exists := configTree.GetValue("features")
	if !exists {
		t.Fatal("features should exist")
	}

	featuresArray, ok := features.([]any)
	if !ok {
		t.Fatalf("features should be an array, got %T", features)
	}

	if len(featuresArray) != 3 {
		t.Errorf("features length = %d, want 3", len(featuresArray))
	}

	if featuresArray[0] != "ssl" {
		t.Errorf("features[0] = %v, want 'ssl'", featuresArray[0])
	}

	// Test object array
	servers, exists := configTree.GetValue("servers")
	if !exists {
		t.Fatal("servers should exist")
	}

	serversArray, ok := servers.([]any)
	if !ok {
		t.Fatalf("servers should be an array, got %T", servers)
	}

	if len(serversArray) != 2 {
		t.Errorf("servers length = %d, want 2", len(serversArray))
	}

	// Check first server
	server1, ok := serversArray[0].(map[string]any)
	if !ok {
		t.Fatalf("servers[0] should be a map, got %T", serversArray[0])
	}

	if server1["host"] != "server1.com" {
		t.Errorf("servers[0].host = %v, want 'server1.com'", server1["host"])
	}
}

func TestParseToTree_MultiSource(t *testing.T) {
	// This test demonstrates the multi-source capability
	// Load the same config from two different sources

	tmpDir := t.TempDir()
	file1 := filepath.Join(tmpDir, "default.yaml")
	file2 := filepath.Join(tmpDir, "override.yaml")

	defaultContent := `
server:
  host: localhost
  port: 8080
`

	overrideContent := `
server:
  host: prod.example.com
`

	os.WriteFile(file1, []byte(defaultContent), 0644)
	os.WriteFile(file2, []byte(overrideContent), 0644)

	// Load default config
	configTree, err := ParseToTree(file1, tree.SourceDefault)
	if err != nil {
		t.Fatalf("ParseToTree() error = %v", err)
	}

	// Load override config and merge
	overrideTree, err := ParseToTree(file2, tree.SourceFile)
	if err != nil {
		t.Fatalf("ParseToTree() error = %v", err)
	}

	configTree.Merge(overrideTree, tree.SourceFile)

	// Verify priority: File should override Default
	host, _ := configTree.GetValue("server.host")
	if host != "prod.example.com" {
		t.Errorf("server.host = %v, want 'prod.example.com' (File should win)", host)
	}

	// Port should still come from default
	port, _ := configTree.GetValue("server.port")
	if port != 8080 {
		t.Errorf("server.port = %v, want 8080", port)
	}

	// Check sources
	hostNode := configTree.Get("server.host")
	values := hostNode.GetAllValues()

	if len(values) != 2 {
		t.Fatalf("server.host should have 2 sources, got %d", len(values))
	}

	// First value should be from File (higher priority)
	if values[0].Source != tree.SourceFile {
		t.Errorf("Highest priority source = %v, want SourceFile", values[0].Source)
	}

	if values[0].Value != "prod.example.com" {
		t.Errorf("File source value = %v, want 'prod.example.com'", values[0].Value)
	}

	// Second value should be from Default
	if values[1].Source != tree.SourceDefault {
		t.Errorf("Second source = %v, want SourceDefault", values[1].Source)
	}

	if values[1].Value != "localhost" {
		t.Errorf("Default source value = %v, want 'localhost'", values[1].Value)
	}
}

func TestParseToTree_NestedStructure(t *testing.T) {
	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "config.yaml")

	yamlContent := `
app:
  backend:
    api:
      v1:
        endpoint: /api/v1
        timeout: 30
      v2:
        endpoint: /api/v2
        timeout: 60
`

	if err := os.WriteFile(yamlFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	configTree, err := ParseToTree(yamlFile, tree.SourceFile)
	if err != nil {
		t.Fatalf("ParseToTree() error = %v", err)
	}

	// Test deeply nested paths
	endpoint, exists := configTree.GetValue("app.backend.api.v1.endpoint")
	if !exists || endpoint != "/api/v1" {
		t.Errorf("app.backend.api.v1.endpoint = %v, want '/api/v1'", endpoint)
	}

	timeout, exists := configTree.GetValue("app.backend.api.v2.timeout")
	if !exists || timeout != 60 {
		t.Errorf("app.backend.api.v2.timeout = %v, want 60", timeout)
	}
}
