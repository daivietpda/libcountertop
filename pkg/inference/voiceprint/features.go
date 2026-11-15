package voiceprint

import (
	"math"

	"github.com/ygelfand/libcountertop/pkg/audio/fft"
)

const (
	SampleRate = 16000

	frameLen  = 400
	hop       = 160
	fftLen    = 512
	bins      = fftLen / 2
	Bands     = 72
	lowHz     = 20
	highHz    = 7600
	preemph   = 0.97
	eps       = 1e-8
	maxEnergy = 1 / eps
)

type Frontend struct {
	window [frameLen]float32
	bank   [Bands]band
	fft    *fft.FFT
	buf    []complex64
}

type band struct {
	first   int
	weights []float32
}

func NewFrontend() *Frontend {
	f := &Frontend{fft: fft.New(fftLen), buf: make([]complex64, fftLen)}
	for n := range f.window {
		f.window[n] = float32(0.54 - 0.46*math.Cos(2*math.Pi*float64(n)/(frameLen-1)))
	}
	lo, hi := hzToMel(lowHz), hzToMel(highHz)
	edge := func(i int) float64 { return lo + (hi-lo)*float64(i)/(Bands+1) }
	for b := range Bands {
		l, c, u := edge(b), edge(b+1), edge(b+2)
		for k := 1; k < bins; k++ {
			m := hzToMel(float64(k) * (SampleRate / 2) / (bins - 1))
			w := max(0, min((m-l)/(c-l), (u-m)/(u-c)))
			if w == 0 {
				continue
			}
			if len(f.bank[b].weights) == 0 {
				f.bank[b].first = k
			}
			f.bank[b].weights = append(f.bank[b].weights, float32(w))
		}
	}
	return f
}

func Frames(samples int) int {
	if samples+2*(hop/2) < frameLen {
		return 0
	}
	return (samples+2*(hop/2)-frameLen)/hop + 1
}

func (f *Frontend) Features(samples []float32) []float32 {
	frames := Frames(len(samples))
	out := make([]float32, Bands*frames)
	if frames == 0 {
		return out
	}
	padded := f.emphasized(samples)
	var power [bins]float32
	for t := range frames {
		frame := padded[t*hop : t*hop+frameLen]
		for n, v := range frame {
			f.buf[n] = complex(v*f.window[n], 0)
		}
		clear(f.buf[frameLen:])
		f.fft.Forward(f.buf)
		for k := range power {
			re, im := real(f.buf[k]), imag(f.buf[k])
			power[k] = min(max(re*re+im*im, eps), maxEnergy)
		}
		for b, bd := range f.bank {
			var e float32
			for i, w := range bd.weights {
				e += power[bd.first+i] * w
			}
			out[b*frames+t] = float32(math.Log(float64(min(max(e, eps), maxEnergy) + eps)))
		}
	}
	for b := range Bands {
		row := out[b*frames : (b+1)*frames]
		var sum float64
		for _, v := range row {
			sum += float64(v)
		}
		mean := float32(sum / float64(frames))
		for i := range row {
			row[i] -= mean
		}
	}
	return out
}

func (f *Frontend) emphasized(samples []float32) []float32 {
	var sum, sq float64
	for _, v := range samples {
		sum += float64(v)
	}
	mean := sum / float64(len(samples))
	for _, v := range samples {
		d := float64(v) - mean
		sq += d * d
	}
	scale := 1 / (math.Sqrt(sq/float64(len(samples))) + eps)
	norm := func(i int) float32 { return float32((float64(samples[i]) - mean) * scale) }

	pad := hop / 2
	out := make([]float32, len(samples)+2*pad)
	prev := norm(min(1, len(samples)-1))
	for i := range samples {
		v := norm(i)
		out[pad+i] = v - preemph*prev
		prev = v
	}
	return out
}

func hzToMel(hz float64) float64 { return 2595 * math.Log10(1+hz/700) }
