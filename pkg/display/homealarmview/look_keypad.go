package homealarmview

import (
	"slices"

	gogui "github.com/go-gui-org/go-gui/gui"

	"github.com/ygelfand/libcountertop/pkg/display/widgets"
	"github.com/ygelfand/libcountertop/pkg/homeassistant/homealarm"
	"github.com/ygelfand/libcountertop/pkg/say"
)

func keypadLook(f *face) (gogui.View, skin) {
	sk := f.wear(skin{
		back: hex(0x23262d), panel: hex(0x2c3038), key: hex(0x3a3f49), ink: hex(0xf2f3f5),
		muted: hex(0x9aa0aa), calm: hex(0x4ade80), armed: hex(0xef4444), warn: hex(0xfacc15),
		alarm: hex(0xef4444), chosen: hex(0x4a5a78), chosenInk: hex(0xffffff),
		radius: f.unit() * 0.018,
	})
	u := f.unit()
	m := u * 0.045
	top := u * 0.1
	views := []gogui.View{keypadBar(f, sk, m, top)}
	if f.tall() {
		views = append(views, keypadTall(f, sk, m, top)...)
	} else {
		views = append(views, keypadWide(f, sk, m, top)...)
	}
	return gogui.Stack(gogui.ContainerCfg{
		Width:   f.vw,
		Height:  f.vh,
		Sizing:  gogui.FixedFixed,
		Padding: gogui.NoPadding,
		Content: views,
	}), sk
}

func keypadWide(f *face, sk skin, m, top float32) []gogui.View {
	u := f.unit()
	rightW := f.vw * 0.36
	rx := f.vw - m - rightW
	leftW := rx - 2*m
	body := f.vh - top - m
	head, headH := keypadHead(f, sk, leftW)
	foot := u * 0.12
	views := []gogui.View{place(m, top, leftW, headH, head)}
	gridH := body - headH - foot - u*0.03
	if mid := keypadMiddle(f, sk, leftW, gridH); mid != nil {
		views = append(views, place(m, top+headH, leftW, gridH, mid))
	}
	return append(views,
		place(m, f.vh-m-foot, leftW, foot, keypadToggles(f, sk, leftW, foot)),
		place(rx, top, rightW, body, keypadPanel(f, sk, rightW, body)),
	)
}

func keypadTall(f *face, sk skin, m, top float32) []gogui.View {
	u := f.unit()
	gap := u * 0.02
	w := f.vw - 2*m
	head, headH := keypadHead(f, sk, w)
	foot := u * 0.1
	panelH := keypadPanelHeight(f, u*0.3, m)
	panelY := f.vh - m - foot - gap - panelH
	views := []gogui.View{place(m, top, w, headH, head)}
	if gridH := panelY - gap - top - headH; gridH > 0 {
		if mid := keypadMiddle(f, sk, w, gridH); mid != nil {
			views = append(views, place(m, top+headH, w, gridH, mid))
		}
	}
	return append(views,
		place(m, panelY, w, panelH, keypadPanel(f, sk, w, panelH)),
		place(m, f.vh-m-foot, w, foot, keypadToggles(f, sk, w, foot)),
	)
}

func keypadBar(f *face, sk skin, m, top float32) gogui.View {
	u := f.unit()
	chip := func(label string, lit bool, c gogui.Color) gogui.View {
		dot := sk.muted.WithOpacity(0.4)
		if lit {
			dot = c
		}
		return gogui.Row(gogui.ContainerCfg{
			Padding: gogui.NoPadding,
			Spacing: gogui.SpacingPx(u * 0.012),
			VAlign:  gogui.VAlignMiddle,
			Content: []gogui.View{
				gogui.Column(gogui.ContainerCfg{
					Width: u * 0.028, Height: u * 0.028, Sizing: gogui.FixedFixed,
					Radius: gogui.RadiusPx(u * 0.014), Color: dot, Padding: gogui.NoPadding,
				}),
				sk.label(label, u*0.032, sk.ink, false),
			},
		})
	}
	s := f.state.Status
	return place(m, 0, f.vw-2*m-f.cornerWidth(), top, gogui.Row(gogui.ContainerCfg{
		Sizing: gogui.FillFill, Padding: gogui.NoPadding, Spacing: gogui.SpacingPx(u * 0.04),
		VAlign: gogui.VAlignMiddle,
		Content: []gogui.View{
			chip(say.T("homealarm.chip.ready"), s == homealarm.Disarmed, sk.calm),
			chip(say.T("homealarm.chip.armed"), s.Armed(), sk.armed),
			chip(
				say.T("homealarm.chip.delay"),
				s == homealarm.Arming || s == homealarm.Pending,
				sk.warn,
			),
			chip(say.T("homealarm.chip.alarm"), s == homealarm.Triggered, sk.alarm),
			gogui.Row(gogui.ContainerCfg{Sizing: gogui.FillFit, Padding: gogui.NoPadding}),
			sk.label(f.clock()+" "+f.meridiem(), u*0.045, sk.ink, true),
		},
	}))
}

