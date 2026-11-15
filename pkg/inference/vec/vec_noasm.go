//go:build (!arm64 && !arm) || noasm

package vec

func Dot(a, b []float32) float32 { return dotGo(a, b) }

func Dot4(x, w0, w1, w2, w3 []float32) (out [4]float32) {
	dot4Go(&out, x, w0, w1, w2, w3)
	return out
}

func AXPY(dst []float32, gain float32, x []float32) { axpyGo(dst, gain, x) }

func MulAdd(dst, a, b []float32) { mulAddGo(dst, a, b) }

func Add(dst, a, b []float32) { addGo(dst, a, b) }

func MulAddTiled(dst, src, w []float32) { mulAddTiledGo(dst, src, w) }

func Mul(dst, a, b []float32) { mulGo(dst, a, b) }

func Dot4x4(x []float32, xs int, w []float32, ws int, n int) (out [16]float32) {
	dot4x4Go(&out, x, xs, w, ws, n)
	return out
}
