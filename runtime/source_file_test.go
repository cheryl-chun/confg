package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cheryl-chun/confgen/internal/tree"
)

func TestFileSource_Load_Success(t *testing.T) {
	content := `
server:
  host: localhost
  port: 8080
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	configTree := tree.NewConfigTree()
	source := &FileSource{Path: configPath}

	if err := source.Load(configTree); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	hostNode := configTree.Get("server.host")
	if hostNode == nil || hostNode.GetValue() != "localhost" {
		t.Fatalf("server.host = %v, want %v", valueOrNil(hostNode), "localhost")
	}

	portNode := configTree.Get("server.port")
	if portNode == nil || portNode.GetValue() != 8080 {
		t.Fatalf("server.port = %v, want %v", valueOrNil(portNode), 8080)
	}
}

func TestFileSource_Load_FileNotFound(t *testing.T) {
	configTree := tree.NewConfigTree()
	source := &FileSource{Path: "/path/not/exist/config.yaml"}

	err := source.Load(configTree)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestFileSource_Priority(t *testing.T) {
	source := &FileSource{}
	if source.Priority() != tree.SourceFile {
		t.Fatalf("Priority() = %v, want %v", source.Priority(), tree.SourceFile)
	}
}

func valueOrNil(node *tree.ConfigNode) any {
	if node == nil {
		return nil
	}
	return node.GetValue()
}
