# confg - Go Config Struct Generator

A powerful code generator that creates type-safe Go configuration structs from YAML/JSON files, with runtime support for multiple config sources.

## Features

- 🚀 **Auto-generate** strongly-typed config structs from YAML/JSON
- 🌲 **Trie-based runtime** for efficient configuration queries (faster than Viper's nested maps)
- 📦 **Multi-source support**: File and Environment Variables (remote sources planned)
- 👀 **Watch callbacks** on config path changes
- 🛠️ **Two usage modes**: CLI tool or `go generate`

## Installation

```bash
go install github.com/cheryl-chun/confgen/cmd/confg@latest
```

## Usage

### Method 1: Direct CLI Usage

```bash
# Generate from YAML
confg --path=config.yaml --out=config.go

# Specify package name
confg --path=config.json --out=internal/config/config.go --package=config

# Watch mode for development (auto-regenerate on file changes)
confg --path=config.yaml --out=config.go --watch

# Dry run (print to stdout)
confg --path=config.yaml --dry-run
```

### Method 2: Using `go generate`

Add this comment to any `.go` file in your package:

```go
//go:generate confg --path=config.yaml --out=config_gen.go --package=mypackage
```

Then run:

```bash
go generate ./...
```

## Example

Given a `config.yaml`:

```yaml
server:
  host: "localhost"
  port: 8080
  
database:
  host: "localhost"
  port: 5432
  max_connections: 100
```

Running `confg --path=config.yaml --out=config_gen.go` generates:


## Runtime Usage

Load configs from multiple sources with priority:

```go
import "github.com/cheryl-chun/confgen/runtime"

cfg := &Config{}
loader := runtime.NewLoader()

// Load from multiple sources (priority: env > file)
loader.AddFile("config.yaml")   // Base configuration
loader.AddEnv("APP_")           // Override with env vars

// Fill generated config struct
if err := loader.Fill(cfg); err != nil {
  panic(err)
}

// Access via strong types
fmt.Println(cfg.Server.Host)

// Dynamic queries via generated helpers
host := cfg.GetString("server.host")

// Update values without exposing internal tree types
_ = cfg.Set("server.host", "prod.example.com", runtime.SourceSystemEnv)

// Watch path changes
unwatch := cfg.Watch("server.host", func(event runtime.WatchEvent) {
  fmt.Printf("%s: %v -> %v\n", event.Path, event.OldValue, event.NewValue)
})
defer unwatch()
```

## Project Status

✅ **Core Features Ready**

- [x] CLI framework with Cobra
- [x] Project structure
- [x] YAML/JSON parser (factory pattern)
- [x] Type inference engine (ConfigNode → Go types)
- [x] Code generator (Go struct with tags)
- [x] Naming conventions (snake_case → PascalCase, API/ID/URL uppercase)
- [x] Nested objects and arrays support
- [x] Dry-run mode
- [x] Runtime library with Trie
- [x] Environment variable support
- [x] Config path watch callbacks
- [ ] Remote config (etcd/zookeeper)
- [x] Hot reload
- [ ] Watch mode for development

## Command Line Options

```
Flags:
  -p, --path string      Path to config file (YAML/JSON) [required]
  -o, --out string       Output file path (default "config_gen.go")
      --package string   Package name for generated code (default "main")
  -w, --watch           Watch config file and regenerate on changes
      --dry-run         Print generated code to stdout without writing file
  -h, --help            Help for confg
```

## License

MIT

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.