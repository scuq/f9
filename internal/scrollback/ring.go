package scrollback

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/klauspost/compress/zstd"
)

// forcedLineSplit bounds memory against pathological input: a line longer than
// this without any newline is force-split and counted as multiple lines.
const forcedLineSplit = 4 << 20

type sealedChunk struct {
	raw       []byte // set until compressed, then nil
	comp      []byte // set after compression
	rawLen    int
	firstLine int
	lines     int
}

type ring struct {
	cfg Config

	mu          sync.Mutex
	sealed      []*sealedChunk
	active      []byte
	activeEnds  []uint32 // positions of '\n' in active
	firstAvail  int      // absolute line number of the oldest retained line
	activeFirst int      // absolute line number of the first line in active
	totalBytes  int64    // raw active + raw pending + compressed sealed
	onSeal      []func(chunk []byte, firstLine, lastLine int)
	closed      bool

	cacheKeys []*sealedChunk
	cache     map[*sealedChunk][]byte // decompress cache, FIFO, max 4

	sealCh chan *sealedChunk
	wg     sync.WaitGroup
	enc    *zstd.Encoder
	dec    *zstd.Decoder
}

func newRing(cfg Config) *ring {
	if cfg.ChunkSize <= 0 {
		cfg.ChunkSize = 1 << 20
	}
	if cfg.MaxLines <= 0 {
		cfg.MaxLines = 5_000_000
	}
	if cfg.MaxBytes <= 0 {
		cfg.MaxBytes = 512 << 20
	}
	enc, err := zstd.NewWriter(nil)
	if err != nil {
		panic("scrollback: zstd encoder: " + err.Error())
	}
	dec, err := zstd.NewReader(nil)
	if err != nil {
		panic("scrollback: zstd decoder: " + err.Error())
	}
	r := &ring{
		cfg:    cfg,
		cache:  map[*sealedChunk][]byte{},
		sealCh: make(chan *sealedChunk, 8),
		enc:    enc,
		dec:    dec,
	}
	r.wg.Add(1)
	go r.compressor()
	return r
}

// Append is the hot path: copy bytes, index newlines, maybe cut a chunk.
// Compression, eviction bookkeeping for compressed sizes and OnSeal callbacks
// all happen on the compressor goroutine.
func (r *ring) Append(p []byte) {
	if len(p) == 0 {
		return
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	base := len(r.active)
	r.active = append(r.active, p...)
	for i := 0; i < len(p); {
		j := bytes.IndexByte(p[i:], '\n')
		if j < 0 {
			break
		}
		r.activeEnds = append(r.activeEnds, uint32(base+i+j))
		i += j + 1
	}
	r.totalBytes += int64(len(p))
	notify := r.maybeSealLocked()
	r.mu.Unlock()
	for _, f := range notify {
		f()
	}
}

// maybeSealLocked cuts full chunks at the last newline (the partial tail is
// carried into the fresh active chunk, so sealed chunks always end at a
// newline). Returns OnSeal notifications to fire after unlocking (only the
// rare synchronous-compression fallback produces any).
func (r *ring) maybeSealLocked() []func() {
	var notify []func()
	for len(r.active) >= r.cfg.ChunkSize {
		var cut, lines int
		if n := len(r.activeEnds); n > 0 {
			cut = int(r.activeEnds[n-1]) + 1
			lines = n
		} else if len(r.active) >= forcedLineSplit {
			cut = len(r.active)
			lines = 1
		} else {
			return notify // no newline yet; wait for one
		}
		s := &sealedChunk{raw: r.active[:cut:cut], rawLen: cut, firstLine: r.activeFirst, lines: lines}
		tail := r.active[cut:]
		na := make([]byte, len(tail), r.cfg.ChunkSize)
		copy(na, tail)
		r.active = na
		r.activeEnds = r.activeEnds[:0]
		r.activeFirst += lines
		r.sealed = append(r.sealed, s)
		select {
		case r.sealCh <- s:
		default:
			// Compressor backlogged: compress inline (never block on the
			// channel while holding the lock — the compressor needs it).
			notify = append(notify, r.compressLocked(s)...)
		}
	}
	return notify
}

// compressLocked compresses a chunk, updates accounting and eviction, and
// returns the OnSeal notifications. Callers must hold r.mu.
func (r *ring) compressLocked(s *sealedChunk) []func() {
	comp := r.enc.EncodeAll(s.raw, nil)
	s.comp = comp
	r.totalBytes += int64(len(comp)) - int64(s.rawLen)
	s.raw = nil
	r.evictLocked()
	first, last := s.firstLine, s.firstLine+s.lines-1
	notify := make([]func(), 0, len(r.onSeal))
	for _, f := range r.onSeal {
		f := f
		notify = append(notify, func() { f(comp, first, last) })
	}
	return notify
}

func (r *ring) compressor() {
	defer r.wg.Done()
	for s := range r.sealCh {
		if s.comp != nil {
			continue // already handled by the inline fallback
		}
		comp := r.enc.EncodeAll(s.raw, nil)
		r.mu.Lock()
		s.comp = comp
		r.totalBytes += int64(len(comp)) - int64(s.rawLen)
		s.raw = nil
		r.evictLocked()
		first, last := s.firstLine, s.firstLine+s.lines-1
		notify := make([]func(), 0, len(r.onSeal))
		for _, f := range r.onSeal {
			f := f
			notify = append(notify, func() { f(comp, first, last) })
		}
		r.mu.Unlock()
		for _, f := range notify {
			f()
		}
	}
}

// evictLocked drops oldest compressed chunks while over either cap.
func (r *ring) evictLocked() {
	retained := r.activeFirst - r.firstAvail + r.activeLinesLocked()
	for len(r.sealed) > 0 {
		head := r.sealed[0]
		if head.comp == nil {
			return // transient: still raw/pending; next compression retries
		}
		if r.totalBytes <= r.cfg.MaxBytes && retained <= r.cfg.MaxLines {
			return
		}
		r.totalBytes -= int64(len(head.comp))
		r.firstAvail += head.lines
		retained -= head.lines
		delete(r.cache, head)
		r.sealed = r.sealed[1:]
	}
}

func (r *ring) activeLinesLocked() int {
	n := len(r.activeEnds)
	tailStart := 0
	if n > 0 {
		tailStart = int(r.activeEnds[n-1]) + 1
	}
	if len(r.active) > tailStart {
		n++
	}
	return n
}

func (r *ring) Len() (int, int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.activeFirst - r.firstAvail + r.activeLinesLocked(), r.totalBytes
}

func (r *ring) FirstLine() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.firstAvail
}

