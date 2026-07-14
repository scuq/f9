package sshx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// Dial establishes the (possibly multi-hop) connection. Proxyjump hops fold
// into a TCP-forwarding chain; a shell-hop (last hop only, for bastions that
// forbid forwarding) returns a client whose sessions run `ssh user@target`
// on the hop's shell.
func Dial(ctx context.Context, host string, port int, user string, p Prompter, o DialOpts) (Client, error) {
	if o.Timeout <= 0 {
		o.Timeout = 10 * time.Second
	}
	if port <= 0 {
		port = 22
	}
	if user == "" {
		user = os.Getenv("USER")
		if user == "" {
			user = os.Getenv("USERNAME")
		}
		if user == "" {
			return nil, errors.New("sshx: user required")
		}
	}
	khPath := o.KnownHostsPath
	if khPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("sshx: resolve home for known_hosts: %w", err)
		}
		khPath = filepath.Join(home, ".config", "f9", "known_hosts")
	}
	tf, err := newTOFU(khPath, p)
	if err != nil {
		return nil, err
	}
	keyFiles := o.KeyFiles
	if keyFiles == nil {
		keyFiles = defaultKeyFiles()
	}
	var agentSockets []string
	if !o.NoAgent {
		agentSockets = resolveAgentSockets(o.AgentSockets)
	}
	dialer := &net.Dialer{Timeout: o.Timeout}

	// firstAct records inbound data on the first physical TCP leg — the one a
	// VPN/wifi drop kills. Tunneled legs ride it, so tracking it covers all.
	var firstAct *activityConn
	connect := func(h string, prt int, usr string, via *ssh.Client) (*ssh.Client, error) {
		addr := net.JoinHostPort(h, strconv.Itoa(prt))
		var raw net.Conn
		var derr error
		if via == nil {
			raw, derr = dialer.DialContext(ctx, "tcp", addr)
			if derr == nil {
				ac := newActivityConn(raw)
				firstAct = ac
				raw = ac
			}
		} else {
			raw, derr = via.Dial("tcp", addr)
		}
		if derr != nil {
			return nil, fmt.Errorf("sshx: dial %s: %w", addr, derr)
		}
		cfg := &ssh.ClientConfig{
			User:            usr,
			Auth:            buildAuth(usr, addr, keyFiles, agentSockets, p),
			HostKeyCallback: tf.check,
			Timeout:         o.Timeout,
		}
		if o.OnBanner != nil {
			cfg.BannerCallback = func(msg string) error {
				o.OnBanner(msg)
				return nil
			}
		}
		// Bound the handshake even over hop-provided conns (which do not
		// support deadlines): kill the conn if the handshake stalls.
		timer := time.AfterFunc(o.Timeout, func() { raw.Close() })
		c, chans, reqs, herr := ssh.NewClientConn(raw, addr, cfg)
		timer.Stop()
		if herr != nil {
			raw.Close()
			return nil, fmt.Errorf("sshx: handshake %s: %w", addr, herr)
		}
		return ssh.NewClient(c, chans, reqs), nil
	}

	var chain []io.Closer
	fail := func(err error) (Client, error) {
		closeAll(chain)
		return nil, err
	}

	var prev *ssh.Client
	for i, hop := range o.JumpChain {
		switch hop.Mode {
		case "", "proxyjump", "shell-hop":
		default:
			return fail(fmt.Errorf("sshx: unknown hop mode %q", hop.Mode))
		}
		hu := hop.User
		if hu == "" {
			hu = user
		}
		hp := hop.Port
		if hp <= 0 {
			hp = 22
		}
		hc, err := connect(hop.Host, hp, hu, prev)
		if err != nil {
			return fail(err)
		}
		chain = append(chain, hc)
		if hop.Mode == "shell-hop" {
			if i != len(o.JumpChain)-1 {
				return fail(errors.New("sshx: shell-hop must be the last hop (mixed chains: TODO)"))
			}
			cmd, err := shellHopCommand(host, port, user)
			if err != nil {
				return fail(err)
			}
			shc := &shellHopClient{
				hop:           hc,
				cmd:           cmd,
				closers:       chain,
				serverVersion: string(hc.ServerVersion()),
			}
			shc.startKeepalive(o.KeepaliveInterval, firstAct)
			return shc, nil
		}
		prev = hc
	}

	final, err := connect(host, port, user, prev)
	if err != nil {
		return fail(err)
	}
	chain = append(chain, final)
	nc := &nativeClient{c: final, closers: chain}
	nc.startKeepalive(o.KeepaliveInterval, firstAct)
	if o.SocksPort > 0 {
		// best-effort: a bind failure must not sink the SSH session.
		if sp, err := startSocks(o.SocksPort, final); err == nil {
			nc.socks = sp
		}
	}
	return nc, nil
}

