package settings

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/ygelfand/libcountertop/pkg/display/menu"
	"github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/network/wifi"
	"github.com/ygelfand/libcountertop/pkg/network/wpa"
	"github.com/ygelfand/libcountertop/pkg/runtime/safe"
	"github.com/ygelfand/libcountertop/pkg/say"
)

const (
	wifiScanMost  = 20 * time.Second
	wifiShortest  = 8
	wifiJoinedFor = time.Second
)

type Wifi interface {
	Network() string
	Scan(ctx context.Context) ([]wpa.Network, error)
	Start(ssid, passphrase string) error
	State() wifi.MoveState
}

type WifiPages struct {
	wifi  Wifi
	stack *shell.Shell

	mu       sync.Mutex
	found    []wpa.Network
	scanning bool
	scanned  bool
	failed   string
	mine     time.Time
	seen     time.Time
	closing  time.Time
	ticking  bool
	chosen   string
	focus    bool
	refused  *shell.Busy
}

func NewWifiPages(w Wifi, stack *shell.Shell) *WifiPages {
	return &WifiPages{wifi: w, stack: stack}
}

func (p *WifiPages) Picker() *shell.Page {
	return &shell.Page{
		Title:  say.T("wifi.title"),
		OnOpen: p.scan,
		Build:  p.list,
		Actions: []shell.Action{
			{Label: say.T("wifi.scan"), Role: shell.Plain, Do: p.scan},
		},
		Busy: p.busy,
	}
}

func (p *WifiPages) list() ([]menu.Row, []func(int)) {
	p.mu.Lock()
	found, scanning, scanned, failed := p.found, p.scanning, p.scanned, p.failed
	chosen, focus := p.chosen, p.focus
	p.focus = false
	p.mu.Unlock()
	now := p.wifi.Network()

	var rows []menu.Row
	var acts []func(int)
	note := func(text string) {
		rows = append(rows, menu.Row{Label: text, Dim: true})
		acts = append(acts, nil)
	}
	switch {
	case failed != "":
		note(failed)
	case scanning && len(found) == 0:
		note(say.T("wifi.scanning"))
	case scanned && len(found) == 0:
		note(say.T("wifi.none"))
	}
	for _, n := range found {
		row := menu.Row{Glyph: strength(n), Label: n.SSID}
		switch {
		case n.SSID == now:
			row.Value, row.Chosen = say.T("wifi.connected"), true
			acts = append(acts, nil)
		case !n.Security.Supported():
			row.Dim = true
			row.Value = say.F("wifi.unsupported", map[string]any{"Security": n.Security.String()})
			acts = append(acts, nil)
		case n.Security == wpa.Open:
			acts = append(acts, func(int) { p.start(n, "") })
		default:
			acts = append(acts, func(int) { p.choose(n.SSID) })
		}
		rows = append(rows, row)
		if n.SSID == chosen && n.SSID != now && n.Security != wpa.Open {
			rows = append(rows, menu.Row{
				Label:  say.T("wifi.password"),
				Kind:   menu.Password,
				Action: say.T("wifi.join"),
				Focus:  focus,
				Save:   func(v string) { p.start(n, v) },
			})
			acts = append(acts, nil)
		}
	}
	return rows, acts
}

func (p *WifiPages) choose(ssid string) {
	p.mu.Lock()
	p.chosen, p.focus = ssid, true
	p.mu.Unlock()
	p.stack.Redraw()
}

func strength(n wpa.Network) string {
	level := 1
	switch {
	case n.Signal >= -55:
		level = 4
	case n.Signal >= -65:
		level = 3
	case n.Signal >= -75:
		level = 2
	}
	glyph := fmt.Sprintf("mdi:wifi-strength-%d", level)
	if n.Security != wpa.Open {
		glyph += "-lock"
	}
	return glyph
}

