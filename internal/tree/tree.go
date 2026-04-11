package tree

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ConfigTree is a Trie tree structure for configuration data
type ConfigTree struct {
	Root *ConfigNode

	mu            sync.RWMutex
	watchers      map[string]map[uint64]WatchCallback
	nextWatcherID uint64
	events        chan WatchEvent
	closeOnce     sync.Once
	closed        chan struct{}
	wg            sync.WaitGroup
}

// WatchEvent is emitted when the effective value of a path changes.
type WatchEvent struct {
	Path      string
	OldValue  any
	NewValue  any
	Source    SourceType
	ValueType ValueType
	Time      time.Time
}

// WatchCallback handles tree change events.
type WatchCallback func(event WatchEvent)

type sourceSnapshotEntry struct {
	value     any
	valueType ValueType
}

func NewConfigTree() *ConfigTree {
	root := NewConfigNode("root")
	root.Type = TypeObject // Root is always an object
	t := &ConfigTree{
		Root:     root,
		watchers: make(map[string]map[uint64]WatchCallback),
		events:   make(chan WatchEvent, 128),
		closed:   make(chan struct{}),
	}
	t.wg.Add(1)
	go t.runEventLoop()
	return t
}

// Close stops the internal watch dispatcher.
func (t *ConfigTree) Close() {
	t.closeOnce.Do(func() {
		close(t.closed)
		t.wg.Wait()
	})
}

// Watch registers a callback on an exact path and returns an unsubscribe function.
func (t *ConfigTree) Watch(path string, callback WatchCallback) func() {
	if path == "" || callback == nil {
		return func() {}
	}

	id := atomic.AddUint64(&t.nextWatcherID, 1)

	t.mu.Lock()
	if _, ok := t.watchers[path]; !ok {
		t.watchers[path] = make(map[uint64]WatchCallback)
	}
	t.watchers[path][id] = callback
	t.mu.Unlock()

	return func() {
		t.unwatch(path, id)
	}
}

func (t *ConfigTree) unwatch(path string, id uint64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	watchersByPath, ok := t.watchers[path]
	if !ok {
		return
	}

	delete(watchersByPath, id)
	if len(watchersByPath) == 0 {
		delete(t.watchers, path)
	}
}

func (t *ConfigTree) runEventLoop() {
	defer t.wg.Done()

	for {
		select {
		case <-t.closed:
			return
		case event := <-t.events:
			t.dispatchEvent(event)
		}
	}
}

func (t *ConfigTree) dispatchEvent(event WatchEvent) {
	t.mu.RLock()
	watchersByPath := t.watchers[event.Path]
	callbacks := make([]WatchCallback, 0, len(watchersByPath))
	for _, cb := range watchersByPath {
		callbacks = append(callbacks, cb)
	}
	t.mu.RUnlock()

	for _, cb := range callbacks {
		cb(event)
	}
}

func (t *ConfigTree) emitEvent(event WatchEvent) {
	select {
	case <-t.closed:
		return
	default:
	}

	select {
	case t.events <- event:
		return
	default:
		go func() {
			select {
			case <-t.closed:
			case t.events <- event:
			}
		}()
	}
}

