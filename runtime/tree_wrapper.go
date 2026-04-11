package runtime

import (
	"fmt"
	"math"
	"reflect"
	"time"

	"github.com/cheryl-chun/confgen/internal/tree"
)

// SourceType is the public source type for runtime callers.
type SourceType = tree.SourceType

const (
	SourceDefault         SourceType = tree.SourceDefault
	SourceRemote          SourceType = tree.SourceRemote
	SourceFile            SourceType = tree.SourceFile
	SourceRuntimeOverride SourceType = tree.SourceRuntimeOverride
	SourceSessionEnv      SourceType = tree.SourceSessionEnv
	SourceSystemEnv       SourceType = tree.SourceSystemEnv
	SourceCodeOverride    SourceType = tree.SourceCodeOverride
)

// ValueType is the public value type for runtime callers.
type ValueType = tree.ValueType

const (
	TypeString ValueType = tree.TypeString
	TypeInt    ValueType = tree.TypeInt
	TypeFloat  ValueType = tree.TypeFloat
	TypeBool   ValueType = tree.TypeBool
	TypeArray  ValueType = tree.TypeArray
	TypeObject ValueType = tree.TypeObject
	TypeNull   ValueType = tree.TypeNull
)

// WatchEvent is the public watch event delivered by runtime.Tree.
type WatchEvent struct {
	Path      string
	OldValue  any
	NewValue  any
	Source    SourceType
	ValueType ValueType
	Time      time.Time
}

// WatchCallback handles runtime watch events.
type WatchCallback func(event WatchEvent)

// Tree is a public runtime wrapper around the internal trie tree.
type Tree struct {
	inner *tree.ConfigTree
}

func wrapTree(inner *tree.ConfigTree) *Tree {
	if inner == nil {
		return nil
	}
	return &Tree{inner: inner}
}

// Get returns the effective value at path or nil.
func (t *Tree) Get(path string) any {
	if t == nil || t.inner == nil {
		return nil
	}
	v, ok := t.inner.GetValue(path)
	if !ok {
		return nil
	}
	return v
}

// GetValue returns the effective value at path.
func (t *Tree) GetValue(path string) (any, bool) {
	if t == nil || t.inner == nil {
		return nil, false
	}
	return t.inner.GetValue(path)
}

// GetString returns string value or empty string.
func (t *Tree) GetString(path string) string {
	v, ok := t.GetValue(path)
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// GetInt returns int value or zero.
func (t *Tree) GetInt(path string) int {
	v, ok := t.GetValue(path)
	if !ok {
		return 0
	}
	i, _ := v.(int)
	return i
}

// GetBool returns bool value or false.
func (t *Tree) GetBool(path string) bool {
	v, ok := t.GetValue(path)
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}

// GetFloat returns float64 value or zero.
func (t *Tree) GetFloat(path string) float64 {
	v, ok := t.GetValue(path)
	if !ok {
		return 0
	}
	f, _ := v.(float64)
	return f
}

// Set infers value type from Go value and writes it with source priority.
func (t *Tree) Set(path string, value any, source SourceType) error {
	if t == nil || t.inner == nil {
		return fmt.Errorf("tree is nil")
	}

	normalized, valueType, err := normalizeValue(value)
	if err != nil {
		return err
	}

	return t.inner.Set(path, normalized, source, valueType)
}

// SetWithType writes a value with explicit type.
func (t *Tree) SetWithType(path string, value any, source SourceType, valueType ValueType) error {
	if t == nil || t.inner == nil {
		return fmt.Errorf("tree is nil")
	}
	return t.inner.Set(path, value, source, valueType)
}

// Watch registers callback on exact path.
func (t *Tree) Watch(path string, callback WatchCallback) func() {
	if t == nil || t.inner == nil || callback == nil {
		return func() {}
	}

	return t.inner.Watch(path, func(e tree.WatchEvent) {
		callback(WatchEvent{
			Path:      e.Path,
			OldValue:  e.OldValue,
			NewValue:  e.NewValue,
			Source:    e.Source,
			ValueType: e.ValueType,
			Time:      e.Time,
		})
	})
}

// Close releases tree watch resources.
func (t *Tree) Close() {
	if t == nil || t.inner == nil {
		return
	}
	t.inner.Close()
}

func normalizeValue(value any) (any, ValueType, error) {
	if value == nil {
		return nil, TypeNull, nil
	}

	switch v := value.(type) {
	case string:
		return v, TypeString, nil
	case bool:
		return v, TypeBool, nil
	case int:
		return v, TypeInt, nil
	case int8:
		return int(v), TypeInt, nil
	case int16:
		return int(v), TypeInt, nil
	case int32:
		return int(v), TypeInt, nil
	case int64:
		if v > int64(math.MaxInt) || v < int64(math.MinInt) {
			return nil, TypeInt, fmt.Errorf("int64 overflows int")
		}
		return int(v), TypeInt, nil
	case uint:
		if uint64(v) > uint64(math.MaxInt) {
			return nil, TypeInt, fmt.Errorf("uint overflows int")
		}
		return int(v), TypeInt, nil
	case uint8:
		return int(v), TypeInt, nil
	case uint16:
		return int(v), TypeInt, nil
	case uint32:
		return int(v), TypeInt, nil
	case uint64:
		if v > uint64(math.MaxInt) {
			return nil, TypeInt, fmt.Errorf("uint64 overflows int")
		}
		return int(v), TypeInt, nil
	case float32:
		return float64(v), TypeFloat, nil
	case float64:
		return v, TypeFloat, nil
	}

	rv := reflect.ValueOf(value)
	for rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return nil, TypeNull, nil
		}
		rv = rv.Elem()
	}

	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		arr := make([]any, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			nv, _, err := normalizeValue(rv.Index(i).Interface())
			if err != nil {
				return nil, TypeArray, err
			}
			arr[i] = nv
		}
		return arr, TypeArray, nil
	case reflect.Map:
		obj := make(map[string]any)
		iter := rv.MapRange()
		for iter.Next() {
			k := fmt.Sprint(iter.Key().Interface())
			nv, _, err := normalizeValue(iter.Value().Interface())
			if err != nil {
				return nil, TypeObject, err
			}
			obj[k] = nv
		}
		return obj, TypeObject, nil
	}

	return value, TypeObject, nil
}