func (p *WifiPages) scan() {
	p.mu.Lock()
	if p.scanning {
		p.mu.Unlock()
		return
	}
	p.scanning, p.scanned, p.failed = true, false, ""
	p.found, p.chosen = nil, ""
	p.mu.Unlock()
	p.stack.Redraw()

	safe.Go("wifi scan", func() {
		ctx, stop := context.WithTimeout(context.Background(), wifiScanMost)
		defer stop()
		found, err := p.wifi.Scan(ctx)
		named := found[:0]
		for _, n := range found {
			if n.SSID != "" {
				named = append(named, n)
			}
		}
		p.mu.Lock()
		p.scanning, p.scanned = false, true
		if err != nil {
			slog.Warn("wifi scan failed", "err", err)
			p.failed = say.T("wifi.scan.failed")
		} else {
			p.found = named
		}
		p.mu.Unlock()
		p.stack.Redraw()
	})
}

func (p *WifiPages) start(n wpa.Network, password string) {
	if n.Security != wpa.Open && len(password) < wifiShortest {
		p.refuse(n.SSID, say.F("wifi.short", map[string]any{"N": wifiShortest}))
		return
	}
	if err := p.wifi.Start(n.SSID, password); err != nil {
		why := err.Error()
		if errors.Is(err, wifi.ErrMoving) {
			why = say.T("wifi.busy")
		}
		p.refuse(n.SSID, why)
		return
	}
	p.mu.Lock()
	p.mine, p.chosen = p.wifi.State().Until, ""
	p.mu.Unlock()
	p.tick()
}

func (p *WifiPages) refuse(ssid, why string) {
	p.mu.Lock()
	p.refused = &shell.Busy{
		Text:    say.F("wifi.failed", map[string]any{"Network": ssid}),
		Detail:  why,
		Outcome: shell.Failed,
		Close:   say.T("wifi.ok"),
		Dismiss: func() {
			p.mu.Lock()
			p.refused = nil
			p.mu.Unlock()
			p.stack.Redraw()
		},
	}
	p.mu.Unlock()
	p.stack.Redraw()
}

func (p *WifiPages) tick() {
	p.mu.Lock()
	if p.ticking {
		p.mu.Unlock()
		return
	}
	p.ticking = true
	p.mu.Unlock()
	safe.Go("wifi move countdown", func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for range t.C {
			p.stack.Redraw()
			if !p.wifi.State().Moving {
				break
			}
		}
		p.mu.Lock()
		p.ticking = false
		p.mu.Unlock()
		p.stack.Redraw()
	})
}

func (p *WifiPages) busy() (shell.Busy, bool) {
	s := p.wifi.State()
	p.mu.Lock()
	mine, seen, refused := p.mine, p.seen, p.refused
	p.mu.Unlock()
	switch {
	case refused != nil:
		return *refused, true
	case s.Moving:
		p.tick()
		left := max(0, int(time.Until(s.Until).Seconds()))
		return shell.Busy{
			Text:   say.F("wifi.joining", map[string]any{"Network": s.SSID}),
			Detail: say.F("wifi.left", map[string]any{"N": left}),
		}, true
	case s.Until.IsZero() || !s.Until.Equal(mine) || s.Until.Equal(seen):
		return shell.Busy{}, false
	case s.Failed != nil:
		return shell.Busy{
			Text:    say.F("wifi.failed", map[string]any{"Network": s.SSID}),
			Detail:  failure(s.Failed),
			Outcome: shell.Failed,
			Close:   say.T("wifi.ok"),
			Dismiss: p.dismiss(s.Until),
		}, true
	default:
		p.close(s.Until)
		return shell.Busy{
			Text:    say.F("wifi.joined", map[string]any{"Network": s.SSID}),
			Outcome: shell.Succeeded,
		}, true
	}
}

func (p *WifiPages) close(until time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closing.Equal(until) {
		return
	}
	p.closing = until
	time.AfterFunc(wifiJoinedFor, func() {
		p.mu.Lock()
		p.seen = until
		p.mu.Unlock()
		p.stack.Pop()
	})
}

func failure(err error) string {
	switch {
	case errors.Is(err, wpa.ErrRefused):
		return say.T("wifi.refused")
	case errors.Is(err, context.DeadlineExceeded):
		return say.F("wifi.timeout", map[string]any{"Minutes": int(wifi.MoveWindow.Minutes())})
	default:
		return say.T("wifi.unreachable")
	}
}

func (p *WifiPages) dismiss(until time.Time) func() {
	return func() {
		p.mu.Lock()
		p.seen = until
		p.mu.Unlock()
		p.stack.Redraw()
	}
}
