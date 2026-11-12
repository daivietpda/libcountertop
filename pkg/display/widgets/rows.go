package widgets

import (
	"fmt"
	"strings"

	gogui "github.com/go-gui-org/go-gui/gui"

	"github.com/ygelfand/libcountertop/pkg/display/icons"
	"github.com/ygelfand/libcountertop/pkg/display/interaction"
	"github.com/ygelfand/libcountertop/pkg/display/menu"
	"github.com/ygelfand/libcountertop/pkg/display/style"
)

type RowRenderer struct {
	Field   func(string, menu.Row) gogui.View
	Preview func(menu.Row) gogui.View
	Grip    func(gogui.View) gogui.View
}

func icon(glyph string, c gogui.Color) gogui.View {
	st := gogui.CurrentTheme().TextStyleIconMedium
	st.Color = c
	if name, ok := strings.CutPrefix(glyph, "mdi:"); ok {
		st.Family = icons.Family
		glyph = icons.Glyph(icons.Named(name))
	}
	return gogui.Label(glyph, st)
}

func (kit *Toolkit) Row(
	id, page string,
	r menu.Row,
	tap func(int),
	glyphs bool,
	render RowRenderer,
) gogui.View {
	label := []gogui.View{gogui.Text(gogui.TextCfg{Text: r.Label, Mode: gogui.TextModeWrap})}
	if r.Hint != "" {
		label = append(
			label,
			gogui.Text(
				gogui.TextCfg{Text: r.Hint, TextStyle: Secondary(), Mode: gogui.TextModeWrap},
			),
		)
	}
	head := gogui.Column(
		gogui.ContainerCfg{Sizing: gogui.FillFit, Padding: gogui.NoPadding, Content: label},
	)
	key := page + "/" + id
	stacked := r.Kind == menu.Segments && kit.stacked[key]
	if stacked {
		head = gogui.Column(gogui.ContainerCfg{
			Sizing:  gogui.FillFit,
			Padding: gogui.NoPadding,
			Spacing: gogui.SpacingSmall,
			Content: []gogui.View{head, kit.wrappedSegments(id, r, tap)},
		})
	}

	content := []gogui.View{head}
	if glyphs {
		var mark []gogui.View
		switch {
		case r.Picture != nil:
			mark = []gogui.View{r.Picture}
		case r.Glyph != "":
			mark = []gogui.View{icon(r.Glyph, gogui.CurrentTheme().Cfg.ColorTextSecondary)}
		}
		content = []gogui.View{gogui.Row(gogui.ContainerCfg{
			Width:   style.Reach(),
			Sizing:  gogui.FixedFit,
			Padding: gogui.NoPadding,
			HAlign:  gogui.HAlignCenter,
			Content: mark,
		}), head}
	}
	var onClick func(gogui.EventCtx)
	switch r.Kind {
	case menu.Toggle:
		content = append(content, kit.controls().Toggle(id+"-switch", r.On))
		onClick = interaction.Tapped(tap, 0)
	case menu.Slider:
		snap := r.Snap
		says := r.Value
		if says == "" {
			says = fmt.Sprintf("%d%%", r.Level)
		}
		content = append(content, gogui.Row(gogui.ContainerCfg{
			Padding: gogui.NewPadding(0, style.Reach()*0.4, 0, 0),
			Content: []gogui.View{gogui.Label(says, Secondary())},
		}))
		level := gogui.SliderCfg{
			ID:     id + "-slider",
			Value:  float32(r.Level),
			Min:    0,
			Max:    100,
			Sizing: gogui.FillFit,
			Height: style.Reach(),
			OnChange: func(v float32, e gogui.EventCtx) {
				level := int(v + 0.5)
				if snap != nil {
					level = snap(level)
				}
				if tap != nil {
					tap(level)
				}
				e.Window.InvalidateLayout()
			},
		}
		content = append(content, render.Grip(kit.Slider(level)))
	case menu.Field, menu.Password:
		content = append(content, render.Field(page+"/"+id, r))
	case menu.Segments:
		if !stacked {
			content = append(content, kit.segments(id, r, tap))
		}
	case menu.Chevron:
		if r.Value != "" {
			content = append(content, gogui.Label(r.Value, Secondary()))
		}
		content = append(
			content,
			icon(gogui.IconArrowRight, gogui.CurrentTheme().Cfg.ColorTextSecondary),
		)
		onClick = interaction.Tapped(tap, 0)
	default:
		if r.Value != "" {
			content = append(content, gogui.Label(r.Value, Secondary()))
		}
		if r.Preview != nil {
			content = append(content, render.Preview(r))
		}
		if r.Chosen {
			content = append(content, icon(gogui.IconCheck, gogui.CurrentTheme().Cfg.ColorAccent))
		}
		onClick = interaction.Tapped(tap, 0)
	}
	cfg := gogui.ContainerCfg{
		ID:       id,
		Sizing:   gogui.FillFit,
		VAlign:   gogui.VAlignMiddle,
		Padding:  gogui.PaddingMedium,
		Spacing:  gogui.SpacingMedium,
		Radius:   gogui.RadiusMedium,
		Disabled: r.Dim,
		Content:  content,
	}
	if r.Kind == menu.Segments {
		cfg.AmendLayout = kit.fitSegments(key, r.Label, stacked)
	}
	kit.ListRow(&cfg, Chosen(r.Kind == menu.Toggle && r.Chosen))
	return kit.Pressable(gogui.Row, cfg, onClick)
}

