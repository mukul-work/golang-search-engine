package crawler

// Task is a URL waiting to be crawled, with its distance from a seed.
type Task struct {
	URL   string
	Depth int // seeds are 0; a link found on a depth-d page is d+1
}

type Frontier struct {
	in   chan Task     // workers send newly discovered tasks here
	out  chan Task     // workers receive tasks from here
	quit chan struct{} // closed by Close to stop run
}

func NewFrontier() *Frontier {
	f := &Frontier{
		in:   make(chan Task, 2000), // 2000 for handling immediate bursts
		out:  make(chan Task),       // unbuffered: hands one task to one worker
		quit: make(chan struct{}),
	}
	go f.run()
	return f
}

func (f *Frontier) Push(t Task) {
	f.in <- t
}

// Close stops the frontier. run closes out, which ends every worker's range
// loop. Call it once, after the last Push.
func (f *Frontier) Close() {
	close(f.quit)
}

func (f *Frontier) run() {
	defer close(f.out) // run is the only sender on out, so it is the one to close it
	var queue []Task
	for {
		var out chan Task
		var next Task
		if len(queue) > 0 {
			out = f.out
			next = queue[0]
		}

		select {
		case t := <-f.in:
			queue = append(queue, t)
		case out <- next:
			queue = queue[1:]
		case <-f.quit:
			return
		}
	}
}
