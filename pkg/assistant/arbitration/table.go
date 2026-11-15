package arbitration

import (
	"strconv"
	"time"

	"github.com/ygelfand/libcountertop/pkg/say"
	setting "github.com/ygelfand/libcountertop/pkg/settings"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

const Group setting.Group = "Arbitration"

const (
	domain     = "assistants"
	waitStep   = 50
	offsetStep = 2
)

type Setting struct {
	Read func() schema.Wake
	Save func(schema.Wake) error
}

func (s Setting) Wait() time.Duration {
	return time.Duration(s.Read().Arbitration) * time.Millisecond
}

func (s Setting) Offset() float64 { return float64(s.Read().ArbitrationOffset) }

func (s Setting) Controls() *setting.Controls[schema.Wake] {
	return &setting.Controls[schema.Wake]{
		Table: Table(),
		Read:  s.Read,
		Save: func(row setting.Setting[schema.Wake], v string) error {
			w := s.Read()
			if err := row.Write(&w, v); err != nil {
				return err
			}
			return s.Save(w)
		},
	}
}

func Waits() []int { return steps(0, schema.MaxArbitration, waitStep) }

func Offsets() []int {
	return steps(-schema.MaxArbitrationOffset, schema.MaxArbitrationOffset, offsetStep)
}

func steps(from, to, by int) []int {
	out := make([]int, 0, (to-from)/by+1)
	for v := from; v <= to; v += by {
		out = append(out, v)
	}
	return out
}

func Table() *setting.Table[schema.Wake] {
	wait := setting.Setting[schema.Wake]{
		Name:   "arbitration_wait",
		Group:  Group,
		Kind:   setting.Number,
		Slider: true,
		Min:    0,
		Max:    schema.MaxArbitration,
		Unit:   "ms",
	}
	wait.Read = func(w *schema.Wake) string { return strconv.Itoa(w.Arbitration) }
	wait.Write = func(w *schema.Wake, v string) error {
		n, err := wait.Number(v)
		if err != nil {
			return err
		}
		w.Arbitration = n
		return nil
	}

	offset := setting.Setting[schema.Wake]{
		Name:   "arbitration_offset",
		Group:  Group,
		Kind:   setting.Number,
		Slider: true,
		Min:    -schema.MaxArbitrationOffset,
		Max:    schema.MaxArbitrationOffset,
		Unit:   "dB",
	}
	offset.Read = func(w *schema.Wake) string { return strconv.Itoa(w.ArbitrationOffset) }
	offset.Write = func(w *schema.Wake, v string) error {
		n, err := offset.Number(v)
		if err != nil {
			return err
		}
		w.ArbitrationOffset = n
		return nil
	}

	return setting.NewTable(
		domain,
		[]setting.Group{Group},
		[]setting.Setting[schema.Wake]{wait, offset},
		setting.Messages{Text: say.T, Missing: say.Missing},
	)
}
