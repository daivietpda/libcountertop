package compositor

import (
	"image"
	"math"
	"sync"
	"time"

	"github.com/ygelfand/libcountertop/pkg/display/compositor/internal/wayland"
	"github.com/ygelfand/libcountertop/pkg/display/compositor/internal/wire"
	"github.com/ygelfand/libcountertop/pkg/display/compositor/internal/xdg"
	"github.com/ygelfand/libcountertop/pkg/hook"
)

// Frame pixels are wl_shm ARGB8888: B, G, R, A in memory.
type Frame struct {
	Pixels        []byte
	Stride        int
	Width, Height int
	Damage        []image.Rectangle
}

type Window struct {
	cl      *client
	surface *surface
	top     *xdg.Toplevel
	xs      *xdg.Surface

	mu            sync.Mutex
	title         string
	width, height int
	scale         uint32
	suspended     bool
	held          []*wayland.Callback

	Frames hook.Hook[Frame]
	Titled hook.Hook[string]
	Closed hook.Hook[struct{}]
}

func (w *Window) Title() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.title
}

func (w *Window) Size() (width, height int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.width, w.height
}

func (w *Window) Resize(width, height int) {
	w.mu.Lock()
	w.width, w.height = width, height
	w.mu.Unlock()
	w.configure()
}

func (w *Window) SetScale(scale float64) {
	n := uint32(math.Round(scale * fractionDenominator))
	w.mu.Lock()
	changed := n != w.scale
	w.scale = n
	w.mu.Unlock()
	if changed {
		w.sendScale()
	}
}

func (w *Window) sendScale() {
	w.mu.Lock()
	n := w.scale
	w.mu.Unlock()
	if n == 0 {
		return
	}
	w.cl.mu.Lock()
	f := w.surface.fraction
	w.cl.mu.Unlock()
	if f != nil {
		f.PreferredScale(n)
	}
}

func (w *Window) Suspend() {
	w.mu.Lock()
	w.suspended = true
	w.mu.Unlock()
}

func (w *Window) Resume() {
	w.mu.Lock()
	held := w.held
	w.suspended, w.held = false, nil
	w.mu.Unlock()
	w.cl.answer(held)
}

func (w *Window) hold(frames []*wayland.Callback) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.suspended {
		return false
	}
	w.held = append(w.held, frames...)
	return true
}

func (w *Window) TouchDown(id int, x, y float64) {
	if t := w.cl.touchFor(); t != nil {
		t.Down(w.cl.next(), stamp(), w.surface.s, int32(id), wire.FixedFloat(x), wire.FixedFloat(y))
		t.Frame()
	}
}

func (w *Window) TouchMotion(id int, x, y float64) {
	if t := w.cl.touchFor(); t != nil {
		t.Motion(stamp(), int32(id), wire.FixedFloat(x), wire.FixedFloat(y))
		t.Frame()
	}
}

func (w *Window) TouchUp(id int) {
	if t := w.cl.touchFor(); t != nil {
		t.Up(w.cl.next(), stamp(), int32(id))
		t.Frame()
	}
}

func (w *Window) configure() {
	width, height := w.Size()
	w.top.Configure(
		int32(width),
		int32(height),
		states(xdg.ToplevelStateActivated, xdg.ToplevelStateFullscreen),
	)
	w.xs.Configure(w.cl.next())
}

func (w *Window) retitle(title string) {
	w.mu.Lock()
	w.title = title
	w.mu.Unlock()
	w.Titled.Emit(title)
}

func states(s ...xdg.ToplevelState) []byte {
	out := make([]byte, 0, 4*len(s))
	for _, v := range s {
		out = append(out, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
	}
	return out
}

const fractionDenominator = 120

var epoch = time.Now()

func stamp() uint32 { return uint32(time.Since(epoch).Milliseconds()) }
