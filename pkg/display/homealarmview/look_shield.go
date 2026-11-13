package homealarmview

import gogui "github.com/go-gui-org/go-gui/gui"

func shieldLook(f *face) (gogui.View, skin) {
	sk := f.wear(skin{
		ink: hex(0xffffff), panel: hex(0xffffff),
		calm: hex(0x1f5c45), armed: hex(0x8c2a2a), warn: hex(0x8a5a12), alarm: hex(0xb3261e),
		chosen: hex(0xffffff), radius: f.unit() * 0.04,
	})
	sk.back = f.tone(sk)
	sk.muted = mix(sk.back, hex(0xffffff), 0.75)
	sk.chosenInk = sk.back
	sk.key = mix(sk.back, hex(0xffffff), 0.18)
	u := f.unit()
	m := u * 0.06
	tall := f.tall()
	cardsW := f.vw * 0.38
	leftW := f.vw - cardsW - 3*m
	leftH := f.vh - 2*m - u*0.1
	if tall {
		cardsW = f.vw - 2*m
		leftW = cardsW
		leftH = f.vh * 0.42
	}

	title := f.title()
	tsize := f.fit(sk, title, leftW, u*0.2, true)
	left := []gogui.View{
		icon(f.glyph(), u*0.22, sk.ink),
		sk.label(title, tsize, sk.ink, true),
	}
	if d := f.detail(); d != "" {
		left = append(left, gogui.Row(gogui.ContainerCfg{
			Color: hex(
				0xffc94d,
			),
			Radius:  gogui.RadiusPx(u * 0.05),
			Padding: gogui.PadVH(u*0.015, u*0.03),
			Content: []gogui.View{sk.label(d, u*0.04, hex(0x3b2a00), true)},
		}))
	}
	views := []gogui.View{
		place(m, m, f.vw*0.5, u*0.08,
			sk.label(f.clock()+" "+f.meridiem()+"   "+f.date(), u*0.04, sk.muted, true)),
		place(m, m+u*0.1, leftW, leftH, gogui.Column(gogui.ContainerCfg{
			Sizing: gogui.FillFill, Padding: gogui.NoPadding, Spacing: gogui.SpacingPx(u * 0.03),
			VAlign: gogui.VAlignMiddle, Content: left,
		})),
	}
	acts := f.actions()
	emH := u * 0.13
	cardsY := m + u*0.1
	avail := f.vh - 2*m - u*0.1
	if tall {
		cardsY += leftH + u*0.03
		avail = f.vh - m - cardsY
	}
	if f.emergency() {
		avail -= emH + u*0.03
	}
	gap := u * 0.03
	n := max(len(acts), 1)
	ch := min((avail-float32(n-1)*gap)/float32(n), u*0.24)
	cards := make([]gogui.View, 0, len(acts))
	for _, a := range acts {
		cards = append(cards, f.key(key{
			id: a.id, w: cardsW, h: ch, fill: hex(0xffffff), ink: sk.back, radius: sk.radius,
			align: gogui.HAlignLeft,
			content: []gogui.View{
				icon(a.glyph, ch*0.34, sk.back),
				sk.label(a.label, f.fit(sk, a.label, cardsW*0.6, ch*0.3, true), sk.back, true),
			},
			run: a.run,
		}))
	}
	views = append(
		views,
		place(f.vw-m-cardsW, cardsY, cardsW, avail, gogui.Column(gogui.ContainerCfg{
			Sizing: gogui.FillFill, Padding: gogui.NoPadding, Spacing: gogui.SpacingPx(gap),
			VAlign: gogui.VAlignMiddle, Content: cards,
		})),
	)
	if f.emergency() {
		views = append(views, place(f.vw-m-cardsW, f.vh-m-emH, cardsW, emH,
			f.emergencyKey(sk, cardsW, emH, hex(0xe5484d), hex(0xffffff))))
	}
	return gogui.Stack(gogui.ContainerCfg{
		Width:   f.vw,
		Height:  f.vh,
		Sizing:  gogui.FixedFixed,
		Padding: gogui.NoPadding,
		Content: views,
	}), sk
}
