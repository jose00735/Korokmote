package router

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jose00735/korokmote/daemon/internal/platform"
)

type slowInput struct {
	mu    sync.Mutex
	calls []string
	gate  chan struct{}
}

func (s *slowInput) rec(c string) error {
	s.mu.Lock()
	s.calls = append(s.calls, c)
	s.mu.Unlock()
	return nil
}
func (s *slowInput) MoveRel(dx, dy int32) error {
	if s.gate != nil {
		<-s.gate
		s.gate = nil
	}
	return s.rec("move")
}
func (s *slowInput) MoveAbs(x, y uint16) error              { return s.rec("abs") }
func (s *slowInput) Button(b platform.Button, d bool) error { return s.rec("button") }
func (s *slowInput) Scroll(dx, dy int32) error              { return s.rec("scroll") }
func (s *slowInput) Key(vk uint16, d bool) error            { return s.rec("key") }
func (s *slowInput) Text(string) error                      { return s.rec("text") }

func TestWorkerCoalescesAndKeepsOrder(t *testing.T) {
	in := &slowInput{gate: make(chan struct{})}
	w := NewWorker(in)
	defer w.Stop()
	w.Move(1, 1) // blocks inside the platform until the gate opens
	time.Sleep(20 * time.Millisecond)
	for i := 0; i < 50; i++ {
		w.Move(1, 0)
	}
	w.Do(func() error { return in.Key(0x41, true) })
	w.Move(5, 5)
	close(in.gate)
	_ = w.DoWait(func() error { return nil })
	in.mu.Lock()
	got := strings.Join(in.calls, ",")
	in.mu.Unlock()
	if got != "move,move,key,move" {
		t.Errorf("calls = %s", got)
	}
}
