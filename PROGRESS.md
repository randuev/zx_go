
## PASS 48h (2026-10-19) — wrong key labels + missing banners (Seva report)
Seva: "Instructions are not showing correct keys... when I try to play a hand
there is no error message like before, I thought you restored them all"

**Bug 1 — KCH packed grid stride (input.c).** PASS-48f's RAMTOP compaction
packed the 8x5 label grid into one literal with stride 2, but single-char
labels occupied ONE byte — every decode slid right-to-left from the first
multi-char entry: '5'->"SP", '0'->"YM", 'S'->"ER", ENTER->"NL". '0' did
start the game because PLAY=0 was always the binding — only the labels lied.
FIX: every label padded to exactly two bytes; KCH[81] = "CSZ X C V A S D F G
Q W E R T 1 2 3 4 5 0 9 8 7 6 P O I U Y ENL K J H SPSYM N B ". LAW:
program-generate pack literals (python/awk), NEVER type them — hand-built
copies drop boundary spaces (caught twice). Verified from the shipped tap:
LEFT=5 RIGHT=8 SEL=7 PLAY=0 DISC=6 SORT=S ENTER=EN SPACE=SP SYM=SY M=M.

**Bug 2 — banners born invisible (main.c).** Since the 48e restructure the
only banner_paint call sites were draw_board (round start) and shop repaint —
no path stamped the band when banner_at fired mid-poll, so every mid-round
error banner (SELECT CARDS / NO HANDS LEFT / MAX 5 CARDS...) was set but
never drawn. FIX: banner_at() paints immediately (plate + centered text, its
own 8 rows only — local-repaint law), banner_exp self-heal unchanged.

**Harness.** New gate balatroc_ctl48h_test.go proves both from the tap +
live emulator (band-22 ink/yellow census on PLAY@0sel; grid byte-search in
the shipped tap). Full gates resynced +3 to the new map — swept EVERY hex
const against board.map; two half-word companions ($FCD5 = old _sfxp hi ->
$FCD8, q() garbage byte $FCD2) had slipped the symbol-pair resync. Sustain
watchdog: qctr is a BYTE odometer — mod-256 arithmetic (signed compare reads
wrap as frozen). Storm test rebuilt: music.bin is retired, the tap has NO
music block (songs ride inside the board block; balBootLive's md-load was
dead code); music load removed.

SUITE: 17/17 PASS (STORM/SUSTAIN/CTL/MUTE/F10/ANTE5/SORTCLR/SHOPSTACK/...).
Deterministic tap: board.tap md5 7d6e15dfb663ef0638c6c8da0c9d2662, 32,700 B.
Sent to Seva (notif-6358446d).
