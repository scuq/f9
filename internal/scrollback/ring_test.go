package scrollback

import (
	"fmt"
	"regexp"
	"testing"
	"time"
)

func TestLinesBasic(t *testing.T) {
	b := New(Config{})
	defer b.Close()

	b.Append([]byte("alpha\nbravo\ncharlie"))
	if n, _ := b.Len(); n != 3 {
		t.Fatalf("Len = %d, want 3 (partial tail counts)", n)
	}
	got, err := b.Lines(0, 3)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"alpha", "bravo", "charlie"}
	for i := range want {
		if string(got[i]) != want[i] {
			t.Fatalf("line %d = %q, want %q", i, got[i], want[i])
		}
	}

	b.Append([]byte(" delta\n"))
	if n, _ := b.Len(); n != 3 {
		t.Fatalf("Len after tail completion = %d, want 3", n)
	}
	got, err = b.Lines(2, 3)
	if err != nil {
		t.Fatal(err)
	}
	if string(got[0]) != "charlie delta" {
		t.Fatalf("tail merge: got %q, want %q", got[0], "charlie delta")
	}
}

func TestSealAndReadAcrossChunks(t *testing.T) {
	b := New(Config{ChunkSize: 64})
	defer b.Close()

	const n = 100
	for i := 0; i < n; i++ {
		b.Append([]byte(fmt.Sprintf("l%03d-xxxxxxxxxx\n", i)))
	}
	if got, _ := b.Len(); got != n {
		t.Fatalf("Len = %d, want %d", got, n)
	}
	if b.FirstLine() != 0 {
		t.Fatalf("FirstLine = %d, want 0", b.FirstLine())
	}
	lines, err := b.Lines(0, n)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		want := fmt.Sprintf("l%03d-xxxxxxxxxx", i)
		if string(lines[i]) != want {
			t.Fatalf("line %d = %q, want %q", i, lines[i], want)
		}
	}
	mid, err := b.Lines(95, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(mid) != 5 || string(mid[0]) != "l095-xxxxxxxxxx" {
		t.Fatalf("partial range wrong: %q", mid)
	}
}

func TestRangeValidation(t *testing.T) {
	b := New(Config{})
	defer b.Close()
	b.Append([]byte("one\ntwo\n"))
	if _, err := b.Lines(1, 0); err == nil {
		t.Fatal("expected error for inverted range")
	}
	if _, err := b.Lines(0, 3); err == nil {
		t.Fatal("expected error beyond end")
	}
	if got, err := b.Lines(1, 1); err != nil || len(got) != 0 {
		t.Fatalf("empty range: got %v, %v", got, err)
	}
}

func TestEvictionByBytes(t *testing.T) {
	b := New(Config{ChunkSize: 2048, MaxBytes: 8192})
	defer b.Close()

	line := []byte("0123456789012345678901234567890123456789012345678901234567890\n") // 64 B
	for i := 0; i < 4096; i++ {                                                       // 256 KiB total >> 8 KiB cap
		b.Append(line)
	}
	deadline := time.Now().Add(3 * time.Second)
	for b.FirstLine() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("eviction did not occur within deadline")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := b.Lines(0, 1); err == nil {
		t.Fatal("expected evicted-line error for Lines(0,1)")
	}
	first := b.FirstLine()
	if got, err := b.Lines(first, first+1); err != nil || len(got) != 1 {
		t.Fatalf("oldest retained line unreadable: %v, %v", got, err)
	}
}

func TestOnSeal(t *testing.T) {
	b := New(Config{ChunkSize: 1024})
	defer b.Close()

	type seal struct{ first, last int }
	ch := make(chan seal, 16)
	b.OnSeal(func(chunk []byte, firstLine, lastLine int) {
		if len(chunk) == 0 {
			t.Error("empty compressed chunk")
		}
		ch <- seal{firstLine, lastLine}
	})

	for i := 0; i < 200; i++ {
		b.Append([]byte(fmt.Sprintf("interface Ethernet1/%03d\n  no shutdown\n", i)))
	}
	select {
	case s := <-ch:
		if s.first != 0 {
			t.Fatalf("first sealed chunk starts at line %d, want 0", s.first)
		}
		if s.last < s.first {
			t.Fatalf("invalid seal range [%d,%d]", s.first, s.last)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no OnSeal callback within deadline")
	}
}

func TestMillionLines(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	b := New(Config{})
	defer b.Close()

	var block []byte
	for i := 0; i < 1000; i++ {
		block = append(block, []byte(fmt.Sprintf("line %07d payload\n", i))...)
	}
	// 1000 blocks x 1000 lines, rewriting the counter per block for uniqueness
	for blk := 0; blk < 1000; blk++ {
		b.Append(block)
	}
	if n, _ := b.Len(); n != 1_000_000 {
		t.Fatalf("Len = %d, want 1000000", n)
	}
	it, err := b.Grep(regexp.MustCompile(`^line 0000042 `), GrepOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer it.Close()
	count := 0
	for {
		_, ok := it.Next()
		if !ok {
			break
		}
		count++
	}
	if count != 1000 { // pattern repeats once per block
		t.Fatalf("grep count = %d, want 1000", count)
	}
}

func BenchmarkAppend(b *testing.B) {
	buf := New(Config{MaxBytes: 64 << 20})
	defer buf.Close()

	var payload []byte
	for len(payload) < 64<<10 {
		payload = append(payload, []byte("ip route 10.21.194.0 255.255.255.0 10.21.1.1 name to-rz1-core\n")...)
	}
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Append(payload)
	}
}
