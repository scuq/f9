package osdetect

import (
	"bytes"
	"testing"
)

func feed(d Detector, chunks ...string) {
	for _, c := range chunks {
		d.ObserveOutput([]byte(c))
	}
}

func TestDetectIOS(t *testing.T) {
	d := New()
	d.ObserveServerVersion("SSH-2.0-Cisco-1.25")
	feed(d,
		"sw1-core#show run | inc snmp\r\n",
		"% Invalid input detected at '^' marker.\r\n",
		" --More-- \r\n",
		"sw1-core#",
	)
	g := d.Guess()
	if g.Family != FamilyIOS {
		t.Fatalf("family = %s, want ios (%+v)", g.Family, g)
	}
	if g.Confidence < 0.7 {
		t.Fatalf("confidence = %.2f, want >= 0.7", g.Confidence)
	}
}

func TestDetectNXOS(t *testing.T) {
	d := New()
	d.ObserveServerVersion("SSH-2.0-OpenSSH_8.3")
	feed(d,
		"Cisco Nexus Operating System (NX-OS) Software\n",
		"nx1-lab# ",
	)
	g := d.Guess()
	if g.Family != FamilyNXOS || g.Confidence < 0.8 {
		t.Fatalf("guess = %+v, want nxos >= 0.8", g)
	}
}

func TestDetectLinux(t *testing.T) {
	d := New()
	d.ObserveServerVersion("SSH-2.0-OpenSSH_9.7 Debian-2")
	feed(d,
		"Linux lyrael 6.12.43+deb13-arm64 #1 SMP Debian 6.12.43-1 aarch64\n",
		"Last login: Fri Apr  5 23:10:57 2024\n",
		"scuq@lyrael:~$ ",
	)
	g := d.Guess()
	if g.Family != FamilyLinux || g.Confidence < 0.8 {
		t.Fatalf("guess = %+v, want linux >= 0.8", g)
	}
}

func TestDetectOpenBSD(t *testing.T) {
	d := New()
	d.ObserveServerVersion("SSH-2.0-OpenSSH_9.7")
	feed(d, "OpenBSD 7.5 (GENERIC.MP) #82: Wed Mar 20 15:48:40 MDT 2024\n")
	g := d.Guess()
	if g.Family != FamilyOpenBSD || g.Confidence < 0.8 {
		t.Fatalf("guess = %+v, want openbsd >= 0.8", g)
	}
}

func TestDetectPANOS(t *testing.T) {
	d := New()
	feed(d,
		"PAN-OS 11.1.4-h7\n",
		"Invalid syntax.\n",
		"admin@PA-3220> ",
	)
	g := d.Guess()
	if g.Family != FamilyPANOS || g.Confidence < 0.7 {
		t.Fatalf("guess = %+v, want panos >= 0.7", g)
	}
}

func TestPromptAcrossChunks(t *testing.T) {
	d := New()
	feed(d, "nx1", "-lab#")
	if g := d.Guess(); g.Family == FamilyUnknown || g.Confidence <= 0 {
		t.Fatalf("split prompt not detected: %+v", g)
	}
}

func TestANSIStrippedPrompt(t *testing.T) {
	d := New()
	feed(d, "\x1b[01;32mscuq@lyrael\x1b[00m:\x1b[01;34m~\x1b[00m$ ")
	if g := d.Guess(); g.Family != FamilyLinux {
		t.Fatalf("colored prompt not detected: %+v", g)
	}
}

func TestRulesFireOnce(t *testing.T) {
	once, twice := New(), New()
	feed(once, "% Invalid input detected at '^' marker.\n")
	feed(twice,
		"% Invalid input detected at '^' marker.\n",
		"% Invalid input detected at '^' marker.\n",
	)
	if a, b := once.Guess(), twice.Guess(); a != b {
		t.Fatalf("repeated evidence changed the guess: %+v vs %+v", a, b)
	}
}

func TestObserveBudget(t *testing.T) {
	d := New()
	junk := bytes.Repeat([]byte("aaaa\n"), 1024) // 5 KiB per feed
	for i := 0; i < 60; i++ {                    // ~300 KiB > budget
		d.ObserveOutput(junk)
	}
	feed(d, "OpenBSD 7.5 (GENERIC.MP)\n")
	if g := d.Guess(); g.Family != FamilyUnknown {
		t.Fatalf("detector observed past its budget: %+v", g)
	}
}

func TestWeakEvidenceLowConfidence(t *testing.T) {
	d := New()
	d.ObserveServerVersion("SSH-2.0-OpenSSH_9.7")
	if g := d.Guess(); g.Confidence >= DefaultThreshold {
		t.Fatalf("OpenSSH alone must stay below threshold: %+v", g)
	}
}
