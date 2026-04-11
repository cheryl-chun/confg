package tree

import (
	"testing"
	"time"
)

func TestConfigTree_SetAndGet(t *testing.T) {
	cases := []struct {
		name       string
		path       string
		value      any
		source     SourceType
		valueType  ValueType
		wantExists bool
	}{
		{
			name:       "set simple string",
			path:       "server.host",
			value:      "localhost",
			source:     SourceFile,
			valueType:  TypeString,
			wantExists: true,
		},
		{
			name:       "set integer",
			path:       "server.port",
			value:      8080,
			source:     SourceFile,
			valueType:  TypeInt,
			wantExists: true,
		},
		{
			name:       "set deeply nested",
			path:       "app.database.connection.host",
			value:      "db.example.com",
			source:     SourceFile,
			valueType:  TypeString,
			wantExists: true,
		},
		{
			name:       "set boolean",
			path:       "debug",
			value:      true,
			source:     SourceSystemEnv,
			valueType:  TypeBool,
			wantExists: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tree := NewConfigTree()

			// Set value
			err := tree.Set(tc.path, tc.value, tc.source, tc.valueType)
			if err != nil {
				t.Fatalf("Set() error = %v", err)
			}

			// Get value
			got, exists := tree.GetValue(tc.path)
			if exists != tc.wantExists {
				t.Errorf("GetValue() exists = %v, want %v", exists, tc.wantExists)
			}

			if got != tc.value {
				t.Errorf("GetValue() = %v, want %v", got, tc.value)
			}

			// Get node
			node := tree.Get(tc.path)
			if node == nil {
				t.Fatal("Get() returned nil node")
			}

			if node.Type != tc.valueType {
				t.Errorf("Node type = %v, want %v", node.Type, tc.valueType)
			}
		})
	}
}

func TestConfigTree_GetNonExistent(t *testing.T) {
	tree := NewConfigTree()

	cases := []struct {
		name string
		path string
	}{
		{"non-existent path", "server.host"},
		{"partial path", "server.database.host"},
		{"empty path", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node := tree.Get(tc.path)
			if node != nil {
				t.Errorf("Get(%q) should return nil for non-existent path", tc.path)
			}

			value, exists := tree.GetValue(tc.path)
			if exists {
				t.Errorf("GetValue(%q) should return exists=false", tc.path)
			}
			if value != nil {
				t.Errorf("GetValue(%q) should return nil value", tc.path)
			}
		})
	}
}

func TestConfigTree_MultiSourcePriority(t *testing.T) {
	cases := []struct {
		name          string
		path          string
		setOperations []struct {
			value  any
			source SourceType
		}
		wantValue  any
		wantSource SourceType
	}{
		{
			name: "env overrides file",
			path: "server.host",
			setOperations: []struct {
				value  any
				source SourceType
			}{
				{"file.com", SourceFile},
				{"env.com", SourceSystemEnv},
			},
			wantValue:  "env.com",
			wantSource: SourceSystemEnv,
		},
		{
			name: "code override wins",
			path: "debug",
			setOperations: []struct {
				value  any
				source SourceType
			}{
				{false, SourceDefault},
				{false, SourceFile},
				{true, SourceSystemEnv},
				{false, SourceCodeOverride},
			},
			wantValue:  false,
			wantSource: SourceCodeOverride,
		},
		{
			name: "system env > session env",
			path: "api.key",
			setOperations: []struct {
				value  any
				source SourceType
			}{
				{"session-key", SourceSessionEnv},
				{"system-key", SourceSystemEnv},
			},
			wantValue:  "system-key",
			wantSource: SourceSystemEnv,
		},
		{
			name: "file > remote",
			path: "timeout",
			setOperations: []struct {
				value  any
				source SourceType
			}{
				{30, SourceRemote},
				{60, SourceFile},
			},
			wantValue:  60,
			wantSource: SourceFile,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tree := NewConfigTree()

			// Perform all set operations
			for _, op := range tc.setOperations {
				err := tree.Set(tc.path, op.value, op.source, TypeString)
				if err != nil {
					t.Fatalf("Set() error = %v", err)
				}
			}

			// Get final value
			got, exists := tree.GetValue(tc.path)
			if !exists {
				t.Fatal("GetValue() should exist")
			}

			if got != tc.wantValue {
				t.Errorf("GetValue() = %v, want %v", got, tc.wantValue)
			}

			// Verify source
			node := tree.Get(tc.path)
			if node == nil {
				t.Fatal("Get() returned nil")
			}

			values := node.GetAllValues()
			if len(values) == 0 {
				t.Fatal("Node has no values")
			}

			if values[0].Source != tc.wantSource {
				t.Errorf("Highest priority source = %v, want %v", values[0].Source, tc.wantSource)
			}
		})
	}
}

