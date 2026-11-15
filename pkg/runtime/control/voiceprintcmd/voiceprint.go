package voiceprintcmd

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ygelfand/libcountertop/pkg/assistant/voicematch"
	"github.com/ygelfand/libcountertop/pkg/audio/capture"
	"github.com/ygelfand/libcountertop/pkg/inference/voiceprint"
)

const limit = 2 * time.Minute

type Source interface {
	Recent(ctx context.Context) ([]float32, time.Duration, error)
	Enroll(
		ctx context.Context,
		id, name, person string,
		say func(voicematch.Step),
	) (voicematch.Profile, voicematch.Match, error)
	Check(
		ctx context.Context,
		n int,
		say func(voicematch.Step),
		scored func(voicematch.Match),
	) error
	Profiles() []voicematch.Profile
	SetProfiles([]voicematch.Profile) error
	Last() []int16
}

func prompts(c *cobra.Command) func(voicematch.Step) {
	return func(st voicematch.Step) {
		switch st.Stage {
		case voicematch.Prompt:
			label := fmt.Sprintf("say the wake word, then %q", st.Phrase)
			if st.Confirm {
				label = "confirm: " + label
			}
			c.Printf("%s %d/%d\n", label, st.Number, st.Of)
		case voicematch.Heard:
			c.Printf("  got it (%.1f s)\n", st.Took.Seconds())
		case voicematch.Missed:
			c.Printf("  didn't hear you\n")
		}
	}
}

func Command(src func() Source) *cobra.Command {
	root := &cobra.Command{
		Use:   "voiceprint",
		Short: "Embed the last 3 seconds of what wake detection hears and print the voiceprint",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(c.Context(), limit)
			defer cancel()
			print, took, err := src().Recent(ctx)
			if err != nil {
				return err
			}
			var sq float64
			for _, v := range print {
				sq += float64(v) * float64(v)
			}
			c.Printf("%d values in %v, norm %.3f, first %v\n",
				len(print), took.Round(time.Millisecond), math.Sqrt(sq), print[:min(4, len(print))])
			return nil
		},
	}
	root.AddCommand(&cobra.Command{
		Use:   "enroll NAME",
		Short: "Record NAME saying the wake word five times, confirm with one more, and keep the profile on this device",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(c.Context(), limit)
			defer cancel()
			name := args[0]
			id := strings.ToLower(name)
			s := src()
			p, m, err := s.Enroll(ctx, id, name, "", prompts(c))
			if errors.Is(err, voicematch.ErrNotConfirmed) {
				c.Printf(
					"not confirmed: closest %s at %.3f; enroll again\n",
					m.Profile.Name,
					m.Score,
				)
				return nil
			}
			if err != nil {
				return err
			}
			profiles := slices.DeleteFunc(
				s.Profiles(),
				func(q voicematch.Profile) bool { return q.ID == id },
			)
			if err := s.SetProfiles(append(profiles, p)); err != nil {
				return err
			}
			c.Printf("enrolled %s, confirmed at %.3f\n", name, m.Score)
			return nil
		},
	})
	root.AddCommand(&cobra.Command{
		Use:   "check [COUNT]",
		Short: "Score COUNT wake words (10 by default) against this device's profiles without saving anything",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			n := 10
			if len(args) == 1 {
				v, err := strconv.Atoi(args[0])
				if err != nil || v < 1 {
					return fmt.Errorf("COUNT must be a positive number, got %q", args[0])
				}
				n = v
			}
			ctx, cancel := context.WithTimeout(c.Context(), time.Duration(n)*time.Minute)
			defer cancel()
			var scores []float64
			err := src().Check(ctx, n, prompts(c), func(m voicematch.Match) {
				scores = append(scores, m.Score)
				c.Printf("  %s %.3f\n", m.Profile.Name, m.Score)
			})
			if len(scores) > 0 {
				var sum float64
				for _, s := range scores {
					sum += s
				}
				c.Printf("%d scores: lowest %.3f, mean %.3f, highest %.3f\n",
					len(scores), slices.Min(scores), sum/float64(len(scores)), slices.Max(scores))
			}
			return err
		},
	})
	root.AddCommand(&cobra.Command{
		Use:   "last PATH",
		Short: "Write the clip the last voice match used to a 16 kHz mono WAV",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			clip := src().Last()
			if len(clip) == 0 {
				return errors.New("no voice match has run since the daemon started")
			}
			if err := capture.Write(args[0], clip, voiceprint.SampleRate, 1); err != nil {
				return err
			}
			c.Printf("%s %.3f s\n", args[0], float64(len(clip))/voiceprint.SampleRate)
			return nil
		},
	})
	root.AddCommand(&cobra.Command{
		Use:   "profiles",
		Short: "List the voice profiles this device matches against",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			for _, p := range src().Profiles() {
				c.Printf("%s\t%s\t%s\n", p.ID, p.Name, p.Person)
			}
			return nil
		},
	})
	return root
}
