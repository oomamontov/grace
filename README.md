# grace

[![codecov](https://codecov.io/github/oomamontov/grace/graph/badge.svg)](https://codecov.io/github/oomamontov/grace)
[![GoDoc](https://godoc.org/github.com/oomamontov/grace?status.png)](https://godoc.org/github.com/oomamontov/grace)

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
- **Graceful shutdown on any trigger:** OS signals (`os.Interrupt`
and `SIGTERM` by default), cancellation of the context passed to `Run`
or a failure of any task stop the layers in reverse order.
A second signal aborts the shutdown.
- **Deadline-aware stop:** Tasks implementing `Stop(context.Context) error`
get a context that expires with the exit timeout of their layer,
just what `http.Server.Shutdown` needs.
- **Timeouts:** Limit the stop time of each layer and of the whole shutdown.
- **Background tasks:** Register background tasks that run
alongside main services, with configurable error handling.
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
		WithDefaultLayerExitTimeout(5*time.Second). // each layer gets at most 5 seconds to stop
		WithShutdownTimeout(20*time.Second).        // and the whole shutdown at most 20 seconds
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
- **Sequential execution:** Layers are initialized one after another
and stopped in reverse order, ensuring dependencies are respected.

### Runners and Tasks

- **Runner interface:** Any struct implementing `Run(context.Context) error`
methods can be registered as a service or background task. `Run` should
block until its context is cancelled and return `nil` then.
- **Optional `Init(context.Context) error`:** Prepares the component before
any task starts running.
- **Optional `Stop(context.Context) error`:** Stops the component within
the deadline of the provided context.
- **Task:** Internal wrapper for runners, used for lifecycle management.
Tasks are named after the type of their runner unless `task.WithName` is used.

### Lifecycle

1. **Init:** Layers are initialized one after another, tasks of a layer
in parallel. Do everything a component needs to be ready here: `Run` of all
layers is started only after all layers are initialized.
2. **Run:** `Run` of all tasks is started, and the application works until
a shutdown is requested by a signal, by cancellation of the context passed to
`Config.Run` or by a failure of any task: an error or a premature exit.
3. **Stop:** Layers are stopped in reverse order. For each layer, the context
of `Run` is cancelled and `Stop` is called concurrently; the next layer is
stopped when all tasks of the current one returned or its exit timeout expired.

A shutdown requested during initialization cancels the context of `Init`,
and the tasks that are already initialized are stopped in reverse order.
A second signal aborts the shutdown, and `Run` returns `shutdown.ErrForcedExit`.
`Run` returns `nil` when the shutdown was requested by a signal or the context
and all tasks stopped cleanly.

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
- `Config.WithInterruptSignals(signals...)` — Customize shutdown signals.
- `Config.WithFallibleBackgroundTasks(allowed)` — Allow background task
errors without stopping the application.
- `Config.WithDefaultLayerExitTimeout(timeout)` — Limit the stop time
of layers without their own timeout.
- `Config.WithShutdownTimeout(timeout)` — Limit the time of the whole shutdown.
- `Config.Run(ctx)` — Run the application until shutdown.
- `shutdown.NewLayer(runners, opts...)` — Create a new
layer with options.
- `shutdown.WithLayerName(name)` — Name a layer for error reporting.
- `shutdown.WithExitTimeout(timeout)` — Limit the stop time of a layer.
- `shutdown.WithBackgroundTasks(runners...)` — Add background tasks
to a layer.
- `task.Runner`, `task.Initer`, `task.Stopper` — Interfaces of components.
- `task.New(runner, task.WithName(name))` — Configurable runner wrapper.
