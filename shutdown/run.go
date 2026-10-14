package shutdown

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oomamontov/grace/pkg/optional"
	"github.com/oomamontov/grace/shutdown/task"
)

// Run initializes registered layers one after another, runs all of them and blocks until a
// shutdown is requested by one of the configured signals, by cancellation of ctx or by a
// failure of any task: an error or a premature exit of a main task.
//
// Layers are stopped in reverse order of registration. To stop a layer, Run cancels the
// context passed to Run of its tasks and calls Stop of the tasks implementing
// task.Stopper. The next layer is stopped only when all tasks of the current one returned
// or the exit timeout of the layer expired.
//
// A shutdown requested during the initialization cancels the context passed to Init;
// tasks that are already initialized are stopped in reverse order. The second signal
// aborts the shutdown: remaining layers are cancelled without waiting, and Run returns
// ErrForcedExit.
//
// Run returns nil if the shutdown was requested by a signal or by ctx and all tasks
// stopped cleanly. If ctx is done before the call, Run returns without starting anything.
func (c Config) Run(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return RunError{Inner: err}
	}

	r := newRunner(ctx, c)
	defer r.close()

	stopWatching := r.watch(ctx)
	defer stopWatching()

	if r.init() {
		r.start()
	}

	<-r.stopCtx.Done()
	r.shutdown(r.forceCtx) //nolint:contextcheck // ctx may be already cancelled: it is one of the shutdown triggers

	return r.result()
}

// runner holds the state of a single Config.Run call.
type runner struct {
	cfg  Config
	base context.Context // carries values of the Run context, but never gets cancelled

	stopCtx     context.Context // done when the shutdown is requested
	requestStop context.CancelFunc
	forceCtx    context.Context // done when the shutdown is aborted
	force       context.CancelCauseFunc

	layers []*layerRun // layers that were initialized, at least partially

	mu   sync.Mutex
	errs []error
}

func newRunner(ctx context.Context, cfg Config) *runner {
	base := context.WithoutCancel(ctx)
	stopCtx, requestStop := context.WithCancel(base)
	forceCtx, force := context.WithCancelCause(base)

	return &runner{
		cfg:         cfg,
		base:        base,
		stopCtx:     stopCtx,
		requestStop: requestStop,
		forceCtx:    forceCtx,
		force:       force,
	}
}

func (r *runner) close() {
	r.requestStop()
	r.force(nil)
}

// watch converts signals and cancellation of ctx into the shutdown request; the second
// signal aborts the shutdown. Only signals are counted, so the same signal delivered both
// directly and through ctx (e.g. made by signal.NotifyContext) does not abort anything.
// The returned function stops watching.
func (r *runner) watch(ctx context.Context) func() {
	sigCh := make(chan os.Signal, 1)
	if signals, ok := r.cfg.signals.Get(); ok && len(signals) > 0 {
		signal.Notify(sigCh, signals...)
	}

	done := make(chan struct{})
	finished := make(chan struct{})

	go func() {
		defer close(finished)

		ctxDone := ctx.Done()
		received := 0

		for {
			select {
			case <-sigCh:
				received++
				if received > 1 {
					r.force(ErrForcedExit)

					return
				}

				r.requestStop()
			case <-ctxDone:
				ctxDone = nil

				r.requestStop()
			case <-done:
				return
			}
		}
	}()

	return func() {
		signal.Stop(sigCh)
		close(done)
		<-finished
	}
}

// fail records the error and requests the shutdown.
func (r *runner) fail(err error) {
	r.addErr(err)
	r.requestStop()
}

func (r *runner) addErr(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.errs = append(r.errs, err)
}

func (r *runner) result() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.errs) == 0 {
		return nil
	}

	return RunError{Inner: errors.Join(r.errs...)}
}

// init initializes layers in registration order. It reports whether all layers were
// initialized and no shutdown was requested meanwhile.
func (r *runner) init() bool {
	for _, layer := range r.cfg.layers {
		if r.stopCtx.Err() != nil {
			return false
		}

		l := newLayerRun(r.base, layer)
		r.layers = append(r.layers, l)

		if err := l.init(r.stopCtx); err != nil {
			// cancellation caused by the shutdown request is not a failure
			if r.stopCtx.Err() == nil || !errors.Is(err, context.Canceled) {
				r.fail(LayerError{Name: l.name, Inner: err})
			}

			return false
		}
	}

	return r.stopCtx.Err() == nil
}

