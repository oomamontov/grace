package shutdown

import (
	"context"
	"fmt"
	"github.com/oomamontov/grace/pkg/itertool"
	"github.com/oomamontov/grace/pkg/optional"
	"github.com/oomamontov/grace/shutdown/task"
	"golang.org/x/sync/errgroup"
	"os"
	"os/signal"
	"slices"
	"syscall"
)

type LayerError struct {
	Name  optional.Value[string]
	Inner error
}

func (e LayerError) Error() string {
	if name, ok := e.Name.Get(); ok {
		return fmt.Sprintf("run layer %q: %s", name, e.Inner.Error())
	}
	return fmt.Sprintf("run layer: %s", e.Inner.Error())
}

func (e LayerError) Unwrap() error {
	return e.Inner
}

type Layer struct {
	name            optional.Value[string]
	tasks           []task.Task
	backgroundTasks []task.Task
}

func WithBackgroundTasks(rs ...task.Runner) func(*Layer) {
	return func(layer *Layer) {
		tasks := make([]task.Task, 0, len(rs))
		for _, r := range rs {
			if t, ok := r.(task.Task); ok {
				tasks = append(tasks, t)
				continue
			}
			tasks = append(tasks, task.New(r))
		}
		layer.backgroundTasks = tasks
	}
}

func WithLayerName(name string) func(*Layer) {
	return func(layer *Layer) {
		layer.name.Set(name)
	}
}

func NewLayer(rs []task.Runner, opts ...func(*Layer)) Layer {
	tasks := make([]task.Task, 0, len(rs))
	for _, r := range rs {
		if t, ok := r.(task.Task); ok {
			tasks = append(tasks, t)
			continue
		}
		tasks = append(tasks, task.New(r))
	}
	res := Layer{tasks: tasks}
	for _, opt := range opts {
		opt(&res)
	}
	return res
}

type Config struct {
	layers                  []Layer
	signals                 optional.Value[[]os.Signal] // default: os.Interrupt, syscall.SIGTERM
	fallibleBackgroundTasks optional.Value[bool]        // default: false; if unset: false
}

// New returns empty shutdown config.
func New() Config {
	return Config{}
}

// WithDefaultValues returns config with empty fields set to some reasonable defaults.
func (c Config) WithDefaultValues() Config {
	c.signals.SetIfUnset([]os.Signal{os.Interrupt, syscall.SIGTERM})
	c.fallibleBackgroundTasks.SetIfUnset(false)
	return c
}

func (c Config) WithInterruptSignals(signals ...os.Signal) Config {
	c.signals.Set(signals)
	return c
}

func (c Config) WithFallibleBackgroundTasks(allowed bool) Config {
	c.fallibleBackgroundTasks.Set(allowed)
	return c
}

// Register registers individual runners to run on Run call.
// Runners provided within single Register call will be initialized and stopped in parallel.
// Runners provided within multiple different Register calls will be initialized and stopped sequentially.
// Provided runner should return nil on context errors and should not return before context cancellation
// or critical error.
func (c Config) Register(runners ...task.Runner) Config {
	if len(runners) == 0 {
		return c
	}
	c.layers = append(c.layers, NewLayer(runners))
	return c
}

// RegisterLayer registers provided layer of parallel tasks.
// The layer might be customized beforehand.
func (c Config) RegisterLayer(layer Layer) Config {
	c.layers = append(c.layers, layer)
	return c
}

type RunError struct {
	Inner error
}

func (e RunError) Error() string {
	return fmt.Sprintf("run layers: %s", e.Inner.Error())
}

func (e RunError) Unwrap() error {
	return e.Inner
}

type BackgroundTaskError struct {
	Inner error
}

func (e BackgroundTaskError) Error() string {
	return fmt.Sprintf("run background task: %s", e.Inner.Error())
}

func (e BackgroundTaskError) Unwrap() error {
	return e.Inner
}

// Run runs Init and then Run on registered runners.
// Provided context might be used to stop initialization and return on Init stage, but not on Run stage.
// If one runner returns error, all other runners are stopped forcefully.
func (c Config) Run(ctx context.Context) error {
	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, c.signals.GetOrDefault()...)

	for _, layer := range c.layers {
		if err := ctx.Err(); err != nil { // do not run Init if context is cancelled
			return RunError{Inner: err}
		}
		initEg, initCtx := errgroup.WithContext(ctx)
		for t := range itertool.Concat(slices.Values(layer.tasks), slices.Values(layer.backgroundTasks)) {
			initEg.Go(func() error {
				return t.Init(initCtx)
			})
		}
		if err := initEg.Wait(); err != nil {
			return RunError{
				Inner: LayerError{
					Name:  layer.name,
					Inner: err,
				},
			}
		}
	}

	g, runCtx := errgroup.WithContext(context.WithoutCancel(ctx))
	if err := ctx.Err(); err != nil { // do not run if context is cancelled before goroutines start
		return RunError{Inner: err}
	}

	// ctx cancellation does nothing from now on

	layerStopped := make(chan struct{})
	cancelFuncs := make([]context.CancelFunc, 0, len(c.layers))

	for _, layer := range c.layers {
		localCtx, cancel := context.WithCancel(runCtx)
		defer cancel()
		lg, layerCtx := errgroup.WithContext(localCtx)

		cancelFuncs = append(cancelFuncs, cancel)

		g.Go(func() error {
			if len(layer.tasks) == 0 {
				// localCtx is not cancelled after successful wait
				context.AfterFunc(localCtx, func() {
					layerStopped <- struct{}{} // will be executed after shutdown command on layer cancel command
				})
			} else {
				defer func() { layerStopped <- struct{}{} }() // will be executed after lg.Wait()
			}

			for _, t := range layer.backgroundTasks {
				lg.Go(func() error {
					if err := t.Run(layerCtx); err != nil && !c.fallibleBackgroundTasks.GetOrDefault() {
						return BackgroundTaskError{Inner: err}
					}
					return nil
				})
			}

			for _, t := range layer.tasks {
				lg.Go(func() error {
					if err := t.Run(layerCtx); err != nil {
						return err
					}
					return nil
				})
			}

			if err := lg.Wait(); err != nil {
				return LayerError{
					Name:  layer.name,
					Inner: err,
				}
			}
			return nil
		})
	}

	g.Go(func() error {
		select {
		case <-stopCh:
			for _, f := range slices.Backward(cancelFuncs) {
				f()
				select {
				case <-layerStopped:
					continue
				case <-runCtx.Done():
					return nil
				}
			}
		case <-runCtx.Done():
			return nil
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		return RunError{Inner: err}
	}

	return nil
}
