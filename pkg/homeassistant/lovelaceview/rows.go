package lovelaceview

import (
	"log/slog"
	"strconv"

	gogui "github.com/go-gui-org/go-gui/gui"

	"github.com/ygelfand/libcountertop/pkg/display/menu"
	"github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/say"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

type pick struct {
	name   string
	glyph  string
	slider bool
}

var scaleSteps = menu.Steps(
	(schema.LovelaceScaleMax-schema.LovelaceScaleMin)/schema.LovelaceScaleStep + 1,
)

func (f *Feature) Rows(saver bool) ([]menu.Row, []func(int)) {
	picks := []pick{
		{"user", gogui.IconUser, false},
		{"dashboard", gogui.IconLayout, false},
		{"view", gogui.IconPictureSquare, false},
		{"scale", gogui.IconScale, true},
	}
	if saver {
		picks = append(picks,
			pick{"saver_dashboard", gogui.IconMoon, false},
			pick{"saver_view", gogui.IconPicture, false},
			pick{"saver_scale", gogui.IconScale, true},
		)
	}
	picks = append(picks, pick{"theme", gogui.IconPalette, false})
	rows := make([]menu.Row, 0, len(picks))
	taps := make([]func(int), 0, len(picks))
	for _, p := range picks {
		value, _ := f.knobs.Value(p.name)
		if p.slider {
			row, tap := f.scaleSlider(p, value)
			rows, taps = append(rows, row), append(taps, tap)
			continue
		}
		rows = append(rows, menu.Row{
			Glyph: p.glyph,
			Label: say.T("lovelace.setting." + p.name),
			Kind:  menu.Chevron,
			Value: value,
			Open:  func() shell.View { return f.page(p.name) },
		})
		taps = append(taps, nil)
	}
	return rows, taps
}

func (f *Feature) scaleSlider(p pick, value string) (menu.Row, func(int)) {
	percent, err := strconv.Atoi(value)
	if err != nil {
		percent = schema.LovelaceScaleMax
	}
	row := menu.Row{
		Glyph: p.glyph,
		Label: say.T("lovelace.setting." + p.name),
		Kind:  menu.Slider,
		Level: scaleSteps.Level((percent - schema.LovelaceScaleMin) / schema.LovelaceScaleStep),
		Snap:  scaleSteps.Snap,
		Value: say.F("lovelace.scale.value", map[string]any{"N": percent}),
	}
	tap := func(level int) {
		chosen := schema.LovelaceScaleMin + scaleSteps.Index(level)*schema.LovelaceScaleStep
		if err := f.knobs.Change(p.name, strconv.Itoa(chosen)); err != nil {
			slog.Warn("the dashboard setting could not be saved", "setting", p.name, "err", err)
		}
	}
	return row, tap
}

func (f *Feature) page(name string) *shell.Page {
	return &shell.Page{
		Title: say.T("lovelace.setting." + name),
		Build: func() ([]menu.Row, []func(int)) {
			s, ok := f.knobs.Table.Find(name)
			if !ok {
				return nil, nil
			}
			now := s.Read(new(f.o.Settings.Read()))
			choices := s.Choices()
			rows := make([]menu.Row, 0, len(choices))
			taps := make([]func(int), 0, len(choices))
			for _, o := range choices {
				rows = append(rows, menu.Row{Label: s.Label(o), Chosen: o.Value == now})
				taps = append(taps, func(int) {
					if err := f.knobs.Change(name, o.Value); err != nil {
						slog.Warn(
							"the dashboard setting could not be saved",
							"setting",
							name,
							"err",
							err,
						)
					}
				})
			}
			return rows, taps
		},
	}
}
