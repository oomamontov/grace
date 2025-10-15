package task

import (
	"context"
	"fmt"
	"github.com/oomamontov/grace/optional"
)

type Runner interface {
	Run(ctx context.Context) error
}

type Initer interface {
	Init(ctx context.Context) error
}

const (
	ActionInit = "init"
	ActionRun  = "run"
)

type RunError struct {
	Name   optional.Value[string]
	Action string
	Inner  error
}

func (e RunError) Error() string {
	if name, ok := e.Name.Get(); ok {
		return fmt.Sprintf("%s task %q: %s", e.Action, name, e.Inner.Error())
	}
	return fmt.Sprintf("%s task: %s", e.Action, e.Inner.Error())
}

func (e RunError) Unwrap() error {
	return e.Inner
}

type Task struct {
	name   optional.Value[string]
	runner Runner
}

func New(runner Runner) Task {
	return Task{
		runner: runner,
	}
}

func (t Task) Init(ctx context.Context) error {
	if i, ok := t.runner.(Initer); ok {
		if err := i.Init(ctx); err != nil {
			return RunError{
				Name:   t.name,
				Action: ActionInit,
				Inner:  err,
			}
		}
	}
	return nil
}

func (t Task) Run(ctx context.Context) error {
	if err := t.runner.Run(ctx); err != nil {
		return RunError{
			Name:   t.name,
			Action: ActionRun,
			Inner:  err,
		}
	}
	return nil
}
