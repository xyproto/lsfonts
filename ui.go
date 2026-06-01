package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"
	"os/signal"
	"strings"

	"github.com/xyproto/imagepreview"
	"github.com/xyproto/vt"
)

// UI layout palette. Page bg is darkest so the slight inset gap between
// cards reads as a divider; cards sit on top in a slightly lighter shade;
// the selected card uses a calm blue tint. The label is given a dimmer
// foreground than the sample so it reads as a header, à la Google Fonts.
var (
	pageBg           = color.RGBA{0x0b, 0x0d, 0x12, 0xff}
	cardBg           = color.RGBA{0x18, 0x1c, 0x24, 0xff}
	cardBgSelected   = color.RGBA{0x24, 0x3f, 0x6e, 0xff}
	labelFgNormal    = color.RGBA{0x90, 0x97, 0xa3, 0xff}
	labelFgSelected  = color.RGBA{0xcc, 0xd2, 0xde, 0xff}
	sampleFgNormal   = color.RGBA{0xea, 0xec, 0xf1, 0xff}
	sampleFgSelected = color.RGBA{0xff, 0xff, 0xff, 0xff}
)

// rowsPerEntry is the height (in terminal cells) reserved for each font
// preview card. Four rows leaves enough room for a small header on top
// of a larger sample line, while still showing roughly a dozen cards on
// a typical 50-row terminal.
const rowsPerEntry = 4

// cardGapPx is the vertical pixel gap between adjacent cards. The gap is
// filled with pageBg so cards visually separate.
const cardGapPx = 3

// statusRows is the number of terminal cells reserved at the bottom for
// the help + search prompt strip below the preview image.
const statusRows = 2

// runUI renders the interactive font browser. Returns when the user quits.
// When the terminal lacks an inline image protocol the caller should fall
// back to a plain text listing instead.
func runUI(entries []FontEntry) error {
	tty, err := vt.NewTTY()
	if err != nil {
		return err
	}
	defer tty.Close()
	tty.RawMode()
	defer tty.Restore()

	// Hide cursor and switch to the alternate screen so the preview does
	// not leave artefacts in the user's scrollback.
	fmt.Print("\033[?25l\033[?1049h")
	defer fmt.Print("\033[?1049l\033[?25h\033[0m")

	state := &uiState{entries: entries, all: entries}
	state.applyFilter() // populate filtered with full list

	// Resize handling: SIGWINCH is delivered when the user resizes the
	// terminal. We catch it on a buffered channel and select against the
	// key reader so the next frame uses the new dimensions immediately
	// instead of waiting for the user to press something.
	winch := make(chan os.Signal, 4)
	notifyWinch(winch)
	defer signal.Stop(winch)

	// Keys are read in a goroutine because vt.ReadKey blocks; we want to
	// be able to wake the UI on resize without first waiting for input.
	// The goroutine is intentionally leaked on exit: main returns right
	// after runUI, so process teardown reclaims it.
	keys := make(chan string, 4)
	go func() {
		for {
			k := tty.ReadKey()
			if k == "" {
				continue
			}
			keys <- k
		}
	}()

	for {
		state.refreshGeometry()
		if state.nVisible <= 0 {
			// Window is too small to draw anything useful. Clear the
			// screen and wait for either a resize or a quit key.
			fmt.Print("\033[2J\033[H")
			select {
			case <-winch:
				continue
			case k := <-keys:
				if k == "q" || k == "c:27" || k == "c:3" || k == "c:4" {
					return nil
				}
			}
			continue
		}
		state.ensureCursorVisible()
		state.drawFrame()

		select {
		case <-winch:
			// Drain any coalesced resize signals so the next frame
			// renders exactly once for the final geometry.
			drainSignals(winch)
		case k := <-keys:
			if state.searchMode {
				if !state.handleSearchKey(k) {
					return nil
				}
			} else {
				if !state.handleNavKey(k) {
					return nil
				}
			}
		}
	}
}

// drainSignals empties a buffered signal channel without blocking. Used
// after a SIGWINCH burst to coalesce a rapid drag-resize into one redraw.
func drainSignals(ch <-chan os.Signal) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

// uiState bundles the mutable bits of the UI loop. Keeping it as one struct
// keeps the keyhandlers small and avoids threading a long parameter list.
type uiState struct {
	all      []FontEntry // immutable: every discovered font
	entries  []FontEntry // current view = all when no filter
	filtered []int       // indices into all that pass the filter
	cursor   int         // selected index inside filtered
	offset   int         // first visible index inside filtered

	// geometry (refreshed per frame)
	cols, rows uint
	cellW      uint
	cellH      uint
	nVisible   int // number of entries that fit in the preview area
	pixW       int
	pixRow     int // pixel height of one entry row

	// search state
	searchMode bool
	query      string
}

