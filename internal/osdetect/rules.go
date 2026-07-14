package osdetect

import "regexp"

// The scoring table. Weights are relative; each rule fires at most once per
// detector (a prompt repeating 500 times is one piece of evidence, not 500).
// Extend freely — Guess() normalizes, so absolute values only matter relative
// to each other. Rough scale: 5 = conclusive banner, 3-4 = strong idiom or
// vendor version string, 1-2 = shape-level hints shared between families.

type famWeight struct {
	fam Family
	w   float64
}

type rule struct {
	hint    []byte         // fast substring for version/line rules
	re      *regexp.Regexp // prompt rules: matched against the ANSI-stripped tail
	weights []famWeight
	// hostBanner marks motd/banner-style evidence a unix jumphost emits;
	// relay detectors (shell-hop sessions) ignore these so the hop's OS
	// doesn't contaminate the target's detection.
	hostBanner bool
}

// versionRules match the SSH server version string (e.g. "SSH-2.0-Cisco-1.25").
var versionRules = []rule{
	{hint: []byte("Cisco"), weights: []famWeight{{FamilyIOS, 4}}},
	{hint: []byte("Windows"), weights: []famWeight{{FamilyWindows, 4}}},
	{hint: []byte("FreeBSD"), weights: []famWeight{{FamilyJunos, 1}}},
	// OpenSSH runs everywhere — nearly zero signal on its own.
	{hint: []byte("OpenSSH"), weights: []famWeight{{FamilyLinux, 0.5}, {FamilyOpenBSD, 0.5}}},
}

// lineRules match complete output lines: banners, error idioms, pager markers.
var lineRules = []rule{
	{hint: []byte("Cisco Nexus"), weights: []famWeight{{FamilyNXOS, 5}}},
	{hint: []byte("NX-OS"), weights: []famWeight{{FamilyNXOS, 5}}},
	{hint: []byte("Cisco IOS"), weights: []famWeight{{FamilyIOS, 5}}},
	{hint: []byte("% Invalid input detected"), weights: []famWeight{{FamilyIOS, 3}}},
	{hint: []byte("% Invalid command"), weights: []famWeight{{FamilyNXOS, 2}}},
	{hint: []byte("--More--"), weights: []famWeight{{FamilyIOS, 1.5}, {FamilyNXOS, 1.5}}},
	{hint: []byte("PAN-OS"), weights: []famWeight{{FamilyPANOS, 5}}},
	{hint: []byte("Invalid syntax"), weights: []famWeight{{FamilyPANOS, 2}}},
	{hint: []byte("JUNOS"), weights: []famWeight{{FamilyJunos, 5}}},
	{hint: []byte("OpenBSD"), weights: []famWeight{{FamilyOpenBSD, 4}}, hostBanner: true},
	{hint: []byte("Microsoft Windows"), weights: []famWeight{{FamilyWindows, 5}}},
	{hint: []byte("Linux "), weights: []famWeight{{FamilyLinux, 3}}, hostBanner: true},
	{hint: []byte("Debian"), weights: []famWeight{{FamilyLinux, 2}}, hostBanner: true},
	{hint: []byte("Ubuntu"), weights: []famWeight{{FamilyLinux, 2}}, hostBanner: true},
	{hint: []byte("Red Hat Enterprise Linux"), weights: []famWeight{{FamilyLinux, 2}}, hostBanner: true},
}

// promptRules match the tail (the line a prompt sits on, never newline-
// terminated). Shapes overlap between families, hence the small weights.
var promptRules = []rule{
	// hostname(config)# / hostname(config-if)# — Cisco config mode
	{re: regexp.MustCompile(`^[\w.()/-]+\(config[^)]*\)#\s?$`),
		weights: []famWeight{{FamilyIOS, 3}, {FamilyNXOS, 1}}},
	// hostname# / hostname> without a user@ part — device CLI
	{re: regexp.MustCompile(`^[\w./-]+[>#]\s?$`),
		weights: []famWeight{{FamilyIOS, 1}, {FamilyNXOS, 1}}},
	// user@host> / user@host# — PAN-OS and Junos operational/config CLI
	{re: regexp.MustCompile(`^\w[\w.-]*@[\w.-]+[>#]\s?$`),
		weights: []famWeight{{FamilyPANOS, 1.5}, {FamilyJunos, 1.5}}},
	// user@host:~/dir$ — Debian-style shell prompt
	{re: regexp.MustCompile(`^\w[\w.-]*@[\w.-]+:.*[$#]\s?$`),
		weights: []famWeight{{FamilyLinux, 1.5}}},
	// [user@host dir]$ — RHEL-style shell prompt
	{re: regexp.MustCompile(`^\[\w[\w.-]*@[\w.-]+[^\]]*\][$#]\s?$`),
		weights: []famWeight{{FamilyLinux, 1}}},
}
