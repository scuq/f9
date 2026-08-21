package app

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// Scrollback memory governor. Every open terminal owns a ring buffer whose
// byte cap defaults to 512 MiB; with many sessions that is unbounded in
// practice. The app instead splits one global budget — a quarter of physical
// RAM, clamped to [256 MiB, 2 GiB] — evenly across open terminals and the
// rings evict their oldest chunks to fit. Constants are exported for tests.
const (
	scrollbackBudgetMin   = 256 << 20
	scrollbackBudgetMax   = 2 << 30
	scrollbackPerTermMin  = 16 << 20
	scrollbackBudgetShare = 4 // 1/4 of physical memory
)

// physicalMemory returns total RAM in bytes, or 0 when unknown (non-Linux).
func physicalMemory() int64 {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "MemTotal:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0
		}
		kb, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return 0
		}
		return kb << 10
	}
	return 0
}

// scrollbackBudget is the global byte cap shared by all open terminals.
func scrollbackBudget(physical int64) int64 {
	if physical <= 0 {
		return 1 << 30 // unknown platform: 1 GiB
	}
	b := physical / scrollbackBudgetShare
	if b < scrollbackBudgetMin {
		b = scrollbackBudgetMin
	}
	if b > scrollbackBudgetMax {
		b = scrollbackBudgetMax
	}
	return b
}

// perTerminalCap splits budget across n open terminals.
func perTerminalCap(budget int64, n int) int64 {
	if n < 1 {
		n = 1
	}
	c := budget / int64(n)
	if c < scrollbackPerTermMin {
		c = scrollbackPerTermMin
	}
	return c
}

// rebalanceScrollback re-applies the per-terminal cap after a terminal is
// opened or closed. Must be called WITHOUT a.tmu held.
func (a *App) rebalanceScrollback() {
	a.tmu.Lock()
	bufs := make([]interface{ SetMaxBytes(int64) }, 0, len(a.terms))
	for _, t := range a.terms {
		bufs = append(bufs, t.sb)
	}
	a.tmu.Unlock()
	c := perTerminalCap(scrollbackBudget(physicalMemory()), len(bufs))
	for _, b := range bufs {
		b.SetMaxBytes(c)
	}
}
