package tflite

import (
	"fmt"
	"math"

	"github.com/ygelfand/libcountertop/pkg/inference/vec"
)

func conv2D(s Splitter, o *OpDesc, in, out []*Tensor) error {
	p := o.conv
	x, w, y := in[0], in[1], out[0]
	if len(x.Shape) != 4 || len(w.Shape) != 4 {
		return fmt.Errorf("expected 4-D input and filter, got %v and %v", x.Shape, w.Shape)
	}

	batch, ih, iw, ic := x.Shape[0], x.Shape[1], x.Shape[2], x.Shape[3]
	oc, kh, kw := w.Shape[0], w.Shape[1], w.Shape[2]
	if w.Shape[3] != ic {
		return fmt.Errorf("filter has %d input channels, input has %d", w.Shape[3], ic)
	}

	var bias []float32
	if len(in) > 2 && in[2] != nil {
		bias = in[2].F32
	}

	oh := outputSize(ih, kh, p.strideH, p.dilationH, p.samePad)
	ow := outputSize(iw, kw, p.strideW, p.dilationW, p.samePad)
	y.resize([]int{batch, oh, ow, oc})

	if kh == 1 && kw == 1 && p.strideH == 1 && p.strideW == 1 {
		pointwise(s, y.F32, x.F32, w.F32, bias, batch*ih*iw, ic, oc, p.act)
		return nil
	}

	padH, padW := 0, 0
	if p.samePad {
		padH = padFor(p.strideH, p.dilationH, ih, kh, oh)
		padW = padFor(p.strideW, p.dilationW, iw, kw, ow)
	}

	xd, wd, yd := x.F32, w.F32, y.F32
	fs := kh * kw * ic
	yi := 0
	for b := 0; b < batch; b++ {
		for oy := 0; oy < oh; oy++ {
			for ox := 0; ox < ow; ox++ {
				f := 0
				for ; ic > 1 && f+4 <= oc; f += 4 {
					var acc [4]float32
					if bias != nil {
						copy(acc[:], bias[f:f+4])
					}
					for ky := 0; ky < kh; ky++ {
						iy := oy*p.strideH - padH + ky*p.dilationH
						if iy < 0 || iy >= ih {
							continue
						}
						for kx := 0; kx < kw; kx++ {
							ix := ox*p.strideW - padW + kx*p.dilationW
							if ix < 0 || ix >= iw {
								continue
							}
							xo := ((b*ih+iy)*iw + ix) * ic
							wo := ((f*kh+ky)*kw + kx) * ic
							s := vec.Dot4(
								xd[xo:xo+ic],
								wd[wo:wo+ic],
								wd[wo+fs:wo+fs+ic],
								wd[wo+2*fs:wo+2*fs+ic],
								wd[wo+3*fs:wo+3*fs+ic],
							)
							acc[0] += s[0]
							acc[1] += s[1]
							acc[2] += s[2]
							acc[3] += s[3]
						}
					}
					for _, a := range acc {
						yd[yi] = activate(a, p.act)
						yi++
					}
				}
				for ; f < oc; f++ {
					var acc float32
					if bias != nil {
						acc = bias[f]
					}
					for ky := 0; ky < kh; ky++ {
						iy := oy*p.strideH - padH + ky*p.dilationH
						if iy < 0 || iy >= ih {
							continue
						}

						if ic == 1 && p.dilationW == 1 {
							x0 := ox*p.strideW - padW
							lo, hi := max(0, -x0), min(kw, iw-x0)
							if hi > lo {
								xo := (b*ih+iy)*iw + x0 + lo
								wo := (f*kh+ky)*kw + lo
								acc += vec.Dot(wd[wo:wo+hi-lo], xd[xo:xo+hi-lo])
							}
							continue
						}

						for kx := 0; kx < kw; kx++ {
							ix := ox*p.strideW - padW + kx*p.dilationW
							if ix < 0 || ix >= iw {
								continue
							}
							xo := ((b*ih+iy)*iw + ix) * ic
							wo := ((f*kh+ky)*kw + kx) * ic
							acc += vec.Dot(xd[xo:xo+ic], wd[wo:wo+ic])
						}
					}
					yd[yi] = activate(acc, p.act)
					yi++
				}
			}
		}
	}
	return nil
}

func pointwise(
	s Splitter,
	yd, xd, wd, bias []float32,
	pixels, ic, oc int,
	act Activation,
) {
	spread(s, pixels*oc*ic/8, (pixels+3)/4, func(lo, hi int) {
		r0, r1 := lo*4, min(hi*4, pixels)
		ys := yd[r0*oc : r1*oc]
		matMulT(ys, xd[r0*ic:r1*ic], wd, r1-r0, oc, ic)
		if bias != nil {
			for at := 0; at < len(ys); at += oc {
				row := ys[at : at+oc]
				addRun(row, row, bias[:oc], 1, 1)
			}
		}
		activateAll(ys, act)
	})
}

