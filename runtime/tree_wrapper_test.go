package runtime

import (
	"testing"
	"time"
)

func TestTree_Set_InferPrimitiveTypes(t *testing.T) {
	tree := wrapTree(NewLoader().GetTree())
	defer tree.Close()

	if err := tree.Set("app.name", "demo", SourceFile); err != nil {
		t.Fatalf("Set string error = %v", err)
	}
	if err := tree.Set("app.port", 8080, SourceFile); err != nil {
		t.Fatalf("Set int error = %v", err)
	}
	if err := tree.Set("app.ratio", 1.25, SourceFile); err != nil {
		t.Fatalf("Set float error = %v", err)
	}
	if err := tree.Set("app.enabled", true, SourceFile); err != nil {
		t.Fatalf("Set bool error = %v", err)
	}

	if got := tree.GetString("app.name"); got != "demo" {
		t.Fatalf("GetString = %q, want %q", got, "demo")
	}
	if got := tree.GetInt("app.port"); got != 8080 {
		t.Fatalf("GetInt = %d, want %d", got, 8080)
	}
	if got := tree.GetFloat("app.ratio"); got != 1.25 {
		t.Fatalf("GetFloat = %v, want %v", got, 1.25)
	}
	if got := tree.GetBool("app.enabled"); !got {
		t.Fatal("GetBool should be true")
	}
}

func TestTree_Set_InferArrayAndObject(t *testing.T) {
	tree := wrapTree(NewLoader().GetTree())
	defer tree.Close()

	value := map[string]any{
		"hosts": []string{"a", "b"},
		"meta":  map[string]any{"env": "dev"},
	}
	if err := tree.Set("server", value, SourceFile); err != nil {
		t.Fatalf("Set object error = %v", err)
	}

	got, ok := tree.GetValue("server")
	if !ok {
		t.Fatal("server should exist")
	}
	obj, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("server value type = %T, want map[string]any", got)
	}
	if obj["meta"].(map[string]any)["env"] != "dev" {
		t.Fatalf("server.meta.env = %v, want dev", obj["meta"].(map[string]any)["env"])
	}
}

func TestTree_Watch_WithPublicEvent(t *testing.T) {
	tree := wrapTree(NewLoader().GetTree())
	defer tree.Close()

	events := make(chan WatchEvent, 2)
	tree.Watch("server.host", func(event WatchEvent) {
		events <- event
	})

	if err := tree.Set("server.host", "localhost", SourceFile); err != nil {
		t.Fatalf("Set error = %v", err)
	}

	select {
	case event := <-events:
		if event.Path != "server.host" {
			t.Fatalf("event.Path = %s, want server.host", event.Path)
		}
		if event.NewValue != "localhost" {
			t.Fatalf("event.NewValue = %v, want localhost", event.NewValue)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("timed out waiting for watch event")
	}
}
