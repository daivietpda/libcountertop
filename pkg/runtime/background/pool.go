package background

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

const linger = 500 * time.Microsecond

type Pool struct {
	helpers int
	nice    int
	jobs    chan job
}

type job struct {
	lo, hi  int
	part    func(lo, hi int)
	pending *atomic.Int32
	done    *sync.WaitGroup
}

func New(helpers, nice int) *Pool {
	p := &Pool{helpers: helpers, nice: nice, jobs: make(chan job, max(helpers, 1))}
	for range helpers {
		go p.serve()
	}
	return p
}

func (p *Pool) Split(n int, part func(lo, hi int)) {
	ways := min(p.helpers+1, n)
	if ways <= 1 {
		part(0, n)
		return
	}
	size := (n + ways - 1) / ways
	var pending atomic.Int32
	var done sync.WaitGroup
	for lo := size; lo < n; lo += size {
		pending.Add(1)
		done.Add(1)
		p.jobs <- job{lo: lo, hi: min(lo+size, n), part: part, pending: &pending, done: &done}
	}
	part(0, size)
	for deadline := time.Now().Add(linger); pending.Load() > 0 && time.Now().Before(deadline); {
	}
	done.Wait()
}

func (p *Pool) Run(fn func()) {
	var done sync.WaitGroup
	done.Add(1)
	go func() {
		runtime.LockOSThread()
		lower(p.nice)
		fn()
		done.Done()
	}()
	done.Wait()
}

func (p *Pool) Close() { close(p.jobs) }

func (p *Pool) serve() {
	runtime.LockOSThread()
	lower(p.nice)
	for {
		j, ok := p.next()
		if !ok {
			return
		}
		j.part(j.lo, j.hi)
		j.pending.Add(-1)
		j.done.Done()
	}
}

func (p *Pool) next() (job, bool) {
	for deadline := time.Now().Add(linger); time.Now().Before(deadline); {
		select {
		case j, ok := <-p.jobs:
			return j, ok
		default:
		}
	}
	j, ok := <-p.jobs
	return j, ok
}
