package crawler

type Frontier struct {
	in   chan string   // workers send newly discovered URLs here
	out  chan string   // workers receive URLs from here
	quit chan struct{} // closed by Close to stop run
}

func NewFrontier() *Frontier {
	f := &Frontier{
		in:   make(chan string, 2000), // 2000 for handling immediate bursts
		out:  make(chan string),       // unbuffered: hands one URL to one worker
		quit: make(chan struct{}),
	}
	go f.run()
	return f
}

func (f *Frontier) Push(url string) {
	f.in <- url
}

// 'Close' stops the frontier. 'run' closes 'ou't', which ends every worker's range
// loop. Call it once, after the last Push.
func (f *Frontier) Close() {
	close(f.quit)
}

func (f *Frontier) run() {
	defer close(f.out) // run is the only sender on out, so it is the one to close it
	var queue []string
	for {
		var out chan string
		var next string
		if len(queue) > 0 {
			out = f.out
			next = queue[0]
		}

		select {
		case url := <-f.in:
			queue = append(queue, url)
		case out <- next:
			queue = queue[1:]
		case <-f.quit:
			return
		}
	}
}
