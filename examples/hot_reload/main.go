package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cheryl-chun/confgen/runtime"
)

func main() {
	cfg := &Config{}
	loader := runtime.NewLoader().AddFile("config.yaml")

	// StartHotReload 先执行初始 Fill，然后启动后台监听。
	// 返回的 stop 函数用于优雅退出：关闭 fsnotify watcher 并等待 goroutine 结束。
	stop, err := loader.StartHotReload(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "start hot reload failed: %v\n", err)
		os.Exit(1)
	}

	// 注册 Watch 回调：当文件里的值变化后，热更新会通过 ReplaceSource
	// 触发 effective value 比对，只有实际生效的值发生变化才会回调。
	cfg.Watch("app.log_level", func(e runtime.WatchEvent) {
		fmt.Printf("\n[watch] log_level: %v → %v\n\n", e.OldValue, e.NewValue)
	})
	cfg.Watch("server.port", func(e runtime.WatchEvent) {
		fmt.Printf("\n[watch] server.port: %v → %v\n\n", e.OldValue, e.NewValue)
	})
	cfg.Watch("feature.enable_cache", func(e runtime.WatchEvent) {
		fmt.Printf("\n[watch] feature.enable_cache: %v → %v\n\n", e.OldValue, e.NewValue)
	})

	fmt.Println("Watching config.yaml for changes. Edit the file and save to see hot reload in action.")
	fmt.Println("Press Ctrl+C to exit.")
	fmt.Println()

	printConfig(cfg)

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-ticker.C:
			printConfig(cfg)
		case <-quit:
			fmt.Println("\nShutting down...")
			done := make(chan struct{})
			go func() {
				_ = stop()
				close(done)
			}()

			select {
			case <-done:
			case <-time.After(2 * time.Second):
				fmt.Println("force exit after stop timeout")
			}
			return
		}
	}
}

func printConfig(cfg *Config) {
	fmt.Printf("[config] app=%s log=%s debug=%v | server=%s:%d timeout=%ds | cache=%v max=%d\n",
		cfg.App.Name,
		cfg.App.LogLevel,
		cfg.App.Debug,
		cfg.Server.Host,
		cfg.Server.Port,
		cfg.Server.TimeoutSeconds,
		cfg.Feature.EnableCache,
		cfg.Feature.MaxCacheSize,
	)
}
