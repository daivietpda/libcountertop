package vec

import (
	"math"
	"math/rand/v2"
	"strconv"
	"testing"
)

// The tflite convolutions reduce over 32 and 64 input channels, mel filters are 512 taps, the AEC filter 512 or 1024.
var lengths = []int{
	0, 1, 2, 3, 4, 5, 6, 7, 8, 11, 12, 13, 15, 16, 17, 19, 20, 28, 31, 32, 33, 47, 63, 64, 65, 96,
	127, 512, 513, 1000, 1024,
}

func random(n int, seed uint64) []float32 {
	r := rand.New(rand.NewPCG(seed, 7))
	out := make([]float32, n)
	for i := range out {
		out[i] = float32(r.NormFloat64())
	}
	return out
}

func TestDotAgreesWithTheGoImplementation(t *testing.T) {
	for _, n := range lengths {
		a, b := random(n, uint64(n)), random(n, uint64(n)+1000)

		var scale float64
		for i := range a {
			scale += math.Abs(float64(a[i]) * float64(b[i]))
		}

		want, got := dotGo(a, b), Dot(a, b)
		if d := math.Abs(float64(got-want)) / math.Max(1, scale); d > 1e-6 {
			t.Errorf("n=%d: %v against %v, off by %.3g of what was added", n, got, want, d)
		}
	}
}

func TestDotExactly(t *testing.T) {
	for _, n := range lengths {
		a, b := make([]float32, n), make([]float32, n)
		for i := range a {
			a[i], b[i] = 2, float32(i%4+1)
		}

		var want float32
		for i := range a {
			want += a[i] * b[i]
		}
		if got := Dot(a, b); got != want {
			t.Errorf("n=%d: %v, want %v", n, got, want)
		}
	}
}

func TestDot4AgreesWithFourDots(t *testing.T) {
	for _, n := range lengths {
		x := random(n, uint64(n)+6000)
		var w [4][]float32
		for k := range w {
			w[k] = random(n+3, uint64(n*10+k))
		}

		got := Dot4(x, w[0], w[1], w[2], w[3])
		for k := range w {
			var scale float64
			for i := range x {
				scale += math.Abs(float64(x[i]) * float64(w[k][i]))
			}
			want := dotGo(x, w[k][:n])
			if d := math.Abs(float64(got[k]-want)) / math.Max(1, scale); d > 1e-6 {
				t.Errorf("n=%d filter %d: %v against %v, off by %.3g", n, k, got[k], want, d)
			}
		}
	}
}

func TestDot4PanicsOnAShortFilter(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("no panic")
		}
	}()
	x := make([]float32, 8)
	Dot4(x, x, x, make([]float32, 4), x)
}

func BenchmarkDot4(b *testing.B) {
	for _, n := range []int{32, 64} {
		x := random(n, 1)
		var w [4][]float32
		for k := range w {
			w[k] = random(n, uint64(k+2))
		}
		b.Run("dot4/"+strconv.Itoa(n), func(b *testing.B) {
			for range b.N {
				s := Dot4(x, w[0], w[1], w[2], w[3])
				sink = s[0]
			}
		})
		b.Run("4xdot/"+strconv.Itoa(n), func(b *testing.B) {
			for range b.N {
				sink = Dot(x, w[0]) + Dot(x, w[1]) + Dot(x, w[2]) + Dot(x, w[3])
			}
		})
	}
}

func TestDotOfEmptySlices(t *testing.T) {
	if got := Dot(nil, nil); got != 0 {
		t.Errorf("dot of nothing is %v", got)
	}
}

func TestDotPanicsOnAShortSecondSlice(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("no panic")
		}
	}()
	Dot(make([]float32, 8), make([]float32, 4))
}

func TestAXPYAgreesWithTheGoImplementation(t *testing.T) {
	const gain = 0.375

	for _, n := range lengths {
		x := random(n, uint64(n)+2000)
		want, got := random(n, uint64(n)+3000), make([]float32, n)
		copy(got, want)

		axpyGo(want, gain, x)
		AXPY(got, gain, x)

		for i := range want {
			if d := math.Abs(float64(got[i] - want[i])); d > 1e-6 {
				t.Fatalf("n=%d index %d: %v, want %v", n, i, got[i], want[i])
			}
		}
	}
}

func TestMulAddAgreesWithTheGoImplementation(t *testing.T) {
	for _, n := range lengths {
		a, b := random(n, uint64(n)+6000), random(n, uint64(n)+7000)
		want, got := random(n, uint64(n)+8000), make([]float32, n)
		copy(got, want)

		mulAddGo(want, a, b)
		MulAdd(got, a, b)

		for i := range want {
			if d := math.Abs(float64(got[i] - want[i])); d > 1e-5 {
				t.Fatalf("n=%d index %d: %v, want %v", n, i, got[i], want[i])
			}
		}
	}
}

func TestDot4x4AgreesWithDot(t *testing.T) {
	for _, n := range lengths {
		stride := n + 3
		x, w := random(4*stride, uint64(n)+9000), random(4*stride, uint64(n)+9500)
		got := Dot4x4(x, stride, w, stride, n)
		for i := range 4 {
			for j := range 4 {
				want := dotGo(x[i*stride:i*stride+n], w[j*stride:j*stride+n])
				if d := math.Abs(
					float64(got[i*4+j] - want),
				); d > 1e-4*math.Max(
					1,
					math.Abs(float64(want)),
				) {
					t.Fatalf("n=%d (%d,%d): %v, want %v", n, i, j, got[i*4+j], want)
				}
			}
		}
	}
}

