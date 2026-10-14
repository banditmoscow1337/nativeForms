package platform

import "sync"

// CloseAction is the result of a native close request (for example the window's
// close button). A programmatic Window.Close always destroys the window.
type CloseAction uint8

const (
	CloseWindow CloseAction = iota
	HideWindow
	QuitApp
	CancelClose
)

// AppQueue serializes application callbacks on the UI goroutine. A native
// adapter supplies a wake function while its event loop is active. Callbacks
// posted by a callback run on the next drain, never reentrantly.
type AppQueue struct {
	mu       sync.Mutex
	pending  []func()
	wake     func()
	quitting bool
}

func (q *AppQueue) SetWake(wake func()) {
	q.mu.Lock()
	q.wake = wake
	queued := len(q.pending) != 0 || q.quitting
	q.mu.Unlock()
	if queued && wake != nil {
		wake()
	}
}

func (q *AppQueue) Post(fn func()) {
	if fn == nil {
		return
	}
	q.mu.Lock()
	if q.quitting {
		q.mu.Unlock()
		return
	}
	q.pending = append(q.pending, fn)
	wake := q.wake
	q.mu.Unlock()
	if wake != nil {
		wake()
	}
}

func (q *AppQueue) Drain() bool {
	q.mu.Lock()
	pending := q.pending
	q.pending = nil
	quitting := q.quitting
	q.mu.Unlock()
	if quitting {
		return true
	}
	for _, fn := range pending {
		fn()
		if q.Quitting() {
			return true
		}
	}
	q.mu.Lock()
	quitting = q.quitting
	q.mu.Unlock()
	return quitting
}

func (q *AppQueue) Quit() {
	q.mu.Lock()
	if q.quitting {
		q.mu.Unlock()
		return
	}
	q.quitting = true
	q.pending = nil
	wake := q.wake
	q.mu.Unlock()
	if wake != nil {
		wake()
	}
}

func (q *AppQueue) Quitting() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.quitting
}