func matMulTOn(s Splitter, y, a, b []float32, m, n, k int) {
	spread(s, m*n*k/8, (m+3)/4, func(lo, hi int) {
		r0, r1 := lo*4, min(hi*4, m)
		matMulT(y[r0*n:r1*n], a[r0*k:r1*k], b, r1-r0, n, k)
	})
}

func matMulT(y, a, b []float32, m, n, k int) {
	i := 0
	for ; i+4 <= m; i += 4 {
		ab := a[i*k:]
		j := 0
		for ; j+4 <= n; j += 4 {
			s := vec.Dot4x4(ab, k, b[j*k:], k, k)
			for r := range 4 {
				row := y[(i+r)*n+j : (i+r)*n+j+4 : (i+r)*n+j+4]
				row[0], row[1], row[2], row[3] = s[r*4], s[r*4+1], s[r*4+2], s[r*4+3]
			}
		}
		for ; j < n; j++ {
			bj := b[j*k : (j+1)*k]
			for r := range 4 {
				y[(i+r)*n+j] = vec.Dot(ab[r*k:(r+1)*k], bj)
			}
		}
	}
	for ; i < m; i++ {
		ai := a[i*k : (i+1)*k]
		for j := range n {
			y[i*n+j] = vec.Dot(ai, b[j*k:(j+1)*k])
		}
	}
}

func transposed(dst, src []float32, rows, cols int) []float32 {
	if cap(dst) < rows*cols {
		dst = make([]float32, rows*cols)
	}
	dst = dst[:rows*cols]
	for r := range rows {
		for c, v := range src[r*cols : (r+1)*cols] {
			dst[c*rows+r] = v
		}
	}
	return dst
}

func depthwiseConv2D(s Splitter, o *OpDesc, in, out []*Tensor) error {
	p := o.conv
	x, w, y := in[0], in[1], out[0]
	if len(x.Shape) != 4 || len(w.Shape) != 4 {
		return fmt.Errorf("expected 4-D input and filter, got %v and %v", x.Shape, w.Shape)
	}

	batch, ih, iw, ic := x.Shape[0], x.Shape[1], x.Shape[2], x.Shape[3]
	kh, kw, oc := w.Shape[1], w.Shape[2], w.Shape[3]
	if ic == 0 || oc%ic != 0 {
		return fmt.Errorf("filter has %d channels for %d input channels", oc, ic)
	}
	mult := oc / ic

	var bias []float32
	if len(in) > 2 && in[2] != nil {
		bias = in[2].F32
	}

	oh := outputSize(ih, kh, p.strideH, p.dilationH, p.samePad)
	ow := outputSize(iw, kw, p.strideW, p.dilationW, p.samePad)
	y.resize([]int{batch, oh, ow, oc})

	padH, padW := 0, 0
	if p.samePad {
		padH = padFor(p.strideH, p.dilationH, ih, kh, oh)
		padW = padFor(p.strideW, p.dilationW, iw, kw, ow)
	}

	xd, wd, yd := x.F32, w.F32, y.F32
	rowLen := ow * oc
	tiled := mult == 1 && p.strideW == 1
	spread(s, batch*oh*rowLen*kh*kw/4, batch*oh, func(lo, hi int) {
		for r := lo; r < hi; r++ {
			b, oy := r/oh, r%oh
			yr := yd[r*rowLen : (r+1)*rowLen]
			for px := 0; px < len(yr); px += oc {
				if bias != nil {
					copy(yr[px:px+oc], bias)
				} else {
					clear(yr[px : px+oc])
				}
			}
			for ky := 0; ky < kh; ky++ {
				iy := oy*p.strideH - padH + ky*p.dilationH
				if iy < 0 || iy >= ih {
					continue
				}
				xrow := (b*ih + iy) * iw
				for kx := 0; kx < kw; kx++ {
					ws := wd[(ky*kw+kx)*oc : (ky*kw+kx+1)*oc]
					if tiled {
						x0 := kx*p.dilationW - padW
						l, h := max(0, -x0), min(ow, iw-x0)
						if h > l {
							vec.MulAddTiled(yr[l*oc:h*oc], xd[(xrow+l+x0)*ic:(xrow+h+x0)*ic], ws)
						}
						continue
					}
					for ox := 0; ox < ow; ox++ {
						ix := ox*p.strideW - padW + kx*p.dilationW
						if ix < 0 || ix >= iw {
							continue
						}
						xs := xd[(xrow+ix)*ic : (xrow+ix+1)*ic]
						row := yr[ox*oc : (ox+1)*oc]
						if mult == 1 {
							vec.MulAdd(row, xs, ws)
							continue
						}
						for i, v := range xs {
							vec.AXPY(row[i*mult:(i+1)*mult], v, ws[i*mult:(i+1)*mult])
						}
					}
				}
			}
			activateAll(yr, p.act)
		}
	})
	return nil
}

