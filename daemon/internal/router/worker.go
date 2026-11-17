package router

import (
	"log/slog"
	"sync"

	"github.com/jose00735/korokmote/daemon/internal/platform"
)

// Worker serialises all input injection through one goroutine. Pointer and
// scroll deltas that arrive while it is busy are coalesced; key and button
// events keep their order relative to motion and are never dropped.
type Worker struct {
	in   platform.Input
	mu   sync.Mutex
	q    []func() error
	mx   int32
	my   int32
	sx   int32
	sy   int32
	wake chan struct{}
	stop chan struct{}
}

// NewWorker starts an input worker.
func NewWorker(in platform.Input) *Worker {
	w := &Worker{in: in, wake: make(chan struct{}, 1), stop: make(chan struct{})}
	go w.run()
	return w
}

func (w *Worker) signal() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// flushMotionLocked turns pending motion into queued ops.
func (w *Worker) flushMotionLocked() {
	if w.mx != 0 || w.my != 0 {
		dx, dy := w.mx, w.my
		w.q = append(w.q, func() error { return w.in.MoveRel(dx, dy) })
		w.mx, w.my = 0, 0
	}
	if w.sx != 0 || w.sy != 0 {
		dx, dy := w.sx, w.sy
		w.q = append(w.q, func() error { return w.in.Scroll(dx, dy) })
		w.sx, w.sy = 0, 0
	}
}

// Move queues relative pointer motion (coalesced).
func (w *Worker) Move(dx, dy int32) {
	w.mu.Lock()
	w.mx += dx
	w.my += dy
	w.mu.Unlock()
	w.signal()
}

// Scroll queues wheel motion (coalesced).
func (w *Worker) Scroll(dx, dy int32) {
	w.mu.Lock()
	w.sx += dx
	w.sy += dy
	w.mu.Unlock()
	w.signal()
}

// Do queues an ordered operation.
func (w *Worker) Do(op func() error) {
	w.mu.Lock()
	w.flushMotionLocked()
	w.q = append(w.q, op)
	w.mu.Unlock()
	w.signal()
}

// DoWait queues an ordered operation and waits for its result.
func (w *Worker) DoWait(op func() error) error {
	done := make(chan error, 1)
	w.Do(func() error {
		err := op()
		done <- err
		return err
	})
	return <-done
}

// Stop ends the worker.
func (w *Worker) Stop() { close(w.stop) }

func (w *Worker) run() {
	for {
		select {
		case <-w.stop:
			return
		case <-w.wake:
		}
		w.mu.Lock()
		w.flushMotionLocked()
		ops := w.q
		w.q = nil
		w.mu.Unlock()
		for _, op := range ops {
			if err := op(); err != nil {
				slog.Debug("input op failed", "err", err)
			}
		}
	}
}
