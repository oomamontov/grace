package task

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
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
