//go:build arm64 && !noasm

package vec

// Taking the address of the first element of an empty slice panics.

func Dot(a, b []float32) float32 {
	if len(a) == 0 {
		return 0
	}
	b = b[:len(a)]
	return dotNEON(&a[0], &b[0], len(a))
}

func Dot4(x, w0, w1, w2, w3 []float32) (out [4]float32) {
	dot4Go(&out, x, w0, w1, w2, w3)
	return out
}

func AXPY(dst []float32, gain float32, x []float32) {
	if len(x) == 0 {
		return
	}
	dst = dst[:len(x)]
	axpyNEON(&dst[0], &x[0], len(x), gain)
}

func Dot4x4(x []float32, xs int, w []float32, ws int, n int) (out [16]float32) {
	dot4x4Go(&out, x, xs, w, ws, n)
	return out
}

func Add(dst, a, b []float32) { addGo(dst, a, b) }

func Mul(dst, a, b []float32) { mulGo(dst, a, b) }

func MulAddTiled(dst, src, w []float32) { mulAddTiledGo(dst, src, w) }

func MulAdd(dst, a, b []float32) {
	if len(a) == 0 {
		return
	}
	dst, b = dst[:len(a)], b[:len(a)]
	mulAddNEON(&dst[0], &a[0], &b[0], len(a))
}

//go:noescape
func mulAddNEON(dst, a, b *float32, n int)

//go:noescape
func dotNEON(a, b *float32, n int) float32

//go:noescape
func axpyNEON(dst, x *float32, n int, gain float32)
