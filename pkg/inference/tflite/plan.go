package tflite

import "sort"

type arena struct{ free [][]float32 }

func (a *arena) take(n int) []float32 {
	best := -1
	for i, b := range a.free {
		if cap(b) >= n && (best < 0 || cap(b) < cap(a.free[best])) {
			best = i
		}
	}
	if best < 0 {
		return make([]float32, n)
	}
	b := a.free[best]
	a.free[best] = a.free[len(a.free)-1]
	a.free = a.free[:len(a.free)-1]
	return b[:n]
}

func (a *arena) put(b []float32) {
	if cap(b) > 0 {
		a.free = append(a.free, b[:0])
	}
}

type plan struct {
	arena   arena
	release [][]int
	members map[int][]int
	born    map[int]int
	dies    map[int]int
	size    map[int]int
	fixed   bool
}

func planFor(g *Subgraph, tensors []*Tensor) *plan {
	for _, o := range g.Ops {
		if o.Op == OpIf {
			return nil
		}
	}

	parent := make([]int, len(tensors))
	for i := range parent {
		parent[i] = i
	}
	var root func(int) int
	root = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	for _, o := range g.Ops {
		switch o.Op {
		case OpReshape, OpSqueeze, OpExpandDims:
			if len(o.Inputs) > 0 && len(o.Outputs) > 0 && o.Inputs[0] >= 0 {
				parent[root(o.Outputs[0])] = root(o.Inputs[0])
			}
		}
	}

	keep := make([]bool, len(tensors))
	for _, i := range append(append([]int(nil), g.Inputs...), g.Outputs...) {
		keep[root(i)] = true
	}
	for i, t := range tensors {
		if t.Const || t.Type != Float32 || g.Tensors[i].Variable {
			keep[root(i)] = true
		}
	}

	p := &plan{
		release: make([][]int, len(g.Ops)),
		members: map[int][]int{},
		born:    map[int]int{},
		dies:    map[int]int{},
		size:    map[int]int{},
	}
	for at, o := range g.Ops {
		for _, i := range o.Inputs {
			if i >= 0 {
				p.dies[root(i)] = at
			}
		}
		for _, i := range o.Outputs {
			r := root(i)
			if _, ok := p.born[r]; !ok {
				p.born[r] = at
			}
			p.dies[r] = max(p.dies[r], at)
		}
	}

	for i := range tensors {
		r := root(i)
		if _, ok := p.born[r]; keep[r] || !ok {
			continue
		}
		p.members[r] = append(p.members[r], i)
	}
	for r, ms := range p.members {
		p.release[p.dies[r]] = append(p.release[p.dies[r]], r)
		for _, i := range ms {
			tensors[i].arena = &p.arena
			tensors[i].F32 = nil
		}
	}
	return p
}

func (p *plan) done(at int, tensors []*Tensor) {
	for _, r := range p.release[at] {
		var buf []float32
		for _, i := range p.members[r] {
			if buf == nil {
				buf = tensors[i].F32
			}
			p.size[r] = max(p.size[r], len(tensors[i].F32))
			tensors[i].F32 = nil
		}
		if !p.fixed {
			p.arena.put(buf)
		}
	}
}

func (p *plan) fix(tensors []*Tensor) {
	type placed struct{ r, off, size int }
	order := make([]int, 0, len(p.members))
	for r := range p.members {
		order = append(order, r)
	}
	sort.Slice(order, func(i, j int) bool {
		if p.size[order[i]] != p.size[order[j]] {
			return p.size[order[i]] > p.size[order[j]]
		}
		return order[i] < order[j]
	})

	var done []placed
	total := 0
	for _, r := range order {
		var busy []placed
		for _, q := range done {
			if p.born[q.r] <= p.dies[r] && p.born[r] <= p.dies[q.r] {
				busy = append(busy, q)
			}
		}
		sort.Slice(busy, func(i, j int) bool { return busy[i].off < busy[j].off })
		off := 0
		for _, q := range busy {
			if off+p.size[r] <= q.off {
				break
			}
			off = max(off, q.off+q.size)
		}
		done = append(done, placed{r, off, p.size[r]})
		total = max(total, off+p.size[r])
	}

	slab := make([]float32, total)
	for _, q := range done {
		slot := slab[q.off : q.off+q.size : q.off+q.size]
		for _, i := range p.members[q.r] {
			tensors[i].slot = slot
		}
	}
	p.arena.free = nil
	p.fixed = true
}
