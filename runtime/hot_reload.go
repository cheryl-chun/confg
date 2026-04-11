package runtime

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cheryl-chun/confgen/internal/tree"
	"github.com/fsnotify/fsnotify"
)

// StartHotReload watches file sources and refreshes the loaded config when files change.
// It performs an initial Fill, then listens for file-system events and reapplies file-source
// snapshots into the shared config tree. Effective value changes continue to flow through the
// existing tree watch callbacks.
func (l *Loader) StartHotReload(cfg interface{}) (func() error, error) {
	if err := l.Fill(cfg); err != nil {
		return nil, err
	}

	reloader, err := newFileHotReloader(l, cfg)
	if err != nil {
		return nil, err
	}

	return reloader.start()
}

type fileHotReloader struct {
	loader      *Loader
	cfg         interface{}
	fileSources []*FileSource
	paths       map[string]struct{}
	dirs        map[string]struct{}
	baseNames   map[string]struct{}
	watcher     *fsnotify.Watcher
	mu          sync.Mutex
}

func newFileHotReloader(loader *Loader, cfg interface{}) (*fileHotReloader, error) {
	fileSources := collectFileSources(loader.sources)
	if len(fileSources) == 0 {
		return nil, fmt.Errorf("hot reload requires at least one file source")
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	paths, dirs, baseNames, err := normalizeHotReloadPaths(fileSources)
	if err != nil {
		_ = watcher.Close()
		return nil, err
	}

	for dir := range dirs {
		if err := watcher.Add(dir); err != nil {
			_ = watcher.Close()
			return nil, fmt.Errorf("failed to watch directory %s: %w", dir, err)
		}
	}

	return &fileHotReloader{
		loader:      loader,
		cfg:         cfg,
		fileSources: fileSources,
		paths:       paths,
		dirs:        dirs,
		baseNames:   baseNames,
		watcher:     watcher,
	}, nil
}

func (r *fileHotReloader) start() (func() error, error) {
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		r.loop(ctx)
	}()

	stop := func() error {
		cancel()
		// Close the watcher first so watcher.Events is closed.
		// This gives the loop goroutine a second exit path (!ok case)
		// in addition to ctx.Done(), ensuring wg.Wait() is never stuck.
		err := r.watcher.Close()
		wg.Wait()
		return err
	}
	return stop, nil
}

func (r *fileHotReloader) loop(ctx context.Context) {
	const debounce = 200 * time.Millisecond
	var timer *time.Timer
	var timerC <-chan time.Time

	stopTimer := func() {
		if timer != nil {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer = nil
			timerC = nil
		}
	}

	schedule := func() {
		if timer == nil {
			timer = time.NewTimer(debounce)
			timerC = timer.C
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(debounce)
	}
	defer stopTimer()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timerC:
			stopTimer()
			_ = r.reload()
		case event, ok := <-r.watcher.Events:
			if !ok {
				return
			}
			if !shouldHandleHotReloadEvent(event) {
				continue
			}
			if shouldScheduleHotReload(event.Name, r.paths, r.dirs, r.baseNames) {
				schedule()
			}
		case <-r.watcher.Errors:
			// Keep hot reload alive on transient watcher errors.
		}
	}
}

func (r *fileHotReloader) reload() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	fileTree := tree.NewConfigTree()
	defer fileTree.Close()

	for _, source := range r.fileSources {
		parsed, err := source.parseTree()
		if err != nil {
			return err
		}
		fileTree.ReplaceSource(parsed, tree.SourceFile)
		parsed.Close()
	}

	r.loader.tree.ReplaceSource(fileTree, tree.SourceFile)
	return r.loader.applyTreeToConfig(r.cfg)
}

func collectFileSources(sources []Source) []*FileSource {
	result := make([]*FileSource, 0)
	for _, source := range sources {
		if fileSource, ok := source.(*FileSource); ok {
			result = append(result, fileSource)
		}
	}
	return result
}

func normalizeHotReloadPaths(fileSources []*FileSource) (map[string]struct{}, map[string]struct{}, map[string]struct{}, error) {
	paths := make(map[string]struct{}, len(fileSources))
	dirs := make(map[string]struct{}, len(fileSources))
	baseNames := make(map[string]struct{}, len(fileSources))
	for _, source := range fileSources {
		absPath, err := filepath.Abs(source.Path)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to resolve path %s: %w", source.Path, err)
		}
		cleanPath := filepath.Clean(absPath)
		paths[cleanPath] = struct{}{}
		dirs[filepath.Dir(cleanPath)] = struct{}{}
		baseNames[filepath.Base(cleanPath)] = struct{}{}
	}
	return paths, dirs, baseNames, nil
}

func shouldHandleHotReloadEvent(event fsnotify.Event) bool {
	return event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Rename) || event.Has(fsnotify.Remove) || event.Has(fsnotify.Chmod)
}

func shouldScheduleHotReload(eventName string, paths, dirs, baseNames map[string]struct{}) bool {
	if eventName == "" {
		return false
	}

	clean := filepath.Clean(eventName)
	if _, ok := paths[clean]; ok {
		return true
	}

	dir := filepath.Dir(clean)
	if _, ok := dirs[dir]; !ok {
		return false
	}

	base := filepath.Base(clean)
	if _, ok := baseNames[base]; ok {
		return true
	}

	// Kubernetes projected ConfigMap/Secret updates use atomic writer
	// with entries like "..data" and timestamped "..2026_*" paths.
	return strings.HasPrefix(base, "..")
}
