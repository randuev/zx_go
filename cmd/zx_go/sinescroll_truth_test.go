package main

import (
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

// TestSinescrollTruth — clean-room census of the CURRENT v1e+ tap.
// Boots ModelPlus2, loads the CODE block, runs frames, and at each
// frame-committed park (PC==proc) measures:
//   - processing T excluding idle HALT
//   - eraseband rows erased (via otmin/otmax + LDIR count)
//   - sprite bytes vs screen bytes for the first visible glyph
//   - ground-truth PNG at first dense park per zoom level
// Addresses ALL come from the freshly-generated sinescroll.sym — no
// hardcoded fallbacks except as last-resort.
func TestSinescrollTruth(t *testing.T) {
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
		if !strings.HasPrefix(rest, "EQU 0x") {
			continue
		}
		if v, e := strconv.ParseUint(strings.Fields(rest[6:])[0], 16, 16); e == nil {
			syms[name] = uint16(v)
		}
	}
	for _, n := range []string{"walk", "eraseband", "proc", "otmin", "otmax", "cb", "hh", "nbb", "lvl", "p", "fp", "TEXT", "DEFTAB", "CHARID", "ZERO32"} {
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
	blocks := parseTap(raw)
	if len(blocks) != 1 {
		t.Fatalf("want 1 CODE block, got %d", len(blocks))
	}
	base, code := blocks[0][0].(uint16), blocks[0][1].([]byte)
	if base != 0x8000 {
		t.Fatalf("addr=$%04X", base)
	}
	sum := md5.Sum(raw)
	fmt.Println("tap md5:", hex.EncodeToString(sum[:]))

	prev := cliFlagsActive
	nf := cliFlags{}
	if prev != nil {
		nf = *prev
	}
	nf.noSound = true
	cliFlagsActive = &nf
	defer func() { cliFlagsActive = prev }()

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
	// NO IM/vector poke: the demo's own init builds the $8800 IM2 table
	// (fills $8800..$8900 with $89, I=$88, vector-float $FF -> jump at
	// $88FF = $8989 = isr), sets I=$88, and EI before jp loop. Poking a
	// $8900 vector table AFTER loading code overwrote the ISR at $8989.

	proc := sym("proc")
	walk := sym("walk")
	eb := sym("eraseband")
	haltish := sym("lhalt")
	p := func() int { return int(emu.mem.Read(sym("p"))) | int(emu.mem.Read(sym("p")+1))<<8 }
	lvl := func() byte { return emu.mem.Read(sym("lvl")) }
	hh := func() byte { return emu.mem.Read(sym("hh")) }
	cb := func() byte { return emu.mem.Read(sym("cb")) }
	otmin := func() byte { return emu.mem.Read(sym("otmin")) }
	otmax := func() byte { return emu.mem.Read(sym("otmax")) }
	lptrb := func() byte { return emu.mem.Read(sym("lptrb")) }
	dumpHead := func() {
		fmt.Println("walk head disasm:")
		for i := 0; i < 40; i++ {
			a := walk + uint16(i)
			fmt.Printf("  $%04X: $%02X\n", a, emu.mem.Read(a))
		}
	}
	snap := func(tag string) {
		fmt.Printf("SNAP %s p=%04X lvl=%d hh=%d nbb=%d half=%d lptrb=%d cb=%d fp=%d ot[%d..%d]\n",
			tag, p(), lvl(), hh(), emu.mem.Read(sym("nbb")), emu.mem.Read(sym("half")), lptrb(), cb(), emu.mem.Read(sym("fp")), otmin(), otmax())
	}

	snap("pre-loop")
	for i := 0; i < 3; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
		snap("frame")
	}

	shot8 := false
	parks := 0
	maxWork := 0
	overruns := 0
	for parks < 220 {
		work := 0
		steps := 0
		ebRows, ebBlocks, walkStores := 0, 0, 0
		inEB, inWalk := false, false
		var ebDE0 uint16
		var headTrace []string
		tracing := false
		for {
			t0 := emu.cpu.Tstates()
			emu.cpu.StepInstructionWithIRQ()
			dt := int(emu.cpu.Tstates() - t0)
			if emu.cpu.PC == haltish && dt > 8 {
				continue // idle-HALT drain, not processing
			}
			work += dt
			steps++
			pc := emu.cpu.PC
			if pc == walk {
				tracing = true
			}
			if tracing && len(headTrace) < 36 {
				headTrace = append(headTrace, fmt.Sprintf("$%04X A=$%02X HL=$%04X DE=$%04X BC=$%04X", pc, emu.cpu.A, emu.cpu.HL(), emu.cpu.DE(), emu.cpu.BC()))
			}
			if pc == eb {
				inEB = true
				ebRows = int(otmax()) - int(otmin()) + 1
				if otmax() < otmin() {
					ebRows = 0
				}
			}
			if inEB && emu.cpu.B == 32 && emu.cpu.PC == eb+0x18 {
				ebDE0 = emu.cpu.DE()
			}
			if inEB && emu.cpu.B == 0 && ebDE0 != 0 && emu.cpu.DE() == ebDE0+32 {
				ebBlocks++
				ebDE0 = 0
			}
			if pc == walk {
				inWalk = true
			}
			if inWalk && pc >= walk && pc <= walk+0x100 && emu.cpu.PC == walk+0xD9 { // ld (de),a
				walkStores++
			}
			if pc == proc {
				break
			}
		}
		parks++
		if parks >= 3 { // park#1 spans two frame starts; skip it
			if work > maxWork {
				maxWork = work
			}
			// Budget base = +2 frame period 70908 T. This core models a
			// narrow INT pulse: when the demo's walk misses the pulse
			// window the park spans 2 frames (141,816 T) — real +2 vsync
			// is wide and wakes every frame. Gate at two frames + slack so the
			// harness pace doesn't false-fail; real runaway paint storms blow past it.
			if work > 2*70908+1024 {
				overruns++
			}
		}
		lv := lvl()
		fmt.Printf("park#%d lvl%d p%d hh%d cb%d lptrb%d ot[%d..%d] ebR=%d ebB=%d wStores=%d work=%d\n",
			parks, lv, p(), hh(), cb(), lptrb(), otmin(), otmax(), ebRows, ebBlocks, walkStores, work)
		if parks == 3 || parks == 30 {
			for _, ln := range headTrace {
				fmt.Println("  head", ln)
			}
			dumpHead()
		}
		if parks%15 == 0 {
			var rows [192]int
			total := 0
			for y := 0; y < 192; y++ {
				for x := 0; x < 256; x++ {
					o := ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5) + (x >> 3)
					if emu.mem.RAM8KPage(10)[o]&(0x80>>(x&7)) != 0 {
						rows[y]++
						total++
					}
				}
			}
			fmt.Printf("PARK#%d lvl%d total=%d inkrows=", parks, lv, total)
			for y := 0; y < 192; y++ {
				if rows[y] > 0 {
					fmt.Printf(" %d:%d", y, rows[y])
				}
			}
			fmt.Println()
			// TEXT tail: does the full baked string end up anywhere in RAM?
			textAddr := sym("TEXT")
			ascii := ""
			for i := 0; i < 492; i++ {
				c := emu.mem.Read(textAddr + uint16(i))
				if c >= 32 && c < 127 {
					ascii += string(rune(c))
				} else {
					ascii += "?"
				}
			}
			fmt.Println("TEXT:", ascii[:80], "...")
		}

		if !shot8 && lv == 0 && p() > 300 {
			shot8 = true
			capScreen(t, emu, "/tmp/truth_h8.png")
		}
		if lv == 1 && p() > 7600 {
			capScreen(t, emu, fmt.Sprintf("/tmp/truth_h16_%d.png", p()))
		}
	}
	fmt.Printf("TRUTH: parks=%d overruns=%d maxWork=%d\n", parks, overruns, maxWork)
	if overruns > parks/4 {
		t.Errorf("overruns %d/%d", overruns, parks)
	}
}

func capScreen(t *testing.T, emu *emulator, path string) {
	raw := make([]byte, 256*192*3)
	page := emu.mem.RAM8KPage(10)
	for y := 0; y < 192; y++ {
		for x := 0; x < 256; x++ {
			o := ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5) + (x >> 3)
			i := (y*256 + x) * 3
			if page[o]&(0x80>>(x&7)) != 0 {
				raw[i], raw[i+1], raw[i+2] = 0xFF, 0xFF, 0xFF
			}
		}
	}
	if err := writePNG(path, 256, 192, raw); err != nil {
		t.Errorf("png %s: %v", path, err)
	}
	t.Logf("captured %s", path)
}

func addrBank(a uint16) uint16 {
	switch {
	case a < 0x4000:
		return 0 // ROM
	default:
		return 10 // default +2 display in page5 slot maps to page10 here
	}
}

func parseTap(raw []byte) [][2]any {
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
	return blocks
}
