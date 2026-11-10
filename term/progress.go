package term

import (
	"fmt"
	"io"
	"sync"
	"time"
)

const (
	clearLine    = "\r\x1b[2K"
	tickInterval = 80 * time.Millisecond
	colorCyan    = "\x1b[36m"
	colorGreen   = "\x1b[32m"
	colorRed     = "\x1b[31m"
	colorReset   = "\x1b[0m"
	successGlyph = "✓"
	failureGlyph = "✗"
)

// Task is one step with an active spinner or bar line. Done or Fail replaces
// that line with a single result line; calls after the first are ignored.
type Task interface {
	Update(done int64)
	Done(msg string)
	Fail(msg string)
}

// Reporter starts tasks. Spin is for waits of unknown length. Bar shows a
// progress bar, and falls back to a spinner when total is not positive.
type Reporter interface {
	Spin(label string) Task
	Bar(label string, total int64) Task
}

// NewReporter writes to w (stderr) when mode.Progress is set, else returns a no-op.
func NewReporter(w io.Writer, mode Mode) Reporter {
	return newReporter(w, mode, tickInterval)
}

// newReporter with interval 0 disables the animation goroutine.
func newReporter(w io.Writer, mode Mode, interval time.Duration) Reporter {
	if !mode.Progress {
		return noop{}
	}
	return &terminal{w: w, color: mode.Color, interval: interval}
}

type noop struct{}

func (noop) Spin(string) Task       { return noopTask{} }
func (noop) Bar(string, int64) Task { return noopTask{} }

type noopTask struct{}

func (noopTask) Update(int64) {}
func (noopTask) Done(string)  {}
func (noopTask) Fail(string)  {}

type terminal struct {
	w        io.Writer
	color    bool
	interval time.Duration
}

func (t *terminal) Spin(label string) Task { return t.start(label, 0) }

func (t *terminal) Bar(label string, total int64) Task { return t.start(label, total) }

func (t *terminal) start(label string, total int64) Task {
	k := &task{term: t, label: label, total: total, stop: make(chan struct{})}
	k.draw()
	if t.interval > 0 {
		k.wg.Add(1)
		go k.animate()
	}
	return k
}

func (t *terminal) paint(color, s string) string {
	if !t.color {
		return s
	}
	return color + s + colorReset
}

type task struct {
	term  *terminal
	label string
	total int64

	mu       sync.Mutex
	frame    int
	done     int64
	finished bool

	stop chan struct{}
	wg   sync.WaitGroup
}

func (k *task) Update(done int64) {
	k.mu.Lock()
	k.done = done
	k.mu.Unlock()
}

func (k *task) Done(msg string) { k.finish(k.term.paint(colorGreen, successGlyph) + " " + msg) }

func (k *task) Fail(msg string) { k.finish(k.term.paint(colorRed, failureGlyph) + " " + msg) }

func (k *task) finish(line string) {
	k.mu.Lock()
	if k.finished {
		k.mu.Unlock()
		return
	}
	k.finished = true
	k.mu.Unlock()

	close(k.stop)
	k.wg.Wait()
	fmt.Fprint(k.term.w, clearLine+line+"\n")
}

func (k *task) animate() {
	defer k.wg.Done()
	ticker := time.NewTicker(k.term.interval)
	defer ticker.Stop()
	for {
		select {
		case <-k.stop:
			return
		case <-ticker.C:
			k.mu.Lock()
			k.frame++
			k.mu.Unlock()
			k.draw()
		}
	}
}

func (k *task) draw() {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.finished {
		return
	}
	var line string
	if k.total > 0 {
		line = k.term.paint(colorCyan, RenderBar(k.done, k.total))
	} else {
		line = k.term.paint(colorCyan, SpinnerFrame(k.frame)) + " " + k.label
	}
	fmt.Fprint(k.term.w, clearLine+line)
}
