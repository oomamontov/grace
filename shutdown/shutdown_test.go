package shutdown

import (
	"context"
	"errors"
	"os"
	"slices"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/oomamontov/grace/pkg/optional"
	"github.com/oomamontov/grace/shutdown/task"
)

var (
	errInit       = errors.New("init boom")
	errRun        = errors.New("boom")
	errBackground = errors.New("bg failed")
	errStop       = errors.New("stop boom")
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

// events records the lifecycle of runners in order.
type events struct {
	mu   sync.Mutex
	list []string
}

func (e *events) add(event string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.list = append(e.list, event)
}

func (e *events) get() []string {
	e.mu.Lock()
	defer e.mu.Unlock()

	return slices.Clone(e.list)
}

func requireBefore(t *testing.T, list []string, first, second string) {
	t.Helper()

	i, j := slices.Index(list, first), slices.Index(list, second)
	require.NotEqual(t, -1, i, "%q not found in %v", first, list)
	require.NotEqual(t, -1, j, "%q not found in %v", second, list)
	require.Less(t, i, j, "%q should happen before %q in %v", first, second, list)
}

// recordingRunner records its lifecycle and takes stopDelay to return after cancellation.
type recordingRunner struct {
	name      string
	events    *events
	stopDelay time.Duration
}

func (r *recordingRunner) Init(context.Context) error {
	r.events.add(r.name + ":init")

	return nil
}

func (r *recordingRunner) Run(ctx context.Context) error {
	<-ctx.Done()
	r.events.add(r.name + ":cancelled")
	time.Sleep(r.stopDelay)
	r.events.add(r.name + ":stopped")

	return nil
}

// stoppingRunner is a recordingRunner implementing task.Stopper.
type stoppingRunner struct {
	recordingRunner

	stopErr     error
	deadline    time.Time
	hasDeadline bool
}

func (s *stoppingRunner) Stop(ctx context.Context) error {
	s.deadline, s.hasDeadline = ctx.Deadline()
	s.events.add(s.name + ":stop")

	return s.stopErr
}

// blockingIniter blocks in Init until the context is cancelled.
type blockingIniter struct {
	events *events
}

func (b *blockingIniter) Init(ctx context.Context) error {
	<-ctx.Done()
	b.events.add("blocking:init-cancelled")

	return ctx.Err()
}

func (b *blockingIniter) Run(ctx context.Context) error {
	<-ctx.Done()

	return nil
}

// failingRunner returns err after the delay.
type failingRunner struct {
	after time.Duration
	err   error
}

func (f *failingRunner) Run(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return nil
	case <-time.After(f.after):
		return f.err
	}
}

type prematureRunner struct{}

func (p *prematureRunner) Run(context.Context) error {
	return nil
}

type unresponsiveRunner struct {
	unlocker chan struct{}
}

func (u *unresponsiveRunner) Run(context.Context) error {
	<-u.unlocker

	return nil
}

func cancelAfter(t *testing.T, d time.Duration) context.Context {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	time.AfterFunc(d, cancel)

	return ctx
}

func TestRun_SingleSuccess(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		runner := &mockIniterRunner{}

		cfg := New().
			Register(runner)

		require.NoError(t, cfg.Run(cancelAfter(t, 100*time.Millisecond)))
		require.True(t, runner.initialized.Load())
		require.True(t, runner.started.Load())
		require.True(t, runner.stopped.Load())
	})
}

func TestRun_MultiSuccess(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		r1, r2, r3 := &mockIniterRunner{}, &mockIniterRunner{}, &mockIniterRunner{}

		cfg := New().
			Register(r1).
			Register(r2, r3)

		require.NoError(t, cfg.Run(cancelAfter(t, 100*time.Millisecond)))

		for _, r := range []*mockIniterRunner{r1, r2, r3} {
			require.True(t, r.initialized.Load())
			require.True(t, r.started.Load())
			require.True(t, r.stopped.Load())
		}
	})
}