func TestAddAndMulAgreeWithTheGoImplementation(t *testing.T) {
	for _, n := range lengths {
		a, b := random(n, uint64(n)+10000), random(n, uint64(n)+11000)
		for name, pair := range map[string][2]func(dst, a, b []float32){
			"add": {Add, addGo},
			"mul": {Mul, mulGo},
		} {
			got, want := make([]float32, n+1), make([]float32, n)
			got[n] = 42
			pair[0](got[:n], a, b)
			pair[1](want, a, b)
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("%s n=%d index %d: %v, want %v", name, n, i, got[i], want[i])
				}
			}
			if got[n] != 42 {
				t.Fatalf("%s n=%d: wrote past the end", name, n)
			}
		}
	}
}

func TestMulAddTiledRepeatsTheWeights(t *testing.T) {
	for _, c := range []int{4, 8, 12, 36, 72, 5} {
		for _, rows := range []int{1, 2, 7} {
			n := rows * c
			src, w := random(n, uint64(n)+12000), random(c, uint64(c)+13000)
			got, want := random(n+1, 99), make([]float32, n)
			copy(want, got)
			got[n] = 42
			MulAddTiled(got[:n], src, w)
			for i := range want {
				want[i] += src[i] * w[i%c]
				if d := math.Abs(float64(got[i] - want[i])); d > 1e-5 {
					t.Fatalf("c=%d rows=%d index %d: %v, want %v", c, rows, i, got[i], want[i])
				}
			}
			if got[n] != 42 {
				t.Fatalf("c=%d rows=%d: wrote past the end", c, rows)
			}
		}
	}
}

func TestMulAddStaysInBounds(t *testing.T) {
	for _, n := range lengths {
		dst := make([]float32, n+1)
		dst[n] = 42
		a, b := random(n, 1), random(n, 2)
		MulAdd(dst[:n], a, b)
		if dst[n] != 42 {
			t.Fatalf("n=%d: wrote past the end", n)
		}
	}
}

func TestAXPYExactly(t *testing.T) {
	for _, n := range lengths {
		x := make([]float32, n)
		dst := make([]float32, n)
		want := make([]float32, n)
		for i := range x {
			x[i] = float32(i%4 + 1)
			dst[i], want[i] = 8, 8+2*float32(i%4+1)
		}

		AXPY(dst, 2, x)
		for i := range want {
			if dst[i] != want[i] {
				t.Fatalf("n=%d index %d: %v, want %v", n, i, dst[i], want[i])
			}
		}
	}
}

func TestAXPYStaysInBounds(t *testing.T) {
	for _, n := range lengths {
		if n == 0 {
			continue
		}

		buf := make([]float32, n+16)
		for i := range buf {
			buf[i] = 99
		}

		x := make([]float32, n)
		for i := range x {
			x[i] = 1
		}

		AXPY(buf[:n], 1, x)
		for i := n; i < len(buf); i++ {
			if buf[i] != 99 {
				t.Fatalf("n=%d: wrote past the end at index %d", n, i)
			}
		}
	}
}

func TestAXPYOfEmptySlices(t *testing.T) {
	AXPY(nil, 1, nil)
}

func TestAXPYPanicsOnAShortDestination(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("no panic")
		}
	}()
	AXPY(make([]float32, 4), 1, make([]float32, 8))
}

func TestSumSquaresExactly(t *testing.T) {
	for _, n := range lengths {
		a := make([]float32, n)
		for i := range a {
			a[i] = float32(i%4 + 1)
		}

		var want float32
		for _, v := range a {
			want += v * v
		}
		if got := SumSquares(a); got != want {
			t.Errorf("n=%d: %v, want %v", n, got, want)
		}
	}
}

var sink float32

func BenchmarkDot(b *testing.B) {
	for _, n := range []int{32, 64, 512, 1024} {
		x, y := make([]float32, n), make([]float32, n)
		for i := range x {
			x[i], y[i] = float32(i)*0.001, float32(n-i)*0.001
		}

		b.Run("asm/"+strconv.Itoa(n), func(b *testing.B) {
			b.SetBytes(int64(n * 4))
			for range b.N {
				sink = Dot(x, y)
			}
		})
		b.Run("go/"+strconv.Itoa(n), func(b *testing.B) {
			b.SetBytes(int64(n * 4))
			for range b.N {
				sink = dotGo(x, y)
			}
		})
	}
}

func BenchmarkAXPY(b *testing.B) {
	for _, n := range []int{32, 64, 512, 1024} {
		x, dst := make([]float32, n), make([]float32, n)
		for i := range x {
			x[i], dst[i] = float32(i)*0.001, float32(n-i)*0.001
		}

		b.Run("asm/"+strconv.Itoa(n), func(b *testing.B) {
			b.SetBytes(int64(n * 4))
			for range b.N {
				AXPY(dst, 1e-9, x)
			}
		})
		b.Run("go/"+strconv.Itoa(n), func(b *testing.B) {
			b.SetBytes(int64(n * 4))
			for range b.N {
				axpyGo(dst, 1e-9, x)
			}
		})
	}
}
