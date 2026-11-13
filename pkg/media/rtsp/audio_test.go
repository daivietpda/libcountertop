package rtsp

import (
	"context"
	"encoding/binary"
	"math"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	aac "github.com/tphakala/go-aac"
)

func toneAUs(t *testing.T, enc *aac.Encoder, frames int) [][]byte {
	t.Helper()
	var out [][]byte
	pcm := make([]float32, aac.FrameSize)
	for f := range frames {
		for i := range pcm {
			n := f*aac.FrameSize + i
			pcm[i] = 0.3 * float32(math.Sin(2*math.Pi*440*float64(n)/48000))
		}
		au, err := enc.EncodeFrame(nil, [][]float32{pcm})
		if err != nil {
			t.Fatal(err)
		}
		if len(au) > 0 {
			out = append(out, au)
		}
	}
	return out
}

func TestFFmpegHearsAudio(t *testing.T) {
	video := testH264(t)
	enc, err := aac.NewEncoder(aac.EncoderConfig{SampleRate: 48000, Channels: 1, Bitrate: 48000})
	if err != nil {
		t.Fatal(err)
	}
	sound := toneAUs(t, enc, 200)

	srv := NewServer()
	st := NewStream()
	st.Audio = &AAC{Rate: 48000, Channels: 1, Config: enc.AudioSpecificConfig()}
	srv.Handle("main", st)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()

	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go func() {
		tick := time.NewTicker(time.Second / 15)
		defer tick.Stop()
		for i := 0; ; i++ {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
			st.Write(video[i%len(video)], time.Duration(i)*time.Second/15)
		}
	}()
	go func() {
		frame := time.Duration(aac.FrameSize) * time.Second / 48000
		tick := time.NewTicker(frame)
		defer tick.Stop()
		for i := 0; ; i++ {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
			st.WriteAudio(sound[i%len(sound)], time.Duration(i)*frame)
		}
	}()

	for _, transport := range []string{"tcp", "udp"} {
		t.Run(transport, func(t *testing.T) {
			run, cancel := context.WithTimeout(ctx, 20*time.Second)
			defer cancel()
			wav := filepath.Join(t.TempDir(), "heard.wav")
			out, err := exec.CommandContext(
				run,
				"ffmpeg",
				"-v",
				"error",
				"-rtsp_transport",
				transport,
				"-i",
				"rtsp://"+ln.Addr().String()+"/main",
				"-t",
				"2",
				"-map",
				"0:a",
				"-ac",
				"1",
				"-ar",
				"48000",
				"-f",
				"wav",
				wav,
			).CombinedOutput()
			if err != nil {
				t.Fatalf("ffmpeg: %v\n%s", err, out)
			}
			raw, err := os.ReadFile(wav)
			if err != nil {
				t.Fatal(err)
			}
			if len(raw) < 44+48000 {
				t.Fatalf("decoded %d bytes of wav, want at least half a second", len(raw))
			}
			var sum float64
			samples := raw[44:]
			for i := 0; i+1 < len(samples); i += 2 {
				v := float64(int16(binary.LittleEndian.Uint16(samples[i:]))) / (1 << 15)
				sum += v * v
			}
			rms := math.Sqrt(sum / float64(len(samples)/2))
			if rms < 0.1 {
				t.Errorf("decoded audio rms %.3f, want the 0.3 tone (about 0.21)", rms)
			}
		})
	}
}
