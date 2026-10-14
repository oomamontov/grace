// Package task wraps application components into tasks managed by the shutdown package.
package task

import (
	"context"
	"fmt"

	"github.com/oomamontov/grace/pkg/optional"
)

// Runner is a component with a main loop.
//
// Run must block until ctx is cancelled or a critical error occurs. It should return nil
// when it stops because ctx was cancelled.
type Runner interface {
	Run(ctx context.Context) error
}

// Initer is an optional interface for runners that have to be prepared before any task
// starts running, e.g. to connect to a database or to bind a port.
//
// Init should return promptly once ctx is cancelled: it happens when a shutdown is
// requested during the startup.
type Initer interface {
	Init(ctx context.Context) error
}

// Stopper is an optional interface for runners that need an explicit stop step bounded by
// a deadline, e.g. http.Server.Shutdown.
//
// Stop is called concurrently with the cancellation of the context passed to Run. The ctx
// passed to Stop expires when the exit timeout of the layer runs out. Stop is called for
// every task whose Init succeeded, even if Run was never started because the startup was
// aborted.
type Stopper interface {
	Stop(ctx context.Context) error
}

// Actions of a task, reported in RunError.
const (
	ActionInit = "init"
	ActionRun  = "run"
	ActionStop = "stop"
)

// RunError is returned when one of the task actions fails.
type RunError struct {
	Name   optional.Value[string]
	Action string
	Inner  error
}

func (e RunError) Error() string {
	if name, ok := e.Name.Get(); ok {
		return fmt.Sprintf("%s task %q: %s", e.Action, name, e.Inner.Error())
	}

	return e.Action + " task: " + e.Inner.Error()
}

func (e RunError) Unwrap() error {
	return e.Inner
}

// Task is a named wrapper of a Runner used for lifecycle management.
type Task struct {
	Name   optional.Value[string]
	runner Runner
}

// WithName sets the name of the task used in errors. By default, the name is the type
// of the runner.
func WithName(name string) func(*Task) {
	return func(task *Task) {
		task.Name.Set(name)
	}
}

// New wraps the runner into a task.
func New(runner Runner, opts ...func(*Task)) Task {
	res := Task{runner: runner}
	res.Name.Set(fmt.Sprintf("%T", runner))

	for _, opt := range opts {
		opt(&res)
	}

	return res
}

// Init calls Init of the runner if it implements Initer.
func (t Task) Init(ctx context.Context) error {
	if i, ok := t.runner.(Initer); ok {
		if err := i.Init(ctx); err != nil {
			return RunError{
				Name:   t.Name,
				Action: ActionInit,
				Inner:  err,
			}
		}
	}

	return nil
}

// Run calls Run of the runner.
func (t Task) Run(ctx context.Context) error {
	if err := t.runner.Run(ctx); err != nil {
		return RunError{
			Name:   t.Name,
			Action: ActionRun,
			Inner:  err,
		}
	}

	return nil
}

// Stop calls Stop of the runner if it implements Stopper.
func (t Task) Stop(ctx context.Context) error {
	if s, ok := t.runner.(Stopper); ok {
		if err := s.Stop(ctx); err != nil {
			return RunError{
				Name:   t.Name,
				Action: ActionStop,
				Inner:  err,
			}
		}
	}

	return nil
}