func (r *ring) OnSeal(f func(chunk []byte, firstLine, lastLine int)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onSeal = append(r.onSeal, f)
}

func (r *ring) Lines(from, to int) ([][]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if from < 0 || to < from {
		return nil, fmt.Errorf("scrollback: invalid range [%d,%d)", from, to)
	}
	if from < r.firstAvail {
		return nil, fmt.Errorf("scrollback: line %d evicted (first retained is %d)", from, r.firstAvail)
	}
	end := r.activeFirst + r.activeLinesLocked()
	if to > end {
		return nil, fmt.Errorf("scrollback: line %d beyond end %d", to, end)
	}
	out := make([][]byte, 0, to-from)
	for _, s := range r.sealed {
		if s.firstLine+s.lines <= from {
			continue
		}
		if s.firstLine >= to {
			return out, nil
		}
		raw, err := r.rawOfLocked(s)
		if err != nil {
			return nil, err
		}
		for i, ln := range splitLines(raw) {
			g := s.firstLine + i
			if g >= from && g < to {
				out = append(out, copyBytes(ln))
			}
		}
	}
	if to > r.activeFirst {
		for i, ln := range splitLines(r.active) {
			g := r.activeFirst + i
			if g >= from && g < to {
				out = append(out, copyBytes(ln))
			}
		}
	}
	return out, nil
}

// rawOfLocked returns the raw bytes of a sealed chunk, via the small FIFO
// decompress cache. Callers must hold r.mu.
func (r *ring) rawOfLocked(s *sealedChunk) ([]byte, error) {
	if s.raw != nil {
		return s.raw, nil
	}
	if b, ok := r.cache[s]; ok {
		return b, nil
	}
	b, err := r.dec.DecodeAll(s.comp, nil)
	if err != nil {
		return nil, fmt.Errorf("scrollback: decompress: %w", err)
	}
	r.cache[s] = b
	r.cacheKeys = append(r.cacheKeys, s)
	for len(r.cacheKeys) > 4 {
		old := r.cacheKeys[0]
		r.cacheKeys = r.cacheKeys[1:]
		delete(r.cache, old)
	}
	return b, nil
}

// chunkView is an immutable snapshot of one chunk for lock-free iteration:
// exactly one of raw/comp is non-nil and neither is mutated after capture.
type chunkView struct {
	raw  []byte
	comp []byte
}

