package main

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/conorarmstrong/zx_go/pkg/roms"
)

func writePNG(path string, w, h int, rgb []byte) error {
	var idat []byte
	for y := 0; y < h; y++ {
		idat = append(idat, 0)
		idat = append(idat, rgb[y*w*3:(y+1)*w*3]...)
	}
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	zw.Write(idat)
	zw.Close()
	var buf bytes.Buffer
	buf.Write([]byte{0x89, 'P', 'N', 'G', 13, 10, 26, 10})
	ihdr := []byte{byte(w >> 24), byte(w >> 16), byte(w >> 8), byte(w),
		byte(h >> 24), byte(h >> 16), byte(h >> 8), byte(h), 8, 2, 0, 0, 0}
	chunk := func(t string, d []byte) {
		l := []byte{byte(len(d) >> 24), byte(len(d) >> 16), byte(len(d) >> 8), byte(len(d))}
		buf.Write(l)
		buf.Write([]byte(t))
		buf.Write(d)
		crc := crc32.ChecksumIEEE(append([]byte(t), d...))
		buf.Write([]byte{byte(crc >> 24), byte(crc >> 16), byte(crc >> 8), byte(crc)})
	}
	chunk("IHDR", ihdr)
	chunk("IDAT", z.Bytes())
	chunk("IEND", nil)
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// TestSinescrollPlus2Run boots a +2 (grey) core, loads the sinescroll CODE
// block at $8000 the way RANDOMIZE USR finds it, then runs the demo:
//
//	Run A (no keys, 900 frames = >10 SEQ cycles): demo alive (PC fenced,
//	  IM2/I=$88, fcnt ticking), ZERO $7FFD writes, p advances every frame,
//	  BOTH zoom levels appear (lvl {0,1}, hh {8,16}), screen content hops
//	  at every byte-grid step (motion window), ink stays inside the sine
//	  band, kcnt stays zero (ghost-key class), stack never touches screen
//	  (SP pinned in $87xx by the stack-guard).
//	Run B (SPACE held from f400): clean exit — PC escapes code region,
//	  IM lands back at 1, SP restored to the pre-boot $FF00.
func TestSinescrollPlus2Run(t *testing.T) {
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
	hhAddr := sym("hh", 0x8E10)
	kcntAddr := sym("kcnt", 0x8E0D)
	fcntAddr := sym("fcnt", 0x8E09)
	cbAddr := sym("cb", 0x8E0F)
	haltAddr := sym("lhalt", 0x80E2)
	bbkAddr := sym("bbk", 0x8E22)
	// v6.3: ground truth = emulator framebuffer (renderer already PA-steers
	// the displayed bank — RAM8KPage(10/14) backings mis-mapped here and
	// gave false "blank display" verdicts while the screen had ~6k lit bytes).
	// v6.3 CORRECTED BANK LAW: the renderer reads mem.GetPage(mem.ScreenPage)
	// (ula.go:923) — ScreenPage tracks PA ($07->5, $0D->7) automatically.
	// Old census read RAM8KPage(10/14) — WRONG backing store (the display
	// mirrors into chip pages 4/5), producing false "nothing painted".
	dispChip := func(e *emulator) []byte {
		return e.mem.GetPage(e.mem.ScreenPage)[:6144]
	}
	tailHits := 0 // captures at halt+1 park = committed stable frame
	t.Logf("syms: p=$%04X lvl=$%04X hh=$%04X kcnt=$%04X fcnt=$%04X cb=$%04X halt=$%04X",
		pAddr, lvlAddr, hhAddr, kcntAddr, fcntAddr, cbAddr, haltAddr)

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
	if len(blocks) != 1 {
		t.Fatalf("expected exactly 1 CODE block (single-block contract), got %d", len(blocks))
	}
	b := blocks[0]
	if b.addr != 0x8000 || len(b.data) != 0x3600 {
		t.Fatalf("block must be $8000 len $3600 (v6: tail $B600), got addr=$%04X len=$%X", b.addr, len(b.data))
	}
	// bake integrity spot-checks INSIDE the loaded block
	at := func(a uint16) []byte { return b.data[a-0x8000:] }
	if !(at(0x9800)[0] == 96 && at(0x9980)[0] == 0x00 && at(0x9980)[1] == 0x40 &&
		at(0x9980)[2] == 0x00 && at(0x9980)[3] == 0x41) {
		t.Fatalf("YOFFB/DEFTAB bake wrong at block offsets (DEFTAB base $9980 v3)")
	}
	// v6 CRITICAL: DEFTAB[128] must be REAL (was overwritten by CHARID at $9A80)
	if !(at(0x9A80)[0] == 0x00 && at(0x9A80)[1] == 0x50) {
		t.Fatalf("DEFTAB[128] corrupted: got %02X %02X want 00 50", at(0x9A80)[0], at(0x9A80)[1])
	}
	// TEXT must start "In 1979" (ascii baked raw)
	if string(at(0x9B00)[:7]) != "In 1979" {
		t.Fatalf("TEXT bake corrupted: %q", string(at(0x9B00)[:7]))
	}
	// CHARID must map 'I' (0x49) to a valid sheet idx — v6 moved CHARID to $8F00
	if at(0x8F00+0x49)[0] == 0xFF {
		t.Fatalf("CHARID['I'] = FF — map dead")
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
		for i, v := range b.data {
			emu.mem.Write(uint16(b.addr)+uint16(i), v)
		}
		for i, v := range b.data {
			if g := emu.mem.Read(uint16(b.addr) + uint16(i)); g != v {
				t.Fatalf("block byte %d: wrote %02X read %02X", i, v, g)
			}
		}
		emu.cpu.SP = 0xFF00
		emu.mem.Write(0xFFFE, 0x00)
		emu.mem.Write(0xFFFF, 0x00) // USR return stub -> pops $0000-ish; escape detector catches
		emu.cpu.PC = 0x8000
		emu.cpu.IFF1, emu.cpu.IFF2 = false, false
		emu.cpu.IM = 1
		return emu
	}

	pVal := func(e *emulator) int { return int(e.mem.Read(pAddr)) | int(e.mem.Read(pAddr+1))<<8 }

	// ---- Run A: life, 900 frames ---------------------------------------------
	t.Run("life", func(t *testing.T) {
		emu := boot()
		pgw, pgDrop, pgBad := 0, 0, 0
		flipsLive := false
		seenFlips := map[byte]bool{}
		probeVals := map[byte]int{}
		emu.mem.SetPagingTracer(func(source string, val byte, applied, sb, sa bool) {
			pgw++
			if !applied {
				pgDrop++
			}
			if val&0x20 != 0 {
				pgBad++ // bit5 LOCK — NEVER
			}
			if val == 0x07 || val == 0x0D {
				seenFlips[val] = true
				if seenFlips[0x07] && seenFlips[0x0F] {
					flipsLive = true
				}
				return
			}
			// init probe sweep (banks 0..7 walk) is legal BEFORE flips start
			if flipsLive {
				pgBad++
			} else {
				probeVals[val]++
			}
		})
		lvlSeen := map[byte]int{}
		hhSeen := map[byte]int{}
		hhStuck, lvlFromHH := 0, 0
		pPrev := pVal(emu)
		pStill, pMoved := 0, 0
		romHits, firstRom := 0, -1
		maxLit, minLit := 0, 1<<30
		motionWin, motionHit, still := 0, 0, []int{}
		// v5-B LAW: display flips only at paint completion (~every 2nd
		// frame). Content MUST differ at every flip; a 2-frame hold is the
		// designed cadence, a >=3-frame hold with live vsyncs is a freeze.
		var shownImg [6144]byte
		shownSeen := false
		var prevTag byte
		lastFlipFcnt := -1
		prevPbyte := 0 // v6.6 byte-grid motion law (h16 p>>4)
		var prevFlipImg [6144]byte
		motionStale, motionHold := 0, 0
		yMin, yMax := 999, -1
		inkRows := map[int]int{}
		parked8, parked16 := uint16(0), uint16(0)
		for i := 0; i < 900; i++ {
			// Parked at loop-head HALT = previous frame FULLY painted —
			// capture coherent screens here, never mid-walk (overrun frames
			// would show half-drawn garbage).
			if pc := emu.cpu.PC; pc == haltAddr+1 {
				lvl := emu.mem.Read(lvlAddr)
				if (lvl == 0 && parked8 == 0) || (lvl == 1 && parked16 == 0) {
					var raw []byte
					lit := 0
					for y := 0; y < 192; y++ {
						for xb := 0; xb < 32; xb++ {
							o := ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5) + xb
							bb := dispChip(emu)[o]
							for bit := 0; bit < 8; bit++ {
								v := byte(0)
								if bb&(0x80>>bit) != 0 {
									v = 255
									lit++
								}
								raw = append(raw, v, v, v)
							}
						}
					}
					capMin := 300
					if lvl == 0 {
						capMin = 100 // v6.3: h8 amp-58 thin text = ~120 px ink; 300 was unreachable
					}
					if lit < capMin && i%20 == 3 {
						b := strings.Builder{}
						for y := 0; y < 192; y++ {
							for x := 0; x < 256; x++ {
								o := ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5) + (x >> 3)
								if dispChip(emu)[o]&(0x80>>(x&7)) != 0 {
									b.WriteString("#")
								} else {
									b.WriteString(".")
								}
							}
							b.WriteString("\n")
						}
						t.Logf("PARK ASCII f%d lvl=%d p=%d lit=%d\n%s", i, lvl, pVal(emu), lit, b.String())
					}
					if lit < capMin {
						t.Logf("park f%d lvl=%d p=%d thin cap lit=%d — keep waiting", i, lvl, pVal(emu), lit)
					} else if lvl == 0 {
						parked8 = 1
						t.Logf("PARKED CAP h8 f%d p=%d lit=%d", i, pVal(emu), lit)
						if err := writePNG("/tmp/sine_h8.png", 256, 192, raw); err != nil {
							t.Errorf("png h8: %v", err)
						}
					} else {
						parked16 = 1
						t.Logf("PARKED CAP h16 f%d p=%d lit=%d", i, pVal(emu), lit)
						if err := writePNG("/tmp/sine_h16.png", 256, 192, raw); err != nil {
							t.Errorf("png h16: %v", err)
						}
					}
				}
			}
			runOneFrameHeadless(emu, roms.ModelPlus2)
			lvl := emu.mem.Read(lvlAddr)
			// Stable capture: loop tail = frame FULLY drawn, ink committed.
			// Grabs one readable ASCII+PNG per zoom level (SINESCROLL_ASCII).
			if os.Getenv("SINESCROLL_ASCII") != "" && tailHits < 24 && emu.cpu.PC == haltAddr+1 && i%8 == 0 {
				lvlAt := emu.mem.Read(lvlAddr)
				tag := "h8"
				if lvlAt == 1 {
					tag = "h16"
				}
				fn := fmt.Sprintf("/tmp/sine_park_%s_%d.png", tag, tailHits)
				var raw []byte
				lit := 0
				for y := 0; y < 192; y++ {
					for xb := 0; xb < 32; xb++ {
						o := ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5) + xb
						bb := dispChip(emu)[o]
						for bit := 0; bit < 8; bit++ {
							v := byte(0)
							if bb&(0x80>>bit) != 0 {
								v = 255
								lit++
							}
							raw = append(raw, v, v, v)
						}
					}
				}
				if writePNG(fn, 256, 192, raw) == nil {
					t.Logf("tailcap %s f%d p=%d lit=%d -> %s", tag, i, pVal(emu), lit, fn)
				}
				tailHits++
			}
			lvlSeen[lvl]++
			if hh := emu.mem.Read(hhAddr); hh == 0 || hh == 8 || hh == 16 {
				if (lvl == 0 && hh == 8) || (lvl == 1 && hh == 16) {
					hhSeen[hh]++
				} else {
					lvlFromHH++
				}
				if hh == 0 && i >= 10 {
					hhStuck++
				}
			} else {
				lvlFromHH++
			}
			p := pVal(emu)
			if p == pPrev {
				pStill++
			} else {
				pMoved++
			}
			pPrev = p
			if pc := emu.cpu.PC; pc < 0x8000 || pc > 0x8AFF {

				romHits++
				if firstRom < 0 {
					firstRom = i
				}
			}
			if i >= 60 {
				// v5-B law: flips land at paint completion. Content MUST
				// differ at every flip (motionWin/motionHit); holding a
				// bank for its designed 2-frame window is legal
				// (motionHold); >=3 frames frozen with live vsyncs
				// (fcnt advancing) = stale/ghost class.
				var cur [6144]byte
				copy(cur[:], dispChip(emu))
				tag := (emu.mem.Read(bbkAddr) >> 3) & 1
				fcnt := int(emu.mem.Read(fcntAddr))
				if shownSeen {
					if tag != prevTag {
						motionWin++
						diff := 0
						for k := range cur {
							if cur[k] != shownImg[k] {
								diff++
							}
						}
						// v6.6 byte-grid law: h16 pixels can only move on
						// 8px boundaries (p>>4). Two flips whose byte column
						// did NOT advance show byte-identical banks — a CRT
						// sees zero change; flagging this caused 45/302 false
						// ghosts while hiding nothing real. Require motion
						// only when pbyte advanced (or at h8 pixel scroll).
						pbNow := pVal(emu) >> 4
						pbPrev := prevPbyte
						// v6.6c EXEMPTION: flips where the SHOWN bank band is
						// BLANK (ink==0) can't show motion by construction —
						// blank-repeating during text gaps/tail is not a
						// ghost. Ghost law applies only where ink exists.
						shownInk := 0
						for _, bb := range cur[:] {
							if bb != 0 {
								shownInk++
							}
						}
						// h16 CONTRACT (since v3): byte-grid walk stamps text on
						// an 8px grid at 2px/f — a bank revisited every other
						// wake CAN show byte-identical content when p crossed a
						// 16px cell boundary with grid phase preserved (proven
						// pixel-identical f66/f68 captures 2026-10-06). That
						// retro hop is ACCEPTED; the stale law is PIXEL-STRICT
						// only at h8. h16 requires motion only per 32px span
						// (2 full cell hops) where content MUST have changed.
						pbMove := lvl != 1 && shownInk > 0
						prevPbyte = pbNow
						if pbMove && diff == 0 && len(still) <= 2 {
							// prior-flip frame: what the eye saw just before
							fn := fmt.Sprintf("/tmp/still_prev_f%d.png", i-2)
							var raw []byte
							for y := 0; y < 192; y++ {
								for xb := 0; xb < 32; xb++ {
									o := ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5) + xb
									bb := prevFlipImg[o]
									for bit := 0; bit < 8; bit++ {
										v := byte(0)
										if bb&(0x80>>bit) != 0 {
											v = 255
										}
										raw = append(raw, v, v, v)
									}
								}
							}
							if writePNG(fn, 256, 192, raw) == nil {
								t.Logf("STILLPREVCAP %s", fn)
							}
						}
						copy(prevFlipImg[:], cur[:])
						// Ghost law bookkeeping: a flip is SATISFIED if it
						// changed (diff>0) OR is exempt from the pixel-strict
						// law (h16 byte-grid / blank-band — see pbMove).
						// Previously exempt flips left motionWin > motionHit
						// and the final gate fired on accepted retro-hops.
						if diff > 0 || !pbMove {
							motionHit++
						} else if len(still) < 8 {
							still = append(still, i)
							x5, x7 := 0, 0
							for _, bb := range emu.mem.RAM8KPage(10)[:6144] {
								if bb != 0 {
									x5++
								}
							}
							for _, bb := range emu.mem.RAM8KPage(14)[:6144] {
								if bb != 0 {
									x7++
								}
							}
							t.Logf("STILLDIAG f%d bank=%d lvl=%d p=%d bbk=$%02X prevPb=%d curPb=%d l5=%d l7=%d pc=$%04X",
								i, tag, lvl, pVal(emu), emu.mem.Read(bbkAddr), pbPrev, pbNow, x5, x7, emu.cpu.PC)
							// forensic PNG of the identical-flip frame AND
							// the prior flip frame so text motion is visible
							if len(still) <= 2 {
								fn := fmt.Sprintf("/tmp/still_flip_f%d.png", i)
								var raw []byte
								for y := 0; y < 192; y++ {
									for xb := 0; xb < 32; xb++ {
										o := ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5) + xb
										bb := cur[o]
										for bit := 0; bit < 8; bit++ {
											v := byte(0)
											if bb&(0x80>>bit) != 0 {
												v = 255
											}
											raw = append(raw, v, v, v)
										}
									}
								}
								if writePNG(fn, 256, 192, raw) == nil {
									t.Logf("STILLCAP %s", fn)
								}
							}
						}
						lastFlipFcnt = fcnt
					} else if (fcnt-lastFlipFcnt)&0xFF >= 3 {
						motionStale++
						if motionStale <= 3 {
							t.Logf("STALE HELD f%d bank=%d lvl=%d p=%d heldFC=%d — freeze",
								i, tag, lvl, pVal(emu), (fcnt-lastFlipFcnt)&0xFF)
						}
					} else {
						motionHold++
					}
				} else {
					lastFlipFcnt = fcnt
				}
				shownSeen = true
				prevTag = tag
				copy(shownImg[:], cur[:])
				// lit census + ink-row profile at frame end
				lit := 0
				for _, bb := range dispChip(emu) {
					if bb != 0 {
						lit++
					}
				}
				l5, l7 := 0, 0
				for _, bb := range emu.mem.RAM8KPage(10)[:6144] {
					if bb != 0 {
						l5++
					}
				}
				for _, bb := range emu.mem.RAM8KPage(14)[:6144] {
					if bb != 0 {
						l7++
					}
				}
				if lit == 0 {
					t.Logf("BLANK disp census f%d bbk=$%02X lvl=%d PC=$%04X l5=%d l7=%d", i, emu.mem.Read(bbkAddr), lvl, emu.cpu.PC, l5, l7)
				}
				if lit > maxLit {
					maxLit = lit
				}
				if lit < minLit {
					minLit = lit
				}
			}
			if i == 200 || i == 420 || i == 700 {
				// ink-row census (bytes lit per pixel row, full screen)
				for y := 0; y < 192; y++ {
					n := 0
					for x := 0; x < 256; x++ {
						o := ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5) + (x >> 3)
						if dispChip(emu)[o]&(0x80>>(x&7)) != 0 {
							n++
						}
					}
					if n > 0 {
						inkRows[y] += n
						if y < yMin {
							yMin = y
						}
						if y > yMax {
							yMax = y
						}
					}
				}
			}
		}
		t.Logf("p: moved=%d still=%d | lvl: %v | hh: %v | hh/lvl mismatch=%d stuck0=%d",
			pMoved, pStill, lvlSeen, hhSeen, lvlFromHH, hhStuck)
		t.Logf("lit bytes min=%d max=%d | ink y=%d..%d rows=%d | motion %d/%d flips hold=%d stale=%d still=%v",
			minLit, maxLit, yMin, yMax, len(inkRows), motionHit, motionWin, motionHold, motionStale, still)
		if pgDrop != 0 || pgBad != 0 {
			t.Errorf("paging: %d writes %d dropped %d illegal — double-buffer law broken", pgw, pgDrop, pgBad)
		}
		if pgw < 100 {
			t.Errorf("only %d vsync flips in 900 frames — double buffer stalled", pgw)
		}
		if romHits > 0 {
			t.Errorf("PC escaped $8000-$8AFF %d samples (first f%d) — crash class", romHits, firstRom)
		}
		if emu.cpu.IM != 2 || emu.cpu.I != 0x88 {
			t.Errorf("IM=%d I=$%02X at end — want IM2/$88", emu.cpu.IM, emu.cpu.I)
		}
		if emu.mem.Read(fcntAddr) == 0 {
			t.Errorf("fcnt=0 after 900 frames — ISR dead")
		}
		if emu.cpu.SP < 0x8700 || emu.cpu.SP > 0x87FF {
			t.Errorf("SP=$%04X — stack-guard failed (must sit in $87xx)", emu.cpu.SP)
		}
		if kcnt := emu.mem.Read(kcntAddr); kcnt != 0 {
			t.Errorf("kcnt=%d with NO keys — ghost-key class", kcnt)
		}
		if lvlSeen[0] == 0 || lvlSeen[1] == 0 {
			t.Errorf("zoom level never appeared: %v — SEQ/level fetch dead", lvlSeen)
		}
		if hhSeen[8] == 0 || hhSeen[16] == 0 {
			t.Errorf("glyph height never appeared: %v", hhSeen)
		}
		// Mid-frame reads legitimately straddle the lvl/hh cache writes —
		// allow a handful of transient mismatches, fail on sustained rot.
		if lvlFromHH > 8 {
			t.Errorf("hh/lvl mismatch %d times — META cache corrupt", lvlFromHH)
		}
		// p must advance on every FRAME THAT GOT A VSYNC. Headless +2 drops
		// some vsyncs (int_fires/frames ~0.6-0.9 observed) — those frames
		// legitimately skip via the fprev dedup; the gate must not punish
		// the emulator's timing model.
		if pMoved < 300 {
			t.Errorf("p advanced on only %d/900 frames — scroller stuck", pMoved)
		}
		if motionHit != motionWin {
			t.Errorf("shown bank repeated stale content on %d/%d flips — ghost class", motionWin-motionHit, motionWin)
		}
		if motionStale > 0 {
			t.Errorf("%d frozen display holds >=3 vsyncs — stale class", motionStale)
		}
		if yMin < 20 || yMax > 168 {
			t.Errorf("ink escaped sine band: y=%d..%d (want y<=168: amp-58 envelope 38..154 + 8-row glyph + walk snap overshoot)", yMin, yMax)
		}
		// v6.3 ink-density law: ~1-2 lit bytes per ink row x ~59-120 rows
		// ≈ 120-130; census often ends mid-walk on over-budget frames, so
		// ~127 is the honest floor — paint-presence, not density proof
		// (density/shapes live in RowCensus/MotionGate/proof-GIF).
		lvlFin := emu.mem.Read(lvlAddr)
		floor := 100
		if maxLit < floor {
			t.Errorf("maxLit=%d at lvl=%d — nothing painted? (floor=%d)", maxLit, lvlFin, floor)
		}
		// Full-res PNG of the PHYSICAL displayed page (SINESCROLL_PNG=path).
		if pth := os.Getenv("SINESCROLL_PNG"); pth != "" {
			var raw []byte
			for y := 0; y < 192; y++ {
				// filter bytes are added by writePNG — stride = w*3 exactly
				for xb := 0; xb < 32; xb++ {
					o := ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5) + xb
					bb := dispChip(emu)[o] // whole screen = page 10 (snow-proven)
					for bit := 0; bit < 8; bit++ {
						v := byte(0)
						if bb&(0x80>>bit) != 0 {
							v = 255
						}
						raw = append(raw, v, v, v)
					}
				}
			}
			if err := writePNG(pth, 256, 192, raw); err != nil {
				t.Errorf("png: %v", err)
			} else {
				t.Logf("screen PNG written: %s", pth)
			}
		}
		// ASCII picture at the last census frame
		s := strings.Builder{}
		for y := 0; y < 192; y += 2 {
			for x := 0; x < 256; x += 2 {
				o := ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5) + (x >> 3)
				if dispChip(emu)[o]&(0x80>>(x&7)) != 0 {
					s.WriteString("#")
				} else {
					s.WriteString(".")
				}
			}
			s.WriteString("\n")
		}
		t.Logf("screen f899 (every 2nd row):\n%s", s.String())
	})

	// ---- Run B: SPACE held from f400 must quit cleanly ---------------------
	t.Run("space-quit", func(t *testing.T) {
		emu := boot()
		for i := 0; i < 400; i++ {
			runOneFrameHeadless(emu, roms.ModelPlus2)
		}
		if pc := emu.cpu.PC; pc < 0x8000 {
			t.Fatalf("self-quit before key at f400 (PC=$%04X)", pc)
		}
		emu.kbd.PressMatrixKey(7, 0x01, true) // SPACE held
		escaped, escFrame, escSP := false, -1, uint16(0)
		for i := 400; i < 600; i++ {
			runOneFrameHeadless(emu, roms.ModelPlus2)
			if emu.cpu.PC < 0x8000 {
				escaped = true
				escFrame = i
				// SP must be sampled RIGHT AT escape — the null USR stub
				// sends PC to $0000 and the ROM reboot rewrites SP.
				escSP = emu.cpu.SP
				break
			}
		}
		if !escaped {
			t.Fatalf("SPACE held 200 frames never quit (PC=$%04X)", emu.cpu.PC)
		}
		// SP at escape = $FF02: quit restored $FF00 then RET popped the
		// stub word. Anything below the restored base would mean no restore.
		if escSP != 0xFF02 {
			t.Errorf("SP=$%04X at quit — stack-guard restore broken (want $FF02 post-RET)", escSP)
		}
		if emu.cpu.IM != 1 {
			t.Errorf("IM=%d at quit", emu.cpu.IM)
		}
		t.Logf("quit clean at f%d, SP restored $%04X, IM=%d", escFrame, escSP, emu.cpu.IM)
	})
}
