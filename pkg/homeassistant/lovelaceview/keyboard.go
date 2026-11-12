package lovelaceview

import (
	gogui "github.com/go-gui-org/go-gui/gui"

	"github.com/ygelfand/libcountertop/pkg/media/cast/engine"
)

type key struct {
	name string
	code int
	text string
}

var keys = map[gogui.KeyCode]key{
	gogui.KeyBackspace: {name: "Backspace", code: 8},
	gogui.KeyEnter:     {name: "Enter", code: 13, text: "\r"},
}

func (k key) event(kind string) map[string]any {
	e := map[string]any{
		"type":                  kind,
		"key":                   k.name,
		"code":                  k.name,
		"windowsVirtualKeyCode": k.code,
		"nativeVirtualKeyCode":  k.code,
	}
	if kind == "keyDown" {
		e["text"] = k.text
	}
	return e
}

func (v *View) Typing() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.field && v.page != nil
}

func (v *View) Type(ch rune) {
	if page := v.current(); page != nil {
		_ = page.Call("Input.insertText", map[string]any{"text": string(ch)})
	}
}

func (v *View) Press(code gogui.KeyCode) {
	k, ok := keys[code]
	page := v.current()
	if !ok || page == nil {
		return
	}
	down := "rawKeyDown"
	if k.text != "" {
		down = "keyDown"
	}
	_ = page.Call("Input.dispatchKeyEvent", k.event(down))
	_ = page.Call("Input.dispatchKeyEvent", k.event("keyUp"))
}

func (v *View) Hide() {
	if page := v.current(); page != nil {
		answer(page, "countertopHide()")
	}
	v.typing(false)
}

func (v *View) current() *engine.Page {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.page
}

func (v *View) typing(on bool) {
	v.mu.Lock()
	changed := v.field != on
	v.field = on
	v.mu.Unlock()
	if changed {
		v.redraw()
	}
}
