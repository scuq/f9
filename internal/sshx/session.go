package sshx

import (
	"fmt"
	"io"
	"sync"

	"golang.org/x/crypto/ssh"
)

// pendingCap bounds the pre-registration buffer; beyond it the oldest data is
// dropped (a consumer registering that late has bigger problems).
const pendingCap = 1 << 20

type session struct {
	s     *ssh.Session
	stdin io.WriteCloser

	mu       sync.Mutex
	handlers []func([]byte)
	pending  []byte
}

// wrapSession requests a PTY, starts the shell, begins pumping stdout+stderr
// to OnData handlers, and (shell-hop) sends the initial command line.
func wrapSession(s *ssh.Session, termType string, cols, rows int, initialCmd string) (Session, error) {
	if termType == "" {
		termType = "xterm-256color"
	}
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := s.RequestPty(termType, rows, cols, modes); err != nil {
		s.Close()
		return nil, fmt.Errorf("sshx: request pty: %w", err)
	}
	stdin, err := s.StdinPipe()
	if err != nil {
		s.Close()
		return nil, fmt.Errorf("sshx: stdin pipe: %w", err)
	}
	stdout, err := s.StdoutPipe()
	if err != nil {
		s.Close()
		return nil, fmt.Errorf("sshx: stdout pipe: %w", err)
	}
	stderr, err := s.StderrPipe()
	if err != nil {
		s.Close()
		return nil, fmt.Errorf("sshx: stderr pipe: %w", err)
	}
	if err := s.Shell(); err != nil {
		s.Close()
		return nil, fmt.Errorf("sshx: start shell: %w (the pty was granted, so the server refused the shell request itself; the reason is only in its sshd/PAM log \u2014 process or login limits are the usual cause)", err)
	}
	w := &session{s: s, stdin: stdin}
	go w.pump(stdout)
	go w.pump(stderr)
	if initialCmd != "" {
		if _, err := io.WriteString(stdin, initialCmd+"\n"); err != nil {
			s.Close()
			return nil, fmt.Errorf("sshx: send shell-hop command: %w", err)
		}
	}
	return w, nil
}

func (w *session) pump(r io.Reader) {
	buf := make([]byte, 32<<10)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			w.mu.Lock()
			if len(w.handlers) == 0 {
				w.pending = append(w.pending, buf[:n]...)
				if over := len(w.pending) - pendingCap; over > 0 {
					w.pending = w.pending[over:]
				}
				w.mu.Unlock()
			} else {
				hs := make([]func([]byte), len(w.handlers))
				copy(hs, w.handlers)
				data := append([]byte(nil), buf[:n]...)
				w.mu.Unlock()
				for _, h := range hs {
					h(data)
				}
			}
		}
		if err != nil {
			return
		}
	}
}

func (w *session) OnData(f func(p []byte)) {
	w.mu.Lock()
	w.handlers = append(w.handlers, f)
	p := w.pending
	w.pending = nil
	w.mu.Unlock()
	if len(p) > 0 {
		f(p)
	}
}

func (w *session) Stdin() io.Writer { return w.stdin }

func (w *session) Resize(cols, rows int) error {
	return w.s.WindowChange(rows, cols) // x/crypto order: height, width
}

func (w *session) Wait() error  { return w.s.Wait() }
func (w *session) Close() error { return w.s.Close() }