func TestRun_LayersOrder(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		var ev events

		db := &recordingRunner{name: "db", events: &ev, stopDelay: 10 * time.Millisecond}
		cache := &recordingRunner{name: "cache", events: &ev, stopDelay: 10 * time.Millisecond}
		api1 := &recordingRunner{name: "api1", events: &ev, stopDelay: 20 * time.Millisecond}
		api2 := &recordingRunner{name: "api2", events: &ev, stopDelay: 10 * time.Millisecond}

		cfg := New().
			Register(db).
			Register(cache).
			Register(api1, api2)

		start := time.Now()

		require.NoError(t, cfg.Run(cancelAfter(t, time.Second)))
		require.Equal(t, time.Second+40*time.Millisecond, time.Since(start))

		list := ev.get()
		requireBefore(t, list, "db:init", "cache:init")
		requireBefore(t, list, "cache:init", "api1:init")
		requireBefore(t, list, "cache:init", "api2:init")
		requireBefore(t, list, "api1:stopped", "cache:cancelled")
		requireBefore(t, list, "api2:stopped", "cache:cancelled")
		requireBefore(t, list, "cache:stopped", "db:cancelled")
	})
}

func TestRun_StopperGetsLayerDeadline(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		var ev events

		const exitTimeout = 5 * time.Second

		db := &stoppingRunner{recordingRunner: recordingRunner{name: "db", events: &ev}}
		api := &stoppingRunner{recordingRunner: recordingRunner{name: "api", events: &ev}}

		cfg := New().
			Register(db).
			RegisterLayer(NewLayer([]task.Runner{api}, WithExitTimeout(exitTimeout)))

		start := time.Now()

		require.NoError(t, cfg.Run(cancelAfter(t, time.Second)))

		require.True(t, api.hasDeadline)
		require.Equal(t, start.Add(time.Second+exitTimeout), api.deadline)
		require.False(t, db.hasDeadline)

		list := ev.get()
		requireBefore(t, list, "api:stop", "db:stop")
		requireBefore(t, list, "api:stopped", "db:cancelled")
	})
}

func TestRun_StopError(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		var ev events

		runner := &stoppingRunner{recordingRunner: recordingRunner{name: "api", events: &ev}, stopErr: errStop}

		cfg := New().
			Register(task.New(runner, task.WithName("api")))

		err := cfg.Run(cancelAfter(t, time.Second))
		require.ErrorIs(t, err, errStop)

		var runErr task.RunError
		require.ErrorAs(t, err, &runErr)
		require.Equal(t, task.ActionStop, runErr.Action)
		require.Equal(t, "api", runErr.Name.ShouldGet())
	})
}

func TestRun_FailureStopsLayersInOrder(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		var ev events

		db := &recordingRunner{name: "db", events: &ev, stopDelay: 300 * time.Millisecond}
		other := &recordingRunner{name: "other", events: &ev, stopDelay: 50 * time.Millisecond}
		bad := &failingRunner{after: 100 * time.Millisecond, err: errRun}

		cfg := New().
			Register(db).
			Register(bad, other)

		start := time.Now()

		err := cfg.Run(t.Context())
		require.ErrorIs(t, err, errRun)
		require.Equal(t, 450*time.Millisecond, time.Since(start))

		list := ev.get()
		requireBefore(t, list, "other:stopped", "db:cancelled")
		require.Contains(t, list, "db:stopped")
	})
}

func TestRun_EarlyCancel(t *testing.T) {
	t.Parallel()

	runner := &mockIniterRunner{}

	cfg := New().
		Register(runner)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	require.ErrorIs(t, cfg.Run(ctx), context.Canceled)

	require.False(t, runner.initialized.Load())
	require.False(t, runner.started.Load())
	require.False(t, runner.stopped.Load())
}