func TestConfigTree_Merge(t *testing.T) {
	cases := []struct {
		name           string
		setupBaseTree  func() *ConfigTree
		setupMergeTree func() *ConfigTree
		mergeSource    SourceType
		checkPath      string
		expectedValue  any
		expectedSource SourceType
	}{
		{
			name: "merge remote into file config",
			setupBaseTree: func() *ConfigTree {
				tree := NewConfigTree()
				tree.Set("server.host", "localhost", SourceFile, TypeString)
				tree.Set("server.port", 8080, SourceFile, TypeInt)
				return tree
			},
			setupMergeTree: func() *ConfigTree {
				tree := NewConfigTree()
				tree.Set("server.host", "remote.com", SourceRemote, TypeString)
				tree.Set("server.timeout", 30, SourceRemote, TypeInt)
				return tree
			},
			mergeSource:    SourceRemote,
			checkPath:      "server.host",
			expectedValue:  "localhost", // File should win over Remote
			expectedSource: SourceFile,
		},
		{
			name: "merge adds new keys",
			setupBaseTree: func() *ConfigTree {
				tree := NewConfigTree()
				tree.Set("server.host", "localhost", SourceFile, TypeString)
				return tree
			},
			setupMergeTree: func() *ConfigTree {
				tree := NewConfigTree()
				tree.Set("database.host", "db.com", SourceRemote, TypeString)
				return tree
			},
			mergeSource:    SourceRemote,
			checkPath:      "database.host",
			expectedValue:  "db.com",
			expectedSource: SourceRemote,
		},
		{
			name: "merge env overrides file",
			setupBaseTree: func() *ConfigTree {
				tree := NewConfigTree()
				tree.Set("debug", false, SourceFile, TypeBool)
				return tree
			},
			setupMergeTree: func() *ConfigTree {
				tree := NewConfigTree()
				tree.Set("debug", true, SourceSystemEnv, TypeBool)
				return tree
			},
			mergeSource:    SourceSystemEnv,
			checkPath:      "debug",
			expectedValue:  true, // Env should win over File
			expectedSource: SourceSystemEnv,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			baseTree := tc.setupBaseTree()
			mergeTree := tc.setupMergeTree()

			// Merge
			baseTree.Merge(mergeTree, tc.mergeSource)

			// Check result
			got, exists := baseTree.GetValue(tc.checkPath)
			if !exists {
				t.Fatalf("GetValue(%q) should exist after merge", tc.checkPath)
			}

			if got != tc.expectedValue {
				t.Errorf("After merge, value = %v, want %v", got, tc.expectedValue)
			}

			// Check source priority
			node := baseTree.Get(tc.checkPath)
			if node == nil {
				t.Fatal("Node should exist after merge")
			}

			values := node.GetAllValues()
			if len(values) == 0 {
				t.Fatal("Node should have values after merge")
			}

			if values[0].Source != tc.expectedSource {
				t.Errorf("Highest priority source = %v, want %v", values[0].Source, tc.expectedSource)
			}
		})
	}
}

func TestConfigTree_GetAllWithPrefix(t *testing.T) {
	tree := NewConfigTree()
	tree.Set("server.host", "localhost", SourceFile, TypeString)
	tree.Set("server.port", 8080, SourceFile, TypeInt)
	tree.Set("server.timeout", 30, SourceFile, TypeInt)
	tree.Set("database.host", "db.com", SourceFile, TypeString)

	cases := []struct {
		name         string
		prefix       string
		expectedKeys []string
	}{
		{
			name:   "get all server configs",
			prefix: "server",
			expectedKeys: []string{
				"server.host",
				"server.port",
				"server.timeout",
			},
		},
		{
			name:   "get database configs",
			prefix: "database",
			expectedKeys: []string{
				"database.host",
			},
		},
		{
			name:         "non-existent prefix",
			prefix:       "cache",
			expectedKeys: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := tree.GetAllWithPrefix(tc.prefix)

			if tc.expectedKeys == nil {
				if result != nil {
					t.Errorf("GetAllWithPrefix(%q) should return nil", tc.prefix)
				}
				return
			}

			if len(result) != len(tc.expectedKeys) {
				t.Errorf("Result count = %d, want %d", len(result), len(tc.expectedKeys))
			}

			for _, key := range tc.expectedKeys {
				if _, ok := result[key]; !ok {
					t.Errorf("Expected key %q not found in result", key)
				}
			}
		})
	}
}