// refreshGeometry re-queries terminal dimensions and recomputes how many
// entries fit on screen. It is cheap to call every frame.
func (s *uiState) refreshGeometry() {
	w, h := vt.MustTermSize()
	s.cols = w
	s.rows = h
	cellW, cellH := imagepreview.TerminalCellPixels()
	if cellW == 0 {
		cellW = 8
	}
	if cellH == 0 {
		cellH = 16
	}
	s.cellW = cellW
	s.cellH = cellH
	previewRows := int(s.rows) - statusRows
	if previewRows < rowsPerEntry {
		s.nVisible = 0
		return
	}
	s.nVisible = previewRows / rowsPerEntry
	s.pixW = int(s.cols) * int(cellW)
	s.pixRow = rowsPerEntry * int(cellH)
}

// ensureCursorVisible adjusts offset so cursor lies inside the visible window.
func (s *uiState) ensureCursorVisible() {
	if s.cursor < 0 {
		s.cursor = 0
	}
	if n := len(s.filtered); s.cursor >= n {
		s.cursor = n - 1
	}
	if s.cursor < s.offset {
		s.offset = s.cursor
	}
	if s.cursor >= s.offset+s.nVisible {
		s.offset = s.cursor - s.nVisible + 1
	}
	if s.offset < 0 {
		s.offset = 0
	}
}

// drawFrame composes and flushes the preview image plus the status strip.
// The image is sent first (anchored at row 1) so it overwrites whatever
// the terminal had in those cells, then the status text is rendered with
// normal escape sequences below the image.
func (s *uiState) drawFrame() {
	previewRows := uint(s.nVisible * rowsPerEntry)
	pixH := s.nVisible * s.pixRow
	img := image.NewRGBA(image.Rect(0, 0, s.pixW, pixH))

	// Card geometry: 1-cell horizontal inset and a few pixels of vertical
	// gap on each side, all filled by pageBg so neighbouring cards read
	// as visually separated tiles.
	cardX0 := int(s.cellW)
	cardX1 := s.pixW - int(s.cellW)
	cardW := cardX1 - cardX0
	cardContentH := s.pixRow - 2*cardGapPx
	if cardContentH < rowsPerEntry { // pathological tiny cell — give up
		cardContentH = s.pixRow
	}

	// Page background fills every pixel first; per-card draws then overlay
	// the card body and glyph masks on top.
	draw.Draw(img, img.Bounds(), &image.Uniform{C: pageBg}, image.Point{}, draw.Src)

	for i := range s.nVisible {
		idx := s.offset + i
		cardY0 := i*s.pixRow + cardGapPx
		cardY1 := cardY0 + cardContentH
		cardRect := image.Rect(cardX0, cardY0, cardX1, cardY1)

		bg := cardBg
		lFg := labelFgNormal
		sFg := sampleFgNormal
		if idx == s.cursor {
			bg = cardBgSelected
			lFg = labelFgSelected
			sFg = sampleFgSelected
		}
		draw.Draw(img, cardRect, &image.Uniform{C: bg}, image.Point{}, draw.Src)

		if idx >= len(s.filtered) {
			continue
		}
		entry := s.all[s.filtered[idx]]
		strip := renderStrip(entry, cardW, cardContentH)
		if strip == nil {
			continue
		}
		// Mask bounds align with (0,0); cardRect.Min translates them onto
		// the page image. DrawMask clips to cardRect ∩ img.Bounds.
		draw.DrawMask(img, cardRect, &image.Uniform{C: lFg}, image.Point{},
			strip.Label, image.Point{}, draw.Over)
		draw.DrawMask(img, cardRect, &image.Uniform{C: sFg}, image.Point{},
			strip.Sample, image.Point{}, draw.Over)
	}

	vt.BeginSyncUpdate()
	var buf bytes.Buffer
	// Park the cursor at the top-left and flush the image. The image
	// protocol consumes exactly previewRows terminal rows.
	fmt.Fprint(&buf, "\033[H")
	os.Stdout.Write(buf.Bytes())
	imagepreview.FlushRawRGBAWithID(os.Stdout, img, s.cols, previewRows, 1)
	s.drawStatus(int(previewRows))
	vt.EndSyncUpdate()
}

// drawStatus paints the help + search/info line(s) just below the image.
// row0 is the (1-based) terminal row where the status block starts.
func (s *uiState) drawStatus(row0 int) {
	row0++ // convert to 1-based for CUP
	var buf bytes.Buffer

	// Reset attributes and clear the status block.
	for r := range statusRows {
		fmt.Fprintf(&buf, "\033[%d;1H\033[0m\033[2K", row0+r)
	}

	total := len(s.filtered)
	pos := 0
	var sel FontEntry
	if total > 0 {
		pos = s.cursor + 1
		sel = s.all[s.filtered[s.cursor]]
	}

	// Line 1: selected entry's path + position counter (dim).
	fmt.Fprintf(&buf, "\033[%d;1H\033[2;37m", row0)
	if total > 0 {
		fmt.Fprintf(&buf, "%d/%d  %s", pos, total, sel.Path)
		if sel.Index > 0 {
			fmt.Fprintf(&buf, " [face %d]", sel.Index)
		}
	} else {
		fmt.Fprint(&buf, "0/0  (no matching fonts)")
	}
	fmt.Fprint(&buf, "\033[0m")

	// Line 2: search box, active-filter indicator, or keybinding help.
	fmt.Fprintf(&buf, "\033[%d;1H", row0+1)
	switch {
	case s.searchMode:
		fmt.Fprintf(&buf, "\033[1;33m/\033[0m%s\033[7m \033[0m", s.query)
	case s.query != "":
		// Filter is still active after the user pressed Enter. Make this
		// visible so the user doesn't think the filter was discarded.
		fmt.Fprintf(&buf,
			"\033[2;37m↑/↓ move  / refine  Esc clear filter  q quit  "+
				"\033[0m\033[1;33mfilter:\033[0m \033[33m%s\033[0m",
			s.query)
	default:
		fmt.Fprint(&buf, "\033[2;37m"+
			"↑/↓ move  PgUp/PgDn page  g/G top/bottom  / search  q quit"+
			"\033[0m")
	}
	os.Stdout.Write(buf.Bytes())
}

