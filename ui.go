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

	"github.com/xyproto/clip"
	"github.com/xyproto/imagepreview"
	"github.com/xyproto/vt"
)

// Page bg shows through the inter-card gap. Cards sit on top in a lighter
// shade; the selection uses a calm blue. Label fg is dimmer than sample fg
// so the header reads as subordinate, à la Google Fonts.
var (
	pageBg           = color.RGBA{0x0b, 0x0d, 0x12, 0xff}
	cardBg           = color.RGBA{0x18, 0x1c, 0x24, 0xff}
	cardBgSelected   = color.RGBA{0x24, 0x3f, 0x6e, 0xff}
	labelFgNormal    = color.RGBA{0x90, 0x97, 0xa3, 0xff}
	labelFgSelected  = color.RGBA{0xcc, 0xd2, 0xde, 0xff}
	sampleFgNormal   = color.RGBA{0xea, 0xec, 0xf1, 0xff}
	sampleFgSelected = color.RGBA{0xff, 0xff, 0xff, 0xff}
)

const (
	rowsPerEntry = 4 // terminal cells per card
	cardGapPx    = 3 // pixel gap between cards (filled with pageBg)
	statusRows   = 2 // help + search prompt rows below the image
)

// runUI runs the interactive browser. Caller should fall back to a plain
// text listing when the terminal lacks an inline image protocol.
func runUI(entries []FontEntry) error {
	tty, err := vt.NewTTY()
	if err != nil {
		return err
	}
	defer tty.Close()
	tty.RawMode()
	defer tty.Restore()

	// Alternate screen + hidden cursor so the preview leaves no scrollback.
	fmt.Print("\033[?25l\033[?1049h")
	defer fmt.Print("\033[?1049l\033[?25h\033[0m")

	state := &uiState{entries: entries, all: entries}
	state.applyFilter()

	// SIGWINCH wakes the loop so resize redraws without waiting for input.
	winch := make(chan os.Signal, 4)
	notifyWinch(winch)
	defer signal.Stop(winch)

	// vt.ReadKey blocks; goroutine lets the main loop select on input vs
	// resize. Intentionally leaked on exit (process is about to return).
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
			// Window too small. Wait for resize or a quit key.
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
			drainSignals(winch) // coalesce drag-resize bursts
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

func drainSignals(ch <-chan os.Signal) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

type uiState struct {
	all      []FontEntry // every discovered font
	entries  []FontEntry // current view = all when no filter
	filtered []int       // indices into all that pass the filter
	cursor   int         // selected index inside filtered
	offset   int         // first visible index inside filtered

	// refreshed per frame
	cols, rows uint
	cellW      uint
	cellH      uint
	nVisible   int
	pixW       int
	pixRow     int

	searchMode bool
	query      string

	flash string // transient status line ("Copied: …"); cleared on next key
}

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

// drawFrame composes one preview page image + status strip and flushes.
func (s *uiState) drawFrame() {
	previewRows := uint(s.nVisible * rowsPerEntry)
	pixH := s.nVisible * s.pixRow
	img := image.NewRGBA(image.Rect(0, 0, s.pixW, pixH))

	cardX0 := int(s.cellW)
	cardX1 := s.pixW - int(s.cellW)
	cardW := cardX1 - cardX0
	cardContentH := s.pixRow - 2*cardGapPx
	if cardContentH < rowsPerEntry {
		cardContentH = s.pixRow
	}

	draw.Draw(img, img.Bounds(), &image.Uniform{C: pageBg}, image.Point{}, draw.Src)

	for i := range s.nVisible {
		idx := s.offset + i
		cardY0 := i*s.pixRow + cardGapPx
		cardRect := image.Rect(cardX0, cardY0, cardX1, cardY0+cardContentH)

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
		draw.DrawMask(img, cardRect, &image.Uniform{C: lFg}, image.Point{},
			strip.Label, image.Point{}, draw.Over)
		draw.DrawMask(img, cardRect, &image.Uniform{C: sFg}, image.Point{},
			strip.Sample, image.Point{}, draw.Over)
	}

	vt.BeginSyncUpdate()
	var buf bytes.Buffer
	fmt.Fprint(&buf, "\033[H")
	os.Stdout.Write(buf.Bytes())
	imagepreview.FlushRawRGBAWithID(os.Stdout, img, s.cols, previewRows, 1)
	s.drawStatus(int(previewRows))
	vt.EndSyncUpdate()
}

// drawStatus paints the two status rows below the preview. row0 is the
// 0-based row index where the status block starts.
func (s *uiState) drawStatus(row0 int) {
	row0++ // CUP is 1-based
	var buf bytes.Buffer

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

	// Line 1: position + selected path.
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

	// Line 2: flash > prompt > active-filter help > default help.
	fmt.Fprintf(&buf, "\033[%d;1H", row0+1)
	switch {
	case s.flash != "":
		fmt.Fprintf(&buf, "\033[1;32m%s\033[0m", s.flash)
	case s.searchMode:
		fmt.Fprintf(&buf, "\033[1;33m/\033[0m%s\033[7m \033[0m", s.query)
	case s.query != "":
		fmt.Fprintf(&buf,
			"\033[2;37m↑/↓ move  / refine  Esc clear filter  ^C copy  q quit  "+
				"\033[0m\033[1;33mfilter:\033[0m \033[33m%s\033[0m",
			s.query)
	default:
		fmt.Fprint(&buf, "\033[2;37m"+
			"↑/↓ move  PgUp/PgDn page  g/G top/bottom  / search  ^C copy  q quit"+
			"\033[0m")
	}
	os.Stdout.Write(buf.Bytes())
}

// copySelected copies "Family Style" to the clipboard; reports via flash.
func (s *uiState) copySelected() bool {
	if len(s.filtered) == 0 {
		s.flash = "Nothing to copy"
		return false
	}
	entry := s.all[s.filtered[s.cursor]]
	name := entry.CopyName()
	if err := clip.WriteAll(name, false); err != nil {
		s.flash = "Copy failed: " + err.Error()
		return true
	}
	s.flash = "Copied: " + name
	return true
}

// handleNavKey processes a key outside search mode. false = quit.
func (s *uiState) handleNavKey(key string) bool {
	s.flash = "" // any key dismisses the previous flash
	switch key {
	case "q", "c:4":
		return false
	case "c:3": // Ctrl-C: copy
		s.copySelected()
	case "c:27": // Esc: clear filter, else quit
		if s.query != "" {
			s.query = ""
			s.applyFilter()
		} else {
			return false
		}
	case "↓", "j", "c:14":
		s.cursor++
	case "↑", "k", "c:16":
		s.cursor--
	case "⇟", " ", "c:6":
		s.cursor += s.nVisible
	case "⇞", "b", "c:2":
		s.cursor -= s.nVisible
	case "g", "⇱":
		s.cursor = 0
	case "G", "⇲":
		s.cursor = len(s.filtered) - 1
	case "/":
		// Keep any existing query so it can be refined, not retyped.
		s.searchMode = true
	case "c:12": // Ctrl-L: redraw on next loop iteration
	}
	return true
}

// Special keys that browse the list even while the prompt is open. They
// can't be typed into a font name, so routing them avoids the "arrow key
// types ↑" bug.
var navKeyTokens = map[string]bool{
	"↑": true, "↓": true, "←": true, "→": true,
	"⇱": true, "⇲": true, "⇞": true, "⇟": true,
}

// handleSearchKey processes a key while the prompt is open. false = quit.
func (s *uiState) handleSearchKey(key string) bool {
	s.flash = ""
	if navKeyTokens[key] {
		return s.handleNavKey(key)
	}
	switch key {
	case "c:27": // Esc: cancel + clear
		s.searchMode = false
		s.query = ""
		s.applyFilter()
	case "c:13", "c:10": // Enter: keep filter, leave prompt
		s.searchMode = false
	case "c:8", "c:127": // Backspace
		if n := len(s.query); n > 0 {
			s.query = s.query[:n-1]
			s.applyFilter()
		}
	case "c:21": // Ctrl-U: clear query, stay in prompt
		s.query = ""
		s.applyFilter()
	case "c:3":
		// Ctrl-C: copy and drop the prompt -- otherwise the next "q" / "Esc"
		// would land as a literal character in the query.
		s.copySelected()
		s.searchMode = false
	case "c:4":
		return false
	default:
		if isPrintableKey(key) {
			s.query += key
			s.applyFilter()
		}
	}
	return true
}

// applyFilter rebuilds filtered from query and snaps the cursor in range.
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

// isPrintableKey reports whether key is real typed text (not a "c:N" or
// named special token like "F1" / "↑").
func isPrintableKey(key string) bool {
	if key == "" || strings.HasPrefix(key, "c:") {
		return false
	}
	if len(key) > 4 { // "F1", "alt↑", "shift⇱" etc.
		return false
	}
	for _, r := range key {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}
