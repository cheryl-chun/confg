package main

import (
	"fmt"
	"time"

	"github.com/cheryl-chun/confgen/runtime"
)

func main() {
	cfg := &Config{}

	loader := runtime.NewLoader().AddFile("config.yaml")
	if err := loader.Fill(cfg); err != nil {
		panic(err)
	}

	fmt.Printf("loaded config: server=%s:%d db=%s:%d\n", cfg.Server.Host, cfg.Server.Port, cfg.Database.Host, cfg.Database.Port)

	unwatch := cfg.Watch("server.host", func(event runtime.WatchEvent) {
		fmt.Printf("[watch] %s changed: %v -> %v (source=%s)\n", event.Path, event.OldValue, event.NewValue, event.Source)
	})
	defer unwatch()

	_ = cfg.Set("server.host", "dev.local", runtime.SourceRuntimeOverride)
	_ = cfg.Set("server.host", "prod.example.com", runtime.SourceSystemEnv)
	_ = cfg.Set("server.host", "ignored-default", runtime.SourceDefault)

	// Give the async watch dispatcher a short moment to process events.
	time.Sleep(1500 * time.Millisecond)

	fmt.Printf("effective server.host = %v\n", cfg.Get("server.host"))
}
