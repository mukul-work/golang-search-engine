package crawler

type Frontier struct {
	in  chan string //workers send newly discovered URLs here
	out chan string // workers receive URLs from here
}

func NewFrontier() *Frontier {
	f := &Frontier{
		in:  make(chan string, 2000), // 2000 for handling immediate bursts
		out: make(chan string),       //unbuffered to store only the URL that needs to be sent to the worker
	}

	go f.run()
	return f
}

func (f *Frontier) Push(url string) {
	f.in <- url
}

func (f *Frontier) run() {
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
		}

	}
}
