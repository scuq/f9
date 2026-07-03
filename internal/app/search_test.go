package app

import "testing"

func TestGrepTerminal(t *testing.T) {
	a, id, fs := setupConnectedTerminal(t)
	if err := a.OpenTerminal("T1", id, 80, 24); err != nil {
		t.Fatal(err)
	}
	fs.onData([]byte("line one\r\nERROR: disk full\r\nline three\r\nERROR: again\r\n"))

	res, err := a.GrepTerminal("T1", "ERROR", GrepOptsDTO{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != 2 {
		t.Fatalf("count = %d, want 2", res.Count)
	}
	if res.Matches[0].Line != "ERROR: disk full" || res.Matches[0].LineNo != 2 {
		t.Fatalf("match0 = %+v", res.Matches[0])
	}
	if res.Lines < 4 {
		t.Fatalf("lines = %d, want >= 4", res.Lines)
	}

	ctx, err := a.GrepTerminal("T1", "again", GrepOptsDTO{Before: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(ctx.Matches) != 1 || len(ctx.Matches[0].Before) != 1 || ctx.Matches[0].Before[0] != "line three" {
		t.Fatalf("context before = %+v", ctx.Matches)
	}

	if _, err := a.GrepTerminal("T1", "(", GrepOptsDTO{}); err == nil {
		t.Fatal("expected bad-pattern error")
	}
	if _, err := a.GrepTerminal("nope", "x", GrepOptsDTO{}); err == nil {
		t.Fatal("expected terminal-not-open error")
	}
}

func TestTerminalPeek(t *testing.T) {
	a, id, fs := setupConnectedTerminal(t)
	if err := a.OpenTerminal("T1", id, 80, 24); err != nil {
		t.Fatal(err)
	}
	fs.onData([]byte("l0\r\nl1\r\nl2\r\nl3\r\nl4\r\n"))

	p, err := a.TerminalPeek("T1", 2, 1) // around l2, +/-1 -> l1,l2,l3
	if err != nil {
		t.Fatal(err)
	}
	if p.Start != 2 {
		t.Fatalf("start = %d, want 2", p.Start)
	}
	if len(p.Lines) != 3 || p.Lines[0] != "l1" || p.Lines[1] != "l2" || p.Lines[2] != "l3" {
		t.Fatalf("lines = %+v", p.Lines)
	}

	// clamps at the start of the buffer
	edge, err := a.TerminalPeek("T1", 0, 3)
	if err != nil {
		t.Fatal(err)
	}
	if edge.Start != 1 || edge.Lines[0] != "l0" {
		t.Fatalf("edge = %+v", edge)
	}
}
