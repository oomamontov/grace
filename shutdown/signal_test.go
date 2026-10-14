//go:build unix

package shutdown

import (
	"context"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Signals are process-wide, so these tests are neither parallel nor run in synctest bubbles.
// A signal is sent only when Run is known to be watching for it: otherwise it would kill the
// test binary.

func sendSignal(t *testing.T) {
	t.Helper()

	require.NoError(t, syscall.Kill(syscall.Getpid(), syscall.SIGTERM))
}

// hangingStopper blocks in Stop until released.
type hangingStopper struct {
	stopCalled chan struct{}
	release    chan struct{}
}

func (h *hangingStopper) Run(ctx context.Context) error {
	<-ctx.Done()

	return nil
}

func (h *hangingStopper) Stop(context.Context) error {
	close(h.stopCalled)
	<-h.release

	return nil
}

func TestRun_SignalStopsGracefully(t *testing.T) { //nolint:paralleltest // signals are process-wide
	initialized := make(chan struct{})
	runner := &mockIniterRunner{onInit: func() { close(initialized) }}

	cfg := New().WithDefaultValues().
		Register(runner)

	errCh := make(chan error, 1)

	go func() { errCh <- cfg.Run(t.Context()) }()

	<-initialized
	sendSignal(t)

	select {
	case err := <-errCh:
		require.NoError(t, err)
		require.True(t, runner.stopped.Load())
	case <-time.After(5 * time.Second):
		require.FailNow(t, "Run did not return after the signal")
	}
}

func TestRun_SecondSignalForcesExit(t *testing.T) { //nolint:paralleltest // signals are process-wide
	hanging := &hangingStopper{stopCalled: make(chan struct{}), release: make(chan struct{})}
	defer close(hanging.release)

	initialized := make(chan struct{})
	runner := &mockIniterRunner{onInit: func() { close(initialized) }}

	cfg := New().WithDefaultValues().
		Register(hanging).
		Register(runner)

	errCh := make(chan error, 1)

	go func() { errCh <- cfg.Run(t.Context()) }()

	<-initialized
	sendSignal(t)
	<-hanging.stopCalled
	sendSignal(t)

	select {
	case err := <-errCh:
		require.ErrorIs(t, err, ErrForcedExit)
		require.True(t, runner.stopped.Load())
	case <-time.After(5 * time.Second):
		require.FailNow(t, "Run did not return after the second signal")
	}
}
