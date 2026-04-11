package codegen

import (
	"bytes"
	"text/template"

	"github.com/cheryl-chun/confgen/internal/analyzer"
)

// singletonTemplate 单例模式代码模板
const singletonTemplate = `
import (
	"sync"

	"github.com/cheryl-chun/confgen/runtime"
)

var (
	instance *{{.RootStructName}}
	once     sync.Once
)

// Load 加载配置（单例模式，只会执行一次）
//
// 示例:
//   config.Load("config.yaml")
//   config.Load("config.yaml", config.WithEnv("APP_"))
func Load(path string, opts ...Option) error {
	var err error
	once.Do(func() {
		loader := runtime.NewLoader()
		loader.AddFile(path)

		// 应用选项
		for _, opt := range opts {
			opt.Apply(loader)
		}

		instance = &{{.RootStructName}}{}
		err = loader.Fill(instance)
	})
	return err
}

// Get 获取配置单例
//
// 必须先调用 Load() 初始化配置，否则会 panic
func Get() *{{.RootStructName}} {
	if instance == nil {
		panic("config not loaded, call Load() first")
	}
	return instance
}

// MustLoad 加载配置，失败时 panic
func MustLoad(path string, opts ...Option) {
	if err := Load(path, opts...); err != nil {
		panic(err)
	}
}

// Option 配置选项
type Option interface {
	Apply(*runtime.Loader)
}

type optionFunc func(*runtime.Loader)

func (f optionFunc) Apply(l *runtime.Loader) {
	f(l)
}

// WithEnv 添加环境变量配置源
//
// 示例: WithEnv("APP_") 会将 APP_SERVER_HOST 映射到 server.host
func WithEnv(prefix string) Option {
	return optionFunc(func(l *runtime.Loader) {
		l.AddEnv(prefix)
	})
}
`

// dynamicMethodsTemplate 动态查询方法模板
const dynamicMethodsTemplate = `
// Set 动态设置配置值（自动推断值类型）
func (c *{{.RootStructName}}) Set(path string, value interface{}, source runtime.SourceType) error {
	if c.ConfigTree == nil {
		return nil
	}
	return c.ConfigTree.Set(path, value, source)
}

// Watch 监听指定 path 的值变化
func (c *{{.RootStructName}}) Watch(path string, callback runtime.WatchCallback) func() {
	if c.ConfigTree == nil {
		return func() {}
	}
	return c.ConfigTree.Watch(path, callback)
}

// GetString 动态查询字符串值
//
// 示例: cfg.GetString("server.host")
func (c *{{.RootStructName}}) GetString(path string) string {
	if c.ConfigTree == nil {
		return ""
	}
	return c.ConfigTree.GetString(path)
}

// GetInt 动态查询整数值
func (c *{{.RootStructName}}) GetInt(path string) int {
	if c.ConfigTree == nil {
		return 0
	}
	return c.ConfigTree.GetInt(path)
}

// GetBool 动态查询布尔值
func (c *{{.RootStructName}}) GetBool(path string) bool {
	if c.ConfigTree == nil {
		return false
	}
	return c.ConfigTree.GetBool(path)
}

// GetFloat 动态查询浮点数值
func (c *{{.RootStructName}}) GetFloat(path string) float64 {
	if c.ConfigTree == nil {
		return 0
	}
	return c.ConfigTree.GetFloat(path)
}

// Get 动态查询配置值（返回 interface{}）
func (c *{{.RootStructName}}) Get(path string) interface{} {
	if c.ConfigTree == nil {
		return nil
	}
	return c.ConfigTree.Get(path)
}
`

// generateSingletonCodeWithTemplate 使用模板生成单例代码
func (g *Generator) generateSingletonCodeWithTemplate(result *analyzer.AnalyzeResult) (string, error) {
	tmpl, err := template.New("singleton").Parse(singletonTemplate)
	if err != nil {
		return "", err
	}

	data := struct {
		RootStructName string
	}{
		RootStructName: result.RootStruct.Name,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// generateDynamicMethodsWithTemplate 使用模板生成动态方法
func (g *Generator) generateDynamicMethodsWithTemplate(result *analyzer.AnalyzeResult) (string, error) {
	tmpl, err := template.New("methods").Parse(dynamicMethodsTemplate)
	if err != nil {
		return "", err
	}

	data := struct {
		RootStructName string
	}{
		RootStructName: result.RootStruct.Name,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}