func TestConfigTree_Walk(t *testing.T) {
	tree := NewConfigTree()
	tree.Set("server.host", "localhost", SourceFile, TypeString)
	tree.Set("server.port", 8080, SourceFile, TypeInt)
	tree.Set("database.host", "db.com", SourceFile, TypeString)
	tree.Set("debug", true, SourceFile, TypeBool)

	visited := make(map[string]bool)
	expectedPaths := []string{
		"server",
		"server.host",
		"server.port",
		"database",
		"database.host",
		"debug",
	}

	tree.Walk(func(path string, node *ConfigNode) {
		visited[path] = true
	})

	// Check that all expected paths were visited
	for _, path := range expectedPaths {
		if !visited[path] {
			t.Errorf("Path %q was not visited", path)
		}
	}

	// Check that we didn't visit extra paths
	if len(visited) != len(expectedPaths) {
		t.Errorf("Visited %d paths, expected %d", len(visited), len(expectedPaths))
	}
}

func TestConfigTree_ToMap(t *testing.T) {
	tree := NewConfigTree()
	tree.Set("server.host", "localhost", SourceFile, TypeString)
	tree.Set("server.port", 8080, SourceFile, TypeInt)
	tree.Set("database.host", "db.com", SourceFile, TypeString)
	tree.Set("debug", true, SourceFile, TypeBool)

	result := tree.ToMap()

	// Check server section
	server, ok := result["server"].(map[string]any)
	if !ok {
		t.Fatal("server should be a map")
	}

	if server["host"] != "localhost" {
		t.Errorf("server.host = %v, want 'localhost'", server["host"])
	}

	if server["port"] != 8080 {
		t.Errorf("server.port = %v, want 8080", server["port"])
	}

	// Check database section
	database, ok := result["database"].(map[string]any)
	if !ok {
		t.Fatal("database should be a map")
	}

	if database["host"] != "db.com" {
		t.Errorf("database.host = %v, want 'db.com'", database["host"])
	}

	// Check debug
	if result["debug"] != true {
		t.Errorf("debug = %v, want true", result["debug"])
	}
}

func TestConfigTree_ComplexScenario(t *testing.T) {
	// Simulate a real-world configuration loading scenario
	tree := NewConfigTree()

	// 1. Load defaults
	tree.Set("server.host", "localhost", SourceDefault, TypeString)
	tree.Set("server.port", 8080, SourceDefault, TypeInt)
	tree.Set("debug", false, SourceDefault, TypeBool)

	// 2. Load config file
	tree.Set("server.host", "dev.example.com", SourceFile, TypeString)
	tree.Set("server.timeout", 30, SourceFile, TypeInt)

	// 3. Load remote config
	tree.Set("server.timeout", 60, SourceRemote, TypeInt)

	// 4. Apply environment variables
	tree.Set("server.host", "prod.example.com", SourceSystemEnv, TypeString)
	tree.Set("debug", true, SourceSystemEnv, TypeBool)

	// 5. Code override
	tree.Set("debug", false, SourceCodeOverride, TypeBool)

	// Verify final configuration
	cases := []struct {
		path          string
		expectedValue any
		expectedSrc   SourceType
	}{
		{"server.host", "prod.example.com", SourceSystemEnv}, // Env wins
		{"server.port", 8080, SourceDefault},                 // Only default set
		{"server.timeout", 30, SourceFile},                   // File > Remote
		{"debug", false, SourceCodeOverride},                 // Code override wins
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			got, exists := tree.GetValue(tc.path)
			if !exists {
				t.Fatalf("GetValue(%q) should exist", tc.path)
			}

			if got != tc.expectedValue {
				t.Errorf("Value = %v, want %v", got, tc.expectedValue)
			}

			node := tree.Get(tc.path)
			values := node.GetAllValues()
			if values[0].Source != tc.expectedSrc {
				t.Errorf("Source = %v, want %v", values[0].Source, tc.expectedSrc)
			}
		})
	}
}

