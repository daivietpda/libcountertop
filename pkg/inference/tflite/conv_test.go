package tflite

import (
	"math"
	"math/rand/v2"
	"testing"
)

func TestConv2DOverManyChannelsMatchesADirectSum(t *testing.T) {
	const (
		ih, iw, ic = 5, 6, 32
		oc, kh, kw = 6, 3, 3
	)
	r := rand.New(rand.NewPCG(1, 2))
	fill := func(n int) []float32 {
		out := make([]float32, n)
		for i := range out {
			out[i] = float32(r.NormFloat64())
		}
		return out
	}
	x := tf([]int{1, ih, iw, ic}, fill(ih*iw*ic)...)
	w := tf([]int{oc, kh, kw, ic}, fill(oc*kh*kw*ic)...)
	bias := tf([]int{oc}, fill(oc)...)
	y := out()

	p := convParams{samePad: true, strideW: 1, strideH: 1, dilationW: 1, dilationH: 1}
	if err := kernels[OpConv2D](
		&OpDesc{Op: OpConv2D, conv: p},
		[]*Tensor{x, w, bias},
		[]*Tensor{y},
	); err != nil {
		t.Fatal(err)
	}
	shape(t, "conv", y.Shape, []int{1, ih, iw, oc})

	padH, padW := padFor(1, 1, ih, kh, ih), padFor(1, 1, iw, kw, iw)
	for oy := range ih {
		for ox := range iw {
			for f := range oc {
				want := float64(bias.F32[f])
				for ky := range kh {
					for kx := range kw {
						iy, ix := oy-padH+ky, ox-padW+kx
						if iy < 0 || iy >= ih || ix < 0 || ix >= iw {
							continue
						}
						for c := range ic {
							want += float64(
								x.F32[(iy*iw+ix)*ic+c],
							) * float64(
								w.F32[((f*kh+ky)*kw+kx)*ic+c],
							)
						}
					}
				}
				got := y.F32[(oy*iw+ox)*oc+f]
				if math.Abs(float64(got)-want) > 1e-3 {
					t.Fatalf("output (%d,%d) filter %d: %v, want %v", oy, ox, f, got, want)
				}
			}
		}
	}
}

func TestBatchMatMulAdjointsMatchADirectSum(t *testing.T) {
	const batches, m, n, k = 2, 5, 6, 7
	r := rand.New(rand.NewPCG(5, 6))
	fill := func(c int) []float32 {
		out := make([]float32, c)
		for i := range out {
			out[i] = float32(r.NormFloat64())
		}
		return out
	}
	for _, adj := range [][2]bool{{false, false}, {true, false}, {false, true}, {true, true}} {
		adjX, adjY := adj[0], adj[1]
		as, bs := []int{batches, m, k}, []int{batches, k, n}
		if adjX {
			as = []int{batches, k, m}
		}
		if adjY {
			bs = []int{batches, n, k}
		}
		a, b := tf(as, fill(batches*m*k)...), tf(bs, fill(batches*k*n)...)
		y := out()
		o := &OpDesc{adjX: adjX, adjY: adjY}
		if err := kernels[OpBatchMatMul](o, []*Tensor{a, b}, []*Tensor{y}); err != nil {
			t.Fatal(err)
		}
		shape(t, "matmul", y.Shape, []int{batches, m, n})
		for bt := range batches {
			for i := range m {
				for j := range n {
					var want float64
					for p := range k {
						av := a.F32[bt*m*k+i*k+p]
						if adjX {
							av = a.F32[bt*m*k+p*m+i]
						}
						bv := b.F32[bt*k*n+p*n+j]
						if adjY {
							bv = b.F32[bt*k*n+j*k+p]
						}
						want += float64(av) * float64(bv)
					}
					if got := y.F32[bt*m*n+i*n+j]; math.Abs(float64(got)-want) > 1e-4 {
						t.Fatalf("adj %v batch %d (%d,%d): %v, want %v", adj, bt, i, j, got, want)
					}
				}
			}
		}
	}
}

func TestTransposeThreeAxes(t *testing.T) {
	x := tf([]int{2, 3, 4})
	for i := range x.F32 {
		x.F32[i] = float32(i)
	}
	y := out()
	if err := kernels[OpTranspose](
		&OpDesc{},
		[]*Tensor{x, ti([]int{3}, 2, 0, 1)},
		[]*Tensor{y},
	); err != nil {
		t.Fatal(err)
	}
	shape(t, "transpose", y.Shape, []int{4, 2, 3})
	for c := range 4 {
		for a := range 2 {
			for b := range 3 {
				if got, want := y.F32[(c*2+a)*3+b], x.F32[(a*3+b)*4+c]; got != want {
					t.Fatalf("(%d,%d,%d): %v, want %v", c, a, b, got, want)
				}
			}
		}
	}
}