func keypadHead(f *face, sk skin, w float32) (gogui.View, float32) {
	u := f.unit()
	title := f.title()
	most := u * 0.13
	if f.tall() {
		most = u * 0.09
	}
	tsize := f.fit(sk, title, w-most*0.9-u*0.02, most, true)
	lines := []gogui.View{
		row(w, tsize*1.25, u*0.02, gogui.HAlignLeft,
			icon(f.glyph(), tsize*0.9, f.tone(sk)),
			sk.label(title, tsize, sk.ink, true)),
	}
	if d := f.detail(); d != "" {
		lines = append(lines, sk.label(d, u*0.04, sk.warn, true))
	}
	return gogui.Column(gogui.ContainerCfg{
		Sizing: gogui.FillFill, Padding: gogui.NoPadding, Spacing: gogui.SpacingPx(u * 0.01),
		Content: lines,
	}), tsize*1.25 + u*0.07
}

func keypadMiddle(f *face, sk skin, w, h float32) gogui.View {
	u := f.unit()
	if zones := f.zones(); len(zones) > 0 {
		return keypadZones(f, sk, zones, w, h)
	}
	lines := f.entries(6)
	if len(lines) == 0 {
		return nil
	}
	log := []gogui.View{
		widgets.Divider(),
		sk.label(say.T("homealarm.recent"), u*0.03, sk.muted, true),
	}
	for _, l := range lines {
		log = append(log, sk.label(l, u*0.034, sk.ink, false))
	}
	return gogui.Column(gogui.ContainerCfg{
		Sizing: gogui.FillFill, Padding: gogui.NoPadding, Spacing: gogui.SpacingPx(u * 0.02),
		Content: log,
	})
}

func keypadToggles(f *face, sk skin, w, h float32) gogui.View {
	u := f.unit()
	var keys []gogui.View
	for _, t := range f.toggles() {
		tw := min(w*0.25, u*0.3)
		fill := sk.key
		if t.current {
			fill = sk.chosen
		}
		keys = append(keys, f.key(key{
			id:     t.id,
			w:      tw,
			h:      h,
			fill:   fill,
			ink:    sk.ink,
			radius: sk.radius,
			align:  gogui.HAlignCenter,
			content: []gogui.View{
				sk.label(t.label, f.fit(sk, t.label, tw*0.85, h*0.3, true), sk.ink, true),
			},
			run: t.run,
		}))
	}
	if f.emergency() {
		keys = append(keys, f.emergencyKey(sk, w*0.55, h, hex(0xe5484d), hex(0xffffff)))
	}
	return row(w, h, u*0.02, gogui.HAlignLeft, keys...)
}

func keypadPanelHeight(f *face, padH, m float32) float32 {
	u := f.unit()
	return padH + 2*m*0.6 + u*0.11 + u*0.1 + 2*u*0.025
}

func keypadPanel(f *face, sk skin, w, h float32) gogui.View {
	u := f.unit()
	m := u * 0.045
	inner := w - 2*m*0.6
	display := u * 0.11
	acts := f.actions()
	actH := u * 0.1
	padH := h - 2*m*0.6 - display - actH - 2*u*0.025
	actW := inner
	if len(acts) > 0 {
		actW = (inner - float32(len(acts)-1)*u*0.02) / float32(len(acts))
	}
	keys := make([]gogui.View, 0, len(acts))
	for _, a := range acts {
		keys = append(keys, f.key(key{
			id: a.id, w: actW, h: actH, fill: sk.chosen, ink: sk.ink, radius: sk.radius,
			align: gogui.HAlignCenter,
			content: []gogui.View{sk.label(a.label, f.fit(sk, a.label, actW*0.85, actH*0.34, true),
				sk.ink, true)},
			run: a.run,
		}))
	}
	return gogui.Column(gogui.ContainerCfg{
		Sizing: gogui.FillFill, Color: sk.panel, Radius: gogui.RadiusPx(sk.radius * 1.5),
		Padding: gogui.PadAll(m * 0.6), Spacing: gogui.SpacingPx(u * 0.025),
		HAlign: gogui.HAlignCenter,
		Content: []gogui.View{
			gogui.Row(gogui.ContainerCfg{
				Width: inner, Height: display, Sizing: gogui.FixedFixed, Color: hex(0x15171b),
				Radius: gogui.RadiusPx(sk.radius), HAlign: gogui.HAlignCenter,
				VAlign: gogui.VAlignMiddle, Padding: gogui.NoPadding,
				Content: []gogui.View{sk.label(f.dots(sk), display*0.36, sk.muted, true)},
			}),
			row(inner, actH, u*0.02, gogui.HAlignCenter, keys...),
			f.padGrid(sk, inner, padH),
		},
	})
}

