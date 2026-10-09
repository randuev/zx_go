package main

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func loadBalatroSyms(t *testing.T) map[string]uint16 {
	t.Helper()
	syms := map[string]uint16{}
	bs, err := os.ReadFile("/root/nerve-workspace/demos/balatro/balatro.sym")
	if err != nil {
		t.Fatalf("read sym: %v", err)
	}
	for _, ln := range strings.Split(string(bs), "\n") {
		name, rest, ok := strings.Cut(strings.TrimSpace(ln), ":")
		if !ok {
			continue
		}
		rest = strings.TrimSpace(rest)
		if strings.HasPrefix(rest, "EQU 0x") {
			if v, e := strconv.ParseUint(rest[6:], 16, 16); e == nil {
				syms[name] = uint16(v)
			}
		}
	}
	for _, n := range []string{"start", "doQuit", "tryPlay", "tryDiscard", "computeScore", "finalize", "MODE", "CURSOR", "NSEL", "SELMARK", "HAND", "HANDS", "DISCS", "SEED", "DECKPOS", "CHIPS", "TOT", "WINF", "HTIDX", "MULTBIN", "NBWON", "JOKSLOTS", "ANTE", "BLINDIDX", "SCOREV", "SCPH"} {
		if _, ok := syms[n]; !ok {
			t.Fatalf("symbol %s missing", n)
		}
	}
	return syms
}

// bootBalatro boots the +2 core and loads the code block direct to $8000
// (established harness recipe — headless tape trap is stochastic).
func bootBalatro(t *testing.T, syms map[string]uint16) *emulator {
	t.Helper()
	raw, err := os.ReadFile("/root/nerve-workspace/demos/balatro/balatro.tap")
	if err != nil {
		t.Fatalf("read tap: %v", err)
	}
	blocks := parseTap(raw)
	var base uint16
	var code []byte
	for _, b := range blocks {
		if d, ok := b[1].([]byte); ok && len(d) > 4000 {
			base = b[0].(uint16)
			code = d
		}
	}
	if code == nil {
		t.Fatal("no code block")
	}
	prev := cliFlagsActive
	nf := cliFlags{}
	nf.noSound = true
	cliFlagsActive = &nf
	t.Cleanup(func() { cliFlagsActive = prev })
	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	emu.paused.Store(false)
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	for i, v := range code {
		emu.mem.Write(base+uint16(i), v)
	}
	emu.cpu.SP = 0xFF00
	// sentinel return: RET addr into a halt pad (emulates RANDOMIZE USR's
	// BASIC return address on the stack)
	emu.mem.Write(0xBF00, 0x76)
	emu.mem.Write(0xFF00, 0x00) // SP-top return sentinel (low)
	emu.mem.Write(0xFF01, 0xBF) // high
	emu.cpu.PC = base
	return emu
}

// balatroKeys: zx_go matrix coordinates for the game keys.
var balatroKeys = map[string][2]uint8{
	"ENTER": {6, 0x01},
	"SPACE": {7, 0x01},
	"D":     {1, 0x04},
	"4":     {3, 0x08},
	"6":     {4, 0x10},
	"Q":     {2, 0x01},
}

func balPress(t *testing.T, emu *emulator, syms map[string]uint16, name string) {
	t.Helper()
	// full handshake (press -> wait arm -> release -> wait clear): the deal
	// animation busy-loops past fixed step budgets and stale REL arming would
	// silently swallow the next stroke.
	pressOnce(t, emu, syms, name)
}


func balRunFrames(emu *emulator, n int) {
	for i := 0; i < n; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
}

func balPeek(emu *emulator, a uint16) byte { return emu.mem.Read(a) }

func balPeek16(emu *emulator, a uint16) uint16 {
	return uint16(emu.mem.Read(a)) | uint16(emu.mem.Read(a+1))<<8
}

// balBCD3 reads a big-endian 3-byte BCD value (6 digits, packed pairs) at addr.
func balBCD3(emu *emulator, a uint16) int {
	b0, b1, b2 := int(emu.mem.Read(a)), int(emu.mem.Read(a+1)), int(emu.mem.Read(a+2))
	return (b0>>4)*100000 + (b0&15)*10000 +
		(b1>>4)*1000 + (b1&15)*100 +
		(b2>>4)*10 + b2&15
}

// balTextLit counts non-zero glyph bytes in one text char row (row 0..23).
func balTextLit(emu *emulator, row int) int {
	lit := 0
	base := uint16(0x4000 + (row>>3)*0x800 + (row&7)*0x20)   // third stride $800, pixel-row $100 within third => char row
	for x := 0; x < 32; x++ {
		if emu.mem.Read(base+uint16(x)) != 0 {
			lit++
		}
	}
	return lit
}

// rnd16 replays the xorshift16 SEED step; returns low byte.
func balRnd(seed *uint16) byte {
	x := *seed
	x ^= x << 7
	x ^= x >> 9
	x ^= x << 8
	*seed = x
	return byte(x)
}

// balShuffleReplay replays Fisher-Yates k=51..1 exactly as shuffle:.
func balShuffleReplay(seed uint16) []byte {
	deck := make([]byte, 52)
	for i := range deck {
		deck[i] = byte(i)
	}
	// Z80 loop: b=51..1 ; IDXK=52-b (ascending 1..51) ; j=rnd%(k+1)
	for b := 51; b >= 1; b-- {
		r := balRnd(&seed)
		m := byte(53 - b)
		for r >= m {
			r -= m
		}
		k := byte(52 - b)
		deck[r], deck[k] = deck[k], deck[r]
	}
	return deck
}
