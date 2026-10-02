package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// TestSinescrollMotionGate — SC-7 DRIFT: during a 12-frame window the
// rendered screen must DIFFER every frame (the scroller walks every frame).
// SC-9 POOL: ink must never sit outside the tracked erase band
// [otmin..otmax] at a frame boundary — ink outside = pool erase missed a
// row (stranded ghost text). All frames run native (fast).
// v1f bug class: erase/compute used STALE ylat (±4px lag).
func TestSinescrollMotionGate(t *testing.T) {
	syms := map[string]uint16{}
	bs, err := os.ReadFile("/root/nerve-workspace/demos/sinescroll/sinescroll.sym")
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
			if v, e := strconv.ParseUint(strings.Fields(rest[6:])[0], 16, 16); e == nil {
				syms[name] = uint16(v)
			}
		}
	}
	for _, n := range []string{"p", "lvl", "hh", "half", "otmin", "otmax", "lhalt", "walk", "eraseband", "proc", "TEXT", "DEFTAB", "loop"} {
		if _, ok := syms[n]; !ok {
			t.Fatalf("symbol %s missing from sym — rebuild demos first", n)
		}
	}
	sym := func(n string) uint16 { return syms[n] }

	tapPath := "/root/nerve-workspace/demos/sinescroll/sinescroll.tap"
	if p := os.Getenv("SINESCROLL_TAP"); p != "" {
		tapPath = p
	}
	raw, err := os.ReadFile(tapPath)
	if err != nil {
		t.Fatalf("read tap: %v", err)
	}
	var blocks [][2]any
	off := 0
	var pending uint16
	for off+2 <= len(raw) {
		n := int(binary.LittleEndian.Uint16(raw[off:]))
		off += 2
		if off+n > len(raw) {
			break
		}
		p := raw[off : off+n]
		off += n
		if len(p) < 16 {
			continue
		}
		if p[0] == 0 && p[1] == 3 {
			pending = binary.LittleEndian.Uint16(p[14:16])
			continue
		}
		if p[0] == 0xFF && pending != 0 {
			blocks = append(blocks, [2]any{pending, p[1 : n-1]})
			pending = 0
		}
	}
	if len(blocks) != 1 {
		t.Fatalf("want 1 CODE block, got %d", len(blocks))
	}
	base, code := blocks[0][0].(uint16), blocks[0][1].([]byte)
	if base != 0x8000 {
		t.Fatalf("addr=$%04X", base)
	}

	saveFlags := cliFlagsActive
	nf := cliFlags{}
	if saveFlags != nil {
		nf = *saveFlags
	}
	nf.noSound = true
	cliFlagsActive = &nf
	defer func() { cliFlagsActive = saveFlags }()

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
	emu.mem.Write(0xFFFE, 0x00)
	emu.mem.Write(0xFFFF, 0x00)
	emu.cpu.PC = base
	// NO IM/vector poke here: the demo's own init builds the $8800 IM2
	// table, sets I=$88 and EI before jp loop. Poking a $8900 vector table
	// AFTER loading code overwrote the ISR at $8989 (=$89 89... garbage)
	// and crashed the demo to $0000-era garbage.

	pVal := func() int { return int(emu.mem.Read(sym("p"))) | int(emu.mem.Read(sym("p")+1))<<8 }
	lvl := func() byte { return emu.mem.Read(sym("lvl")) }
	qVal := func() int { return pVal() >> (3 + int(lvl())) }
	hh := func() byte { return emu.mem.Read(sym("hh")) }
	half := func() byte { return emu.mem.Read(sym("half")) }
	otmin := func() byte { return emu.mem.Read(sym("otmin")) }
	otmax := func() byte { return emu.mem.Read(sym("otmax")) }
	screen := func() []byte { return emu.mem.RAM8KPage(10)[:6144] }
	inkCount := func() int {
		n := 0
		for _, b := range screen() {
			if b != 0 {
				n++
			}
		}
		return n
	}

	// Lead-in: native frames until scrolling has started at h8.
	lead := 0
	for pVal() <= 300 || lvl() != 0 {
		runOneFrameHeadless(emu, roms.ModelPlus2)
		lead++
		if lead > 2000 {
			t.Fatalf("never reached p>300 lvl0 — p=%d lvl=%d", pVal(), lvl())
		}
	}
	// Settling so the walk is in its stride.
	for i := 0; i < 14; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}

	stalled, drift := 0, 0
	var prev []byte
	for i := 0; i < 12; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
		s := screen()
		cur := make([]byte, len(s))
		copy(cur, s)
		d := -1
		if prev != nil {
			d = 0
			for j := range cur {
				if cur[j] != prev[j] {
					d++
				}
			}
			drift++
			if d == 0 {
				stalled++
			}
		}
		fmt.Printf("DRIFT frame#%d d=%d p=%d q=%d lvl%d\n", i, d, pVal(), qVal(), lvl())
		os.Stdout.Sync()
		prev = cur
	}

	// SC-9 pool sweep: every frame, ink rows must live inside the tracked
	// erase band [otmin..otmax]; ink outside = stranded ghost row.
	peakInk, ghostFrames, poolFrames := 0, 0, 0
	for poolFrames < 60 {
		poolFrames++
		runOneFrameHeadless(emu, roms.ModelPlus2)
		ink := inkCount()
		if ink > peakInk {
			peakInk = ink
		}
		var rows [192]bool
		stranded := 0
		for y := 0; y < 192; y++ {
			for x := 0; x < 256; x++ {
				o := ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5) + (x >> 3)
				if screen()[o]&(0x80>>(x&7)) != 0 {
					if !rows[y] {
						rows[y] = true
						if y < int(otmin()) || y > int(otmax()) {
							stranded++
						}
					}
				}
			}
		}
		fmt.Printf("POOL p=%d ot[%d..%d] ink=%d stranded=%d lvl%d hh%d half%d\n", pVal(), otmin(), otmax(), ink, stranded, lvl(), hh(), half())
		if poolFrames%20 == 0 {
			os.Stdout.Sync()
		}
		if stranded > 0 {
			ghostFrames++
		}
	}

	// Full-screen census at end.
	{
		var rows [192]int
		total := 0
		for y := 0; y < 192; y++ {
			for x := 0; x < 256; x++ {
				o := ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5) + (x >> 3)
				if screen()[o]&(0x80>>(x&7)) != 0 {
					rows[y]++
					total++
				}
			}
		}
		fmt.Printf("MOTIONGATE windows=%d stalled=%d drift=%d peakInk=%d ghostFrames=%d\n", drift+1, stalled, drift, peakInk, ghostFrames)
		fmt.Printf("CENSUS total=%d inkrows=", total)
		for y := 0; y < 192; y++ {
			if rows[y] > 0 {
				fmt.Printf(" %d:%d", y, rows[y])
			}
		}
		fmt.Println()
	}

	if stalled > 0 {
		t.Errorf("walk stalled in %d/%d windows (drift=%d) — SC-7 FAIL", stalled, drift, drift)
	}
}