// handleNavKey handles a keystroke while not in search mode. Returns false
// when the UI should exit.
func (s *uiState) handleNavKey(key string) bool {
	switch key {
	case "q", "c:3", "c:4":
		return false
	case "c:27": // Esc: clear an active filter, otherwise quit
		if s.query != "" {
			s.query = ""
			s.applyFilter()
		} else {
			return false
		}
	case "↓", "j", "c:14": // Ctrl-N
		s.cursor++
	case "↑", "k", "c:16": // Ctrl-P
		s.cursor--
	case "⇟", " ", "c:6": // PgDn, Space, Ctrl-F
		s.cursor += s.nVisible
	case "⇞", "b", "c:2": // PgUp, b, Ctrl-B
		s.cursor -= s.nVisible
	case "g", "⇱":
		s.cursor = 0
	case "G", "⇲":
		s.cursor = len(s.filtered) - 1
	case "/":
		// Enter search mode keeping any existing query so the user can
		// refine the active filter instead of starting from scratch.
		s.searchMode = true
	case "c:12": // Ctrl-L: forced redraw on next loop iteration -- nothing to do
	}
	return true
}

// navKeyTokens is the set of vt-named special keys that should browse the
// list even while the search prompt is active. They cannot occur as part
// of a font family name, so routing them to handleNavKey both fixes the
// "arrow key prints ↑ in the query" bug and lets users scroll through
// matches without first pressing Enter.
var navKeyTokens = map[string]bool{
	"↑": true, "↓": true, "←": true, "→": true,
	"⇱": true, "⇲": true, "⇞": true, "⇟": true,
}

// handleSearchKey handles a keystroke while the search prompt is active.
// Returns false when the UI should exit.
func (s *uiState) handleSearchKey(key string) bool {
	// Special navigation keys browse the filtered list without leaving
	// search mode. handleNavKey only quits on q/Ctrl-C/Ctrl-D/Esc, none
	// of which are in navKeyTokens, so its return value is always true
	// here -- but forwarding it costs nothing and keeps the contract
	// symmetric.
	if navKeyTokens[key] {
		return s.handleNavKey(key)
	}
	switch key {
	case "c:27": // Esc: cancel search and clear filter
		s.searchMode = false
		s.query = ""
		s.applyFilter()
	case "c:13", "c:10": // Enter: keep filter, leave search-input mode
		s.searchMode = false
	case "c:8", "c:127": // Backspace
		if n := len(s.query); n > 0 {
			s.query = s.query[:n-1]
			s.applyFilter()
		}
	case "c:21": // Ctrl-U: clear query without leaving search mode
		s.query = ""
		s.applyFilter()
	case "c:3", "c:4": // Ctrl-C / Ctrl-D
		return false
	default:
		// Treat anything that decodes to a single printable rune as input;
		// ignore other special keys (function keys, modifier combos).
		if isPrintableKey(key) {
			s.query += key
			s.applyFilter()
		}
	}
	return true
}

// applyFilter rebuilds the filtered index list from the current query and
// snaps the cursor back into range.
func (s *uiState) applyFilter() {
	s.filtered = s.filtered[:0]
	q := strings.ToLower(strings.TrimSpace(s.query))
	for i, e := range s.all {
		if q == "" || strings.Contains(strings.ToLower(e.Display), q) ||
			strings.Contains(strings.ToLower(e.Path), q) {
			s.filtered = append(s.filtered, i)
		}
	}
	if s.cursor >= len(s.filtered) {
		s.cursor = len(s.filtered) - 1
	}
	if s.cursor < 0 {
		s.cursor = 0
	}
	s.offset = 0
}

// isPrintableKey reports whether key is a single printable UTF-8 rune (i.e.
// real text input). Control sequences from vt.ReadKey arrive prefixed with
// "c:" or as named tokens like "↑" or "F1" which we exclude here.
func isPrintableKey(key string) bool {
	if key == "" || strings.HasPrefix(key, "c:") {
		return false
	}
	// Named multi-rune tokens (e.g. "F1", "alt↑", "shift⇱") never qualify as
	// search input because they don't represent a typed character.
	if len(key) > 4 {
		return false
	}
	for _, r := range key {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}
