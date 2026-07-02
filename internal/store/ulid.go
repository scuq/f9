package store

import (
	"crypto/rand"
	"time"
)

const ulidAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// NewULID returns a 26-char Crockford-base32 ULID (48-bit millisecond
// timestamp + 80 random bits): lexicographically sortable by creation time,
// collision-free without coordination (ADR-0006).
func NewULID() string {
	var b [16]byte
	ms := uint64(time.Now().UnixMilli())
	b[0] = byte(ms >> 40)
	b[1] = byte(ms >> 32)
	b[2] = byte(ms >> 24)
	b[3] = byte(ms >> 16)
	b[4] = byte(ms >> 8)
	b[5] = byte(ms)
	if _, err := rand.Read(b[6:]); err != nil {
		panic("store: crypto/rand unavailable: " + err.Error())
	}
	out := make([]byte, 26)
	for i := 0; i < 26; i++ {
		out[i] = ulidAlphabet[fiveBitsAt(b[:], 125-5*i)]
	}
	return string(out)
}

// fiveBitsAt extracts bits [start, start+5) (LSB numbering) of the 128-bit
// big-endian integer stored in b; bit positions >= 128 read as zero, which
// zero-pads the two leading bits of the 26-digit encoding per the ULID spec.
func fiveBitsAt(b []byte, start int) byte {
	var v byte
	for k := 4; k >= 0; k-- {
		bit := start + k
		var val byte
		if bit < 128 {
			val = (b[15-bit/8] >> (bit % 8)) & 1
		}
		v = v<<1 | val
	}
	return v
}
