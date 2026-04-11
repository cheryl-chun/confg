package runtime

import (
	"os"
	"testing"

	"github.com/cheryl-chun/confgen/internal/tree"
)

func TestEnvSource_Load_WithPrefix(t *testing.T) {
	t.Setenv("RUNTIME_TEST_SERVER_HOST", "127.0.0.1")
	t.Setenv("RUNTIME_TEST_SERVER_PORT", "9090")
	t.Setenv("UNRELATED_VAR", "ignore-me")

	configTree := tree.NewConfigTree()
	source := &EnvSource{Prefix: "RUNTIME_TEST_"}

	if err := source.Load(configTree); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	hostNode := configTree.Get("server.host")
	if hostNode == nil {
		t.Fatal("server.host should exist")
	}

	if hostNode.Type != tree.TypeString {
		t.Fatalf("server.host type = %v, want %v", hostNode.Type, tree.TypeString)
	}

	if hostNode.GetValue() != "127.0.0.1" {
		t.Fatalf("server.host value = %v, want %v", hostNode.GetValue(), "127.0.0.1")
	}

	if _, ok := hostNode.GetValueFromSource(tree.SourceSystemEnv); !ok {
		t.Fatal("server.host should record SourceSystemEnv")
	}

	if configTree.Get("unrelated.var") != nil {
		t.Fatal("variables without prefix should be ignored")
	}
}

func TestEnvSource_EnvKeyToPath(t *testing.T) {
	source := &EnvSource{}
	path := source.envKeyToPath("APP_DATABASE_MAX_CONNECTIONS")
	if path != "app.database.max.connections" {
		t.Fatalf("envKeyToPath() = %q, want %q", path, "app.database.max.connections")
	}
}

func TestEnvSource_Priority(t *testing.T) {
	source := &EnvSource{}
	if source.Priority() != tree.SourceSystemEnv {
		t.Fatalf("Priority() = %v, want %v", source.Priority(), tree.SourceSystemEnv)
	}
}

func TestEnvSource_Load_HandlesInvalidEnvEntryGracefully(t *testing.T) {
	_ = os.Setenv("RUNTIME_TEST_BROKEN", "ok")

	configTree := tree.NewConfigTree()
	source := &EnvSource{Prefix: "RUNTIME_TEST_"}
	if err := source.Load(configTree); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
}