func (r *runner) start() {
	for _, l := range r.layers {
		for _, u := range l.units {
			u.running.Store(true)
			l.wg.Go(func() {
				defer u.running.Store(false)

				r.handleExit(l, u, u.task.Run(l.ctx))
			})
		}
	}
}

func (r *runner) handleExit(l *layerRun, u *unit, err error) {
	switch {
	case u.background:
		if err != nil && !r.cfg.fallibleBackgroundTasks.GetOrDefault() {
			r.fail(LayerError{Name: l.name, Inner: BackgroundTaskError{Inner: err}})
		}
	case err != nil:
		r.fail(LayerError{Name: l.name, Inner: err})
	case l.ctx.Err() == nil:
		r.fail(LayerError{Name: l.name, Inner: PrematureExitError{TaskName: u.task.Name}})
	}
}

// shutdown stops layers in reverse order until ctx is done.
func (r *runner) shutdown(ctx context.Context) {
	if timeout, ok := r.cfg.shutdownTimeout.Get(); ok {
		var cancel context.CancelFunc

		ctx, cancel = context.WithTimeoutCause(ctx, timeout, ExitTimeoutError{Timeout: timeout})
		defer cancel()
	}

	for _, l := range slices.Backward(r.layers) {
		if err := l.stop(ctx, l.exitTimeout.OrOther(r.cfg.defaultExitTimeout)); err != nil {
			r.addErr(LayerError{Name: l.name, Inner: err})
		}
	}
}

// layerRun holds the state of a layer during a single Config.Run call.
type layerRun struct {
	Layer

	ctx    context.Context // passed to Run of the layer tasks
	cancel context.CancelFunc
	units  []*unit
	wg     sync.WaitGroup // tracks Run of the layer tasks
}

type unit struct {
	task       task.Task
	background bool
	inited     bool
	running    atomic.Bool
	stopping   atomic.Bool
}

func newLayerRun(base context.Context, layer Layer) *layerRun {
	ctx, cancel := context.WithCancel(base)

	units := make([]*unit, 0, len(layer.tasks)+len(layer.backgroundTasks))
	for _, t := range layer.tasks {
		units = append(units, &unit{task: t})
	}

	for _, t := range layer.backgroundTasks {
		units = append(units, &unit{task: t, background: true})
	}

	return &layerRun{
		Layer:  layer,
		ctx:    ctx,
		cancel: cancel,
		units:  units,
	}
}

// init initializes all tasks of the layer in parallel and returns the first error.
// The first error cancels initialization of the other tasks.
func (l *layerRun) init(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		wg       sync.WaitGroup
		once     sync.Once
		firstErr error
	)

	for _, u := range l.units {
		wg.Go(func() {
			if err := u.task.Init(ctx); err != nil {
				once.Do(func() {
					firstErr = err

					cancel()
				})

				return
			}

			u.inited = true
		})
	}

	wg.Wait()

	return firstErr
}

// stop cancels the Run context of the layer, calls Stop of the initialized tasks and waits
// until all of them return or ctx expires.
func (l *layerRun) stop(ctx context.Context, timeout optional.Value[time.Duration]) error {
	if d, ok := timeout.Get(); ok {
		var cancel context.CancelFunc

		ctx, cancel = context.WithTimeoutCause(ctx, d, ExitTimeoutError{Timeout: d})
		defer cancel()
	}

	l.cancel()

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		stopErrs []error
	)

	for _, u := range l.units {
		if !u.inited {
			continue
		}

		u.stopping.Store(true)
		wg.Go(func() {
			defer u.stopping.Store(false)

			if err := u.task.Stop(ctx); err != nil {
				mu.Lock()
				defer mu.Unlock()

				stopErrs = append(stopErrs, err)
			}
		})
	}

	stopped := make(chan struct{})

	go func() {
		wg.Wait()
		l.wg.Wait()
		close(stopped)
	}()

	select {
	case <-stopped:
		mu.Lock()
		defer mu.Unlock()

		return errors.Join(stopErrs...)
	case <-ctx.Done():
		cause := context.Cause(ctx)

		var timeoutErr ExitTimeoutError
		if errors.As(cause, &timeoutErr) {
			timeoutErr.Pending = l.pending()

			return timeoutErr
		}

		return cause
	}
}

// pending returns names of the tasks that are still running or stopping.
func (l *layerRun) pending() []string {
	var names []string

	for _, u := range l.units {
		if u.running.Load() || u.stopping.Load() {
			names = append(names, u.task.Name.Or("unnamed"))
		}
	}

	return names
}