func TestConfigTree_SetByPath(t *testing.T) {
	cases := []struct {
		name      string
		path      []string
		value     any
		source    SourceType
		valueType ValueType
		wantError bool
	}{
		{
			name:      "valid path array",
			path:      []string{"server", "host"},
			value:     "localhost",
			source:    SourceFile,
			valueType: TypeString,
			wantError: false,
		},
		{
			name:      "empty path",
			path:      []string{},
			value:     "value",
			source:    SourceFile,
			valueType: TypeString,
			wantError: true,
		},
		{
			name:      "path with empty strings",
			path:      []string{"server", "", "host"},
			value:     "localhost",
			source:    SourceFile,
			valueType: TypeString,
			wantError: false, // Empty strings are skipped
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tree := NewConfigTree()
			err := tree.SetByPath(tc.path, tc.value, tc.source, tc.valueType)

			if tc.wantError {
				if err == nil {
					t.Error("SetByPath() should return error")
				}
			} else {
				if err != nil {
					t.Errorf("SetByPath() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestConfigTree_Watch_TriggersOnValueChange(t *testing.T) {
	configTree := NewConfigTree()
	defer configTree.Close()

	events := make(chan WatchEvent, 4)
	configTree.Watch("server.host", func(event WatchEvent) {
		events <- event
	})

	if err := configTree.Set("server.host", "localhost", SourceFile, TypeString); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	first := waitEvent(t, events)
	if first.OldValue != nil || first.NewValue != "localhost" {
		t.Fatalf("first event = %#v, want old=nil new=localhost", first)
	}

	if err := configTree.Set("server.host", "127.0.0.1", SourceFile, TypeString); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	second := waitEvent(t, events)
	if second.OldValue != "localhost" || second.NewValue != "127.0.0.1" {
		t.Fatalf("second event = %#v, want old=localhost new=127.0.0.1", second)
	}
}

func TestConfigTree_Watch_NoEventWhenEffectiveValueUnchanged(t *testing.T) {
	configTree := NewConfigTree()
	defer configTree.Close()

	events := make(chan WatchEvent, 4)
	configTree.Watch("server.host", func(event WatchEvent) {
		events <- event
	})

	if err := configTree.Set("server.host", "localhost", SourceFile, TypeString); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	_ = waitEvent(t, events)

	// Same value from same source should not trigger.
	if err := configTree.Set("server.host", "localhost", SourceFile, TypeString); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	assertNoEvent(t, events)

	// Lower priority source should not change effective value.
	if err := configTree.Set("server.host", "default-host", SourceDefault, TypeString); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	assertNoEvent(t, events)
}

func TestConfigTree_Watch_Unsubscribe(t *testing.T) {
	configTree := NewConfigTree()
	defer configTree.Close()

	events := make(chan WatchEvent, 2)
	unwatch := configTree.Watch("debug", func(event WatchEvent) {
		events <- event
	})

	if err := configTree.Set("debug", true, SourceFile, TypeBool); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	_ = waitEvent(t, events)

	unwatch()

	if err := configTree.Set("debug", false, SourceFile, TypeBool); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	assertNoEvent(t, events)
}

func waitEvent(t *testing.T, events <-chan WatchEvent) WatchEvent {
	t.Helper()

	select {
	case event := <-events:
		return event
	case <-time.After(300 * time.Millisecond):
		t.Fatal("timed out waiting for watch event")
		return WatchEvent{}
	}
}

func assertNoEvent(t *testing.T, events <-chan WatchEvent) {
	t.Helper()

	select {
	case event := <-events:
		t.Fatalf("unexpected event: %#v", event)
	case <-time.After(120 * time.Millisecond):
	}
}

func TestConfigTree_ReplaceSource(t *testing.T) {
	configTree := NewConfigTree()
	defer configTree.Close()

	if err := configTree.Set("server.host", "localhost", SourceFile, TypeString); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if err := configTree.Set("server.port", 8080, SourceFile, TypeInt); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if err := configTree.Set("server.host", "env.example.com", SourceSystemEnv, TypeString); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	snapshot := NewConfigTree()
	defer snapshot.Close()
	if err := snapshot.Set("server.host", "reloaded.local", SourceFile, TypeString); err != nil {
		t.Fatalf("snapshot Set() error = %v", err)
	}
	if err := snapshot.Set("server.timeout", 30, SourceFile, TypeInt); err != nil {
		t.Fatalf("snapshot Set() error = %v", err)
	}

	configTree.ReplaceSource(snapshot, SourceFile)

	host, ok := configTree.GetValue("server.host")
	if !ok || host != "env.example.com" {
		t.Fatalf("server.host = %v, want env.example.com", host)
	}

	timeout, ok := configTree.GetValue("server.timeout")
	if !ok || timeout != 30 {
		t.Fatalf("server.timeout = %v, want 30", timeout)
	}

	portNode := configTree.Get("server.port")
	if portNode == nil {
		t.Fatal("server.port node should still exist")
	}
	if _, ok := portNode.GetValueFromSource(SourceFile); ok {
		t.Fatal("server.port file source value should be removed after replace")
	}
	if _, ok := configTree.GetValue("server.port"); ok {
		t.Fatal("server.port should no longer have effective value")
	}
}