func gelu(o *OpDesc, in, out []*Tensor) error {
	x, y := in[0], out[0]
	y.resize(x.Shape)
	if o.approximate {
		k := math.Sqrt(2 / math.Pi)
		for i, v := range x.F32 {
			f := float64(v)
			y.F32[i] = float32(0.5 * f * (1 + math.Tanh(k*(f+0.044715*f*f*f))))
		}
		return nil
	}
	for i, v := range x.F32 {
		f := float64(v)
		y.F32[i] = float32(0.5 * f * (1 + math.Erf(f/math.Sqrt2)))
	}
	return nil
}

func maxPool2D(o *OpDesc, in, out []*Tensor) error {
	p := o.pool
	x, y := in[0], out[0]
	if len(x.Shape) != 4 {
		return fmt.Errorf("expected a 4-D input, got %v", x.Shape)
	}

	batch, ih, iw, c := x.Shape[0], x.Shape[1], x.Shape[2], x.Shape[3]
	oh := outputSize(ih, p.filterH, p.strideH, 1, p.samePad)
	ow := outputSize(iw, p.filterW, p.strideW, 1, p.samePad)
	y.resize([]int{batch, oh, ow, c})

	padH, padW := 0, 0
	if p.samePad {
		padH = padFor(p.strideH, 1, ih, p.filterH, oh)
		padW = padFor(p.strideW, 1, iw, p.filterW, ow)
	}

	xd, yd := x.F32, y.F32
	yi := 0
	for b := 0; b < batch; b++ {
		for oy := 0; oy < oh; oy++ {
			y0 := oy*p.strideH - padH
			for ox := 0; ox < ow; ox++ {
				x0 := ox*p.strideW - padW
				for ch := 0; ch < c; ch++ {
					best := float32(math.Inf(-1))
					for ky := 0; ky < p.filterH; ky++ {
						iy := y0 + ky
						if iy < 0 || iy >= ih {
							continue
						}
						for kx := 0; kx < p.filterW; kx++ {
							ix := x0 + kx
							if ix < 0 || ix >= iw {
								continue
							}
							if v := xd[((b*ih+iy)*iw+ix)*c+ch]; v > best {
								best = v
							}
						}
					}
					yd[yi] = activate(best, p.act)
					yi++
				}
			}
		}
	}
	return nil
}

func fullyConnected(o *OpDesc, in, out []*Tensor) error {
	x, w, y := in[0], in[1], out[0]
	if len(w.Shape) < 2 {
		return fmt.Errorf("expected 2-D weights, got %v", w.Shape)
	}

	inC := w.Shape[len(w.Shape)-1]
	oc := count(w.Shape[:len(w.Shape)-1])
	total := x.Count()
	if inC == 0 || total%inC != 0 {
		return fmt.Errorf("input of %d values does not divide into rows of %d", total, inC)
	}
	batch := total / inC

	var bias []float32
	if len(in) > 2 && in[2] != nil {
		bias = in[2].F32
	}

	y.resize([]int{batch, oc})
	act := o.act
	for b := 0; b < batch; b++ {
		row := x.F32[b*inC : (b+1)*inC]
		for f := 0; f < oc; f++ {
			acc := vec.Dot(row, w.F32[f*inC:(f+1)*inC])
			if bias != nil {
				acc += bias[f]
			}
			y.F32[b*oc+f] = activate(acc, act)
		}
	}
	return nil
}

func svdf(o *OpDesc, in, out []*Tensor) error {
	if len(in) < 5 || in[4] == nil {
		return fmt.Errorf("expected input, feature and time weights, bias and state")
	}
	x, feature, time, state, y := in[0], in[1], in[2], in[4], out[0]
	if len(feature.Shape) != 2 || len(time.Shape) != 2 || o.rank <= 0 {
		return fmt.Errorf("feature %v, time %v, rank %d", feature.Shape, time.Shape, o.rank)
	}
	filters, inSize, memory := feature.Shape[0], feature.Shape[1], time.Shape[1]
	if time.Shape[0] != filters || filters%o.rank != 0 || x.Count()%inSize != 0 {
		return fmt.Errorf(
			"feature %v, time %v, rank %d, input %v",
			feature.Shape,
			time.Shape,
			o.rank,
			x.Shape,
		)
	}
	units := filters / o.rank
	batch := x.Count() / inSize
	if state.Count() != batch*filters*memory {
		return fmt.Errorf("state of %d values, want %d", state.Count(), batch*filters*memory)
	}
	var bias []float32
	if in[3] != nil {
		bias = in[3].F32
	}

	y.resize([]int{batch, units})
	for b := 0; b < batch; b++ {
		row := x.F32[b*inSize : (b+1)*inSize]
		s := state.F32[b*filters*memory : (b+1)*filters*memory]
		copy(s, s[1:])
		for f := 0; f < filters; f++ {
			s[f*memory+memory-1] = vec.Dot(row, feature.F32[f*inSize:(f+1)*inSize])
		}
		for u := 0; u < units; u++ {
			var acc float32
			for r := 0; r < o.rank; r++ {
				f := u*o.rank + r
				acc += vec.Dot(s[f*memory:(f+1)*memory], time.F32[f*memory:(f+1)*memory])
			}
			if bias != nil {
				acc += bias[u]
			}
			y.F32[b*units+u] = activate(acc, o.act)
		}
	}
	return nil
}