func (r *ring) Grep(re *regexp.Regexp, opts GrepOpts) (Iterator, error) {
	if re == nil {
		return nil, errors.New("scrollback: nil regexp")
	}
	if opts.IgnoreCase && !strings.HasPrefix(re.String(), "(?i)") {
		if re2, err := regexp.Compile("(?i)" + re.String()); err == nil {
			re = re2
		}
	}
	r.mu.Lock()
	views := make([]chunkView, 0, len(r.sealed))
	for _, s := range r.sealed {
		views = append(views, chunkView{raw: s.raw, comp: s.comp})
	}
	activeCopy := copyBytes(r.active)
	base := r.firstAvail
	r.mu.Unlock()
	return &grepIter{
		re:         re,
		opts:       opts,
		dec:        r.dec,
		chunks:     views,
		active:     activeCopy,
		globalLine: base,
	}, nil
}

type grepIter struct {
	re   *regexp.Regexp
	opts GrepOpts
	dec  *zstd.Decoder

	chunks     []chunkView
	chunkIdx   int
	active     []byte
	activeDone bool

	lines      [][]byte
	lineIdx    int
	look       [][]byte
	before     [][]byte
	globalLine int
	matches    int
	err        error
}

func (it *grepIter) Next() (Match, bool) {
	if it.err != nil {
		return Match{}, false
	}
	if it.opts.MaxMatches > 0 && it.matches >= it.opts.MaxMatches {
		return Match{}, false
	}
	for {
		it.fillLook(1)
		if it.err != nil || len(it.look) == 0 {
			return Match{}, false
		}
		ln := it.look[0]
		it.look = it.look[1:]
		lineNo := it.globalLine
		it.globalLine++

		matched := it.re.Match(ln)
		if it.opts.Invert {
			matched = !matched
		}
		if !matched {
			it.pushBefore(ln)
			continue
		}

		m := Match{LineNo: lineNo, Line: copyBytes(ln)}
		if it.opts.Before > 0 {
			m.Before = make([][]byte, 0, len(it.before))
			m.Before = append(m.Before, it.before...) // entries are already copies
		}
		if a := it.opts.After; a > 0 {
			it.fillLook(a)
			n := a
			if n > len(it.look) {
				n = len(it.look)
			}
			m.After = make([][]byte, 0, n)
			for i := 0; i < n; i++ {
				m.After = append(m.After, copyBytes(it.look[i]))
			}
		}
		it.pushBefore(ln)
		it.matches++
		return m, true
	}
}

func (it *grepIter) Close() error { return it.err }

func (it *grepIter) pushBefore(ln []byte) {
	if it.opts.Before <= 0 {
		return
	}
	it.before = append(it.before, copyBytes(ln))
	if len(it.before) > it.opts.Before {
		it.before = it.before[1:]
	}
}

// fillLook ensures n lines of lookahead (if the stream has them).
func (it *grepIter) fillLook(n int) {
	for len(it.look) < n {
		ln, ok := it.pullRaw()
		if !ok {
			return
		}
		it.look = append(it.look, ln)
	}
}

func (it *grepIter) pullRaw() ([]byte, bool) {
	for {
		if it.lineIdx < len(it.lines) {
			ln := it.lines[it.lineIdx]
			it.lineIdx++
			return ln, true
		}
		if it.chunkIdx < len(it.chunks) {
			v := it.chunks[it.chunkIdx]
			it.chunkIdx++
			raw := v.raw
			if raw == nil {
				b, err := it.dec.DecodeAll(v.comp, nil)
				if err != nil {
					it.err = fmt.Errorf("scrollback: grep decompress: %w", err)
					return nil, false
				}
				raw = b
			}
			it.lines = splitLines(raw)
			it.lineIdx = 0
			continue
		}
		if !it.activeDone {
			it.activeDone = true
			if len(it.active) > 0 {
				it.lines = splitLines(it.active)
				it.lineIdx = 0
				continue
			}
		}
		return nil, false
	}
}

func (r *ring) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	r.mu.Unlock()
	close(r.sealCh)
	r.wg.Wait()
	r.dec.Close()
	_ = r.enc.Close()
	return nil
}

// splitLines splits raw on '\n', stripping the newline; a trailing segment
// without a newline (partial line) is included.
func splitLines(raw []byte) [][]byte {
	var out [][]byte
	for len(raw) > 0 {
		j := bytes.IndexByte(raw, '\n')
		if j < 0 {
			out = append(out, raw)
			break
		}
		out = append(out, raw[:j])
		raw = raw[j+1:]
	}
	return out
}

func copyBytes(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	return append([]byte(nil), b...)
}
