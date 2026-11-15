package tflite

import (
	"fmt"
	"math"

	"github.com/ygelfand/libcountertop/pkg/inference/vec"
)

func broadcastShape(a, b []int) ([]int, error) {
	rank := max(len(a), len(b))
	out := make([]int, rank)
	for i := range rank {
		da, db := 1, 1
		if d := len(a) - rank + i; d >= 0 {
			da = a[d]
		}
		if d := len(b) - rank + i; d >= 0 {
			db = b[d]
		}
		switch {
		case da == db, db == 1:
			out[i] = da
		case da == 1:
			out[i] = db
		default:
			return nil, fmt.Errorf("shapes %v and %v do not broadcast", a, b)
		}
	}
	return out, nil
}

func strides(src, out []int) []int {
	s := make([]int, len(out))
	stride := 1
	for i := range src {
		d := len(src) - 1 - i
		o := len(out) - 1 - i
		if o < 0 {
			break
		}
		if src[d] != 1 {
			s[o] = stride
		}
		stride *= src[d]
	}
	return s
}

func step1(idx, shape, strides []int, off int) int {
	for d := len(shape) - 1; d >= 0; d-- {
		idx[d]++
		off += strides[d]
		if idx[d] < shape[d] {
			return off
		}
		idx[d] = 0
		off -= strides[d] * shape[d]
	}
	return off
}

func contiguous(shape []int) []int {
	s := make([]int, len(shape))
	stride := 1
	for d := len(shape) - 1; d >= 0; d-- {
		s[d] = stride
		stride *= shape[d]
	}
	return s
}

func unary(f func(float32) float32) kernel {
	return func(o *OpDesc, in, out []*Tensor) error {
		x, y := in[0], out[0]
		y.resize(x.Shape)
		for i, v := range x.F32 {
			y.F32[i] = f(v)
		}
		return nil
	}
}

type runFunc func(dst, a, b []float32, sa, sb int)

func addRun(dst, a, b []float32, sa, sb int) {
	if sa == 1 && sb == 1 {
		vec.Add(dst, a, b)
		return
	}
	for j := range dst {
		dst[j] = a[j*sa] + b[j*sb]
	}
}

func subRun(dst, a, b []float32, sa, sb int) {
	for j := range dst {
		dst[j] = a[j*sa] - b[j*sb]
	}
}

func mulRun(dst, a, b []float32, sa, sb int) {
	if sa == 1 && sb == 1 {
		vec.Mul(dst, a, b)
		return
	}
	for j := range dst {
		dst[j] = a[j*sa] * b[j*sb]
	}
}

func divRun(dst, a, b []float32, sa, sb int) {
	for j := range dst {
		dst[j] = a[j*sa] / b[j*sb]
	}
}

func maxRun(dst, a, b []float32, sa, sb int) {
	for j := range dst {
		dst[j] = max(a[j*sa], b[j*sb])
	}
}

func minRun(dst, a, b []float32, sa, sb int) {
	for j := range dst {
		dst[j] = min(a[j*sa], b[j*sb])
	}
}

func squaredDiffRun(dst, a, b []float32, sa, sb int) {
	for j := range dst {
		d := a[j*sa] - b[j*sb]
		dst[j] = d * d
	}
}

func unravel(r int, shape, idx []int) []int {
	for d := len(shape) - 1; d >= 0; d-- {
		idx[d] = r % shape[d]
		r /= shape[d]
	}
	return idx
}

func offset(idx, strides []int) int {
	o := 0
	for d, i := range idx {
		o += i * strides[d]
	}
	return o
}

func elementwise(run runFunc) splitKernel {
	return func(s Splitter, o *OpDesc, in, out []*Tensor) error {
		a, b, y := in[0], in[1], out[0]
		shape, err := broadcastShape(a.Shape, b.Shape)
		if err != nil {
			return err
		}
		y.resize(shape)
		n := y.Count()
		if n == 0 {
			return nil
		}

		as, bs := strides(a.Shape, shape), strides(b.Shape, shape)
		d, kind := innerRun(shape, func(d int) int {
			k := 0
			if as[d] != 0 {
				k |= 1
			}
			if bs[d] != 0 {
				k |= 2
			}
			return k
		})
		length := count(shape[d:])
		sa, sb := kind&1, kind>>1&1
		runs := n / length
		if runs == 1 {
			spread(s, n, n, func(lo, hi int) {
				run(y.F32[lo:hi], a.F32[lo*sa:(hi-1)*sa+1], b.F32[lo*sb:(hi-1)*sb+1], sa, sb)
				activateAll(y.F32[lo:hi], o.act)
			})
			return nil
		}
		na, nb := (length-1)*sa+1, (length-1)*sb+1
		spread(s, n, runs, func(lo, hi int) {
			idx := unravel(lo, shape[:d], make([]int, d))
			ao, bo := offset(idx, as[:d]), offset(idx, bs[:d])
			for i := lo * length; i < hi*length; i += length {
				run(y.F32[i:i+length], a.F32[ao:ao+na], b.F32[bo:bo+nb], sa, sb)
				ao, bo = step(idx, shape[:d], as[:d], bs[:d], ao, bo)
			}
			activateAll(y.F32[lo*length:hi*length], o.act)
		})
		return nil
	}
}

