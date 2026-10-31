package crawler

import (
	"sync"
	"time"
)

// Task is a URL waiting to be crawled, with its distance from a seed.
type Task struct {
	URL   string
	Depth int // seeds are 0; a link found on a depth-d page is d+1
}

// Frontier hands URLs to workers on out. It keeps one queue per host and only
// offers a URL when its host's cooldown is over, so a busy host never holds
// up the others and workers never sleep waiting for a host.
type Frontier struct {
	interval  time.Duration
	in        chan Task     // workers send newly discovered tasks here
	out       chan Task     // workers receive tasks from here
	quit      chan struct{} // closed by Close to stop run
	drain     chan struct{} // closed by Drain to ignore cooldowns
	drainOnce sync.Once
}

// NewFrontier creates a frontier that spaces requests to one host by interval
// (0 means no spacing).
func NewFrontier(interval time.Duration) *Frontier {
	f := &Frontier{
		interval: interval,
		in:       make(chan Task, 2000), // 2000 for handling immediate bursts
		out:      make(chan Task),       // unbuffered: hands one task to one worker
		quit:     make(chan struct{}),
		drain:    make(chan struct{}),
	}
	go f.run()
	return f
}

func (f *Frontier) Push(t Task) {
	f.in <- t
}

// Drain makes the frontier hand out everything it holds immediately, ignoring
// cooldowns. It is safe to call more than once.
func (f *Frontier) Drain() {
	f.drainOnce.Do(func() { close(f.drain) })
}

// Close stops the frontier. run closes out, which ends every worker's range
// loop. Call it once, after the last Push.
func (f *Frontier) Close() {
	close(f.quit)
}

func (f *Frontier) run() {
	defer close(f.out) // run is the only sender on out, so it is the one to close it

	sched := newHostScheduler(f.interval)

	// One reusable timer, created stopped. time.After inside a loop would
	// leave a fresh timer behind on every iteration.
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	defer timer.Stop()

	drainC := f.drain
	draining := false

	for {
		var out chan Task // stays nil (the send case is disabled) unless there is a task to offer
		var next Task
		var timerC <-chan time.Time // stays nil (the timer case is disabled) unless we must sleep

		now := time.Now()
		if task, ok := sched.peek(now, draining); ok {
			out, next = f.out, task
		} else if at, ok := sched.wakeAt(); ok {
			// Work is waiting but every host is cooling down: sleep until the first is ready.
			if !timer.Stop() {
				select { // the timer already fired: empty its channel before reusing it
				case <-timer.C:
				default:
				}
			}
			timer.Reset(at.Sub(now))
			timerC = timer.C
		}

		select {
		case t := <-f.in:
			sched.add(t)
		case out <- next:
			// Nothing touched sched since peek, so pop removes exactly the task just handed out.
			sched.pop(time.Now())
		case <-timerC:
			// A cooldown ended; the next iteration will offer that host's task.
		case <-drainC:
			draining = true
			drainC = nil // a closed channel is always ready: stop selecting on it
		case <-f.quit:
			return
		}
	}
}
