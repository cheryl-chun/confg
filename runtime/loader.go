package runtime

import (
	"fmt"
	"reflect"

	"github.com/cheryl-chun/confgen/internal/tree"
)

// Loader 配置加载器，支持多源配置
type Loader struct {
	tree    *tree.ConfigTree
	sources []Source
}

// Source 配置源接口
type Source interface {
	// Load 加载配置到 ConfigTree
	Load(tree *tree.ConfigTree) error
	// Priority 返回配置源的优先级
	Priority() tree.SourceType
}

// NewLoader 创建加载器
func NewLoader() *Loader {
	return &Loader{
		tree:    tree.NewConfigTree(),
		sources: make([]Source, 0),
	}
}

// AddSource 添加配置源
func (l *Loader) AddSource(source Source) {
	l.sources = append(l.sources, source)
}

// AddFile 添加文件配置源
func (l *Loader) AddFile(path string) *Loader {
	l.AddSource(&FileSource{Path: path})
	return l
}

// AddEnv 添加环境变量配置源
func (l *Loader) AddEnv(prefix string) *Loader {
	l.AddSource(&EnvSource{Prefix: prefix})
	return l
}

// Fill 填充配置到目标 struct
//
// 流程:
// 1. 从所有配置源加载数据到 tree
// 2. 使用反射将 tree 的值填充到 struct 字段
// 3. 将 tree 设置到 struct 的 ConfigTree 字段（如果存在）
func (l *Loader) Fill(cfg interface{}) error {
	_, err := l.validateConfigTarget(cfg)
	if err != nil {
		return err
	}

	// 2. 加载所有配置源到 tree
	for _, source := range l.sources {
		if err := source.Load(l.tree); err != nil {
			return fmt.Errorf("failed to load source: %w", err)
		}
	}

	return l.applyTreeToConfig(cfg)
}

func (l *Loader) validateConfigTarget(cfg interface{}) (reflect.Value, error) {
	if cfg == nil {
		return reflect.Value{}, fmt.Errorf("config cannot be nil")
	}

	rv := reflect.ValueOf(cfg)
	if rv.Kind() != reflect.Ptr {
		return reflect.Value{}, fmt.Errorf("config must be a pointer")
	}

	if rv.IsNil() {
		return reflect.Value{}, fmt.Errorf("config pointer is nil")
	}

	return rv, nil
}

func (l *Loader) applyTreeToConfig(cfg interface{}) error {
	rv, err := l.validateConfigTarget(cfg)
	if err != nil {
		return err
	}

	// 3. 反射填充 struct
	elem := rv.Elem()
	elem.Set(reflect.Zero(elem.Type()))
	if err := l.fillStruct(elem, ""); err != nil {
		return fmt.Errorf("failed to fill struct: %w", err)
	}

	// 4. 设置 ConfigTree 字段
	if err := l.setConfigTreeField(elem); err != nil {
		// ConfigTree 字段是可选的，不报错
		// fmt.Printf("Warning: failed to set ConfigTree field: %v\n", err)
	}

	return nil
}

// fillStruct 递归填充 struct 字段
func (l *Loader) fillStruct(v reflect.Value, prefix string) error {
	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)

		// 跳过未导出字段
		if !field.CanSet() {
			continue
		}

		// 跳过 ConfigTree 字段（特殊处理）
		if fieldType.Name == "ConfigTree" {
			continue
		}

		// 获取字段的配置路径
		path := l.getFieldPath(prefix, fieldType)

		// 根据字段类型填充
		if err := l.fillField(field, path); err != nil {
			return fmt.Errorf("field %s: %w", fieldType.Name, err)
		}
	}

	return nil
}

// fillField 填充单个字段
func (l *Loader) fillField(field reflect.Value, path string) error {
	// 从 tree 获取值
	node := l.tree.Get(path)
	if node == nil {
		// 对于 struct 类型，即使节点为 nil 也尝试递归填充
		if field.Kind() == reflect.Struct {
			return l.fillStruct(field, path)
		}
		// 配置中不存在该路径，跳过
		return nil
	}

	if !node.HasValue() && field.Kind() != reflect.Struct {
		// 配置中不存在该路径，跳过
		return nil
	}

	value := node.GetValue()

	switch field.Kind() {
	case reflect.String:
		if node.Type == tree.TypeString {
			if v, ok := value.(string); ok {
				field.SetString(v)
			}
		}

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if node.Type == tree.TypeInt {
			if v, ok := value.(int); ok {
				field.SetInt(int64(v))
			}
		}

	case reflect.Float32, reflect.Float64:
		if node.Type == tree.TypeFloat {
			if v, ok := value.(float64); ok {
				field.SetFloat(v)
			}
		}

	case reflect.Bool:
		if node.Type == tree.TypeBool {
			if v, ok := value.(bool); ok {
				field.SetBool(v)
			}
		}

	case reflect.Slice:
		return l.fillSlice(field, node)

	case reflect.Struct:
		// 递归填充嵌套 struct
		return l.fillStruct(field, path)

	default:
		// 不支持的类型，跳过
	}

	return nil
}

