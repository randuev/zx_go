package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/conorarmstrong/zx_go/pkg/roms"
	"github.com/conorarmstrong/zx_go/pkg/ula"
)

// cruxayPlus2Run boots a real +2 (grey) emulator core, loads the cruxay
// code block at $8000 the way RANDOMIZE USR would find it, then runs N
// frames sampling paging, IM2, fcnt cadence, bbuf flips and both screen
// banks. This is the faithful-128 ground truth for the "static stars +
// editor text survives + page 2 black" report on Fuse/+2.
func TestCruxayPlus2Run(t *testing.T) {

	// v20g: derive ALL instrumented addresses from the assembler's fresh
	// symbol file — every code edit before the globals block shifts them,
	// and hardcoded hex constants silently mis-census (cost hours twice).
	// NOTE: Go fmt.Sscanf does NOT support %[...] scansets — the earlier
	// parser silently matched NOTHING and every lookup fell back to stale
	// hex (the v20h "intFrames=0" false alarm). Parse manually instead.
	syms := map[string]uint16{}
	if bs, err := os.ReadFile("/tmp/cruxay.sym"); err == nil {
		for _, ln := range strings.Split(string(bs), "\n") {
			name, rest, ok := strings.Cut(strings.TrimSpace(ln), ":")
			if !ok {
				continue
			}
			rest = strings.TrimSpace(rest)
			if !strings.HasPrefix(rest, "EQU 0x") {
				continue
			}
			rest = strings.Fields(rest[6:])[0] // strip trailing "; module main"
			if v, err := strconv.ParseUint(rest, 16, 16); err == nil {
				syms[name] = uint16(v)
			}
		}
	}
	t.Logf("sym file parsed: %d symbols (fcnt=%04X)", len(syms), syms["fcnt"])
	sym := func(n string, fallback uint16) uint16 {
		if v, ok := syms[n]; ok {
			return v
		}
		return fallback
	}
	prev := cliFlagsActive
	nf := cliFlags{}
	if prev != nil {
		nf = *prev
	}
	// Audio capture: headless+recordAudio routes newEmulator to the SILENT
	// mixer (no oto device racing the WAV consumer — see main.go:988). The
	// census loop pumps PumpAudioFrame once per executed frame; the WAV is
	// the exact AY mix in emulated time. Set CRUXAY_WAV to enable.
	wavPath := os.Getenv("CRUXAY_WAV")
	if wavPath != "" {
		nf.noSound = false
		nf.headless = true
		nf.recordAudio = wavPath
	} else {
		nf.noSound = true
	}
	cliFlagsActive = &nf
	var recUla *ula.ULA
	defer func() {
		if wavPath != "" && recUla != nil {
			if err := recUla.StopRecording(); err != nil {
				t.Logf("StopRecording: %v", err)
			} else {
				t.Logf("CRUXAY_WAV finalised: %s", wavPath)
			}
		}
		cliFlagsActive = prev
	}()

	tapPath := "/root/nerve-workspace/demos/cruxay/cruxay.tap"
	if p := os.Getenv("CRUXAY_TAP"); p != "" {
		tapPath = p
	}
	raw, err := os.ReadFile(tapPath)
	if err != nil {
		t.Fatalf("read tap: %v", err)
	}
	// Parse tap blocks; find the CODE block (type 3) at $8000.
	var blk []byte
	var loadAddr uint16
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
			// TAP header layout: flag(0) type(1) name(2:8) len(8:10) start(10:12) chk(12)
			loadAddr = binary.LittleEndian.Uint16(payload[14:16])
			t.Logf("code header: addr=$%04X", loadAddr)
			continue
		}
		if payload[0] == 0xFF && loadAddr != 0 && blk == nil {
			blk = payload[1 : n-1] // strip ff flag byte + checksum
			t.Logf("code data: len=%d first=%02X", len(blk), blk[0])
		}
	}
	if blk == nil {
		t.Fatal("no CODE block in tap")
	}

	emu, err := newEmulator(roms.ModelPlus2)
	if err != nil {
		t.Fatalf("newEmulator(+2): %v", err)
	}
	emu.paused.Store(false)

	// Boot the +2 ROM to its BASIC prompt so paging/screen are in the
	// state RANDOMIZE USR starts from.
	for i := 0; i < 220; i++ {
		runOneFrameHeadless(emu, roms.ModelPlus2)
	}
	t.Logf("boot: ScreenPage=%d slotmap read=%v write=%v PC=$%04X SP=$%04X",
		emu.mem.ScreenPage, emu.mem.Read(0x5C78), emu.mem.Read(0x5C79), emu.cpu.PC, emu.cpu.SP)
	recUla = emu.ula
	if wavPath != "" {
		maybeStartAudioRecording(emu.ula)
	}

	// Paging tracer: every classic 7FFD write, flagging drops (the
	// bit-5 lock regression class). Installed once; the loop below
	// reads bank pixels for the flip criteria.
	var pgw []string

	// Poke the code block into $8000 (slot RAM — where LOAD put it).
	for i, b := range blk {
		emu.mem.Write(uint16(loadAddr)+uint16(i), b)
	}
	// Fake a RANDOMIZE USR entry: BASIC's USR pushes a return stub on
	// the stack, sets SP below ROM, and jumps to the address.
	emu.cpu.SP = 0xFF00
	emu.mem.Write(0xFEFF, 0x00)
	emu.mem.Write(0xFEFE, 0x00) // fake return addr $0000 (we never return)
	emu.cpu.PC = loadAddr
	emu.cpu.IFF1, emu.cpu.IFF2 = false, false
	emu.cpu.IM = 1

	// Interactive debug mode: CRUXAY_DEBUG_PORT=10000 go test -run TestCruxayPlus2Run
	// boots the +2 core, loads the demo deterministically, then hands live
	// control to a telnet debugger (ZRCP subset): bp/step/peek/poke/regs at
	// any moment WITHOUT relaunching. Frame advancement yields while the
	// debugger is paused so `step` owns the CPU cleanly.
	//
	// ESCAPE CATCHER (debug mode): the pre-fetch hook records every PC in a
	// ring; the first time PC lands OUTSIDE program space ($8000-$8D1E —
	// anything above $8D1E is slack/stack/data, not code), the harness prints
	// the full escape trail, freezes the machine (paused), and stays alive so
	// the debugger can attach to the crash state directly. No re-arm, no
	// missed windows: headless speed can't outrun a hook that stops itself.
	if p := os.Getenv("CRUXAY_DEBUG_PORT"); p != "" {
		var port int
		fmt.Sscanf(p, "%d", &port)
		const codeEnd = 0x8D14 // program tail incl. barTab (v20c syms)
		var eRing [65536]uint16
		var eIdx int
		var escAt [2]uint16 // [prev, first-bad]
		escDone := false
		emu.cpu.AddPreFetchHook("debug-escape", func(pc uint16) {
			if escDone {
				return
			}
			eRing[eIdx&65535] = pc
			eIdx++
			if pc < 0x8000 || pc > codeEnd {
				escAt[0] = eRing[(eIdx-2)&65535]
				escAt[1] = pc
				escDone = true
				emu.paused.Store(true)
			}
		})
		rdbg := newRemoteDebugger(emu, port, false, 4096, false)
		if rdbg == nil {
			t.Fatalf("debugger start on port %d", port)
		}
		// kflag write spy: every quit-flag set logged with the authoring PC
		// and the poll pattern that set it (ghost-key forensics).
		// Hook reports PAGE OFFSET, not CPU addr: globals $89FB-$8A00 live
		// at offset 0x09FB-0x0A00 within the $8000 bank window.
		emu.mem.SetRAMWriteHook(func(bank int, addr uint16, val byte) {
			if addr == 0x09FD { // kflag (CPU $89FD)
				fmt.Printf("KFLAG-WRITE val=%02X pc=$%04X prevkey=$%02X bank=%d\n", val, emu.cpu.PC, emu.mem.Read(0x89FF), bank)
			}
			if addr == 0x09FC { // prevkey
				fmt.Printf("PREVKEY-WRITE val=%02X pc=$%04X\n", val, emu.cpu.PC)
			}
		})
		// PC-range counters + first-PCs head log — printed once at catch.
		var headPC [400]uint16
		var headN int
		var cntInit, cntLoop, cntQuit, cntISR, cntFlip, cntBP, cntKfull int
		emu.cpu.AddPreFetchHook("debug-rangecount", func(pc uint16) {
			if headN < 400 {
				headPC[headN] = pc
				headN++
			}
			switch {
			case pc >= 0x8000 && pc <= 0x80FF:
				cntInit++
			case pc >= 0x8114 && pc <= 0x8127:
				cntLoop++
			case pc >= 0x8128 && pc <= 0x8160:
				cntQuit++
			case pc == 0x8585:
				cntISR++
			case pc >= 0x8618 && pc <= 0x865F:
				cntFlip++
			case pc >= 0x8660 && pc <= 0x8693:
				cntBP++
			case pc >= sym("kfull", 0x8995) && pc <= sym("kfull", 0x8995)+0x25:
				cntKfull++
			}
		})
		fmt.Printf("cruxay +2 live debugger on :%d — demo poked at $%04X, escape catcher armed (code $8000-$8D1E)\n", port, loadAddr)
		emu.paused.Store(false)
		for {
			if escDone {
				fmt.Printf("ESCAPE CAUGHT: last-good PC=$%04X -> bad PC=$%04X (SP=$%04X AF=%02X%02X BC=$%04X DE=$%04X HL=$%04X IX=$%04X IY=$%04X I=$%02X IM=%d IFF1=%v insns=%d)\n",
					escAt[0], escAt[1], emu.cpu.SP, emu.cpu.A, emu.cpu.F, emu.cpu.BC(), emu.cpu.DE(), emu.cpu.HL(), emu.cpu.IX, emu.cpu.IY, emu.cpu.I, emu.cpu.IM, emu.cpu.IFF1, emu.cpu.InstructionCount())
				fmt.Print("TRAIL (last 60 PCs):")
				lo := eIdx - 60
				if lo < 0 {
					lo = 0
				}
				for i := lo; i < eIdx; i++ {
					fmt.Printf(" %04X", eRing[i&65535])
				}
				fmt.Println()
				fmt.Printf("bytes at bad PC $%04X:", escAt[1])
				for i := 0; i < 16; i++ {
					fmt.Printf(" %02X", emu.mem.Read(escAt[1]+uint16(i)))
				}
				fmt.Println()
				fmt.Printf("globals: grace=%02X prevkey=%02X kflag=%02X bpol=%02X fcnt=%02X aclock=%02X\n",
					emu.mem.Read(0x89FE), emu.mem.Read(0x89FF), emu.mem.Read(0x8A00), emu.mem.Read(0x8A01), emu.mem.Read(0x8A02), emu.mem.Read(0x8A03))
				fmt.Print("kbd: all-row=")
				fmt.Printf("%02X", emu.kbd.Scan(0))
				fmt.Print(" rows0-4=")
				for row := 0; row < 5; row++ {
					ah := byte(0xFF ^ (1 << row))
					fmt.Printf(" %02X", emu.kbd.Scan(uint16(ah)<<8))
				}
				fmt.Println()
				fmt.Printf("RANGE cnt: init=%d loop=%d quit=%d isr=%d flipnow=%d bp_go=%d kfull=%d\n",
					cntInit, cntLoop, cntQuit, cntISR, cntFlip, cntBP, cntKfull)
				hn := headN
				if hn > 200 {
					hn = 200
				}
				fmt.Print("FIRST PCs:")
				for i := 0; i < hn; i++ {
					if i%50 == 0 {
						fmt.Print("\n ")
					}
					fmt.Printf(" %04X", headPC[i])
				}
				fmt.Println()
				fmt.Println("Machine FROZEN at escape — debugger attached, stepping available.")
				// Stay alive; never auto-resume. `c` from ZRCP resumes on demand.
				time.Sleep(time.Second)
				continue
			}
			rdbg.WaitIfPaused()
			rdbg.TapTick() // advance `key … tap` frame countdowns
			runOneFrameHeadless(emu, roms.ModelPlus2)
		}
	}

	fcntAddr := sym("fcnt", 0x8A0A)
	bbufAddr := sym("bbuf", 0x8A14)
	prevFC, prevBB, flips := -1, -1, 0
	intFrames, loopFrames := 0, 0
	seenISR := 0
	// v20e AY census: poll the real AY chip state every frame. Proves the
	// Waltzing Matilda sequencer is stepping AND driving audible register
	// values on the +2 core (the pentagon headless starves IM2 INTs, so
	// audio was never observable there — ground truth is this core).
	// volSeen[ch] = every nonzero volume written; toneSeen = nonzero period
	// reg pairs per channel; stepSeen = distinct (wbar,wstep) pairs seen.
	ayNil := false
	volSeen := [3]map[byte]bool{{}, {}, {}}
	toneSeen := [3]int{}
	stepSeen := map[[2]byte]int{}
	prevMel := byte(0xFF)
	melChanges := 0
	var melTrace [][4]int
	if os.Getenv("CRUXAY_MELTRACE") == "1" {
		melTrace = make([][4]int, 0, 1024)
	}
	ayPoll := func() {
		ay := emu.ula.AY()
		if ay == nil {
			ayNil = true
			return
		}
		for ch := 0; ch < 3; ch++ {
			v := ay.ReadRegister(byte(8 + ch))
			if v != 0 {
				volSeen[ch][v] = true
			}
			if ay.ReadRegister(byte(ch*2))|ay.ReadRegister(byte(ch*2+1)) != 0 {
				toneSeen[ch]++
			}
		}
		wbar := emu.mem.Read(sym("wbar", 0x8A12))
		wstep := emu.mem.Read(sym("wstep", 0x8A11))
		stepSeen[[2]byte{wbar, wstep}]++
		if m := emu.mem.Read(sym("wcnt", 0x8A10)); m != prevMel && prevMel != 0xFF {
			melChanges++
		}
		prevMel = emu.mem.Read(sym("wcnt", 0x8A10))
		// CRUXAY_MELTRACE: record per-frame (wstep, chA, chB, chC) periods
		// so the Waltzing Matilda contour can be verified directly from the
		// chip register stream — immune to square-wave harmonic leakage.
		if melTrace != nil {
			pa := int(ay.ReadRegister(0)) | int(ay.ReadRegister(1))<<8
			pb := int(ay.ReadRegister(2)) | int(ay.ReadRegister(3))<<8
			pc := int(ay.ReadRegister(4)) | int(ay.ReadRegister(5))<<8
			melTrace = append(melTrace, [4]int{int(wstep), pa, pb, pc})
		}
	}
	// v14 hardware-faithful criteria: paintbuf's full clear+paint runs
	// with DI held (a flip must never tear mid-paint), so vsyncs are
	// delivered once per loop cycle, not every emulator frame — fcnt and
	// the 32nd-tick flip are loop-paced by design. What must hold:
	//  1. ZERO dropped paging writes (the bit-5 lock regression class),
	//  2. ScreenPage reaches BOTH 5 and 7 (double-buffer actually flips),
	//  3. bank 7 pixels GROW past their boot value (stars paint there),
	//  4. bbuf changes at least twice in the window.
	pgDropped := 0
	// Write-hook census: every write to bank 5 (physical page index 5),
	// bucketed by the writing PC. Answers definitively whether paintbuf's
	// bank-5 branch executes and WHERE its writes land.
	b5writes := map[uint16]int{}
	b5zero := map[uint16]int{}
	b5viaSlot, b5viaDirect := 0, 0
	var b5tup []string
	// Per-frame write accounting: which physical screen bank each paint
	// frame clears/paints, tagged with the bbuf value at frame end. The
	// v18 residual-text question reduces to: on bbuf=0 frames does bank 5
	// receive ZERO-writes (clear) before its star writes?
	curFrame := 0
	frB5z, frB5n, frB7z, frB7n := map[int]int{}, map[int]int{}, map[int]int{}, map[int]int{}
	frBB := map[int]int{}
	var ztup []string
	// Escape-catcher: ring of every fetched PC. When the demo crashes into
	// ROM ($0000 reboot / BASIC editor), walk back to the last in-code PC
	// and print the escape trail — definitive fault location without
	// relaunching.
	var pcRing [32768]uint16
	var pcIdx int
	gwr := make([]string, 0, 64)
	isrVisits := []string{}
	traceOn := false
	samplePC := func(pc uint16) string { return fmt.Sprintf("%04X", pc) }
	var startSeq []string
	trailSaved := false
	var savedRing [32768]uint16
	var savedIdx int
	emu.cpu.AddPreFetchHook("escape-catch", func(pc uint16) {
		pcRing[pcIdx&32767] = pc
		pcIdx++
		if !traceOn && pc == 0x8000 {
			traceOn = true
		}
		if traceOn && len(startSeq) < 400 {
			startSeq = append(startSeq, samplePC(pc))
		}
		if pc >= 0x8585 && pc <= 0x8700 && len(isrVisits) < 400 {
			isrVisits = append(isrVisits, samplePC(pc))
		}
		if traceOn && (pc < 0x8000 || pc > 0x8FFF) {
			for _, q := range startSeq {
				_ = q
			}
		}
		if !trailSaved && (pc < 0x8000 || pc > 0x8FFF) {
			copy(savedRing[:], pcRing[:])
			savedIdx = pcIdx
			trailSaved = true
		}
	})
	emu.mem.SetRAMWriteHook(func(bank int, addr uint16, val byte) {
		if addr == sym("wbar", 0x8A12) && len(gwr) < 200 {
			gwr = append(gwr, fmt.Sprintf("fr=%d $%04X=%02X pc=$%04X b=%d", curFrame, addr|0x8000, val, emu.cpu.PC, bank))
		}
		if val == 0 && len(ztup) < 80 && emu.cpu.PC >= 0x88A2 && emu.cpu.PC <= 0x88DD {
			ztup = append(ztup, fmt.Sprintf("fr=%d b=%d a=$%04X pc=$%04X", curFrame, bank, addr, emu.cpu.PC))
		}
		if bank != 5 && bank != 7 {
			return
		}
		screenOff := addr&0x1FFF - 0 // pixels: CPU $4000-$73FF or $C000-$D7FF
		if addr <= 0x73FF {
			// direct window pixel span
		} else if addr >= 0xC000 && addr-0xC000 < 0x1800 {
			// slot window pixel span
		} else {
			return
		}
		_ = screenOff
		switch bank {
		case 5:
			if val == 0 {
				frB5z[curFrame]++
				b5zero[emu.cpu.PC]++
			} else {
				frB5n[curFrame]++
				b5writes[emu.cpu.PC]++
			}
		case 7:
			if val == 0 {
				frB7z[curFrame]++
			} else {
				frB7n[curFrame]++
			}
		}
	})
	defer emu.mem.SetRAMWriteHook(nil)
	emu.mem.SetPagingTracer(func(source string, val byte, applied, sb, sa bool) {
		pgw = append(pgw, fmt.Sprintf("val=%02X applied=%v src=%s", val, applied, source))
		if !applied {
			pgDropped++
		}
	})
	pagesSeen := map[int]bool{}
	maxB7 := litBytes(emu, 7) // bank-7 baseline (boot editor screen)
	// v14 fuse-report diagnostic: does the demo QUIT back to BASIC on its
	// own after the key-grace expires? Track PC residency per frame-end.
	quitHits, romHits, kfullHits := 0, 0, 0
	firstRomFrame, firstQuitFrame := -1, -1
	prevStar := [5]int{-1, -1, -1, -1, -1}
	starMin := [5]int{999, 999, 999, 999, 999}
	starMax := [5]int{-1, -1, -1, -1, -1}
	starVar := [5]int{}
	for i := 0; i < 700; i++ {
		curFrame++
		runOneFrameHeadless(emu, roms.ModelPlus2)
		if wavPath != "" {
			emu.ula.PumpAudioFrame()
		}
		ayPoll()
		if pc := emu.cpu.PC; pc < 0x8000 {
			romHits++
			if firstRomFrame < 0 {
				firstRomFrame = i
			}
		} else if pc >= 0x8128 && pc <= 0x8180 {
			quitHits++
			if firstQuitFrame < 0 {
				firstQuitFrame = i
			}
		} else if pc >= sym("kfull", 0x8995) && pc <= sym("kfull", 0x8995)+0x25 {
			kfullHits++
		}
		fc := int(emu.mem.Read(fcntAddr))
		if prevFC >= 0 && fc != prevFC {
			intFrames++
		}
		prevFC = fc
		bb := int(emu.mem.Read(bbufAddr))
		if prevBB >= 0 && bb != prevBB {
			flips++
		}
		prevBB = bb
		// Twinkle proof: census EVERY frame. Amp prescale /4 steps the
		// glyph 12.5x/s — sampling only at flip boundaries aliases with
		// the 32-tick cadence and hides the variation.
		{
			disp := 5
			if emu.mem.ScreenPage == 1 {
				disp = 7
			}
			for si, s := range [][2]int{{190, 34}, {198, 150}, {152, 74}, {218, 66}, {174, 110}} {
				n := starWindow(emu, disp, s[0], s[1])
				if prevStar[si] >= 0 && n != prevStar[si] {
					starVar[si]++
				}
				prevStar[si] = n
				if n > starMax[si] {
					starMax[si] = n
				}
				if starMin[si] > n {
					starMin[si] = n
				}
			}
		}
		pagesSeen[emu.mem.ScreenPage] = true
		if n := litBytes(emu, 7); n > maxB7 {
			maxB7 = n
		}
		frBB[i+1] = int(emu.mem.Read(bbufAddr))
		if i < 64 {
			t.Logf("fr=%2d bb=%d B5z=%4d B5n=%3d B7z=%4d B7n=%3d", i+1, frBB[i+1], frB5z[i+1], frB5n[i+1], frB7z[i+1], frB7n[i+1])
		}
		// Text-residue tracker: char row 10 ("Bytes: CRUXAY" home on a
		// loaded tape) — count lit bytes there on BOTH banks every frame,
		// plus bbuf, to prove which paint path never clears its bank.
		{
			h5 := emu.mem.RAM8KPage(10)
			h7 := emu.mem.RAM8KPage(14)
			n5t, n7t := 0, 0
			for xb := 0; xb < 32; xb++ {
				for p := 0; p < 8; p++ {
					off := (p << 8) + ((10 & 7) << 5) + ((10 >> 3) << 11) + xb
					if h5[off] != 0 {
						n5t++
					}
					if h7[off] != 0 {
						n7t++
					}
				}
			}
			if i < 20 || (i > 280 && i < 340) {
				t.Logf("f=%3d bbuf=%d b5text=%d b7text=%d", i, emu.mem.Read(bbufAddr), n5t, n7t)
			}
		}
		// ISR/loop residency by PC sampling at frame boundaries.
		if pc := emu.cpu.PC; pc >= 0x8585 && pc <= 0x85B4 {
			seenISR++
		} else if pc >= 0x8114 && pc <= 0x8127 {
			loopFrames++
		}
		if i%40 == 39 {
			n5 := litBytes(emu, 5)
			n7 := litBytes(emu, 7)
			t.Logf("f=%3d PC=$%04X IFF1=%v IM=%d I=$%02X SP=$%04X ScreenPage=%d fcnt=%d bbuf=%d bcol=%d grace=%d litB5=%d litB7=%d pgwrites=%d",
				i, emu.cpu.PC, emu.cpu.IFF1, emu.cpu.IM, emu.cpu.I, emu.cpu.SP, emu.mem.ScreenPage, fc, bb,
				emu.mem.Read(sym("bcol", 0x8A0F)), emu.mem.Read(sym("grace", 0x8A06)), n5, n7, len(pgw))
		}
	}
	t.Logf("RESULT intFrames(changed fcnt)=%d flips(bbuf)=%d isrHits=%d loopHits=%d pgwrites=%d pgDropped=%d maxB7=%d pages=%v quitHits=%d@f%d romHits=%d@f%d kfullHits=%d",
		intFrames, flips, seenISR, loopFrames, len(pgw), pgDropped, maxB7, pagesSeen,
		quitHits, firstQuitFrame, romHits, firstRomFrame, kfullHits)
	{
		nv := [3]int{}
		for ch := 0; ch < 3; ch++ {
			nv[ch] = len(volSeen[ch])
		}
		if melTrace != nil {
			f, _ := os.Create("/tmp/cruxay_meltrace.txt")
			if f != nil {
				for _, r := range melTrace {
					fmt.Fprintf(f, "%d %d %d %d\n", r[0], r[1], r[2], r[3])
				}
				f.Close()
				t.Logf("CRUXAY_MELTRACE: %d frames -> /tmp/cruxay_meltrace.txt", len(melTrace))
			}
		}
		t.Logf("AY-CENSUS nil=%v volA=%v volB=%v volC=%v toneActiveFrames A/B/C=%d/%d/%d waltzStates=%d melwcntChanges=%d",
			ayNil, sortedVolKeys(volSeen[0]), sortedVolKeys(volSeen[1]), sortedVolKeys(volSeen[2]),
			toneSeen[0], toneSeen[1], toneSeen[2], len(stepSeen), melChanges)
		keys := make([][2]byte, 0, len(stepSeen))
		for k := range stepSeen {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(a, b int) bool {
			if keys[a][0] != keys[b][0] {
				return keys[a][0] < keys[b][0]
			}
			return keys[a][1] < keys[b][1]
		})
		var sb strings.Builder
		for _, k := range keys {
			sb.WriteString(fmt.Sprintf(" b%d.s%d=%d", k[0], k[1], stepSeen[k]))
		}
		t.Logf("AY-STEPS:%s", sb.String())
	}
	// Residue dump: print every lit pixel row as ASCII for both screen
	// banks. Diagnoses "Bytes: CRUXAY persists on ONE page on Fuse":
	// whichever bank's bitmap still shows glyphs proves which paint path
	// never clears it.
	for _, bank := range [2]int{5, 7} {
		h0 := emu.mem.RAM8KPage(bank * 2)
		if h0 == nil {
			continue
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("=== bank %d bitmap ===\n", bank))
		for row := 0; row < 24; row++ {
			for p := 0; p < 8; p++ {
				off := (p << 8) + ((row & 7) << 5) + ((row >> 3) << 11)
				for xb := 0; xb < 32; xb++ {
					b := h0[off+xb]
					for xr := 7; xr >= 0; xr-- {
						if b&(1<<uint(xr)) != 0 {
							sb.WriteString("#")
						} else {
							sb.WriteString(".")
						}
					}
				}
				sb.WriteString("\n")
			}
		}
		t.Log(sb.String())
	}
	for si := range starMax {
		t.Logf("star[%d] window lit min=%d max=%d var=%d", si, starMin[si], starMax[si], starVar[si])
	}
	// Print escape trail: 40 PCs before + 20 after the first escape.
	if trailSaved {
		esc := -1
		for i := 1; i < 32768; i++ {
			if savedRing[(savedIdx-32768+i-1)&32767] >= 0x8000 && savedRing[(savedIdx-32768+i)&32767] < 0x8000 {
				esc = i
				break
			}
		}
		if esc < 0 {
			esc = 32700
		}
		lo, hi := esc-40, esc+20
		if lo < 0 {
			lo = 0
		}
		msg := "ESCAPE TRAIL:"
		for i := lo; i < hi; i++ {
			msg += fmt.Sprintf(" %04X", savedRing[(savedIdx-32768+i)&32767])
		}
		t.Log(msg)
	}
	t.Logf("ISRSEQ %s", strings.Join(isrVisits, " "))
	if traceOn {
		t.Logf("STARTSEQ %s", strings.Join(startSeq, " "))
	}
	for _, g := range gwr {
		t.Logf("GWR %s", g)
	}
	if romHits > 0 {
		t.Errorf("PC escaped to ROM BASIC region (%d frame-ends, first @f%d) — demo quit; kfull ghost-key suspected", romHits, firstRomFrame)
	}
	for j, s := range pgw {
		if j < 30 {
			t.Logf("pgw[%d] %s", j, s)
		}
	}
	t.Logf("B5WRITES total=%d zero=%d viaSlot=%d viaDirect=%d", sumMap(b5writes), sumMap(b5zero), b5viaSlot, b5viaDirect)
	for _, x := range ztup {
		t.Logf("ZTUP %s", x)
	}
	// Display truth at run end: dump the CURRENTLY DISPLAYED bank's text
	// rows (y=80..87 = char row 10) exactly as the screen renders them.
	{
		dsp := 5
		if emu.mem.ScreenPage != 0 {
			dsp = 7
		}
		hd0 := emu.mem.RAM8KPage(dsp * 2)
		hd1 := emu.mem.RAM8KPage(dsp*2 + 1)[:0x1400]
		rd := func(off int) byte {
			if off < 0x2000 {
				return hd0[off]
			}
			return hd1[off-0x2000]
		}
		t.Logf("DISPLAY bank=%d row10 pixels:", dsp)
		for p := 0; p < 8; p++ {
			for xb := 0; xb < 32; xb++ {
				b := rd((p << 8) + ((10 & 7) << 5) + ((10 >> 3) << 11) + xb)
				for xr := 7; xr >= 0; xr-- {
					if b&(1<<uint(xr)) != 0 {
						t.Log("DISPLAYROW:", string([]byte{'#'}))
					}
				}
			}
		}
		for y := 80; y < 88; y++ {
			l := make([]byte, 0, 32)
			yr := y - 80
			for xb := 0; xb < 32; xb++ {
				b := rd((yr << 8) + ((10 & 7) << 5) + ((10 >> 3) << 11) + xb)
				l = append(l, b)
			}
			t.Logf("DISPY%d %x", y, l)
		}
	}
	// Decisive: WHERE does the editor text live NOW? Scan every 8K page's
	// char-row-10 offset ($0A40) and pixel-span totals. The hook says
	// bank5 gets zero clears every frame; if text bytes persist somewhere,
	// find the page that actually holds them — the display source.
	for p := 0; p < 16; p++ {
		h := emu.mem.RAM8KPage(p)
		if h == nil {
			continue
		}
		textBytes := 0
		for xb := 0; xb < 32; xb++ {
			for bRow := 0; bRow < 8; bRow++ {
				off := (bRow << 8) + ((10 & 7) << 5) + ((10 >> 3) << 11) + xb
				if h[off] != 0 {
					textBytes++
				}
			}
		}
		lit := 0
		for _, b := range h {
			if b != 0 {
				lit++
			}
		}
		t.Logf("PAGE[%2d] lit=%4d row10text=%3d off0A40=%02X", p, lit, textBytes, h[0x0A40])
	}
	for _, x := range b5tup {
		t.Logf("b5tup %s", x)
	}
	for pc, n := range b5writes {
		t.Logf("b5wr pc=$%04X n=%d zero=%d", pc, n, b5zero[pc])
	}
	if pgDropped > 0 {
		t.Errorf("%d/$7FFD writes DROPPED — paging lock regression (bit-5 must stay 0)", pgDropped)
	}
	if intFrames < 20 {
		t.Errorf("ISR/fcnt NOT advancing on +2 core (intFrames=%d)", intFrames)
	}
	if !pagesSeen[7] || !pagesSeen[5] {
		t.Errorf("double-buffer never fully flips (pages seen=%v, want both 5 and 7)", pagesSeen)
	}
	if flips < 2 {
		t.Errorf("bbuf flips NOT happening on +2 core (flips=%d)", flips)
	}
	if maxB7 <= 158 {
		t.Errorf("bank 7 never painted (maxB7=%d, boot baseline ~158 editor residue)", maxB7)
	}
	tv := 0
	for _, v := range starVar {
		tv += v
	}
	if tv < 10 {
		t.Errorf("stars NEVER change size across %d flips (starVar=%v) — twinkle amp dead; matches 'stars never become smaller'", flips, starVar)
	}
}

