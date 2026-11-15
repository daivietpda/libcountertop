package vec

func dotGo(a, b []float32) float32 {
	b = b[:len(a)]
	var s0, s1, s2, s3 float32
	i := 0
	for ; i+4 <= len(a); i += 4 {
		s0 += a[i] * b[i]
		s1 += a[i+1] * b[i+1]
		s2 += a[i+2] * b[i+2]
		s3 += a[i+3] * b[i+3]
	}
	s := s0 + s1 + s2 + s3
	for ; i < len(a); i++ {
		s += a[i] * b[i]
	}
	return s
}

func dot4Go(out *[4]float32, x, w0, w1, w2, w3 []float32) {
	n := len(x)
	w0, w1, w2, w3 = w0[:n], w1[:n], w2[:n], w3[:n]
	for i, v := range x {
		out[0] += v * w0[i]
		out[1] += v * w1[i]
		out[2] += v * w2[i]
		out[3] += v * w3[i]
	}
}

func axpyGo(dst []float32, gain float32, x []float32) {
	dst = dst[:len(x)]
	for i, v := range x {
		dst[i] += gain * v
	}
}

func dot4x4Go(out *[16]float32, x []float32, xs int, w []float32, ws int, n int) {
	for i := range 4 {
		xr := x[i*xs : i*xs+n]
		for j := range 4 {
			out[i*4+j] += dotGo(xr, w[j*ws:j*ws+n])
		}
	}
}

func mulAddTiledGo(dst, src, w []float32) {
	c := len(w)
	src = src[:len(dst)]
	for at := 0; at < len(dst); at += c {
		end := min(at+c, len(dst))
		mulAddGo(dst[at:end], src[at:end], w[:end-at])
	}
}

func addGo(dst, a, b []float32) {
	a, b = a[:len(dst)], b[:len(dst)]
	for i := range dst {
		dst[i] = a[i] + b[i]
	}
}

func mulGo(dst, a, b []float32) {
	a, b = a[:len(dst)], b[:len(dst)]
	for i := range dst {
		dst[i] = a[i] * b[i]
	}
}

func mulAddGo(dst, a, b []float32) {
	dst, b = dst[:len(a)], b[:len(a)]
	for i, v := range a {
		dst[i] += v * b[i]
	}
}

func SumSquares(a []float32) float32 {
	var s0, s1, s2, s3 float32
	i := 0
	for ; i+4 <= len(a); i += 4 {
		s0 += a[i] * a[i]
		s1 += a[i+1] * a[i+1]
		s2 += a[i+2] * a[i+2]
		s3 += a[i+3] * a[i+3]
	}
	s := s0 + s1 + s2 + s3
	for ; i < len(a); i++ {
		s += a[i] * a[i]
	}
	return s
}