func TestRun_CancelWhileInit(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		var ev events

		first := &stoppingRunner{recordingRunner: recordingRunner{name: "first", events: &ev}}
		blocking := &blockingIniter{events: &ev}
		never := &recordingRunner{name: "never", events: &ev}

		cfg := New().
			Register(first).
			Register(blocking).
			Register(never)

		start := time.Now()

		require.NoError(t, cfg.Run(cancelAfter(t, time.Second)))
		require.Equal(t, time.Second, time.Since(start))
		require.Equal(t, []string{"first:init", "blocking:init-cancelled", "first:stop"}, ev.get())
	})
}

func TestRun_InitFailure_RollsBack(t *testing.T) {
	t.Parallel()

	var ev events

	good1 := &stoppingRunner{recordingRunner: recordingRunner{name: "good1", events: &ev}}
	badInit := &mockIniterRunner{initErr: errInit}
	good2 := &mockIniterRunner{}

	cfg := New().
		Register(good1).
		Register(badInit).
		Register(good2)

	err := cfg.Run(t.Context())
	require.ErrorIs(t, err, errInit)

	require.Equal(t, []string{"good1:init", "good1:stop"}, ev.get())

	require.True(t, badInit.initialized.Load())
	require.False(t, badInit.started.Load())

	require.False(t, good2.initialized.Load())
	require.False(t, good2.started.Load())
}

func TestRun_PrematureExit(t *testing.T) {
	t.Parallel()

	premature := &prematureRunner{}
	cfg := New().Register(premature)

	err := cfg.Run(t.Context())
	require.Error(t, err)

	var premErr PrematureExitError
	require.ErrorAs(t, err, &premErr)
	require.Equal(t, "*shutdown.prematureRunner", premErr.TaskName.ShouldGet())
}

func TestRun_ExitTimeoutExceeded(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		unlock := make(chan struct{})
		defer close(unlock)

		var ev events

		unresponsive := &unresponsiveRunner{unlocker: unlock}
		db := &recordingRunner{name: "db", events: &ev, stopDelay: 100 * time.Millisecond}

		const (
			timeout   = time.Minute
			stopAfter = 10 * time.Minute
		)

		cfg := New().WithDefaultLayerExitTimeout(timeout).
			Register(db).
			Register(unresponsive)

		start := time.Now()

		err := cfg.Run(cancelAfter(t, stopAfter))
		require.Error(t, err)

		var timeoutErr ExitTimeoutError
		require.ErrorAs(t, err, &timeoutErr)
		require.Equal(t, timeout, timeoutErr.Timeout)
		require.Equal(t, []string{"*shutdown.unresponsiveRunner"}, timeoutErr.Pending)

		// lower layers are still stopped gracefully
		require.Equal(t, stopAfter+timeout+100*time.Millisecond, time.Since(start))
		require.Contains(t, ev.get(), "db:stopped")
	})
}

func TestRun_ShutdownTimeoutExceeded(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		unlock := make(chan struct{})
		defer close(unlock)

		const timeout = time.Minute

		cfg := New().WithShutdownTimeout(timeout).WithDefaultLayerExitTimeout(time.Hour).
			RegisterLayer(NewLayer([]task.Runner{&unresponsiveRunner{unlocker: unlock}}, WithLayerName("db"))).
			RegisterLayer(NewLayer([]task.Runner{&unresponsiveRunner{unlocker: unlock}}, WithLayerName("api")))

		start := time.Now()

		err := cfg.Run(cancelAfter(t, time.Second))
		require.Equal(t, time.Second+timeout, time.Since(start))

		var timeoutErr ExitTimeoutError
		require.ErrorAs(t, err, &timeoutErr)
		require.Equal(t, timeout, timeoutErr.Timeout)
		require.Contains(t, err.Error(), `run layer "api"`)
		require.Contains(t, err.Error(), `run layer "db"`)
	})
}

