package rtspd

import (
	"log/slog"
	"net"
	"strconv"
	"sync"

	"github.com/ygelfand/libcountertop/pkg/camera/camerafeed"
	"github.com/ygelfand/libcountertop/pkg/camera/onvif"
	"github.com/ygelfand/libcountertop/pkg/camera/session"
	"github.com/ygelfand/libcountertop/pkg/media/rtsp"
	"github.com/ygelfand/libcountertop/pkg/network/digest"
)

var paths = [...]string{"main", "sub"}

func StreamName(i int) string {
	if i < 0 || i >= len(paths) {
		return ""
	}
	return paths[i]
}

type Source interface {
	JoinOrCard(at int) (*session.Session, <-chan camerafeed.Frame, error)
	Leave(s *session.Session, frames <-chan camerafeed.Frame)
	Sizes() []session.Size
	RequestKey() error
}

type Options struct {
	Port     int
	Identity rtsp.Identity
	Source   Source
	Helper   string
	Served   func() []int
	FPS      int
	Bitrate  func(width, height int) int
	ONVIF    func() onvif.Config
	Auth     digest.Credentials
	Mic      Mic
	MicMuted func() bool
	Audio    func() bool
}

type Server struct {
	options Options

	mu    sync.Mutex
	srv   *rtsp.Server
	pumps [len(paths)]*pump
	sound *sound
	desc  *onvif.Endpoint
}

func New(options Options) *Server { return &Server{options: options} }

func (s *Server) Profiles() []onvif.Profile {
	sizes := s.options.Source.Sizes()
	var out []onvif.Profile
	for _, i := range s.options.Served() {
		sz := sizes[i]
		out = append(out, onvif.Profile{
			Token:   paths[i],
			Name:    StreamName(i),
			Width:   sz.Width,
			Height:  sz.Height,
			FPS:     s.options.FPS,
			Bitrate: s.options.Bitrate(sz.Width, sz.Height),
			Path:    paths[i],
		})
	}
	return out
}

func (s *Server) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.srv != nil {
		return
	}
	port := s.options.Port
	ln, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	if err != nil {
		slog.Error("the rtsp server could not listen", "port", port, "err", err)
		return
	}
	srv := rtsp.NewServerWithIdentity(s.options.Identity)
	srv.Auth = s.options.Auth
	served := s.options.Served()
	s.sound = nil
	if s.options.Mic != nil && s.options.Audio != nil && s.options.Audio() {
		a, err := newSound(s.options.Mic, s.options.MicMuted)
		if err != nil {
			slog.Warn("rtsp serves video only", "err", err)
		}
		s.sound = a
	}
	for _, i := range served {
		st := rtsp.NewStream()
		p := &pump{
			at:     i,
			stream: st,
			source: s.options.Source,
			fps:    s.options.FPS,
			helper: s.options.Helper,
		}
		st.OnDemand = p.demand
		if a := s.sound; a != nil {
			a.carry(st)
			st.OnDemand = func() {
				p.demand()
				a.demand()
			}
		}
		s.pumps[i] = p
		srv.Handle(paths[i], st)
	}
	s.srv = srv
	s.desc = s.describe()
	go func() {
		if err := srv.Serve(ln); err != nil {
			slog.Warn("the rtsp server stopped", "err", err)
		}
	}()
	for _, i := range served {
		slog.Info("rtsp serving", "url", "rtsp://<device>:"+strconv.Itoa(port)+"/"+paths[i])
	}
}

func (s *Server) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.srv == nil {
		return
	}
	_ = s.srv.Close()
	s.srv = nil
	s.desc.Close()
	s.desc = nil
	for i, p := range s.pumps {
		if p != nil {
			p.halt()
		}
		s.pumps[i] = nil
	}
	if s.sound != nil {
		s.sound.halt()
		s.sound = nil
	}
}

func (s *Server) describe() *onvif.Endpoint {
	if s.options.ONVIF == nil {
		return nil
	}
	cfg := s.options.ONVIF()
	cfg.Device.RTSPPort = s.options.Port
	cfg.Device.Profiles = s.Profiles
	cfg.Auth = s.options.Auth
	return onvif.Serve(cfg)
}
