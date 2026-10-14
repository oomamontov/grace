// Package shutdown runs application components in layers ordered by their dependencies
// and stops them gracefully in reverse order.
package shutdown

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/oomamontov/grace/pkg/optional"
	"github.com/oomamontov/grace/shutdown/task"
)

// ErrForcedExit is returned by Config.Run when the shutdown was aborted by the second signal.
var ErrForcedExit = errors.New("shutdown forced by repeated signal")

// LayerError is returned when a task of the layer fails.
type LayerError struct {
	Name  optional.Value[string]
	Inner error
}

func (e LayerError) Error() string {
	if name, ok := e.Name.Get(); ok {
		return fmt.Sprintf("run layer %q: %s", name, e.Inner.Error())
	}

	return "run layer: " + e.Inner.Error()
}

func (e LayerError) Unwrap() error {
	return e.Inner
}

// PrematureExitError is returned when a main task of the layer returns before the layer
// is stopped.
type PrematureExitError struct {
	TaskName optional.Value[string]
}

func (e PrematureExitError) Error() string {
	if name, ok := e.TaskName.Get(); ok {
		return fmt.Sprintf("task %q exited prematurely", name)
	}

	return "task exited prematurely"
}

// ExitTimeoutError is returned when the layer does not stop within the exit timeout or
// the shutdown does not finish within the shutdown timeout.
type ExitTimeoutError struct {
	Timeout time.Duration
	// Pending holds names of the tasks that were still running or stopping.
	Pending []string
}

func (e ExitTimeoutError) Error() string {
	msg := "task did not exit after " + e.Timeout.String()
	if len(e.Pending) == 0 {
		return msg
	}

	quoted := make([]string, 0, len(e.Pending))
	for _, name := range e.Pending {
		quoted = append(quoted, fmt.Sprintf("%q", name))
	}

	return msg + " (pending: " + strings.Join(quoted, ", ") + ")"
}

// Layer is a group of tasks that are initialized, run and stopped in parallel.
type Layer struct {
	name            optional.Value[string]
	tasks           []task.Task
	backgroundTasks []task.Task
	exitTimeout     optional.Value[time.Duration]
}

// WithBackgroundTasks adds background tasks to the layer. Unlike main tasks, background
// tasks are allowed to finish before the layer is stopped.
func WithBackgroundTasks(rs ...task.Runner) func(*Layer) {
	return func(layer *Layer) {
		layer.backgroundTasks = toTasks(rs)
	}
}

// WithLayerName sets the name of the layer used in errors.
func WithLayerName(name string) func(*Layer) {
	return func(layer *Layer) {
		layer.name.Set(name)
	}
}

// WithExitTimeout limits the time the layer may take to stop.
func WithExitTimeout(timeout time.Duration) func(*Layer) {
	return func(layer *Layer) {
		layer.exitTimeout.Set(timeout)
	}
}

// NewLayer creates a layer of the main tasks.
func NewLayer(rs []task.Runner, opts ...func(*Layer)) Layer {
	res := Layer{tasks: toTasks(rs)}
	for _, opt := range opts {
		opt(&res)
	}

	return res
}

func toTasks(rs []task.Runner) []task.Task {
	tasks := make([]task.Task, 0, len(rs))
	for _, r := range rs {
		if t, ok := r.(task.Task); ok {
			tasks = append(tasks, t)

			continue
		}

		tasks = append(tasks, task.New(r))
	}

	return tasks
}

// Config describes the layers of the application and the shutdown behavior.
//
// Config is immutable: every method returns an updated copy.
type Config struct {
	layers                  []Layer
	signals                 optional.Value[[]os.Signal]   // default: os.Interrupt, syscall.SIGTERM
	fallibleBackgroundTasks optional.Value[bool]          // default: false; if unset: false
	defaultExitTimeout      optional.Value[time.Duration] // used if no layer timeout specified
	shutdownTimeout         optional.Value[time.Duration] // limits the whole shutdown sequence
}

// New returns empty shutdown config.
func New() Config {
	return Config{}
}

func defaultSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
}

// WithDefaultValues returns config with empty fields set to some reasonable defaults.
func (c Config) WithDefaultValues() Config {
	c.signals.SetIfUnset(defaultSignals())
	c.fallibleBackgroundTasks.SetIfUnset(false)

	return c
}

// WithInterruptSignals sets the signals that request a shutdown. The second signal aborts
// the shutdown.
func (c Config) WithInterruptSignals(signals ...os.Signal) Config {
	c.signals.Set(slices.Clone(signals))

	return c
}

// WithFallibleBackgroundTasks allows background tasks to fail without stopping the
// application.
func (c Config) WithFallibleBackgroundTasks(allowed bool) Config {
	c.fallibleBackgroundTasks.Set(allowed)

	return c
}

// WithDefaultLayerExitTimeout limits the time each layer without its own exit timeout may
// take to stop.
func (c Config) WithDefaultLayerExitTimeout(timeout time.Duration) Config {
	c.defaultExitTimeout.Set(timeout)

	return c
}

// WithShutdownTimeout limits the time the whole shutdown sequence may take, counting from
// the shutdown request. Layers that are not stopped by then are abandoned.
func (c Config) WithShutdownTimeout(timeout time.Duration) Config {
	c.shutdownTimeout.Set(timeout)

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

	return c.RegisterLayer(NewLayer(runners))
}

// RegisterLayer registers provided layer of parallel tasks.
// The layer might be customized beforehand.
func (c Config) RegisterLayer(layer Layer) Config {
	// clip to never share the backing array between configs derived from the same one
	c.layers = append(slices.Clip(c.layers), layer)

	return c
}

// RunError is returned by Config.Run when the application did not run or stop cleanly.
type RunError struct {
	Inner error
}

func (e RunError) Error() string {
	return "run layers: " + e.Inner.Error()
}

func (e RunError) Unwrap() error {
	return e.Inner
}

// BackgroundTaskError is returned when a background task fails and background tasks are
// not fallible.
type BackgroundTaskError struct {
	Inner error
}

func (e BackgroundTaskError) Error() string {
	return "run background task: " + e.Inner.Error()
}

func (e BackgroundTaskError) Unwrap() error {
	return e.Inner
}
