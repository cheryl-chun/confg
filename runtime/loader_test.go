package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cheryl-chun/confgen/internal/tree"
)

type testSource struct {
	loadFunc     func(configTree *tree.ConfigTree) error
	priorityType tree.SourceType
}

func (s *testSource) Load(configTree *tree.ConfigTree) error {
	if s.loadFunc == nil {
		return nil
	}
	return s.loadFunc(configTree)
}

func (s *testSource) Priority() tree.SourceType {
	return s.priorityType
}

type testConfig struct {
	Server struct {
		Host   string    `json:"host"`
		Port   int       `json:"port"`
		Rate   float64   `json:"rate"`
		Debug  bool      `json:"debug"`
		Labels []string  `json:"labels"`
		Scores []float64 `json:"scores"`
	} `json:"server"`
	Servers []testServer `json:"servers"`

	ConfigTree *tree.ConfigTree
}

type testServer struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type hotReloadConfig struct {
	Server struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	} `json:"server"`

	ConfigTree *Tree
}

func TestLoader_Fill_ParameterValidation(t *testing.T) {
	loader := NewLoader()

	if err := loader.Fill(nil); err == nil {
		t.Fatal("expected error for nil config")
	}

	nonPtr := testConfig{}
	if err := loader.Fill(nonPtr); err == nil {
		t.Fatal("expected error for non-pointer config")
	}

	var nilPtr *testConfig
	if err := loader.Fill(nilPtr); err == nil {
		t.Fatal("expected error for nil pointer config")
	}
}

func TestLoader_Fill_SuccessWithStructAndConfigTree(t *testing.T) {
	loader := NewLoader()

	loader.AddSource(&testSource{
		priorityType: tree.SourceFile,
		loadFunc: func(configTree *tree.ConfigTree) error {
			if err := configTree.Set("server.host", "localhost", tree.SourceFile, tree.TypeString); err != nil {
				return err
			}
			if err := configTree.Set("server.port", 8080, tree.SourceFile, tree.TypeInt); err != nil {
				return err
			}
			if err := configTree.Set("server.rate", 1.5, tree.SourceFile, tree.TypeFloat); err != nil {
				return err
			}
			if err := configTree.Set("server.debug", true, tree.SourceFile, tree.TypeBool); err != nil {
				return err
			}
			if err := configTree.Set("server.labels", []interface{}{"api", "prod"}, tree.SourceFile, tree.TypeArray); err != nil {
				return err
			}
			if err := configTree.Set("server.scores", []interface{}{9.5, 8.0}, tree.SourceFile, tree.TypeArray); err != nil {
				return err
			}
			return nil
		},
	})

	cfg := &testConfig{}
	if err := loader.Fill(cfg); err != nil {
		t.Fatalf("Fill() error = %v", err)
	}

	if cfg.Server.Host != "localhost" {
		t.Fatalf("Host = %q, want %q", cfg.Server.Host, "localhost")
	}
	if cfg.Server.Port != 8080 {
		t.Fatalf("Port = %d, want %d", cfg.Server.Port, 8080)
	}
	if cfg.Server.Rate != 1.5 {
		t.Fatalf("Rate = %v, want %v", cfg.Server.Rate, 1.5)
	}
	if cfg.Server.Debug != true {
		t.Fatalf("Debug = %v, want %v", cfg.Server.Debug, true)
	}

	if len(cfg.Server.Labels) != 2 || cfg.Server.Labels[0] != "api" || cfg.Server.Labels[1] != "prod" {
		t.Fatalf("Labels = %#v, want [api prod]", cfg.Server.Labels)
	}

	if len(cfg.Server.Scores) != 2 || cfg.Server.Scores[0] != 9.5 || cfg.Server.Scores[1] != 8.0 {
		t.Fatalf("Scores = %#v, want [9.5 8.0]", cfg.Server.Scores)
	}

	if cfg.ConfigTree == nil {
		t.Fatal("ConfigTree should be set by loader")
	}

	node := cfg.ConfigTree.Get("server.host")
	if node == nil || node.GetValue() != "localhost" {
		t.Fatal("ConfigTree should contain server.host")
	}
}

func TestLoader_Fill_PropagatesSourceError(t *testing.T) {
	loader := NewLoader()
	loader.AddSource(&testSource{
		priorityType: tree.SourceFile,
		loadFunc: func(configTree *tree.ConfigTree) error {
			return fmt.Errorf("mock load error")
		},
	})

	cfg := &testConfig{}
	err := loader.Fill(cfg)
	if err == nil {
		t.Fatal("expected error when source load fails")
	}
}

func TestLoader_Fill_ObjectArray(t *testing.T) {
	loader := NewLoader()

	loader.AddSource(&testSource{
		priorityType: tree.SourceFile,
		loadFunc: func(configTree *tree.ConfigTree) error {
			servers := []interface{}{
				map[string]interface{}{"host": "server1.example.com", "port": 9001},
				map[string]interface{}{"host": "server2.example.com", "port": 9002},
			}
			return configTree.Set("servers", servers, tree.SourceFile, tree.TypeArray)
		},
	})

	cfg := &testConfig{}
	if err := loader.Fill(cfg); err != nil {
		t.Fatalf("Fill() error = %v", err)
	}

	if len(cfg.Servers) != 2 {
		t.Fatalf("len(Servers) = %d, want 2", len(cfg.Servers))
	}

	if cfg.Servers[0].Host != "server1.example.com" || cfg.Servers[0].Port != 9001 {
		t.Fatalf("Servers[0] = %#v, want host=server1.example.com port=9001", cfg.Servers[0])
	}

	if cfg.Servers[1].Host != "server2.example.com" || cfg.Servers[1].Port != 9002 {
		t.Fatalf("Servers[1] = %#v, want host=server2.example.com port=9002", cfg.Servers[1])
	}
}

func TestLoader_StartHotReload(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	initial := "server:\n  host: localhost\n  port: 8080\n"
	updated := "server:\n  host: hot.example.com\n  port: 9090\n"

	if err := os.WriteFile(configPath, []byte(initial), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	loader := NewLoader().AddFile(configPath)
	cfg := &hotReloadConfig{}

	stop, err := loader.StartHotReload(cfg)
	if err != nil {
		t.Fatalf("StartHotReload() error = %v", err)
	}
	defer func() {
		if stopErr := stop(); stopErr != nil {
			t.Fatalf("stop() error = %v", stopErr)
		}
	}()

	if cfg.Server.Host != "localhost" || cfg.Server.Port != 8080 {
		t.Fatalf("initial cfg = %#v, want host=localhost port=8080", cfg.Server)
	}

	events := make(chan WatchEvent, 2)
	unwatch := cfg.ConfigTree.Watch("server.host", func(event WatchEvent) {
		events <- event
	})
	defer unwatch()

	// Sleep briefly to avoid editors coalescing writes with the initial file timestamp.
	time.Sleep(250 * time.Millisecond)
	if err := os.WriteFile(configPath, []byte(updated), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	select {
	case event := <-events:
		if event.NewValue != "hot.example.com" {
			t.Fatalf("event.NewValue = %v, want hot.example.com", event.NewValue)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for hot reload event")
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cfg.Server.Host == "hot.example.com" && cfg.Server.Port == 9090 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("cfg after hot reload = %#v, want host=hot.example.com port=9090", cfg.Server)
}