// Get gets the node by path
// path format: "server.host"
func (t *ConfigTree) Get(path string) *ConfigNode {
	if path == "" {
		return nil
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.getByPathUnlocked(strings.Split(path, "."))
}

// GetByPath gets the node by path
// path format: []string{"server", "host"}
func (t *ConfigTree) GetByPath(path []string) *ConfigNode {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.getByPathUnlocked(path)
}

func (t *ConfigTree) getByPathUnlocked(path []string) *ConfigNode {
	node := t.Root
	for _, key := range path {
		if key == "" {
			continue
		}

		child, ok := node.GetChild(key)
		if !ok {
			return nil
		}
		node = child
	}
	return node
}

func (t *ConfigTree) getOrCreateNodeLocked(path []string, valueType ValueType) *ConfigNode {
	node := t.Root
	for i, key := range path {
		if key == "" {
			continue
		}

		child, ok := node.GetChild(key)
		if !ok {
			child = NewConfigNode(key)
			if i == len(path)-1 {
				child.Type = valueType
			} else {
				child.Type = TypeObject
			}
			node.AddChild(child)
		}
		node = child
	}
	return node
}

// GetValue gets the value of the node by path
func (t *ConfigTree) GetValue(path string) (any, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	node := t.getByPathUnlocked(strings.Split(path, "."))
	if node == nil || !node.HasValue() {
		return nil, false
	}
	return node.GetValue(), true
}

// Set sets the value at the specified path and source
// If the path does not exist, intermediate nodes will be created
func (t *ConfigTree) Set(path string, value any, source SourceType, valueType ValueType) error {
	return t.SetByPath(strings.Split(path, "."), value, source, valueType)
}

// SetByPath sets the value using a path array
func (t *ConfigTree) SetByPath(path []string, value any, source SourceType, valueType ValueType) error {
	if len(path) == 0 {
		return fmt.Errorf("path cannot be empty")
	}

	t.mu.Lock()

	// Traverse the path and create missing nodes
	node := t.Root
	for i, key := range path {
		if key == "" {
			continue
		}

		child, ok := node.GetChild(key)
		if !ok {
			child = NewConfigNode(key)

			// if child is the last key, set it to the actual type;
			// otherwise, set it to object
			if i == len(path)-1 {
				child.Type = valueType
			} else {
				child.Type = TypeObject
			}

			node.AddChild(child)
		}
		node = child
	}

	var oldValue any
	oldExists := node.HasValue()
	if oldExists {
		oldValue = node.GetValue()
	}

	node.SetValue(value, source)
	newValue := node.GetValue()
	newExists := node.HasValue()
	pathStr := strings.Join(path, ".")
	normalizedPath := strings.Trim(pathStr, ".")
	changed := oldExists != newExists || !reflect.DeepEqual(oldValue, newValue)
	t.mu.Unlock()

	if changed && normalizedPath != "" {
		t.emitEvent(WatchEvent{
			Path:      normalizedPath,
			OldValue:  oldValue,
			NewValue:  newValue,
			Source:    source,
			ValueType: valueType,
			Time:      time.Now(),
		})
	}

	return nil
}

// Merge is used to merge another ConfigTree into this one
// with a specified source type for the new values
func (t *ConfigTree) Merge(other *ConfigTree, source SourceType) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.mergeNode(t.Root, other.Root, source)
}

// ReplaceSource replaces all values from the specified source with values from snapshot.
// Values from other sources are preserved, and only effective value changes emit watch events.
func (t *ConfigTree) ReplaceSource(snapshot *ConfigTree, source SourceType) {
	var newSnapshot map[string]sourceSnapshotEntry
	if snapshot != nil {
		newSnapshot = snapshot.collectSourceSnapshot(source)
	} else {
		newSnapshot = make(map[string]sourceSnapshotEntry)
	}

	t.mu.Lock()
	currentSnapshot := t.collectSourceSnapshotLocked(source)
	allPaths := make(map[string]struct{}, len(currentSnapshot)+len(newSnapshot))
	for path := range currentSnapshot {
		allPaths[path] = struct{}{}
	}
	for path := range newSnapshot {
		allPaths[path] = struct{}{}
	}

	paths := make([]string, 0, len(allPaths))
	for path := range allPaths {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	events := make([]WatchEvent, 0, len(paths))
	for _, path := range paths {
		segments := strings.Split(path, ".")
		node := t.getByPathUnlocked(segments)

		var oldValue any
		oldExists := false
		if node != nil && node.HasValue() {
			oldValue = node.GetValue()
			oldExists = true
		}

		if entry, ok := newSnapshot[path]; ok {
			node = t.getOrCreateNodeLocked(segments, entry.valueType)
			node.Type = entry.valueType
			node.SetValue(entry.value, source)
		} else if node != nil {
			node.RemoveSource(source)
		}

		node = t.getByPathUnlocked(segments)
		var newValue any
		newExists := false
		valueType := TypeNull
		if node != nil {
			valueType = node.Type
			if node.HasValue() {
				newValue = node.GetValue()
				newExists = true
			}
		}

		if oldExists != newExists || !reflect.DeepEqual(oldValue, newValue) {
			events = append(events, WatchEvent{
				Path:      path,
				OldValue:  oldValue,
				NewValue:  newValue,
				Source:    source,
				ValueType: valueType,
				Time:      time.Now(),
			})
		}
	}
	t.mu.Unlock()

	for _, event := range events {
		t.emitEvent(event)
	}
}

func (t *ConfigTree) mergeNode(target, source *ConfigNode, sourceType SourceType) {
	if source.HasValue() {
		target.SetValue(source.GetValue(), sourceType)
		target.Type = source.Type
	}

	// recursively merge child nodes
	for key, sourceChild := range source.Children {
		targetChild, ok := target.GetChild(key)
		if !ok {
			// target node does not exist, copy directly
			targetChild = copyNode(sourceChild, sourceType)
			target.AddChild(targetChild)
		} else {
			// recursively merge
			t.mergeNode(targetChild, sourceChild, sourceType)
		}
	}

	// merge array elements
	if source.IsArray() {
		target.Items = make([]*ConfigNode, len(source.Items))
		for i, item := range source.Items {
			target.Items[i] = copyNode(item, sourceType)
		}
	}
}

// copyNode (deepcopy)
func copyNode(source *ConfigNode, sourceType SourceType) *ConfigNode {
	node := NewConfigNode(source.Key)
	node.Type = source.Type

	if source.HasValue() {
		node.SetValue(source.GetValue(), sourceType)
	}

	for _, child := range source.Children {
		node.AddChild(copyNode(child, sourceType))
	}

	for _, item := range source.Items {
		node.AddItem(copyNode(item, sourceType))
	}

	return node
}

func (t *ConfigTree) collectSourceSnapshot(source SourceType) map[string]sourceSnapshotEntry {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.collectSourceSnapshotLocked(source)
}

func (t *ConfigTree) collectSourceSnapshotLocked(source SourceType) map[string]sourceSnapshotEntry {
	result := make(map[string]sourceSnapshotEntry)
	t.collectSourceSnapshotFromNodeLocked(t.Root, "", source, result)
	return result
}

func (t *ConfigTree) collectSourceSnapshotFromNodeLocked(node *ConfigNode, path string, source SourceType, result map[string]sourceSnapshotEntry) {
	if node == nil {
		return
	}

	if path != "" {
		if value, ok := node.GetValueFromSource(source); ok {
			result[path] = sourceSnapshotEntry{value: value, valueType: node.Type}
		}
	}

	for key, child := range node.Children {
		childPath := key
		if path != "" {
			childPath = path + "." + key
		}
		t.collectSourceSnapshotFromNodeLocked(child, childPath, source, result)
	}
}

// GetAllWithPrefix gets all nodes with the specified prefix
func (t *ConfigTree) GetAllWithPrefix(prefix string) map[string]*ConfigNode {
	t.mu.RLock()
	defer t.mu.RUnlock()

	node := t.getByPathUnlocked(strings.Split(prefix, "."))
	if node == nil {
		return nil
	}

	result := make(map[string]*ConfigNode)
	for key, child := range node.Children {
		fullPath := prefix + "." + key
		result[fullPath] = child
	}
	return result
}

// Walk traverses the entire tree
func (t *ConfigTree) Walk(fn func(path string, node *ConfigNode)) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	t.walkNode(t.Root, "", fn)
}