// Cephes expf.
func exp32(x float32) float32 {
	switch {
	case x > 88.72283:
		return float32(math.Inf(1))
	case x < -87.33654:
		return 0
	case x != x:
		return x
	}
	n := float32(math.Floor(float64(x*1.442695041 + 0.5)))
	z := x - n*0.693359375 - n*-2.12194440e-4
	p := float32(1.9875691500e-4)
	p = p*z + 1.3981999507e-3
	p = p*z + 8.3334519073e-3
	p = p*z + 4.1665795894e-2
	p = p*z + 1.6666665459e-1
	p = p*z + 5.0000001201e-1
	p = p*z*z + z + 1
	if n > 127 {
		return p * 2 * math.Float32frombits(254<<23)
	}
	return p * math.Float32frombits(uint32(int32(n)+127)<<23)
}

func softmax(s Splitter, o *OpDesc, in, out []*Tensor) error {
	x, y := in[0], out[0]
	n := x.Dim(-1)
	if n == 0 || x.Count()%n != 0 {
		return fmt.Errorf("input %v", x.Shape)
	}
	y.resize(x.Shape)
	spread(s, x.Count()*4, x.Count()/n, func(lo, hi int) {
		for at := lo * n; at < hi*n; at += n {
			row, dst := x.F32[at:at+n], y.F32[at:at+n]
			top := row[0]
			for _, v := range row {
				top = max(top, v)
			}
			var sum float64
			for i, v := range row {
				e := exp32(o.beta * (v - top))
				dst[i] = e
				sum += float64(e)
			}
			inv := float32(1 / sum)
			for i := range dst {
				dst[i] *= inv
			}
		}
	})
	return nil
}

func batchMatMul(s Splitter, o *OpDesc, in, out []*Tensor) error {
	a, b, y := in[0], in[1], out[0]
	adjX, adjY := o.adjX, o.adjY
	if len(a.Shape) < 2 || len(b.Shape) < 2 {
		return fmt.Errorf("expected at least 2-D inputs, got %v and %v", a.Shape, b.Shape)
	}

	m, k := a.Dim(-2), a.Dim(-1)
	if adjX {
		m, k = k, m
	}
	kb, n := b.Dim(-2), b.Dim(-1)
	if adjY {
		kb, n = n, kb
	}
	if k != kb {
		return fmt.Errorf("inner dimensions %d and %d do not match", k, kb)
	}

	batchShape, err := broadcastShape(a.Shape[:len(a.Shape)-2], b.Shape[:len(b.Shape)-2])
	if err != nil {
		return err
	}
	y.resize(append(append([]int{}, batchShape...), m, n))

	batches := count(batchShape)
	as := strides(a.Shape[:len(a.Shape)-2], batchShape)
	bs := strides(b.Shape[:len(b.Shape)-2], batchShape)
	idx := make([]int, len(batchShape))

	var ta, tb []float32
	ao, bo := 0, 0
	for batch := 0; batch < batches; batch++ {
		am := a.F32[ao*m*k : (ao+1)*m*k]
		bm := b.F32[bo*kb*n : (bo+1)*kb*n]
		if adjX {
			ta = transposed(ta, am, k, m)
			am = ta
		}
		if !adjY {
			tb = transposed(tb, bm, k, n)
			bm = tb
		}
		matMulTOn(s, y.F32[batch*m*n:(batch+1)*m*n], am, bm, m, n, k)
		ao, bo = step(idx, batchShape, as, bs, ao, bo)
	}
	return nil
}

func step(idx, shape, as, bs []int, ao, bo int) (int, int) {
	for d := len(shape) - 1; d >= 0; d-- {
		idx[d]++
		ao += as[d]
		bo += bs[d]
		if idx[d] < shape[d] {
			return ao, bo
		}
		idx[d] = 0
		ao -= as[d] * shape[d]
		bo -= bs[d] * shape[d]
	}
	return ao, bo
}
