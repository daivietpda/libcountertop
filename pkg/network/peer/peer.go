package peer

import (
	"errors"
	"net"
	"slices"
	"strconv"
	"time"
)

var (
	ErrNoRoute  = errors.New("peer has no IPv4 address")
	ErrInsecure = errors.New("peer serves no secure port")
)

type Standing int

const (
	Open Standing = iota
	Checking
	Home
	Foreign
)

func (s Standing) String() string {
	switch s {
	case Checking:
		return "checking"
	case Home:
		return "home"
	case Foreign:
		return "foreign"
	}
	return "open"
}

func (s Standing) Trusted() bool { return s == Open || s == Home }

type Peer struct {
	ID          string
	Name        string
	Model       string
	Board       string
	Project     string
	Version     string
	Caps        []string
	Host        string
	Addrs       []net.IP
	Port        int
	Secure      int
	Arbitration int
	Standing    Standing
	Seen        time.Time
}

func (p Peer) Video() bool { return slices.Contains(p.Caps, "video") }

func (p Peer) Syncs() bool { return slices.Contains(p.Caps, "sync") }

func (p Peer) Addr() (string, error) { return p.at(p.Port) }

func (p Peer) URL() (string, error) {
	if p.Secure != 0 {
		addr, err := p.SecureAddr()
		return "https://" + addr, err
	}
	// TODO: remove the plain-HTTP peer path once no device predates the secure port.
	addr, err := p.Addr()
	return "http://" + addr, err
}

func (p Peer) SecureAddr() (string, error) {
	if p.Secure == 0 {
		return "", ErrInsecure
	}
	return p.at(p.Secure)
}

func (p Peer) at(port int) (string, error) {
	for _, a := range p.Addrs {
		if a.To4() != nil {
			return net.JoinHostPort(a.String(), strconv.Itoa(port)), nil
		}
	}
	return "", ErrNoRoute
}