func keypadZones(f *face, sk skin, zones []zoneLine, w, h float32) gogui.View {
	u := f.unit()
	gap := u * 0.015
	cols := 4
	if len(zones) <= 6 {
		cols = 3
	}
	slices.SortStableFunc(zones, func(a, b zoneLine) int {
		switch {
		case a.alert == b.alert:
			return 0
		case a.alert:
			return -1
		}
		return 1
	})
	least := u * 0.12
	var pager gogui.View
	if fit := int((h+gap)/(least+gap)) * cols; len(zones) > fit {
		pagerH := u * 0.075
		per := max(int((h-pagerH-gap+gap)/(least+gap)), 1) * cols
		pages := (len(zones) + per - 1) / per
		page := int(f.v.zonePage.Load()) % pages
		from, to := page*per, min(page*per+per, len(zones))
		pager = row(
			w,
			pagerH,
			0,
			gogui.HAlignRight,
			f.zonePager(sk, pagerH, from, to, len(zones), pages),
		)
		zones = zones[from:to]
		h -= pagerH + 2*gap
	}
	rows := (len(zones) + cols - 1) / cols
	tw := (w - float32(cols-1)*gap) / float32(cols)
	th := min((h-float32(rows-1)*gap)/float32(rows), u*0.16)
	nameW := tw * 0.85
	nameH := th * 0.19
	for _, z := range zones {
		nameH = min(nameH, f.fit(sk, z.name, nameW, th*0.19, false))
	}
	nameH = max(nameH, th*0.13)
	tiles := make([]gogui.View, 0, len(zones))
	for _, z := range zones {
		fill, ink, sub := sk.panel, sk.ink, sk.muted
		switch {
		case z.alert:
			fill, ink, sub = hex(0xf5c518), hex(0x1a1a1a), hex(0x3a3000)
		case z.muted:
			fill, ink = sk.back, sk.muted
		}
		tiles = append(tiles, gogui.Column(gogui.ContainerCfg{
			Width: tw, Height: th, Sizing: gogui.FixedFixed, Color: fill,
			Radius: gogui.RadiusPx(sk.radius), Padding: gogui.PadVH(th*0.07, th*0.1), Clip: true,
			Content: []gogui.View{
				gogui.Row(gogui.ContainerCfg{
					Sizing: gogui.FillFit, Padding: gogui.NoPadding,
					Content: []gogui.View{
						sk.label(z.number, th*0.26, ink, true),
						gogui.Row(
							gogui.ContainerCfg{Sizing: gogui.FillFit, Padding: gogui.NoPadding},
						),
						sk.label(z.state, th*0.17, sub, true),
					},
				}),
				gogui.Row(gogui.ContainerCfg{Sizing: gogui.FillFill, Padding: gogui.NoPadding}),
				sk.label(f.short(sk, z.name, nameW, nameH, false), nameH, ink, false),
			},
		}))
	}
	var lines []gogui.View
	for i := 0; i < len(tiles); i += cols {
		lines = append(
			lines,
			row(w, th, gap, gogui.HAlignLeft, tiles[i:min(i+cols, len(tiles))]...),
		)
	}
	if pager != nil {
		lines = append(
			lines,
			gogui.Column(gogui.ContainerCfg{Sizing: gogui.FillFill, Padding: gogui.NoPadding}),
			pager,
		)
	}
	return gogui.Column(gogui.ContainerCfg{
		Sizing: gogui.FillFill, Padding: gogui.NoPadding, Spacing: gogui.SpacingPx(gap), Clip: true,
		Content: lines,
	})
}