func innerRun(shape []int, kind func(d int) int) (int, int) {
	d, k := len(shape), -1
	for d > 0 {
		if shape[d-1] == 1 {
			d--
			continue
		}
		dk := kind(d - 1)
		if k >= 0 && dk != k {
			break
		}
		k = dk
		d--
	}
	return d, k
}

func activateAll(v []float32, a Activation) {
	switch a {
	case ActNone:
	case ActRelu:
		for i, x := range v {
			v[i] = max(x, 0)
		}
	case ActRelu6:
		for i, x := range v {
			v[i] = min(max(x, 0), 6)
		}
	default:
		for i, x := range v {
			v[i] = activate(x, a)
		}
	}
}

func compare(f func(a, b float32) bool) kernel {
	return func(_ *OpDesc, in, out []*Tensor) error {
		a, b, y := in[0], in[1], out[0]
		shape, err := broadcastShape(a.Shape, b.Shape)
		if err != nil {
			return err
		}
		y.resize(shape)
		as, bs := strides(a.Shape, shape), strides(b.Shape, shape)
		idx := make([]int, len(shape))
		ao, bo := 0, 0
		for i := range y.Count() {
			y.I32[i] = 0
			if f(a.F32[ao], b.F32[bo]) {
				y.I32[i] = 1
			}
			ao, bo = step(idx, shape, as, bs, ao, bo)
		}
		return nil
	}
}

func leakyRelu(o *OpDesc, in, out []*Tensor) error {
	alpha := o.alpha
	x, y := in[0], out[0]
	y.resize(x.Shape)
	for i, v := range x.F32 {
		if v < 0 {
			v *= alpha
		}
		y.F32[i] = v
	}
	return nil
}

type reducer struct {
	init  float32
	merge func(dst, src []float32)
	fold  func(acc float32, src []float32) float32
}

var (
	sumReducer = reducer{
		merge: func(dst, src []float32) { addRun(dst, dst, src, 1, 1) },
		fold: func(acc float32, src []float32) float32 {
			for _, v := range src {
				acc += v
			}
			return acc
		},
	}
	maxReducer = reducer{
		init:  float32(math.Inf(-1)),
		merge: func(dst, src []float32) { maxRun(dst, dst, src, 1, 1) },
		fold: func(acc float32, src []float32) float32 {
			for _, v := range src {
				acc = max(acc, v)
			}
			return acc
		},
	}
)

func reduceOp(r reducer, mean bool) kernel {
	return func(o *OpDesc, in, out []*Tensor) error {
		x, y := in[0], out[0]
		rank := len(x.Shape)

		axes := make([]bool, rank)
		if len(in) > 1 && in[1] != nil && len(in[1].I32) > 0 {
			for _, a := range in[1].I32 {
				d := int(a)
				if d < 0 {
					d += rank
				}
				if d < 0 || d >= rank {
					return fmt.Errorf("axis %d out of range for shape %v", a, x.Shape)
				}
				axes[d] = true
			}
		} else {
			for d := range axes {
				axes[d] = true
			}
		}

		keep := o.keepDims
		var shape []int
		for d, n := range x.Shape {
			switch {
			case !axes[d]:
				shape = append(shape, n)
			case keep:
				shape = append(shape, 1)
			}
		}
		y.resize(shape)

		os := make([]int, rank)
		stride := 1
		for d := rank - 1; d >= 0; d-- {
			if axes[d] {
				continue
			}
			os[d] = stride
			stride *= x.Shape[d]
		}

		for i := range y.F32 {
			y.F32[i] = r.init
		}
		d, kept := innerRun(x.Shape, func(d int) int {
			if axes[d] {
				return 0
			}
			return 1
		})
		run := count(x.Shape[d:])
		idx := make([]int, d)
		yo := 0
		for i := 0; run > 0 && i < x.Count(); i += run {
			src := x.F32[i : i+run]
			if kept == 1 {
				r.merge(y.F32[yo:yo+run], src)
			} else {
				y.F32[yo] = r.fold(y.F32[yo], src)
			}
			yo = step1(idx, x.Shape[:d], os[:d], yo)
		}

		if mean {
			n := 1
			for d, r := range axes {
				if r {
					n *= x.Shape[d]
				}
			}
			inv := 1 / float32(n)
			for i := range y.F32 {
				y.F32[i] *= inv
			}
		}
		return nil
	}
}

