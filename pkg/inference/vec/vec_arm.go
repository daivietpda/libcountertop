//go:build arm && !noasm

package vec

import "golang.org/x/sys/cpu"

var neon = cpu.ARM.HasNEON

// Taking the address of the first element of an empty slice panics.

func Dot(a, b []float32) float32 {
	if len(a) == 0 {
		return 0
	}
	b = b[:len(a)]
	if neon {
		return dotNEON(&a[0], &b[0], len(a))
	}
	return dotVFP(&a[0], &b[0], len(a))
}

func Dot4(x, w0, w1, w2, w3 []float32) (out [4]float32) {
	n := len(x)
	w0, w1, w2, w3 = w0[:n], w1[:n], w2[:n], w3[:n]
	m := n &^ 3
	if !neon || m == 0 {
		dot4Go(&out, x, w0, w1, w2, w3)
		return out
	}
	dot4NEON(&x[0], &w0[0], &w1[0], &w2[0], &w3[0], m, &out)
	if m < n {
		dot4Go(&out, x[m:], w0[m:], w1[m:], w2[m:], w3[m:])
	}
	return out
}

func AXPY(dst []float32, gain float32, x []float32) {
	if len(x) == 0 {
		return
	}
	dst = dst[:len(x)]
	axpyVFP(&dst[0], &x[0], len(x), gain)
}

func Dot4x4(x []float32, xs int, w []float32, ws int, n int) (out [16]float32) {
	if n == 0 {
		return out
	}
	_, _ = x[3*xs+n-1], w[3*ws+n-1]
	m := n &^ 3
	if neon && m > 0 {
		dot4x4NEON(&x[0], &x[xs], &x[2*xs], &x[3*xs], &w[0], &w[ws], &w[2*ws], &w[3*ws], m, &out)
	} else {
		m = 0
	}
	if m < n {
		dot4x4Go(&out, x[m:], xs, w[m:], ws, n-m)
	}
	return out
}

//go:noescape
func dot4x4NEON(x0, x1, x2, x3, w0, w1, w2, w3 *float32, n int, out *[16]float32)

func Add(dst, a, b []float32) {
	if len(dst) == 0 {
		return
	}
	a, b = a[:len(dst)], b[:len(dst)]
	if !neon {
		addGo(dst, a, b)
		return
	}
	addNEON(&dst[0], &a[0], &b[0], len(dst))
}

func Mul(dst, a, b []float32) {
	if len(dst) == 0 {
		return
	}
	a, b = a[:len(dst)], b[:len(dst)]
	if !neon {
		mulGo(dst, a, b)
		return
	}
	mulNEON(&dst[0], &a[0], &b[0], len(dst))
}

//go:noescape
func addNEON(dst, a, b *float32, n int)

//go:noescape
func mulNEON(dst, a, b *float32, n int)

func MulAddTiled(dst, src, w []float32) {
	c := len(w)
	if c == 0 || len(dst) == 0 {
		return
	}
	src = src[:len(dst)]
	rows := len(dst) / c
	if !neon || c%4 != 0 || rows*c != len(dst) {
		mulAddTiledGo(dst, src, w)
		return
	}
	mulAddTiledNEON(&dst[0], &src[0], &w[0], rows, c)
}

//go:noescape
func mulAddTiledNEON(dst, src, w *float32, rows, c int)

func MulAdd(dst, a, b []float32) {
	if len(a) == 0 {
		return
	}
	dst, b = dst[:len(a)], b[:len(a)]
	if !neon {
		mulAddGo(dst, a, b)
		return
	}
	mulAddNEON(&dst[0], &a[0], &b[0], len(a))
}

//go:noescape
func mulAddNEON(dst, a, b *float32, n int)

//go:noescape
func dotVFP(a, b *float32, n int) float32

//go:noescape
func dotNEON(a, b *float32, n int) float32

//go:noescape
func dot4NEON(x, w0, w1, w2, w3 *float32, n int, out *[4]float32)

//go:noescape
func axpyVFP(dst, x *float32, n int, gain float32)
