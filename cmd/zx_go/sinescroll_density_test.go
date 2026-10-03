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

// TestSinescrollDensity — per-frame DENSITY census (Seva's real-+2 photo:
// sparse dot-clusters instead of a continuous scrolling line). Every frame
// the loop parks at lhalt+1 (frame fully committed): log q (via IX), lvl,
// p, lit count, and a 32-char map of which byte columns carry ink. Space
// columns legitimately hold no ink (CHARID=$FF skip) — the census prints
// the visible TEXT window next to the ink map so a reader can see whether
// every non-space glyph landed.
func TestSinescrollDensity(t *testing.T) {
	syms := map[string]uint16{}
	if bs, err := os.ReadFile("/root/nerve-workspace/demos/sinescroll/sinescroll.sym"); err == nil {
		for _, ln := range strings.Split(string(bs), "\n") {
			name, rest, ok := strings.Cut(strings.TrimSpace(ln), ":")
			if !ok {
				continue
			}
			rest = strings.TrimSpace(rest)
			if !strings.HasPrefix(rest, "EQU 0x") {
				continue
			}
			rest = strings.Fields(rest[6:])[0]
			if v, err := strconv.ParseUint(rest, 16, 16); err == nil {
				syms[name] = uint16(v)
			}
		}
	}
	sym := func(n string, fallback uint16) uint16 {
		if v, ok := syms[n]; ok {
			return v
		}
		return fallback
	}
	pAddr := sym("p", 0x8E00)
	lvlAddr := sym("lvl", 0x8E06)
	loopAddr := sym("loop", 0x80DD)
	textAddr := sym("TEXT", 0x9B00)
	ebAddr := sym("eraseband", 0x8258)
	walkAddr := sym("walk", 0x8155)
	lhaltAddr := sym("lhalt", 0x80E2) // CPU parks at halt+1 while awaiting vsync

	prev := cliFlagsActive
	nf := cliFlags{}
	if prev != nil {
		nf = *prev
	}
	nf.noSound = true
	cliFlagsActive = &nf
	defer func() { cliFlagsActive = prev }()

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
		payload := raw[off : off+n]
		off += n
		if len(payload) < 16 {
			continue
		}
		if payload[0] == 0 && payload[1] == 3 {
			pending = binary.LittleEndian.Uint16(payload[14:16])
			continue
		}
		if payload[0] == 0xFF && pending != 0 {
			blocks = append(blocks, [2]any{pending, payload[1 : n-1]})
			pending = 0
		}
	}
	if len(blocks) != 1 {
		t.Fatalf("want 1 CODE block, got %d", len(blocks))
	}
	addr := blocks[0][0].(uint16)
	data := blocks[0][1].([]byte)

	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	emu.paused.Store(false)
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	for i, v := range data {
		emu.mem.Write(addr+uint16(i), v)
	}
	emu.cpu.SP = 0xFF00
	emu.mem.Write(0xFFFE, 0x00)
	emu.mem.Write(0xFFFF, 0x00)
	emu.cpu.PC = 0x8000
	emu.cpu.IFF1, emu.cpu.IFF2 = false, false
	emu.cpu.IM = 1

	pVal := func(e *emulator) int { return int(e.mem.Read(pAddr)) | int(e.mem.Read(pAddr+1))<<8 }
	// TEXT window as printed by the bake (raw ASCII from tap payload).
	textAt := func(a uint16) byte { return data[a-0x8000] }

	text := make([]byte, 492)
	for i := range text {
		text[i] = textAt(textAddr + uint16(i))
	}
	// CHARID map from bake: predicts which byte columns hold ink.
	charid := make([]byte, 256)
	copy(charid, data[0x9A80-0x8000:])

	parks := 0
	lvlDbg := 0
	shotH8, shotH16, lvl1Run := 0, 0, 0
	// v1e census: step every instruction. wall = wake-to-wake period incl.
	// idle-halt. work = PROCESSING T only — dt excluded while PC rests at
	// lhalt+1 (that is vsync-idle, not our cost). overrun = work > 69888.
	wall := 0
	work := 0
	overruns := 0
	maxWork := 0
	for i := 0; i < 900; i++ {
		bracket := parks == 10 || parks == 30
		var ebT, wkT int
		var ebSp, wkSp, ebDE0 uint16
		ebBlocks := 0
		stepPh := 0 // 0=idle,1=in eraseband,2=in walk
		steps := 0
		for {
			tBefore := emu.cpu.Tstates()
			emu.cpu.StepInstructionWithIRQ()
			dt := int(emu.cpu.Tstates() - tBefore)
			wall += dt
			if emu.cpu.PC != lhaltAddr+1 && !(emu.cpu.PC == lhaltAddr && dt > 8) {
				work += dt // exclude HALT-idle: 4T quantum steps AND big drain
			}
			steps++
			if bracket {
				switch stepPh {
				case 1: // inside eraseband: detect LDIR blocks (B 32->0, DE +32)
					ebT += dt
					if emu.cpu.B == 0 && ebDE0 != 0 && emu.cpu.DE() == ebDE0+32 {
						ebBlocks++
					}
					if emu.cpu.B == 32 {
						ebDE0 = emu.cpu.DE()
					}
					if emu.cpu.SP == ebSp && steps > 1 {
						stepPh = 0
					}
				case 2:
					wkT += dt
					if emu.cpu.SP == wkSp {
						stepPh = 0
					}
				default:
					if emu.cpu.PC == ebAddr {
						stepPh = 1
						ebSp = emu.cpu.SP
						ebT = 0
						ebDE0 = 0
					} else if emu.cpu.PC == walkAddr {
						stepPh = 2
						wkSp = emu.cpu.SP
						wkT = 0
					}
				}
			}
			if emu.cpu.PC == loopAddr {
				break
			}
		}
		if bracket {
			t.Logf("BRACKET park#%d lvl%d eraseband=%d T blocks=%d walk=%d T band=[%d..%d]",
				parks, emu.mem.Read(lvlAddr), ebT, ebBlocks, wkT,
				emu.mem.Read(sym("otmin", 0x8E18)), emu.mem.Read(sym("otmax", 0x8E19)))
		}
		// v2.5 budget law: a repaint park (erase+walk+bandsave of the HIDDEN
		// bank) legitimately spans >1 frame; cap at 230000T (measured legit
		// max paint span 160166T + slack). The old 69888 gate was v1-era.
		if work > 230000 {
			overruns++
			t.Logf("OVERRUN park#%d lvl%d wall=%d work=%d (>230000)", parks, emu.mem.Read(lvlAddr), wall, work)
		}
		if work > maxWork {
			maxWork = work
		}
		parks++
		span := work // report work as T=
		wall = 0
		work = 0
		lvl := emu.mem.Read(lvlAddr)
		// q = TEXT glyph-unit index of the left-most screen column.
		// p is pixel position; byte column cb marks where drawing sits.
		q := int(pVal(emu)) >> (3 + lvl)
		if q >= 492 {
			q -= 492
		}
		// glyph-unit prediction at the current pixel pitch
		gw := uint32(8)
		if lvl == 1 {
			gw = 16
		}
		ng := int(256/gw) + 2
		pred := 0
		for k := 0; k < ng; k++ {
			if uint32(q)+uint32(k) < 492 && charid[text[(uint32(q)+uint32(k))%492]] != 0xFF {
				pred++
			}
		}
		// actual distinct pixel-cluster start positions (bank-aware DISPLAY)
		dispPage := 10
		if emu.mem.Read(sym("bbk", 0x8E22))&0x08 != 0 {
			dispPage = 14
		}
		starts := []int{}
		run := -1
		for c := 0; c < 256; c++ {
			xb := c >> 3
			ink := false
			for y := 0; y < 192 && !ink; y++ {
				o := ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5) + xb
				if emu.mem.RAM8KPage(dispPage)[o] != 0 {
					ink = true
				}
			}
			if ink && run < 0 {
				run = c
			} else if !ink && run >= 0 {
				starts = append(starts, run)
				run = -1
			}
		}
		if run >= 0 {
			starts = append(starts, run)
		}
		s := ""
		for _, v := range starts {
			s += fmt.Sprintf("%d ", v)
		}
		q32 := uint32(q) % 492
		vis := make([]byte, ng)
		for k := range vis {
			vis[k] = '.'
			if q32+uint32(k) < 492 {
				vis[k] = text[q32+uint32(k)]
			}
		}
		if (pVal(emu)&7) == 0 || lvl == 1 {
			t.Logf("p%04d lvl%d q%03d pred=%2d clu=%2d T=%6d | %s | %s", pVal(emu), lvl, q&0xFFFF, pred, len(starts), span, s, vis)
		}
		// ground-truth PNGs at first dense parks per level (parked = coherent)
		if lvl == 1 {
			lvl1Run++
		} else {
			lvl1Run = 0
		}
		{
			want := ""
			if lvl == 0 && pred >= 12 && shotH8 == 0 {
				shotH8 = 1
				want = "/tmp/sine_final_h8.png"
			}
			if lvl == 1 && lvl1Run >= 6 && shotH16 == 0 {
				shotH16 = 1
				want = "/tmp/sine_final_h16.png"
			}
			if want != "" {
				raw := make([]byte, 256*192*3)
				for y := 0; y < 192; y++ {
					for x := 0; x < 256; x++ {
						o := ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5) + (x >> 3)
						v := emu.mem.RAM8KPage(dispPage)[o] & (0x80 >> (x & 7))
						i := (y*256 + x) * 3
						if v != 0 {
							raw[i], raw[i+1], raw[i+2] = 0xFF, 0xFF, 0xFF
						} else {
							raw[i], raw[i+1], raw[i+2] = 0, 0, 0
						}
					}
				}
				if err := writePNG(want, 256, 192, raw); err != nil {
					t.Errorf("png %s: %v", want, err)
				} else {
					t.Logf("PNG %s p%d lvl%d pred=%d", want, pVal(emu), lvl, pred)
				}
			}
		}
		if lvl == 1 && lvlDbg < 6 {
			lvlDbg++
			b := strings.Builder{}
			for y := 0; y < 192; y++ {
				for x := 0; x < 256; x++ {
					o := ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5) + (x >> 3)
					if emu.mem.RAM8KPage(dispPage)[o]&(0x80>>(x&7)) != 0 {
						b.WriteString("#")
					} else {
						b.WriteString(".")
					}
				}
				b.WriteString("\n")
			}
			t.Logf("H16 PIXELS p%04d q%03d\n%s", pVal(emu), q&0xFFFF, b.String())
		}
	}
	fmt.Println("parks:", parks)
}