// walkNode recursively traverses nodes
func (t *ConfigTree) walkNode(node *ConfigNode, path string, fn func(string, *ConfigNode)) {
	if path != "" { // skip root node
		fn(path, node)
	}

	for key, child := range node.Children {
		childPath := path
		if childPath != "" {
			childPath += "."
		}
		childPath += key
		t.walkNode(child, childPath, fn)
	}
}

// ToMap converts the tree to a nested map (for serialization)
// Only returns the highest priority values
func (t *ConfigTree) ToMap() map[string]any {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.nodeToMap(t.Root)
}

func (t *ConfigTree) nodeToMap(node *ConfigNode) map[string]any {
	if node.IsPrimitive() {
		return nil // 基本类型直接返回值
	}

	result := make(map[string]any)

	if node.IsObject() {
		for key, child := range node.Children {
			if child.IsPrimitive() {
				result[key] = child.GetValue()
			} else if child.IsObject() {
				result[key] = t.nodeToMap(child)
			} else if child.IsArray() {
				result[key] = t.nodeToArray(child)
			}
		}
	}

	return result
}

// nodeToArray converts an array node
func (t *ConfigTree) nodeToArray(node *ConfigNode) []any {
	result := make([]any, len(node.Items))
	for i, item := range node.Items {
		if item.IsPrimitive() {
			result[i] = item.GetValue()
		} else if item.IsObject() {
			result[i] = t.nodeToMap(item)
		} else if item.IsArray() {
			result[i] = t.nodeToArray(item)
		}
	}
	return result
}

func (t *ConfigTree) Print() {
	t.mu.RLock()
	defer t.mu.RUnlock()

	t.printNode(t.Root, "", true)
}

func (t *ConfigTree) printNode(node *ConfigNode, prefix string, isLast bool) {
	if node.Key != "root" {
		marker := "├─"
		if isLast {
			marker = "└─"
		}
		fmt.Printf("%s%s %s\n", prefix, marker, node.String())

		if isLast {
			prefix += "  "
		} else {
			prefix += "│ "
		}
	}

	children := make([]*ConfigNode, 0, len(node.Children))
	for _, child := range node.Children {
		children = append(children, child)
	}

	for i, child := range children {
		t.printNode(child, prefix, i == len(children)-1)
	}
}