func TestDepthwiseConvMatchesADirectSum(t *testing.T) {
	r := rand.New(rand.NewPCG(7, 8))
	fill := func(n int) []float32 {
		out := make([]float32, n)
		for i := range out {
			out[i] = float32(r.NormFloat64())
		}
		return out
	}
	cases := []struct {
		ih, iw, ic, kh, kw, stride, dilation int
		same                                 bool
	}{
		{5, 9, 36, 3, 3, 1, 1, true},
		{1, 20, 36, 1, 7, 1, 1, true},
		{4, 11, 8, 3, 3, 1, 2, true},
		{6, 10, 12, 3, 3, 1, 1, false},
		{6, 10, 12, 3, 3, 2, 1, true},
	}
	for _, tc := range cases {
		x := tf([]int{1, tc.ih, tc.iw, tc.ic}, fill(tc.ih*tc.iw*tc.ic)...)
		w := tf([]int{1, tc.kh, tc.kw, tc.ic}, fill(tc.kh*tc.kw*tc.ic)...)
		bias := tf([]int{tc.ic}, fill(tc.ic)...)
		y := out()
		p := convParams{
			samePad: tc.same, strideW: tc.stride, strideH: tc.stride,
			dilationW: tc.dilation, dilationH: tc.dilation,
		}
		if err := kernels[OpDepthwiseConv](
			&OpDesc{conv: p},
			[]*Tensor{x, w, bias},
			[]*Tensor{y},
		); err != nil {
			t.Fatal(err)
		}
		oh, ow := y.Shape[1], y.Shape[2]
		padH, padW := 0, 0
		if tc.same {
			padH = padFor(tc.stride, tc.dilation, tc.ih, tc.kh, oh)
			padW = padFor(tc.stride, tc.dilation, tc.iw, tc.kw, ow)
		}
		for oy := range oh {
			for ox := range ow {
				for c := range tc.ic {
					want := float64(bias.F32[c])
					for ky := range tc.kh {
						for kx := range tc.kw {
							iy := oy*tc.stride - padH + ky*tc.dilation
							ix := ox*tc.stride - padW + kx*tc.dilation
							if iy < 0 || iy >= tc.ih || ix < 0 || ix >= tc.iw {
								continue
							}
							want += float64(x.F32[(iy*tc.iw+ix)*tc.ic+c]) *
								float64(w.F32[(ky*tc.kw+kx)*tc.ic+c])
						}
					}
					if got := y.F32[(oy*ow+ox)*tc.ic+c]; math.Abs(float64(got)-want) > 1e-4 {
						t.Fatalf("%+v (%d,%d,%d): %v, want %v", tc, oy, ox, c, got, want)
					}
				}
			}
		}
	}
}

func TestPointwiseConvMatchesADirectSum(t *testing.T) {
	const pixels, ic, oc = 7, 13, 6
	r := rand.New(rand.NewPCG(3, 4))
	fill := func(n int) []float32 {
		out := make([]float32, n)
		for i := range out {
			out[i] = float32(r.NormFloat64())
		}
		return out
	}
	x := tf([]int{1, 1, pixels, ic}, fill(pixels*ic)...)
	w := tf([]int{oc, 1, 1, ic}, fill(oc*ic)...)
	bias := tf([]int{oc}, fill(oc)...)
	y := out()

	p := convParams{strideW: 1, strideH: 1, dilationW: 1, dilationH: 1}
	if err := kernels[OpConv2D](&OpDesc{conv: p}, []*Tensor{x, w, bias}, []*Tensor{y}); err != nil {
		t.Fatal(err)
	}
	shape(t, "pointwise", y.Shape, []int{1, 1, pixels, oc})
	for px := range pixels {
		for f := range oc {
			want := float64(bias.F32[f])
			for c := range ic {
				want += float64(x.F32[px*ic+c]) * float64(w.F32[f*ic+c])
			}
			if got := y.F32[px*oc+f]; math.Abs(float64(got)-want) > 1e-4 {
				t.Fatalf("pixel %d filter %d: %v, want %v", px, f, got, want)
			}
		}
	}
}
