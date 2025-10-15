# grace

A flexible and extensible Go library for organizing graceful
shutdown and managing the lifecycle of services and background tasks.

> [!WARNING]
> This is a **work-in-progress** project!
This library is not production-ready and is actively under development.
**No backward compatibility** guarantees are provided — APIs, behavior, and even the project
structure may change at any time without notice.
Use at your own risk!

## Features

- **Layered initialization and shutdown:**
Organize your services into layers that are initialized and stopped
sequentially, while tasks within a layer run in parallel.
- **Background tasks:** Register background tasks that run
alongside main services, with configurable error handling.
- **Graceful shutdown on OS signals:** Handles `os.Interrupt`
and `SIGTERM` by default, with customizable signal support.
- **Extensible via options:** Easily customize layers
and shutdown behavior with functional options.
- **Clear error reporting:** Rich error types with context
for easier debugging.

## Example

```go
func main() {
	log := slog.Default()
	cfg := config.Load()
	log.Info("Config loaded")
	kvStorage := kv.New(cfg.Storage.KV, log)
	rStorage := relational.New(cfg.Storage.Relational, log)
	cache := caching.New(cfg.Storage.Caching, rStorage, log)
	svc := service.New(cfg.Service, kvStorage, cache, log)
	httpServer := http.New(cfg.Transport.HTTP, svc, log)
	grpcServer := grpc.New(cfg.Transport.GRPC, svc, log)

	builder := shutdown.New().WithDefaultValues().
		Register(kvStorage, rStorage).
		Register(cache). // cache is depending on rStorage, so rStorage should be initialized beforehand
		Register(svc).
		Register(httpServer, grpcServer) // servers are independent, so they could be initialized and stopped concurrently
	if err := builder.Run(context.Background()); err != nil {
		log.With(slog.String("error", err.Error())).Error("error running application")
	}
}
```
See full working example in
[example/internal/shutdown](example/internal/shutdown)

## Installation

```sh
go get github.com/oomamontov/grace
```

## Concepts

### Layers

- **Layer:** A group of tasks (services) that are initialized and
stopped in parallel.
- **Sequential execution:** Layers are initialized and stopped one
after another, ensuring dependencies are respected.

### Runners and Tasks

- **Runner interface:** Any struct implementing `Run(context.Context) error`
methods can be registered as a service or background task. Implement
optional `Init(context.Context) error` method to initialize component
before running.
- **Task:** Internal wrapper for runners, used for lifecycle management.

### Background Tasks

- Run alongside main tasks in a layer.
- Can be configured to allow or disallow errors (see 
`WithFallibleBackgroundTasks`).
- Allowed to finish before application shutdown.

## API Overview
- `shutdown.Config` — Main configuration object.
- `shutdown.New()` — Create a new config.
- `Config.WithDefaultValues()` — Set default options.
- `Config.Register(runners...)` — Register main tasks
(parallel within a layer, sequential between calls).
- `Config.RegisterLayer(layer)` — Register a custom layer.
- `shutdown.NewLayer(runners, opts...)` — Create a new
layer with options.
- `shutdown.WithLayerName(name)` — Name a layer for error reporting.
- `shutdown.WithBackgroundTasks(runners...)` — Add background tasks
to a layer.
- `Config.WithInterruptSignals(signals...)` — Customize shutdown signals.
- `Config.WithFallibleBackgroundTasks(allowed)` — Allow background task
errors without stopping the shutdown.
- `task.Task` - Configurable runner wrapper.
