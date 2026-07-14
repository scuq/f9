package app

import (
	"fmt"
	"strings"
)

// ConnInfoDTO is the transport + route summary shown in the terminal status bar.
type ConnInfoDTO struct {
	ServerVersion string `json:"serverVersion"`
	KeyExchange   string `json:"keyExchange"`
	HostKey       string `json:"hostKey"`
	CipherIn      string `json:"cipherIn"`
	CipherOut     string `json:"cipherOut"`
	MACIn         string `json:"macIn"`
	MACOut        string `json:"macOut"`
	Relay         bool   `json:"relay"` // shell-hop: crypto describes the hop leg
	Host          string `json:"host"`
	Port          int    `json:"port"`
	OnwardUser    string `json:"onwardUser"`
	Chain         string `json:"chain"`
	SocksPort     int    `json:"socksPort"`
}

// ConnInfo returns transport and route details for a connected session.
func (a *App) ConnInfo(sessionID string) (ConnInfoDTO, error) {
	client, ok := a.mgr.Client(sessionID)
	if !ok {
		return ConnInfoDTO{}, fmt.Errorf("app: session not connected")
	}
	ci := client.ConnInfo()
	out := ConnInfoDTO{
		ServerVersion: ci.ServerVersion,
		KeyExchange:   ci.KeyExchange,
		HostKey:       ci.HostKey,
		CipherIn:      ci.CipherIn,
		CipherOut:     ci.CipherOut,
		MACIn:         ci.MACIn,
		MACOut:        ci.MACOut,
		Relay:         ci.Relay,
	}
	s, eff, err := a.st.Resolve(sessionID)
	if err != nil {
		return out, nil
	}
	user, chain, _ := resolveAltRefs(a.Settings().AltUsers, s.User, eff.JumpChain)
	out.Host = s.Host
	out.Port = s.Port
	out.OnwardUser = resolveTargetUser(user, chain)
	segs := make([]string, 0, len(chain))
	for _, h := range chain {
		seg := h.Host
		if h.User != "" {
			seg = h.User + "@" + seg
		}
		segs = append(segs, seg+" ["+h.Mode+"]")
	}
	out.Chain = strings.Join(segs, " \u2192 ")
	if eff.SocksPort != nil {
		out.SocksPort = *eff.SocksPort
	}
	return out, nil
}
