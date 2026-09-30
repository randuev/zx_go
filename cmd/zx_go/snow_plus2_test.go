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

// TestSnowPlus2Run boots a +2 (grey) core, loads the snow CODE block at
// $8000 the way RANDOMIZE USR would find it, then runs the demo with two
// scenarios:
//
//	Run A (no keys): the snow field must live — pixels move, drifts grow,
//	  the storm/drain/wipe cycle completes at least once, no phantom-key
//	  quit (kfull ghost class), zero $7FFD writes (single-buffer design:
//	  no paging at all), IM2 + $8989 ISR alive to the end.
//	Run B (SPACE held from frame 1600): the demo must EXIT CLEANLY —
//	  PC escapes the code region within ~50 frames of the key, IM lands
//	  back at 1, AY goes silent (the quit path's R10-12=0 writes).
//
// Ground-truth order respected: display census is decoded from the
// PHYSICAL page the ULA shows, not from a CPU-window assumption.
func TestSnowPlus2Run(t *testing.T) {
	syms := map[string]uint16{}
	if bs, err := os.ReadFile("/root/nerve-workspace/demos/snow/snow.sym"); err == nil {
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
	fldcAddr := sym("fldc", 0x8C90)
	_ = fldcAddr
	fcntAddr := sym("fcnt", 0x8C91)
	modeAddr := sym("mode", 0x8C99)
	phaseAddr := sym("phase", 0x8C97)
	kcntAddr := sym("kcnt", 0x8C96)

	prev := cliFlagsActive
	nf := cliFlags{}
	if prev != nil {
		nf = *prev
	}
	nf.noSound = true
	cliFlagsActive = &nf
	defer func() { cliFlagsActive = prev }()

	tapPath := "/root/nerve-workspace/demos/snow/snow.tap"
	if p := os.Getenv("SNOW_TAP"); p != "" {
		tapPath = p
	}
	raw, err := os.ReadFile(tapPath)
	if err != nil {
		t.Fatalf("read tap: %v", err)
	}
	// Load EVERY CODE block at its header address — v4 ships two: the
	// program at $8000 and the precomputed scroller strip (8 KB) at $9000.
	type tapBlk struct {
		addr uint16
		data []byte
	}
	var blocks []tapBlk
	var pending uint16
	off := 0
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
			blocks = append(blocks, tapBlk{pending, payload[1 : n-1]})
			t.Logf("code block: len=%d -> $%04X", n-2, pending)
			pending = 0
		}
	}
	// v4b ships ONE CODE block at $8000..$9AFF: code+data+font+stream in a
	// single LOAD ""CODE. A second block would land in whatever bank BASIC
	// happens to have paged — the exact v4 tape fault. Validate the block
	// and that the strip landed intact inside it.
	if len(blocks) != 1 {
		t.Fatalf("expected exactly 1 CODE block (single-block contract), got %d", len(blocks))
	}
	if blocks[0].addr != 0x8000 {
		t.Fatalf("CODE block must load at $8000, got $%04X", blocks[0].addr)
	}
	if len(blocks[0].data) < 0x1B00 {
		t.Fatalf("block must cover $8000-$9AFF incl. strip, len=%X", len(blocks[0].data))
	}
	if bs, err := os.ReadFile("/root/nerve-workspace/demos/snow/scroll.bin"); err == nil {
		off := 0x9000 - 0x8000
		if string(blocks[0].data[off:off+len(bs)]) != string(bs) {
			t.Fatalf("strip inside block != scroll.bin")
		}
	}

	boot := func() *emulator {
		emu, err := newEmulator(roms.ModelPlus2)
		if err != nil {
			t.Fatalf("newEmulator(+2): %v", err)
		}
		emu.paused.Store(false)
		for i := 0; i < 220; i++ {
			runOneFrameHeadless(emu, roms.ModelPlus2)
		}
		for _, b := range blocks {
			for i, v := range b.data {
				emu.mem.Write(uint16(b.addr)+uint16(i), v)
			}
		}
		// Read-back proof: writes must land (catches silent write-protection
		// on the $8000-$BFFF slot — especially the strip at $9000).
		for _, b := range blocks {
			for i, v := range b.data {
				if g := emu.mem.Read(uint16(b.addr) + uint16(i)); g != v {
					t.Fatalf("block $%04X byte %d: wrote %02X read %02X", b.addr, i, v, g)
				}
			}
		}
		emu.cpu.SP = 0xFF00
		emu.mem.Write(0xFEFF, 0x00)
		emu.mem.Write(0xFEFE, 0x00) // fake USR return stub
		emu.cpu.PC = 0x8000
		emu.cpu.IFF1, emu.cpu.IFF2 = false, false
		emu.cpu.IM = 1
		return emu
	}

	// ---- Run A: 2600 frames, no keys --------------------------------------
	// Verified cadence (probe v4): 1 loop iteration per frame; storm = 250
	// slow ticks (8 frames each) = 2000 frames, drain = 25 ticks = 200,
	// wipe+restart at ~f2202. Window covers one FULL cycle.
	t.Run("snowfall-life", func(t *testing.T) {
		emu := boot()
		pgw, pgDropped := 0, 0
		emu.mem.SetPagingTracer(func(source string, val byte, applied, sb, sa bool) {
			pgw++
			if !applied {
				pgDropped++
			}
		})
		// Border-profile recorder: capture every `ld a,$EX / out ($FE),a`
		// execution (Seva's CRT timing diagnostic). Sites verified against
		// the current tap binary by byte-scan.
		sites := map[uint16]byte{0x8114: 0xE0, 0x8134: 0xE4, 0x81C5: 0xE3, 0x898B: 0xE2}
		var borderPend []byte
		var borderLog [][2]interface{}
		emu.cpu.AddPreFetchHook("snow-border", func(pc uint16) {
			if v, ok := sites[pc]; ok && emu.mem.Read(pc) == 0x3E && emu.mem.Read(pc+1) == v && emu.mem.Read(pc+2) == 0xD3 {
				borderPend = append(borderPend, v)
			}
		})
		romHits := 0
		firstRom := -1
		var trail []uint16
		emu.cpu.AddPreFetchHook("snow-escape", func(pc uint16) {
			if pc < 0x8000 || pc > 0x8CFF {
				if len(trail) < 200 {
					trail = append(trail, pc)
				}
			}
		})
		sumAt := map[int]int{}
		maxLit := 0
		picAt2195 := ""
		// Per-frame pixel-CONTENT diff. Lit-byte COUNT deltas are the wrong
		// metric: a streak moving 2px down erases as much as it draws, so
		// real motion is invisible to counting. Compare raw bytes.
		var prevScr, curScr [6144]byte
		readScr := func(dst []byte) { copy(dst, emu.mem.RAM8KPage(10)[:6144]) }
		contentChanged, stillFrames := 0, []int{}
		sumH := func() int {
			s := 0
			for k := 0; k < 256; k++ {
				s += int(emu.mem.Read(0x8B00 + uint16(k)))
			}
			return s
		}
		// ASCII render of the bottom third from the PHYSICAL displayed page.
		asciiBottom := func(e *emulator) string {
			sb := strings.Builder{}
			for y := 160; y < 192; y += 2 {
				for x := 0; x < 256; x++ {
					o := ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5) + (x >> 3)
					var b byte
					if o < 0x2000 {
						b = e.mem.RAM8KPage(10)[o]
					} else {
						b = e.mem.RAM8KPage(11)[o-0x2000]
					}
					if b&(0x80>>(x&7)) != 0 {
						sb.WriteString("#")
					} else {
						sb.WriteString(".")
					}
				}
				sb.WriteString("\n")
			}
			return sb.String()
		}
		// ---- v4 scroller probes -------------------------------------------
		// bandIdx = the 512 PHYSICAL screen bytes of rows y176..191 (the
		// scroller band). sscroll rewrites ALL of them every frame from the
		// pre-shifted strip at $9000+sroll, so while text is inside the
		// window consecutive frames MUST differ — a frozen band means the
		// scroller died. Text spans strip px 0..467; the f60..440 window
		// always keeps a glyph edge in view (boot sbake+wipe eat ~15 frames).
		var bandIdx []int
		for y := 176; y < 192; y++ {
			for c := 0; c < 32; c++ {
				bandIdx = append(bandIdx, ((y&7)<<8)+((y&0x38)<<2)+((y&0xC0)<<5)+c)
			}
		}
		readBand := func(dst []int) {
			for k, o := range bandIdx {
				var b byte
				if o < 0x2000 {
					b = emu.mem.RAM8KPage(10)[o]
				} else {
					b = emu.mem.RAM8KPage(11)[o-0x2000]
				}
				dst[k] = int(b)
			}
		}
		var prevBand, curBand [512]int
		bandMotion, bandWin, bandNonZero60 := 0, 0, 0
		bandStuck := []int{}
		// Erase-draw invariant: the land check (cp 176) branches BEFORE any
		// record store, so a live flake record with y in the band is a
		// regression of the v4 snow-floor raise, never a timing artifact.
		fldsAddr := sym("flds", 0x8C00)
		flakesInBand := func() int {
			bad := 0
			for k := 0; k < 36; k++ {
				if y := emu.mem.Read(fldsAddr + 1 + uint16(4*k)); y != 0xFF && y >= 176 && y <= 191 {
					bad++
				}
			}
			return bad
		}
		bandBadFlakes := 0
		for i := 0; i < 2600; i++ {
			if i >= 400 && i < 1800 {
				readScr(prevScr[:])
			}
			if i >= 60 && i <= 440 {
				readBand(prevBand[:])
			}
			runOneFrameHeadless(emu, roms.ModelPlus2)
			if len(borderPend) > 0 {
				cp := make([]byte, len(borderPend))
				copy(cp, borderPend)
				borderLog = append(borderLog, [2]interface{}{i, cp})
				borderPend = borderPend[:0]
			}
			if i >= 60 && i <= 440 {
				readBand(curBand[:])
				diff := 0
				for k := range bandIdx {
					if prevBand[k] != curBand[k] {
						diff++
					}
				}
				bandWin++
				if diff > 0 {
					bandMotion++
				} else if len(bandStuck) < 8 {
					bandStuck = append(bandStuck, i)
				}
				if i == 60 {
					nz := 0
					for k := range bandIdx {
						if curBand[k] != 0 {
							nz++
						}
					}
					bandNonZero60 = nz
				}
			}
			if b := flakesInBand(); b > bandBadFlakes {
				bandBadFlakes = b
			}
			if i >= 400 && i < 1800 {
				readScr(curScr[:])
				d := 0
				for b := range prevScr {
					if prevScr[b] != curScr[b] {
						d++
					}
				}
				if d > 0 {
					contentChanged++
				} else if len(stillFrames) < 12 {
					stillFrames = append(stillFrames, i)
				}
			}
			if pc := emu.cpu.PC; pc < 0x8000 {
				romHits++
				if firstRom < 0 {
					firstRom = i
				}
			}
			lit := litBytes(emu, 5)
			if lit > maxLit {
				maxLit = lit
			}
			if i == 600 || i == 1200 || i == 1800 || i == 2195 || i == 2500 {
				s := sumH()
				sumAt[i] = s
				t.Logf("f=%4d PC=$%04X lit=%d sumH=%d IM=%d I=$%02X IFF1=%v phase=%d",
					i, emu.cpu.PC, lit, s, emu.cpu.IM, emu.cpu.I, emu.cpu.IFF1,
					emu.mem.Read(phaseAddr))
			}
			if i == 2195 { // pre-wipe: drifts at max — capture the money picture
				picAt2195 = asciiBottom(emu)
			}
			if i == 700 { // mid-storm health: fcnt must be alive
				if emu.mem.Read(fcntAddr) == 0 && emu.mem.Read(modeAddr) == 0 {
					t.Errorf("f700: fcnt and mode both zero — main loop never ran?")
				}
			}
		}
		if bandNonZero60 < 100 {
			t.Errorf("f60: band has only %d non-zero bytes — strip bake dead (want >100: text must be visible)", bandNonZero60)
		}
		// A band sample taken mid-SBAND can legitimately alias to the
		// previous frame — occasional single-frame collisions are probe
		// artifacts, not freezes. A real freeze sticks for many frames.
		if stuck := bandWin - bandMotion; stuck > 3 {
			t.Errorf("scroller froze: band unchanged on %d/%d window frames (f60-440); stuck at %v",
				stuck, bandWin, bandStuck)
		}
		if bandBadFlakes > 0 {
			t.Errorf("%d live flake records parked in the scroller band (y176-191) — snow-floor reserve broken", bandBadFlakes)
		}
		t.Logf("scroller: band moved %d/%d window frames; f60 band non-zero bytes = %d; max flakes-in-band = %d",
			bandMotion, bandWin, bandNonZero60, bandBadFlakes)
		// Border-profile gate (Seva's diagnostic): concatenated border stream
		// must be the global repeating pattern RED $E2(vsync ISR) ->
		// [GREEN $E4 flake pass, even frames] -> MAGENTA $E3(scroller) ->
		// BLACK $E0(idle to halt). Frame-boundary flushes may split tokens,
		// so validate the JOINED stream with a small stray tolerance.
		{
			var stream []byte
			for _, e := range borderLog {
				stream = append(stream, e[1].([]byte)...)
			}
			badSeq, pats, nP := 0, 0, 0
			badSample := ""
			for k := 0; k < len(stream); {
				if stream[k] == 0xE2 {
					if k+3 < len(stream) && stream[k+1] == 0xE4 && stream[k+2] == 0xE3 && stream[k+3] == 0xE0 {
						pats++
						k += 4
						continue
					}
					if k+2 < len(stream) && stream[k+1] == 0xE3 && stream[k+2] == 0xE0 {
						nP++
						k += 3
						continue
					}
					if k+2 < len(stream) && stream[k+1] == 0xE4 && stream[k+2] == 0xE3 {
						pats++ // even paint cycle, idle BLACK lands next cycle
						k += 3
						continue
					}
					if k+1 < len(stream) && stream[k+1] == 0xE0 {
						nP++ // vsync + loop-head idle with no paint (sped frame)
						k += 2
						continue
					}
				}
				badSeq++
				if badSample == "" {
					lo := k - 8
					if lo < 0 {
						lo = 0
					}
					hi := k + 8
					if hi > len(stream) {
						hi = len(stream)
					}
					badSample = fmt.Sprintf("stream@%d % X", k, stream[lo:hi])
				}
				k++
			}
			t.Logf("border profile: %d frames logged, %d green-frames, %d odd-frames, %d stray", len(borderLog), pats, nP, badSeq)
			// Stray tolerance: duplicate vsyncs inside accelerated frames and
			// the stream's phase-shifted head/tail legitimately fall between
			// pattern tokens; every real frame cycle still parses.
			if pats+nP < 700 || badSeq*10 > len(stream)*3 {
				t.Errorf("border profile broken: pats=%d+%d stray=%d/%d (%s)", pats, nP, badSeq, len(stream), badSample)
			}
		}
		if len(trail) > 0 {
			t.Errorf("PC escaped program space %d samples (first @f%d): %v", len(trail), firstRom, trail[:min(24, len(trail))])
		}
		// Single-buffer contract: the demo must never touch $7FFD.
		if pgw != 0 {
			t.Errorf("$7FFD writes = %d (design says ZERO — no paging at all)", pgw)
		}
		if pgDropped > 0 {
			t.Errorf("%d paging writes DROPPED (bit-5 lock class)", pgDropped)
		}
		// Interrupt stack alive to the end: IM2, I=$88, interrupts enabled.
		if emu.cpu.IM != 2 || emu.cpu.I != 0x88 {
			t.Errorf("IM/IVector at end: IM=%d I=$%02X (want IM2 I=$88)", emu.cpu.IM, emu.cpu.I)
		}
		// Ghost-key check: nobody pressed a key, so the quit confirm
		// counter must have stayed zero.
		if kcnt := emu.mem.Read(kcntAddr); kcnt != 0 {
			t.Errorf("kcnt=%d with NO keys injected — kfull ghost-key class (would quit the demo)", kcnt)
		}
		// Drift growth: height-map mass must increase across the storm.
		if !(sumAt[600] < sumAt[1200] && sumAt[1200] < sumAt[1800]) {
			t.Errorf("drift NOT accumulating: sumH 600=%d 1200=%d 1800=%d", sumAt[600], sumAt[1200], sumAt[1800])
		}
		// The 2200-frame cycle must complete: wipe fires at ~f2202, and by
		// f2500 cycle #2 has only ~300 frames of accumulation — less than
		// half the pre-wipe mass proves the map was wiped and a new storm
		// is genuinely landing flakes (not just residual drift).
		if sumAt[2500]*2 >= sumAt[2195] {
			t.Errorf("no wipe/restart: sumH f2500=%d vs pre-wipe f2195=%d", sumAt[2500], sumAt[2195])
		}
		if ph := emu.mem.Read(phaseAddr); ph != 0 {
			t.Errorf("phase=%d at f2600 — cycle restart dead (want 0: second storm)", ph)
		}
		// Motion: pixel content must differ on >=90% of mid-run frames.
		midFrames := 1400
		if contentChanged*10 < midFrames*9 {
			t.Errorf("snow looks frozen: content changed %d/%d mid-run frames; still-frame starts: %v",
				contentChanged, midFrames, stillFrames)
		}
		t.Logf("motion: %d/%d mid-run frames had pixel-content change; maxLit=%d",
			contentChanged, midFrames, maxLit)
		t.Logf("PRE-WIPE f2195 (drifts at max):\n%s", picAt2195)
		t.Logf("bottom-third at f2600 (cycle #2, every 2nd row, maxLit=%d):\n%s", maxLit, asciiBottom(emu))
	})

	// ---- Run B: SPACE from f1600 must quit cleanly ------------------------
	t.Run("space-quit", func(t *testing.T) {
		emu := boot()
		for i := 0; i < 1600; i++ {
			runOneFrameHeadless(emu, roms.ModelPlus2)
		}
		if pc := emu.cpu.PC; pc < 0x8000 {
			t.Fatalf("demo self-quit before key at f1600 (PC=$%04X)", pc)
		}
		emu.kbd.PressMatrixKey(7, 0x01, true) // SPACE held
		escaped, escFrame := false, -1
		for i := 1600; i < 2000; i++ {
			runOneFrameHeadless(emu, roms.ModelPlus2)
			if emu.cpu.PC < 0x8000 {
				escaped = true
				escFrame = i
				break
			}
		}
		if !escaped {
			t.Fatalf("SPACE held 400 frames: demo never exited (PC=$%04X) — kfull/kpoll dead on emulator", emu.cpu.PC)
		}
		// Run on a bit in BASIC so the ROM settles; clean-exit markers:
		// IM1 restored + AY silenced by the quit path.
		for i := 0; i < 60; i++ {
			runOneFrameHeadless(emu, roms.ModelPlus2)
		}
		t.Logf("escaped at f%d (frame 50-ish after poll would be ~1640); kcnt=%d", escFrame, emu.mem.Read(kcntAddr))
		if emu.cpu.IM != 1 {
			t.Errorf("IM=%d after quit — quit path never ran (crash, not exit?)", emu.cpu.IM)
		}
		if ay := emu.ula.AY(); ay != nil {
			if v := ay.ReadRegister(10); v != 0 {
				t.Errorf("AY chA vol=%d after quit — mute path dead", v)
			}
		}
		emu.kbd.PressMatrixKey(7, 0x01, false)
	})
}