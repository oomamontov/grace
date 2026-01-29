package shutdown

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/oomamontov/grace/pkg/optional"
	"github.com/oomamontov/grace/shutdown/task"
)

// mockIniterRunner mocks service with initialization.
type mockIniterRunner struct {
	onInit      func()
	initialized atomic.Bool
	started     atomic.Bool
	stopped     atomic.Bool
	initErr     error
	runEarlyErr error
	runErr      error
}

func (m *mockIniterRunner) Init(context.Context) error {
	if m.onInit != nil {
		m.onInit()
	}

	m.initialized.Store(true)

	return m.initErr
}

func (m *mockIniterRunner) Run(ctx context.Context) error {
	m.started.Store(true)

	if m.runEarlyErr != nil {
		return m.runEarlyErr
	}

	<-ctx.Done()

	m.stopped.Store(true)

	return m.runErr
}

func TestRun_SingleSuccess(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		runner := &mockIniterRunner{}
		stopCh := make(chan struct{})

		cfg := New().
			Register(runner)
		cfg.stopChannel = stopCh

		go func() {
			time.Sleep(100 * time.Millisecond)
			require.True(t, runner.initialized.Load())
			require.True(t, runner.started.Load())
			close(stopCh)
		}()

		require.NoError(t, cfg.Run(context.Background()))
		require.True(t, runner.stopped.Load())
	})
}

func TestRun_MultiSuccess(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		r1, r2, r3 := &mockIniterRunner{}, &mockIniterRunner{}, &mockIniterRunner{}
		stopCh := make(chan struct{})

		cfg := New().
			Register(r1).
			Register(r2, r3)
		cfg.stopChannel = stopCh

		go func() {
			time.Sleep(100 * time.Millisecond)
			require.True(t, r1.initialized.Load())
			require.True(t, r1.started.Load())
			require.True(t, r2.initialized.Load())
			require.True(t, r2.started.Load())
			require.True(t, r3.initialized.Load())
			require.True(t, r3.started.Load())
			close(stopCh)
		}()

		require.NoError(t, cfg.Run(context.Background()))
		require.True(t, r1.stopped.Load())
		require.True(t, r2.stopped.Load())
		require.True(t, r3.stopped.Load())
	})
}

func TestRun_EarlyCancel(t *testing.T) {
	t.Parallel()

	runner := &mockIniterRunner{}

	cfg := New().
		Register(runner)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.ErrorIs(t, cfg.Run(ctx), context.Canceled)

	require.False(t, runner.initialized.Load())
	require.False(t, runner.started.Load())
	require.False(t, runner.stopped.Load())
}

func TestRun_CancelWhileInit(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	runner := &mockIniterRunner{onInit: func() { cancel() }}

	cfg := New().
		Register(runner)

	require.ErrorIs(t, cfg.Run(ctx), context.Canceled)

	require.True(t, runner.initialized.Load())
	require.False(t, runner.started.Load())
	require.False(t, runner.stopped.Load())
}

func TestRun_InitFailure_PreventsNextLayers(t *testing.T) {
	t.Parallel()

	badInit := &mockIniterRunner{initErr: errors.New("init boom")}
	good1, good2 := &mockIniterRunner{}, &mockIniterRunner{}

	cfg := New().
		Register(good1).
		Register(badInit).
		Register(good2)

	err := cfg.Run(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "init boom")

	require.True(t, good1.initialized.Load())
	require.False(t, good1.started.Load())

	require.True(t, badInit.initialized.Load())
	require.False(t, badInit.started.Load())

	require.False(t, good2.initialized.Load())
	require.False(t, good2.started.Load())
}

type prematureRunner struct{}

func (p *prematureRunner) Run(context.Context) error {
	return nil
}

func TestRun_PrematureExit(t *testing.T) {
	t.Parallel()

	premature := &prematureRunner{}
	cfg := New().Register(premature)

	err := cfg.Run(context.Background())
	require.Error(t, err)

	var premErr PrematureExitError
	require.ErrorAs(t, err, &premErr)
}

type unresponsiveRunner struct {
	unlocker chan struct{}
}

func (u *unresponsiveRunner) Run(context.Context) error {
	<-u.unlocker

	return nil
}

func TestRun_ExitTimeoutExceeded(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		unlock := make(chan struct{})
		unresponsive := &unresponsiveRunner{unlocker: unlock}
		stopCh := make(chan struct{})

		const (
			timeout   = time.Minute
			stopAfter = 10 * time.Minute
		)

		cfg := New().WithDefaultLayerExitTimeout(timeout).
			Register(unresponsive)
		cfg.stopChannel = stopCh

		start := time.Now()

		go func() {
			time.Sleep(stopAfter)
			close(stopCh)
		}()

		err := cfg.Run(context.Background())
		require.Error(t, err)

		var timeoutErr ExitTimeoutError
		require.ErrorAs(t, err, &timeoutErr)

		require.Equal(t, timeout+stopAfter, time.Since(start))
		close(unlock)
	})
}

