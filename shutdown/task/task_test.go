package task

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/oomamontov/grace/pkg/optional"
)

var errTimeout = errors.New("timeout")

type simpleRunner struct {
	ran bool
}

func (r *simpleRunner) Run(_ context.Context) error {
	r.ran = true

	return nil
}

type simpleIniterRunner struct {
	simpleRunner

	initialized bool
}

func (r *simpleIniterRunner) Init(_ context.Context) error {
	r.initialized = true

	return nil
}

type failingRunner struct{}

func (r *failingRunner) Init(_ context.Context) error {
	return errTimeout
}

func (r *failingRunner) Run(_ context.Context) error {
	return errTimeout
}

func TestRunner(t *testing.T) {
	t.Parallel()

	var r simpleRunner

	rTask := New(&r)
	require.False(t, r.ran)
	require.NoError(t, rTask.Init(t.Context()))
	require.False(t, r.ran)
	require.NoError(t, rTask.Run(t.Context()))
	require.True(t, r.ran)
}

func TestIniter(t *testing.T) {
	t.Parallel()

	var r simpleIniterRunner

	rTask := New(&r)
	require.False(t, r.initialized)
	require.False(t, r.ran)
	require.NoError(t, rTask.Init(t.Context()))
	require.True(t, r.initialized)
	require.False(t, r.ran)
	require.NoError(t, rTask.Run(t.Context()))
	require.True(t, r.initialized)
	require.True(t, r.ran)
}

type simpleStopperRunner struct {
	simpleRunner

	stopped bool
	stopErr error
}

func (r *simpleStopperRunner) Stop(_ context.Context) error {
	r.stopped = true

	return r.stopErr
}

func TestStopper(t *testing.T) {
	t.Parallel()

	var r simpleStopperRunner

	rTask := New(&r)
	require.NoError(t, rTask.Run(t.Context()))
	require.False(t, r.stopped)
	require.NoError(t, rTask.Stop(t.Context()))
	require.True(t, r.stopped)

	r.stopErr = errTimeout

	var runErr RunError
	require.ErrorAs(t, rTask.Stop(t.Context()), &runErr)
	require.Equal(t, ActionStop, runErr.Action)
	require.ErrorIs(t, runErr, errTimeout)

	// runners without Stop are stopped by the context cancellation only
	require.NoError(t, New(&simpleRunner{}).Stop(t.Context()))
}

func TestFailures(t *testing.T) {
	t.Parallel()

	r := &failingRunner{}
	rTask := New(r, WithName("broken"))

	require.EqualError(t, rTask.Init(t.Context()), `init task "broken": timeout`)
	require.EqualError(t, rTask.Run(t.Context()), `run task "broken": timeout`)
}

func TestErrors(t *testing.T) {
	t.Parallel()

	timeoutError := errTimeout

	errWith := RunError{
		Name:   optional.New("db-conn"),
		Action: ActionInit,
		Inner:  timeoutError,
	}
	require.Equal(t, `init task "db-conn": timeout`, errWith.Error())
	require.ErrorIs(t, errWith, timeoutError)

	errWithout := RunError{
		Action: ActionRun,
		Inner:  timeoutError,
	}
	require.Equal(t, `run task: timeout`, errWithout.Error())
}

func TestWithName(t *testing.T) {
	t.Parallel()

	r := &simpleRunner{}
	task := New(r, WithName("http-server"))
	require.Equal(t, "http-server", task.Name.ShouldGet())
}

func TestDefaultName(t *testing.T) {
	t.Parallel()

	require.Equal(t, "*task.simpleRunner", New(&simpleRunner{}).Name.ShouldGet())
	require.Equal(t, "custom", New(&simpleRunner{}, WithName("custom")).Name.ShouldGet())
}