func padZero(o *OpDesc, in, out []*Tensor) error {
	x, p, y := in[0], in[1], out[0]
	rank := len(x.Shape)
	if len(p.I32) < 2*rank {
		return fmt.Errorf("padding has %d values for a %d-D input", len(p.I32), rank)
	}

	shape := make([]int, rank)
	for d := range rank {
		shape[d] = int(p.I32[2*d]) + x.Shape[d] + int(p.I32[2*d+1])
	}
	y.resize(shape)
	for i := range y.F32 {
		y.F32[i] = 0
	}

	os := contiguous(shape)
	base := 0
	for d := range rank {
		base += int(p.I32[2*d]) * os[d]
	}

	idx := make([]int, rank)
	yo := base
	for i := range x.Count() {
		y.F32[yo] = x.F32[i]
		yo = step1(idx, x.Shape, os, yo)
	}
	return nil
}

func transpose(sp Splitter, o *OpDesc, in, out []*Tensor) error {
	x, p, y := in[0], in[1], out[0]
	rank := len(x.Shape)
	if len(p.I32) != rank {
		return fmt.Errorf("permutation has %d values for a %d-D input", len(p.I32), rank)
	}

	perm := make([]int, rank)
	shape := make([]int, rank)
	for i := range rank {
		d := int(p.I32[i])
		if d < 0 {
			d += rank
		}
		if d < 0 || d >= rank {
			return fmt.Errorf("permutation %v out of range for shape %v", p.I32, x.Shape)
		}
		perm[i] = d
		shape[i] = x.Shape[d]
	}
	y.resize(shape)

	xs := contiguous(x.Shape)
	ps := make([]int, rank)
	for i := range rank {
		ps[i] = xs[perm[i]]
	}

	if rank == 0 {
		copy(y.F32, x.F32)
		return nil
	}
	n, stride := shape[rank-1], ps[rank-1]
	if n == 0 {
		return nil
	}
	if stride != 1 && rank >= 2 {
		transposeBlocks(sp, y.F32, x.F32, shape, ps)
		return nil
	}
	outer, ops := shape[:rank-1], ps[:rank-1]
	spread(sp, len(y.F32), len(y.F32)/n, func(lo, hi int) {
		idx := unravel(lo, outer, make([]int, rank-1))
		xo := offset(idx, ops)
		for i := lo * n; i < hi*n; i += n {
			dst := y.F32[i : i+n]
			if stride == 1 {
				copy(dst, x.F32[xo:xo+n])
			} else {
				for j := range dst {
					dst[j] = x.F32[xo+j*stride]
				}
			}
			xo = step1(idx, outer, ops, xo)
		}
	})
	return nil
}

func transposeBlocks(sp Splitter, yd, xd []float32, shape, ps []int) {
	const block = 16
	rank := len(shape)
	rows, cols := shape[rank-2], shape[rank-1]
	sr, sc := ps[rank-2], ps[rank-1]
	plane := rows * cols
	if plane == 0 {
		return
	}
	outer, ops := shape[:rank-2], ps[:rank-2]
	tile := func(base, xo, r0, r1 int) {
		for ; r0 < r1; r0 += block {
			re := min(r0+block, r1)
			for c0 := 0; c0 < cols; c0 += block {
				ce := min(c0+block, cols)
				for c := c0; c < ce; c++ {
					for r := r0; r < re; r++ {
						yd[base+r*cols+c] = xd[xo+r*sr+c*sc]
					}
				}
			}
		}
	}
	planes := len(yd) / plane
	if planes == 1 {
		spread(sp, plane, (rows+block-1)/block, func(lo, hi int) {
			tile(0, 0, lo*block, min(hi*block, rows))
		})
		return
	}
	spread(sp, len(yd), planes, func(lo, hi int) {
		idx := unravel(lo, outer, make([]int, rank-2))
		xo := offset(idx, ops)
		for p := lo; p < hi; p++ {
			tile(p*plane, xo, 0, rows)
			xo = step1(idx, outer, ops, xo)
		}
	})
}

func share(y, x *Tensor, shape []int) {
	y.Shape = append(y.Shape[:0], shape...)
	y.F32, y.I32 = x.F32, x.I32
}

func copyData(y, x *Tensor) {
	if x.Type == Float32 {
		copy(y.F32, x.F32)
		return
	}
	copy(y.I32, x.I32)
}

