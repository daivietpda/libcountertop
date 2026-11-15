package schema

import "github.com/ygelfand/libcountertop/pkg/say"

type Lovelace struct {
	User           string            `json:"user"`
	Dashboard      LovelaceDashboard `json:"dashboard"`
	View           LovelaceView      `json:"view"`
	Scale          int               `json:"scale"`
	SaverDashboard LovelaceDashboard `json:"saver_dashboard"`
	SaverView      LovelaceView      `json:"saver_view"`
	SaverScale     int               `json:"saver_scale"`
	Theme          LovelaceTheme     `json:"theme,omitempty"`
}

const (
	LovelaceScaleMin  = 50
	LovelaceScaleMax  = 100
	LovelaceScaleStep = 5
)

func DefaultLovelace() Lovelace {
	return Lovelace{Scale: LovelaceScaleMax, SaverScale: LovelaceScaleMax}
}

type LovelaceTheme string

func (t LovelaceTheme) Label() string {
	if t == "" {
		return say.T("lovelace.theme.default")
	}
	return string(t)
}

type LovelaceDashboard string

func (d LovelaceDashboard) Label() string {
	if d == "" {
		return say.T("lovelace.default")
	}
	return string(d)
}

type LovelaceView string

func (v LovelaceView) Label() string {
	if v == "" {
		return say.T("lovelace.first")
	}
	return string(v)
}

type LovelaceUser string

func (u LovelaceUser) Label() string { return string(u) }