func (kit *Toolkit) Tile(
	id, label string,
	selected bool,
	content []gogui.View,
	tap func(int),
	edit func(),
) gogui.View {
	t := gogui.CurrentTheme().Cfg
	content = append(content, kit.tileLabel(id, label, edit))
	cfg := gogui.ContainerCfg{
		ID:         id,
		HAlign:     gogui.HAlignCenter,
		Padding:    gogui.PaddingSmall,
		Spacing:    gogui.SpacingSmall,
		Radius:     gogui.RadiusMedium,
		SizeBorder: gogui.BorderPx(3),
		Clip:       true,
		Content:    content,
	}
	if selected {
		cfg.ColorBorder = t.ColorAccent
	}
	rest := cfg
	kit.controls().Tile(&rest, Chosen(false))
	kit.controls().Tile(&cfg, Chosen(selected))
	if grow := cfg.SizeBorder.Or(0) - rest.SizeBorder.Or(0); grow > 0 {
		p := cfg.Padding
		cfg.Padding = gogui.NewPadding(p.Top-grow, p.Right-grow, p.Bottom-grow, p.Left-grow)
	}
	return kit.Pressable(gogui.Column, cfg, interaction.Tapped(tap, 0))
}

func (kit *Toolkit) tileLabel(id, label string, edit func()) gogui.View {
	if edit == nil {
		return gogui.Label(label, gogui.TextStyle{})
	}
	return gogui.Row(gogui.ContainerCfg{
		Sizing:  gogui.FillFit,
		Padding: gogui.NoPadding,
		VAlign:  gogui.VAlignMiddle,
		Content: []gogui.View{
			gogui.Label(label, gogui.TextStyle{}),
			gogui.Row(gogui.ContainerCfg{Sizing: gogui.FillFit, Padding: gogui.NoPadding}),
			kit.KeyButton(
				id+"-edit",
				gogui.IconEdit,
				gogui.CurrentTheme().Cfg.TextStyleDef,
				Chosen(false),
				func(gogui.EventCtx) { edit() },
			),
		},
	})
}

func (kit *Toolkit) segments(id string, r menu.Row, tap func(int)) gogui.View {
	return gogui.Row(gogui.ContainerCfg{
		Padding: gogui.NoPadding,
		Spacing: gogui.SpacingSmall,
		Content: kit.chips(id, r, tap),
	})
}