func reshape(o *OpDesc, in, out []*Tensor) error {
	x, y := in[0], out[0]

	var target []int
	if len(in) > 1 && in[1] != nil && (len(in[1].I32) > 0 || in[1].Count() == 0) {
		target = []int{}
		for _, v := range in[1].I32 {
			target = append(target, int(v))
		}
	} else {
		target = append(target, o.dims...)
	}
	if target == nil {
		return fmt.Errorf("no target shape")
	}

	// One dimension may be -1, standing for whatever makes the count work out.
	total := x.Count()
	known, free := 1, -1
	for i, d := range target {
		if d < 0 {
			free = i
			continue
		}
		known *= d
	}
	if free >= 0 {
		if known == 0 || total%known != 0 {
			return fmt.Errorf("cannot reshape %d values into %v", total, target)
		}
		target[free] = total / known
	} else if known != total {
		return fmt.Errorf("cannot reshape %d values into %v", total, target)
	}

	share(y, x, target)
	return nil
}

func squeeze(o *OpDesc, in, out []*Tensor) error {
	x, y := in[0], out[0]
	rank := len(x.Shape)

	drop := make([]bool, rank)
	dims := o.dims
	if len(dims) == 0 {
		for d, n := range x.Shape {
			drop[d] = n == 1
		}
	} else {
		for _, d := range dims {
			if d < 0 {
				d += rank
			}
			if d < 0 || d >= rank {
				return fmt.Errorf("squeeze dim %d out of range for shape %v", d, x.Shape)
			}
			if x.Shape[d] != 1 {
				return fmt.Errorf("cannot squeeze axis %d of shape %v", d, x.Shape)
			}
			drop[d] = true
		}
	}

	var shape []int
	for d, n := range x.Shape {
		if !drop[d] {
			shape = append(shape, n)
		}
	}
	share(y, x, shape)
	return nil
}

func expandDims(o *OpDesc, in, out []*Tensor) error {
	x, a, y := in[0], in[1], out[0]
	if len(a.I32) == 0 {
		return fmt.Errorf("no axis")
	}
	axis := int(a.I32[0])
	if axis < 0 {
		axis += len(x.Shape) + 1
	}
	if axis < 0 || axis > len(x.Shape) {
		return fmt.Errorf("axis %d out of range for shape %v", a.I32[0], x.Shape)
	}

	shape := make([]int, 0, len(x.Shape)+1)
	shape = append(shape, x.Shape[:axis]...)
	shape = append(shape, 1)
	shape = append(shape, x.Shape[axis:]...)
	share(y, x, shape)
	return nil
}

var splitKernels = map[Op]splitKernel{
	OpConv2D:        conv2D,
	OpDepthwiseConv: depthwiseConv2D,
	OpBatchMatMul:   batchMatMul,
	OpSoftmax:       softmax,
	OpTranspose:     transpose,

	OpAdd:         elementwise(addRun),
	OpSub:         elementwise(subRun),
	OpMul:         elementwise(mulRun),
	OpDiv:         elementwise(divRun),
	OpMaximum:     elementwise(maxRun),
	OpMinimum:     elementwise(minRun),
	OpSquaredDiff: elementwise(squaredDiffRun),
}

var kernels = withSerial(map[Op]kernel{
	OpMaxPool2D:      maxPool2D,
	OpFullyConnected: fullyConnected,
	OpSVDF:           svdf,

	OpGreaterEqual: compare(func(a, b float32) bool { return a >= b }),

	OpLogistic: unary(func(v float32) float32 {
		return float32(1 / (1 + math.Exp(-float64(v))))
	}),
	OpLog:    unary(func(v float32) float32 { return float32(math.Log(float64(v))) }),
	OpExp:    unary(func(v float32) float32 { return float32(math.Exp(float64(v))) }),
	OpSqrt:   unary(func(v float32) float32 { return float32(math.Sqrt(float64(v))) }),
	OpRsqrt:  unary(func(v float32) float32 { return 1 / float32(math.Sqrt(float64(v))) }),
	OpSquare: unary(func(v float32) float32 { return v * v }),
	OpRelu:   unary(func(v float32) float32 { return max(v, 0) }),
	OpRelu6:  unary(func(v float32) float32 { return min(max(v, 0), 6) }),
	OpTanh:   unary(func(v float32) float32 { return float32(math.Tanh(float64(v))) }),

	OpLeakyRelu: leakyRelu,
	OpGelu:      gelu,

	OpMean:      reduceOp(sumReducer, true),
	OpSum:       reduceOp(sumReducer, false),
	OpReduceMax: reduceOp(maxReducer, false),

	OpPad:        padZero,
	OpReshape:    reshape,
	OpSqueeze:    squeeze,
	OpExpandDims: expandDims,

	OpShape:         shapeOf,
	OpFill:          fill,
	OpPack:          pack,
	OpStridedSlice:  stridedSlice,
	OpConcatenation: concatenation,
	OpGatherND:      gatherND,
	OpBroadcastTo:   broadcastTo,
	OpReduceProd:    reduceProdI32,
})

func withSerial(plain map[Op]kernel) map[Op]kernel {
	for op, k := range splitKernels {
		plain[op] = inOrder(k)
	}
	return plain
}
