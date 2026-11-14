package dhcp

import (
	"bufio"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"time"
)

// FrameworkListener: send "<seq> <cmd> <args>\0", read "<code> <seq> <text>\0" until code >= 200.
const (
	netdSocket = "/dev/socket/netd"
	netdReply  = 5 * time.Second

	// Android's own netIds start at 100.
	netID = "100"
)

type netd struct {
	conn net.Conn
	r    *bufio.Reader
	seq  int
}

func dialNetd() (*netd, error) {
	conn, err := net.Dial("unix", netdSocket)
	if err != nil {
		return nil, fmt.Errorf("netd: %w", err)
	}
	return &netd{conn: conn, r: bufio.NewReader(conn)}, nil
}

func (n *netd) Close() error { return n.conn.Close() }

func (n *netd) cmd(args ...string) error {
	if err := n.conn.SetDeadline(time.Now().Add(netdReply)); err != nil {
		return err
	}

	n.seq++
	quoted := make([]string, 0, len(args))
	for _, a := range args {
		// NUL ends a netd command; one inside an argument would cut the command short.
		quoted = append(quoted, quote(strings.ReplaceAll(a, "\x00", "")))
	}
	line := fmt.Sprintf("%d %s\x00", n.seq, strings.Join(quoted, " "))

	if _, err := n.conn.Write([]byte(line)); err != nil {
		return fmt.Errorf("netd: %s: %w", args[0], err)
	}

	for {
		raw, err := n.r.ReadString(0)
		if err != nil {
			return fmt.Errorf("netd: %s: %w", args[0], err)
		}
		reply := strings.TrimSpace(strings.TrimRight(raw, "\x00"))

		fields := strings.SplitN(reply, " ", 3)
		if len(fields) < 2 {
			continue
		}

		code, err := strconv.Atoi(fields[0])
		if err != nil || code < 200 {
			continue
		}
		if seq, err := strconv.Atoi(fields[1]); err != nil || seq != n.seq {
			continue
		}
		if code >= 400 {
			return fmt.Errorf("netd: %s: %s", strings.Join(args, " "), reply)
		}
		return nil
	}
}

func onLink(addr net.IPNet) string {
	ip := addr.IP.To4()
	if ip == nil || addr.Mask == nil {
		return ""
	}
	if _, bits := addr.Mask.Size(); bits != 32 {
		return ""
	}

	subnet := net.IPNet{IP: ip.Mask(addr.Mask), Mask: addr.Mask}
	return subnet.String()
}

func quote(arg string) string {
	if arg != "" && !strings.ContainsAny(arg, " \"\\") {
		return arg
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(arg) + `"`
}

func useNetd(iface string, was, l *Lease) error {
	n, err := dialNetd()
	if err != nil {
		return err
	}
	defer n.Close()

	return configure(n, iface, was, l)
}

func configure(n *netd, iface string, was, l *Lease) error {
	if was != nil && was.Router != nil && !was.Router.Equal(l.Router) {
		if err := n.cmd("network", "route", "remove", netID, iface,
			"0.0.0.0/0", was.Router.String()); err != nil {
			slog.Warn("the old default route would not come out", "router", was.Router, "err", err)
		}
	}

	setup := [][]string{
		{"network", "create", netID},
		{"network", "interface", "add", netID, iface},
	}

	if subnet := onLink(l.Address); subnet != "" {
		setup = append(setup, []string{"network", "route", "add", netID, iface, subnet})
	}
	if l.Router != nil {
		setup = append(setup,
			[]string{"network", "route", "add", netID, iface, "0.0.0.0/0", l.Router.String()})
	}
	var setupErr error
	for _, cmd := range setup {
		setupErr = errors.Join(setupErr, n.cmd(cmd...))
	}

	dns := make([]string, 0, len(l.DNS))
	for _, ip := range l.DNS {
		dns = append(dns, ip.String())
	}
	if len(dns) == 0 {
		return nil
	}

	if err := n.cmd(
		append([]string{"resolver", "setnetdns", netID, l.Domain}, dns...)...); err != nil {
		return errors.Join(setupErr, err)
	}
	if err := n.cmd("network", "default", "set", netID); err != nil {
		return errors.Join(setupErr, err)
	}
	return nil
}