func TestConfig_DefaultValues(t *testing.T) {
	t.Parallel()

	cfg := New().WithDefaultValues()
	signals, ok := cfg.signals.Get()
	require.True(t, ok)
	require.Equal(t, defaultSignals, signals)

	customSig := []os.Signal{syscall.SIGPIPE}
	cfg2 := New().WithInterruptSignals(customSig...).WithDefaultValues()
	signals2, ok2 := cfg2.signals.Get()
	require.True(t, ok2)
	require.Equal(t, customSig, signals2)
}

func TestLayer_Customization(t *testing.T) {
	t.Parallel()

	bgTask := &mockIniterRunner{}
	mainTask := &mockIniterRunner{}

	layer := NewLayer(
		[]task.Runner{mainTask},
		WithLayerName("test-layer"),
		WithExitTimeout(100*time.Millisecond),
		WithBackgroundTasks(bgTask),
	)

	require.Equal(t, "test-layer", layer.name.ShouldGet())
	require.Equal(t, 100*time.Millisecond, layer.exitTimeout.ShouldGet())
	require.Len(t, layer.backgroundTasks, 1)
	require.Len(t, layer.tasks, 1)
}

func TestRun_Layer(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		main := &mockIniterRunner{}
		bg := &mockIniterRunner{}
		stopCh := make(chan struct{})

		layer := NewLayer(
			[]task.Runner{main},
			WithBackgroundTasks(bg),
			WithLayerName("custom"),
		)

		cfg := New().
			RegisterLayer(layer)
		cfg.stopChannel = stopCh

		go func() {
			time.Sleep(50 * time.Millisecond)
			require.True(t, main.initialized.Load())
			require.True(t, bg.initialized.Load())
			close(stopCh)
		}()

		require.NoError(t, cfg.Run(context.Background()))
		require.True(t, main.stopped.Load())
		require.True(t, bg.stopped.Load())
	})
}

func TestError_WithNames(t *testing.T) {
	t.Parallel()

	timeoutErr := LayerError{
		Name: optional.New("db-layer"),
		Inner: ExitTimeoutError{
			Timeout: 5 * time.Second,
		},
	}
	runErr := RunError{Inner: timeoutErr}
	require.Equal(t, `run layers: run layer "db-layer": task did not exit after 5s`, runErr.Error())

	premErr := PrematureExitError{
		TaskName: optional.New("http-server"),
	}
	require.Equal(t, `task "http-server" exited prematurely`, premErr.Error())
}

func TestRun_BackgroundTaskFails_Strict(t *testing.T) {
	t.Parallel()

	bg := &mockIniterRunner{runEarlyErr: errors.New("bg failed")}

	cfg := New().
		RegisterLayer(NewLayer(nil, WithBackgroundTasks(bg)))

	err := cfg.Run(context.Background())
	require.Error(t, err)

	var bgErr BackgroundTaskError
	require.ErrorAs(t, err, &bgErr)
	require.Contains(t, bgErr.Error(), "bg failed")
}

func TestRun_BackgroundTaskFails_Fallible(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		bg := &mockIniterRunner{runErr: errors.New("bg failed")}
		stopCh := make(chan struct{})

		cfg := New().
			WithFallibleBackgroundTasks(true).
			RegisterLayer(NewLayer(nil, WithBackgroundTasks(bg)))
		cfg.stopChannel = stopCh

		go func() {
			time.Sleep(10 * time.Millisecond)
			close(stopCh)
		}()

		require.NoError(t, cfg.Run(context.Background()))
	})
}

func TestRun_MainTaskFails(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		good := &mockIniterRunner{}
		bad := &mockIniterRunner{runEarlyErr: errors.New("boom")}
		stopCh := make(chan struct{})

		cfg := New().
			Register(good).Register(bad)
		cfg.stopChannel = stopCh

		err := cfg.Run(context.Background())
		require.Error(t, err)
		require.Contains(t, err.Error(), "boom")
	})
}

func TestNewLayer_WithTask(t *testing.T) {
	t.Parallel()

	r := &mockIniterRunner{}
	a := task.New(r, task.WithName("direct-task"))

	layer := NewLayer([]task.Runner{a})
	require.Len(t, layer.tasks, 1)
	require.Equal(t, "direct-task", layer.tasks[0].Name.ShouldGet())
}