func closeAll(closers []io.Closer) {
	for i := len(closers) - 1; i >= 0; i-- {
		_ = closers[i].Close()
	}
}

type nativeClient struct {
	c       *ssh.Client
	closers []io.Closer
	socks   *socksProxy

	kaMu   sync.Mutex
	kaStop chan struct{}
}

func (n *nativeClient) ServerVersion() string { return string(n.c.ServerVersion()) }

func (n *nativeClient) Wait() error { return n.c.Wait() }

func (n *nativeClient) SocksActive() bool { return n.socks != nil }

func (n *nativeClient) NewSession(_ context.Context, termType string, cols, rows int) (Session, error) {
	s, err := n.c.NewSession()
	if err != nil {
		return nil, fmt.Errorf("sshx: new session: %w", err)
	}
	return wrapSession(s, termType, cols, rows, "")
}

// connInfoFrom extracts the negotiated transport parameters from an
// established client (exposed via ssh.AlgorithmsConnMetadata).
func connInfoFrom(c *ssh.Client, relay bool) ConnInfo {
	ci := ConnInfo{ServerVersion: string(c.ServerVersion()), Relay: relay}
	if m, ok := c.Conn.(ssh.AlgorithmsConnMetadata); ok {
		a := m.Algorithms()
		ci.KeyExchange = a.KeyExchange
		ci.HostKey = a.HostKey
		ci.CipherIn = a.Read.Cipher
		ci.CipherOut = a.Write.Cipher
		ci.MACIn = a.Read.MAC
		ci.MACOut = a.Write.MAC
	}
	return ci
}

func (n *nativeClient) ConnInfo() ConnInfo   { return connInfoFrom(n.c, false) }
func (h *shellHopClient) ConnInfo() ConnInfo { return connInfoFrom(h.hop, true) }

func (n *nativeClient) startKeepalive(interval time.Duration, act *activityConn) {
	if interval <= 0 {
		return
	}
	n.kaMu.Lock()
	n.kaStop = make(chan struct{})
	stop := n.kaStop
	n.kaMu.Unlock()
	go runKeepalive(n.c, interval, act, stop)
}

func (n *nativeClient) Close() error {
	n.kaMu.Lock()
	if n.kaStop != nil {
		close(n.kaStop)
		n.kaStop = nil
	}
	n.kaMu.Unlock()
	if n.socks != nil {
		_ = n.socks.Close()
	}
	closeAll(n.closers)
	return nil
}

// shellHopClient runs `ssh user@target` on the hop's shell for every session.
// Auth to the target happens interactively through the data stream (or the
// hop's own agent) — f9 never persists anything (ADR-0005).
type shellHopClient struct {
	hop           *ssh.Client
	cmd           string
	closers       []io.Closer
	serverVersion string

	kaMu   sync.Mutex
	kaStop chan struct{}
}

func (h *shellHopClient) startKeepalive(interval time.Duration, act *activityConn) {
	if interval <= 0 {
		return
	}
	h.kaMu.Lock()
	h.kaStop = make(chan struct{})
	stop := h.kaStop
	h.kaMu.Unlock()
	go runKeepalive(h.hop, interval, act, stop)
}

func (h *shellHopClient) ServerVersion() string { return h.serverVersion }

func (h *shellHopClient) Wait() error { return h.hop.Wait() }

func (h *shellHopClient) SocksActive() bool { return false }

func (h *shellHopClient) NewSession(_ context.Context, termType string, cols, rows int) (Session, error) {
	s, err := h.hop.NewSession()
	if err != nil {
		return nil, fmt.Errorf("sshx: new shell-hop session: %w", err)
	}
	return wrapSession(s, termType, cols, rows, h.cmd)
}

func (h *shellHopClient) Close() error {
	h.kaMu.Lock()
	if h.kaStop != nil {
		close(h.kaStop)
		h.kaStop = nil
	}
	h.kaMu.Unlock()
	closeAll(h.closers)
	return nil
}

// safeArg allowlists what may appear in the shell-hop command line. This is a
// security boundary: session names/hosts must not be able to inject shell
// syntax on the bastion. (Consequence: IPv6 literals are unsupported for
// shell-hop targets; use proxyjump for those.)
var safeArg = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func shellHopCommand(host string, port int, user string) (string, error) {
	if !safeArg.MatchString(host) {
		return "", fmt.Errorf("sshx: unsafe shell-hop target host %q", host)
	}
	if user != "" && !safeArg.MatchString(user) {
		return "", fmt.Errorf("sshx: unsafe shell-hop target user %q", user)
	}
	cmd := "ssh"
	if port > 0 && port != 22 {
		cmd += " -p " + strconv.Itoa(port)
	}
	if user != "" {
		cmd += " " + user + "@" + host
	} else {
		cmd += " " + host
	}
	return cmd, nil
}
