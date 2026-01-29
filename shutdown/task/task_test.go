package task

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/oomamontov/grace/pkg/optional"
)

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

func TestErrors(t *testing.T) {
	t.Parallel()

	timeoutError := errors.New("timeout")

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