// starWindow counts lit pixels in a 17x17 window (centred) on the screen
// of classic bank n, decoded with canonical ULA addressing.
func starWindow(emu *emulator, bank, cx, cy int) int {
	h0 := emu.mem.RAM8KPage(bank * 2)
	if h0 == nil {
		return -1
	}
	h1 := emu.mem.RAM8KPage(bank*2 + 1)[:0x1400]
	rd := func(off int) byte {
		if off < 0x2000 {
			return h0[off]
		}
		return h1[off-0x2000]
	}
	n := 0
	for dy := -8; dy <= 8; dy++ {
		y := cy + dy
		if y < 0 || y >= 192 {
			continue
		}
		for dx := -8; dx <= 8; dx++ {
			x := cx + dx
			if x < 0 || x >= 256 {
				continue
			}
			off := ((y & 7) << 8) + ((y & 0x38) << 2) + ((y & 0xC0) << 5) + (x >> 3)
			bit := byte(0x80 >> (x & 7))
			if rd(off)&bit != 0 {
				n++
			}
		}
	}
	return n
}

// litBytes counts non-zero pixel bytes in the 16K screen of classic
// bank n (bank5/7 => first 0x4000 of the physical page).
func litBytes(emu *emulator, bank int) int {
	n := 0
	// Screen pixels live at $4000-$73FF of bank n; pages are 16K:
	// classic bank n => m.ram[n]; RAM8KPage(2n), RAM8KPage(2n+1) first
	// 0x2000 each cover $4000-$7FFF of that bank.
	h0 := emu.mem.RAM8KPage(bank * 2)
	if h0 == nil {
		return -1
	}
	h1 := emu.mem.RAM8KPage(bank*2 + 1)[:0x1400]
	for _, b := range h0 {
		if b != 0 {
			n++
		}
	}
	for _, b := range h1 {
		if b != 0 {
			n++
		}
	}
	return n
}

func sumMap(m map[uint16]int) int {
	n := 0
	for _, v := range m {
		n += v
	}
	return n
}

func sortedVolKeys(m map[byte]bool) string {
	ks := make([]int, 0, len(m))
	for k := range m {
		ks = append(ks, int(k))
	}
	sort.Ints(ks)
	return fmt.Sprint(ks)
}