func TestConfig_DefaultValues(t *testing.T) {
	t.Parallel()

	cfg := New().WithDefaultValues()
	signals, ok := cfg.signals.Get()
	require.True(t, ok)
	require.Equal(t, defaultSignals(), signals)

	customSig := []os.Signal{syscall.SIGPIPE}
	cfg2 := New().WithInterruptSignals(customSig...).WithDefaultValues()
	signals2, ok2 := cfg2.signals.Get()
	require.True(t, ok2)
	require.Equal(t, customSig, signals2)
}

func TestConfig_DerivedConfigsAreIndependent(t *testing.T) {
	t.Parallel()

	named := func(name string) task.Task {
		return task.New(&prematureRunner{}, task.WithName(name))
	}

	base := New().Register(named("a")).Register(named("b")).Register(named("c"))
	left := base.Register(named("left"))
	right := base.Register(named("right"))

	require.Len(t, base.layers, 3)
	require.Equal(t, "left", left.layers[3].tasks[0].Name.ShouldGet())
	require.Equal(t, "right", right.layers[3].tasks[0].Name.ShouldGet())
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

		layer := NewLayer(
			[]task.Runner{main},
			WithBackgroundTasks(bg),
			WithLayerName("custom"),
		)

		cfg := New().
			RegisterLayer(layer)

		require.NoError(t, cfg.Run(cancelAfter(t, 50*time.Millisecond)))
		require.True(t, main.initialized.Load())
		require.True(t, bg.initialized.Load())
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
	require.Equal(t, `run layer "db-layer": task did not exit after 5s`, timeoutErr.Error())

	pendingErr := ExitTimeoutError{
		Timeout: time.Second,
		Pending: []string{"http", "grpc"},
	}
	require.Equal(t, `task did not exit after 1s (pending: "http", "grpc")`, pendingErr.Error())

	premErr := PrematureExitError{
		TaskName: optional.New("http-server"),
	}
	require.Equal(t, `task "http-server" exited prematurely`, premErr.Error())
}

func TestError_WithoutNames(t *testing.T) {
	t.Parallel()

	require.Equal(t, "run layer: boom", LayerError{Inner: errRun}.Error())
	require.Equal(t, "task exited prematurely", PrematureExitError{}.Error())
	require.Equal(t, "run background task: bg failed", BackgroundTaskError{Inner: errBackground}.Error())
	require.Equal(t, "run layers: boom", RunError{Inner: errRun}.Error())
}

func TestConfig_RegisterNothing(t *testing.T) {
	t.Parallel()

	require.Empty(t, New().Register().layers)
}

func TestRun_BackgroundTaskFails_Strict(t *testing.T) {
	t.Parallel()

	bg := &mockIniterRunner{runEarlyErr: errBackground}

	cfg := New().
		RegisterLayer(NewLayer(nil, WithBackgroundTasks(bg)))

	err := cfg.Run(t.Context())
	require.Error(t, err)

	var bgErr BackgroundTaskError
	require.ErrorAs(t, err, &bgErr)
	require.ErrorIs(t, bgErr, errBackground)
}

func TestRun_BackgroundTaskFails_Fallible(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		bg := &mockIniterRunner{runErr: errBackground}

		cfg := New().
			WithFallibleBackgroundTasks(true).
			RegisterLayer(NewLayer(nil, WithBackgroundTasks(bg)))

		require.NoError(t, cfg.Run(cancelAfter(t, 10*time.Millisecond)))
	})
}

func TestRun_BackgroundTaskFinishesEarly(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		main := &mockIniterRunner{}

		cfg := New().
			RegisterLayer(NewLayer([]task.Runner{main}, WithBackgroundTasks(&prematureRunner{})))

		require.NoError(t, cfg.Run(cancelAfter(t, 10*time.Millisecond)))
		require.True(t, main.stopped.Load())
	})
}

func TestRun_MainTaskFails(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		good := &mockIniterRunner{}
		bad := &mockIniterRunner{runEarlyErr: errRun}

		cfg := New().
			Register(good).Register(bad)

		err := cfg.Run(t.Context())
		require.ErrorIs(t, err, errRun)
		require.True(t, good.stopped.Load())
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
