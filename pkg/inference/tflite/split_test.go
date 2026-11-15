package tflite

import (
	"math/rand/v2"
	"slices"
	"sync"
	"testing"
)

type threeWay struct{}

func (threeWay) Split(n int, part func(lo, hi int)) {
	var wg sync.WaitGroup
	size := (n + 2) / 3
	for lo := 0; lo < n; lo += size {
		wg.Go(func() { part(lo, min(lo+size, n)) })
	}
	wg.Wait()
}

func TestSplitKernelsMatchSerial(t *testing.T) {
	r := rand.New(rand.NewPCG(9, 10))
	rnd := func(shape ...int) *Tensor {
		x := tf(shape)
		for i := range x.F32 {
			x.F32[i] = float32(r.NormFloat64())
		}
		return x
	}
	unit := convParams{strideW: 1, strideH: 1, dilationW: 1, dilationH: 1}
	same := unit
	same.samePad = true
	strided := same
	strided.strideW, strided.strideH = 2, 2

	cases := []struct {
		name string
		op   Op
		o    *OpDesc
		in   []*Tensor
	}{
		{
			"pointwise",
			OpConv2D,
			&OpDesc{conv: unit},
			[]*Tensor{rnd(1, 1, 301, 32), rnd(36, 1, 1, 32), rnd(36)},
		},
		{
			"depthwise",
			OpDepthwiseConv,
			&OpDesc{conv: same},
			[]*Tensor{rnd(1, 36, 150, 32), rnd(1, 3, 3, 32), rnd(32)},
		},
		{
			"depthwise strided",
			OpDepthwiseConv,
			&OpDesc{conv: strided},
			[]*Tensor{rnd(1, 36, 150, 32), rnd(1, 3, 3, 32), rnd(32)},
		},
		{"matmul", OpBatchMatMul, &OpDesc{adjY: true}, []*Tensor{rnd(4, 150, 9), rnd(4, 150, 9)}},
		{"matmul plain", OpBatchMatMul, &OpDesc{}, []*Tensor{rnd(4, 150, 150), rnd(4, 150, 9)}},
		{"softmax", OpSoftmax, &OpDesc{beta: 1}, []*Tensor{rnd(4, 150, 150)}},
		{
			"transpose planes",
			OpTranspose,
			&OpDesc{},
			[]*Tensor{rnd(1, 72, 300, 16), ti([]int{4}, 0, 1, 3, 2)},
		},
		{
			"transpose plane",
			OpTranspose,
			&OpDesc{},
			[]*Tensor{rnd(1, 300, 1152), ti([]int{3}, 0, 2, 1)},
		},
		{
			"transpose runs",
			OpTranspose,
			&OpDesc{},
			[]*Tensor{rnd(1, 75, 4, 180), ti([]int{4}, 0, 2, 1, 3)},
		},
		{
			"add one run",
			OpAdd,
			&OpDesc{act: ActRelu},
			[]*Tensor{rnd(1, 36, 300, 32), rnd(1, 36, 300, 32)},
		},
		{"mul broadcast", OpMul, &OpDesc{}, []*Tensor{rnd(1, 36, 300, 32), rnd(1, 36, 1, 32)}},
		{"sub scalar", OpSub, &OpDesc{}, []*Tensor{rnd(1), rnd(1, 72, 300, 16)}},
	}
	for _, tc := range cases {
		want, got := out(), out()
		if err := kernels[tc.op](tc.o, tc.in, []*Tensor{want}); err != nil {
			t.Fatalf("%s serial: %v", tc.name, err)
		}
		if err := splitKernels[tc.op](threeWay{}, tc.o, tc.in, []*Tensor{got}); err != nil {
			t.Fatalf("%s split: %v", tc.name, err)
		}
		shape(t, tc.name, got.Shape, want.Shape)
		if !slices.Equal(got.F32, want.F32) {
			t.Errorf("%s: split result differs from serial", tc.name)
		}
	}
}