func (kit *Toolkit) wrappedSegments(id string, r menu.Row, tap func(int)) gogui.View {
	return gogui.Row(gogui.ContainerCfg{
		Sizing:  gogui.FillFit,
		Padding: gogui.NoPadding,
		Spacing: gogui.SpacingSmall,
		Wrap:    true,
		Content: kit.chips(id, r, tap),
	})
}

func (kit *Toolkit) chips(id string, r menu.Row, tap func(int)) []gogui.View {
	var chips []gogui.View
	for i, option := range r.Options {
		st := gogui.CurrentTheme().Cfg.TextStyleDef
		cfg := gogui.ContainerCfg{
			ID:      fmt.Sprintf("%s-segment-%d", id, i),
			Padding: gogui.PaddingMedium,
			VAlign:  gogui.VAlignMiddle,
		}
		kit.Tab(&cfg, &st, Chosen(i == r.Level))
		cfg.Content = []gogui.View{gogui.Label(option, st)}
		if i < len(r.Marks) {
			if mark := markView(
				fmt.Sprintf("%s-mark-%d", id, i),
				r.Marks[i],
				st.Size*1.1,
			); mark != nil {
				cfg.Spacing = gogui.SpacingSmall
				cfg.Content = append([]gogui.View{mark}, cfg.Content...)
			}
		}
		chips = append(chips, kit.Pressable(gogui.Row, cfg, func(e gogui.EventCtx) {
			if tap != nil && i != r.Level {
				tap(i)
			}
			e.Consume()
			e.Window.InvalidateLayout()
		}))
	}
	return chips
}

func (kit *Toolkit) fitSegments(key, label string, stacked bool) func(gogui.EventCtx) {
	return func(e gogui.EventCtx) {
		kids := e.Layout.Children
		if len(kids) == 0 {
			return
		}
		t := gogui.CurrentTheme().Cfg
		var head, chips *gogui.Layout
		if stacked {
			head = &kids[len(kids)-1]
			if n := len(head.Children); n > 0 {
				chips = &head.Children[n-1]
			}
		} else if len(kids) >= 2 {
			head, chips = &kids[len(kids)-2], &kids[len(kids)-1]
		}
		if head == nil || chips == nil {
			return
		}
		room := head.Shape.Width
		if !stacked {
			room += t.SpacingMedium + chips.Shape.Width
		}
		want := e.Window.TextWidth(
			label,
			t.TextStyleDef,
		) + t.SpacingMedium + oneLine(
			chips,
			t.SpacingSmall,
		)
		stack := want > room+0.5
		if stack == stacked {
			return
		}
		if kit.stacked == nil {
			kit.stacked = map[string]bool{}
		}
		kit.stacked[key] = stack
		e.Window.InvalidateLayout()
	}
}

func oneLine(chips *gogui.Layout, gap float32) float32 {
	var width float32
	n := 0
	var walk func(l *gogui.Layout)
	walk = func(l *gogui.Layout) {
		for i := range l.Children {
			c := &l.Children[i]
			if strings.Contains(c.Shape.ID, "-segment-") {
				width += c.Shape.Width
				n++
				continue
			}
			walk(c)
		}
	}
	walk(chips)
	if n > 1 {
		width += float32(n-1) * gap
	}
	return width
}

func markView(id string, m menu.Mark, side float32) gogui.View {
	switch {
	case m.SVG != "":
		return gogui.Svg(gogui.SvgCfg{ID: id, SvgData: m.SVG, Width: side, Height: side})
	case m.Image != nil:
		b := m.Image.Bounds()
		width := side * float32(b.Dx()) / float32(b.Dy())
		src := ImageSource(fmt.Sprintf("mark/%p", m.Image), m.Image)
		return gogui.Image(gogui.ImageCfg{ID: id, Src: src, Width: width, Height: side})
	}
	return nil
}
