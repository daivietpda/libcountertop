package screens

import (
	gogui "github.com/go-gui-org/go-gui/gui"

	"github.com/ygelfand/libcountertop/pkg/display/dashboard"
	"github.com/ygelfand/libcountertop/pkg/display/theme"
	"github.com/ygelfand/libcountertop/pkg/display/ui"
	"github.com/ygelfand/libcountertop/pkg/display/widgets"
	"github.com/ygelfand/libcountertop/pkg/homeassistant/homecontrol"
)

func homeGlyph(t dashboard.Tab) string {
	return glyphFor(homecontrol.Selection{Key: t.Key})
}

func (a *App) homeBoard(_ *gogui.Window, at ui.Rect, t dashboard.Tab, pal theme.Theme) gogui.View {
	s, ok := a.o.Home.Dash().Find(t.Key)
	if !ok {
		return nil
	}
	return a.tileBoard(at, s, pal)
}

const (
	tabText  = 1.6
	tabLeast = 0.6
)

type tabFit struct {
	scale  float32
	labels bool
}

func (a *App) tabGlyph(t dashboard.Tab) string {
	if kind, ok := a.boards[t.Kind]; ok && kind.Glyph != nil {
		return kind.Glyph(t)
	}
	return ""
}

func (a *App) tabWidth(w *gogui.Window, t dashboard.Tab, label bool, scale float32) float32 {
	cfg := gogui.CurrentTheme().Cfg
	st := cfg.TextStyleDef
	st.Size *= tabText * scale
	width := reach() * 1.4 * scale
	glyph := a.tabGlyph(t)
	if glyph != "" {
		mark := widgets.IconStyle(st.Color)
		mark.Size = st.Size
		width += w.TextWidth(glyph, mark)
	}
	if label || glyph == "" {
		if glyph != "" {
			width += cfg.SpacingMedium
		}
		width += w.TextWidth(t.Name, st)
	}
	return width
}

func (a *App) fitTabs(w *gogui.Window, at ui.Rect, tabs []dashboard.Tab, open string) tabFit {
	room := at.W
	gaps := gogui.CurrentTheme().Cfg.SpacingMedium * float32(max(len(tabs)-1, 0))
	full := gogui.CurrentTheme().Cfg.TextStyleDef.Size * tabText
	scale := func(labels bool) float32 {
		size := widgets.Shrink(full, float32(room)-gaps, func(size float32) float32 {
			var total float32
			for _, t := range tabs {
				total += max(a.tabWidth(w, t, labels || t.Key == open, size/full), float32(at.H))
			}
			return total
		})
		return size / full
	}
	if s := scale(true); s >= tabLeast {
		return tabFit{scale: s, labels: true}
	}
	return tabFit{scale: max(scale(false), tabLeast)}
}

func (a *App) tabStrip(
	w *gogui.Window,
	at ui.Rect,
	tabs []dashboard.Tab,
	open string,
) gogui.View {
	fit := a.fitTabs(w, at, tabs, open)
	var pills []gogui.View
	for _, t := range tabs {
		st := gogui.CurrentTheme().Cfg.TextStyleDef
		st.Size *= tabText * fit.scale
		cfg := gogui.ContainerCfg{
			ID:       "tab-" + t.Key,
			Height:   float32(at.H),
			MinWidth: float32(at.H),
			Sizing:   gogui.FitFixed,
			HAlign:   gogui.HAlignCenter,
			Padding: gogui.NewPadding(
				0,
				reach()*0.8*fit.scale,
				0,
				reach()*0.6*fit.scale,
			),
			Spacing: gogui.SpacingMedium,
			VAlign:  gogui.VAlignMiddle,
		}
		a.ui.Toolkit.Tab(&cfg, &st, widgets.Chosen(t.Key == open))
		var content []gogui.View
		glyph := a.tabGlyph(t)
		if glyph != "" {
			mark := widgets.IconStyle(st.Color)
			mark.Size = st.Size
			content = append(content, gogui.Label(glyph, mark))
		}
		if fit.labels || t.Key == open || glyph == "" {
			content = append(content, gogui.Label(t.Name, st))
		}
		cfg.Content = content
		pills = append(
			pills,
			a.ui.Presses.Pressable(
				gogui.Row,
				cfg,
				func(gogui.EventCtx) { a.o.Tabs.Show(t.Key) },
			),
		)
	}
	return widgets.Placed(at, gogui.Row(gogui.ContainerCfg{
		Sizing:  gogui.FillFill,
		Padding: gogui.NoPadding,
		Spacing: gogui.SpacingMedium,
		VAlign:  gogui.VAlignMiddle,
		Content: pills,
	}))
}