// fillSlice 填充切片字段
func (l *Loader) fillSlice(field reflect.Value, node *tree.ConfigNode) error {
	if node.Type != tree.TypeArray || !node.HasValue() {
		return nil
	}

	arrValue, ok := node.GetValue().([]interface{})
	if !ok {
		return nil
	}

	elemType := field.Type().Elem()
	slice := reflect.MakeSlice(field.Type(), len(arrValue), len(arrValue))

	for i, item := range arrValue {
		elem := slice.Index(i)
		if err := l.setReflectValue(elem, item, elemType); err != nil {
			return err
		}
	}

	field.Set(slice)
	return nil
}

// setReflectValue 设置反射值
func (l *Loader) setReflectValue(v reflect.Value, val interface{}, t reflect.Type) error {
	switch t.Kind() {
	case reflect.String:
		if s, ok := val.(string); ok {
			v.SetString(s)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if i, ok := val.(int); ok {
			v.SetInt(int64(i))
		}
	case reflect.Float32, reflect.Float64:
		if f, ok := val.(float64); ok {
			v.SetFloat(f)
		}
	case reflect.Bool:
		if b, ok := val.(bool); ok {
			v.SetBool(b)
		}
	case reflect.Slice:
		arr, ok := val.([]interface{})
		if !ok {
			return nil
		}

		elemType := t.Elem()
		slice := reflect.MakeSlice(t, len(arr), len(arr))
		for i, item := range arr {
			if err := l.setReflectValue(slice.Index(i), item, elemType); err != nil {
				return err
			}
		}
		v.Set(slice)
	case reflect.Struct:
		obj, ok := val.(map[string]interface{})
		if !ok {
			return nil
		}

		for i := 0; i < v.NumField(); i++ {
			field := v.Field(i)
			fieldType := t.Field(i)

			if !field.CanSet() || fieldType.Name == "ConfigTree" {
				continue
			}

			mapKey := l.getMapKey(fieldType)
			raw, exists := obj[mapKey]
			if !exists {
				continue
			}

			if err := l.setReflectValue(field, raw, fieldType.Type); err != nil {
				return err
			}
		}
	}
	return nil
}

// getMapKey picks the lookup key for map values based on struct tags.
func (l *Loader) getMapKey(field reflect.StructField) string {
	if tag := field.Tag.Get("json"); tag != "" && tag != "-" {
		return tag
	}

	if tag := field.Tag.Get("yaml"); tag != "" && tag != "-" {
		return tag
	}

	return field.Name
}

// getFieldPath 获取字段的配置路径
func (l *Loader) getFieldPath(prefix string, field reflect.StructField) string {
	// 优先使用 json tag
	if tag := field.Tag.Get("json"); tag != "" && tag != "-" {
		if prefix == "" {
			return tag
		}
		return prefix + "." + tag
	}

	// 其次使用 yaml tag
	if tag := field.Tag.Get("yaml"); tag != "" && tag != "-" {
		if prefix == "" {
			return tag
		}
		return prefix + "." + tag
	}

	// 最后使用字段名（小写）
	fieldName := field.Name
	if prefix == "" {
		return fieldName
	}
	return prefix + "." + fieldName
}

// setConfigTreeField 设置 ConfigTree 字段（导出字段）
func (l *Loader) setConfigTreeField(v reflect.Value) error {
	treeField := v.FieldByName("ConfigTree")
	if !treeField.IsValid() {
		return fmt.Errorf("ConfigTree field not found")
	}

	if !treeField.CanSet() {
		return fmt.Errorf("ConfigTree field cannot be set")
	}

	internalTreeType := reflect.TypeOf((*tree.ConfigTree)(nil))
	publicTreeType := reflect.TypeOf((*Tree)(nil))

	// Backward compatibility for old generated code.
	if treeField.Type() == internalTreeType {
		treeField.Set(reflect.ValueOf(l.tree))
		return nil
	}

	if treeField.Type() == publicTreeType {
		treeField.Set(reflect.ValueOf(wrapTree(l.tree)))
		return nil
	}

	return fmt.Errorf("ConfigTree field has unsupported type: %s", treeField.Type())
}

// GetTree 获取内部的 ConfigTree
func (l *Loader) GetTree() *tree.ConfigTree {
	return l.tree
}
