package main

import (
	"image"
	"image/draw"
	"os"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// sampleText is the per-entry preview phrase. ASCII only so that fonts
// without extended coverage still produce a useful baseline rendering.
const sampleText = "AaBbCc 0123 The quick brown fox jumps over the lazy dog"

// stripCache memoizes rendered preview cards so scrolling does not re-rasterize.
// Key: "path|index|pixW|pixH". Value: *cardStrip.
var stripCache sync.Map

// cardStrip holds the two alpha masks that make up a single preview card:
// the label (drawn in a sans-serif UI font) and the sample (drawn in the
// entry's own font). The UI layer composites each mask with its own
// foreground colour so the label can be visually subordinate to the sample.
type cardStrip struct {
	Label  *image.Alpha
	Sample *image.Alpha
}

// parsedFontCache memoizes the parsed *opentype.Font for each (path, index).
// Parsing TTC collections is the slow part of preview rendering, so we keep
// the result alive across cache misses for the same file.
var parsedFontCache sync.Map

// loadParsedFont loads the entry's face from disk, picking the right face out
// of a TTC collection when Index > 0. The result is cached. Returns nil when
// the file cannot be parsed.
func loadParsedFont(entry FontEntry) *opentype.Font {
	key := entry.Path + "|" + itoa(entry.Index)
	if v, ok := parsedFontCache.Load(key); ok {
		return v.(*opentype.Font)
	}
	data, err := os.ReadFile(entry.Path)
	if err != nil {
		return nil
	}
	coll, err := opentype.ParseCollection(data)
	if err != nil {
		return nil
	}
	if entry.Index >= coll.NumFonts() {
		return nil
	}
	f, err := coll.Font(entry.Index)
	if err != nil || f == nil {
		return nil
	}
	parsedFontCache.Store(key, f)
	return f
}

// renderStrip rasterizes a card-style preview for one font entry into two
// alpha masks of size pixW x pixH (label on top, sample below). Returns nil
// when the font cannot be loaded.
//
// The mask shape lets the UI layer paint glyphs in any colour against any
// background simply by calling draw.DrawMask -- no per-colour re-rasterization.
// Splitting label and sample into separate masks lets the UI tint each one
// independently (e.g. dim header + bright sample, à la Google Fonts).
func renderStrip(entry FontEntry, pixW, pixH int) *cardStrip {
	if pixW <= 0 || pixH <= 0 {
		return nil
	}
	key := entry.Path + "|" + itoa(entry.Index) + "|" + itoa(pixW) + "|" + itoa(pixH)
	if v, ok := stripCache.Load(key); ok {
		return v.(*cardStrip)
	}
	parsed := loadParsedFont(entry)
	if parsed == nil {
		return nil
	}

	// Card layout (vertical):
	//
	//   +-- padX --+-------------------------+-- padX --+
	//   | padTop                                        |
	//   |          label  (UI font, ~28 % of height)    |
	//   |          gap                                  |
	//   |          sample (entry font, ~55 % of height) |
	//   | padBot                                        |
	//   +-----------------------------------------------+
	padX := pixH / 5
	padTop := pixH / 8
	padBot := pixH / 10
	gap := pixH / 14

	labelH := max(pixH*28/100, 12)
	sampleY0 := padTop + labelH + gap
	sampleH := pixH - padBot - sampleY0
	if sampleH < labelH {
		// Card is unusually short: split the inside roughly evenly so
		// neither line gets squashed into a single-pixel sliver.
		inside := pixH - padTop - padBot - gap
		labelH = inside / 3
		sampleH = inside - labelH
		sampleY0 = padTop + labelH + gap
	}

	labelBox := image.Rect(padX, padTop, pixW-padX, padTop+labelH)
	sampleBox := image.Rect(padX, sampleY0, pixW-padX, sampleY0+sampleH)

	sampleSize := float64(sampleH) * 0.65
	if sampleSize < 10 {
		sampleSize = 10
	}
	labelSize := float64(labelH) * 0.72
	if labelSize < 8 {
		labelSize = 8
	}

	sampleFace, err := opentype.NewFace(parsed, &opentype.FaceOptions{
		Size:    sampleSize,
		DPI:     96,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil
	}
	defer sampleFace.Close()

	// The label is rendered in a sans-serif UI font when one is available,
	// so symbol/icon faces (which have no glyphs for their own family name)
	// remain identifiable. When no UI font is found we fall back to the
	// entry's own face and accept the previous behaviour.
	labelFaceFont := uiFont()
	if labelFaceFont == nil {
		labelFaceFont = parsed
	}
	labelFace, err := opentype.NewFace(labelFaceFont, &opentype.FaceOptions{
		Size:    labelSize,
		DPI:     96,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil
	}
	defer labelFace.Close()

	out := &cardStrip{
		Label:  image.NewAlpha(image.Rect(0, 0, pixW, pixH)),
		Sample: image.NewAlpha(image.Rect(0, 0, pixW, pixH)),
	}
	drawClippedString(out.Label, labelBox, labelFace, entry.Display)
	drawClippedString(out.Sample, sampleBox, sampleFace, sampleText)

	stripCache.Store(key, out)
	return out
}

// drawClippedString paints s into the rect clip on dst using face. The
// SubImage trick limits font.Drawer's per-glyph draw.DrawMask call to
// the requested rectangle, so a long label cannot bleed into the sample
// column (and a wide sample cannot bleed off the right margin).
func drawClippedString(dst *image.Alpha, clip image.Rectangle, face font.Face, s string) {
	subDst, ok := dst.SubImage(clip).(draw.Image)
	if !ok {
		return
	}
	metrics := face.Metrics()
	totalH := metrics.Ascent + metrics.Descent
	topPad := (fixed.I(clip.Dy()) - totalH) / 2
	baseline := fixed.I(clip.Min.Y) + topPad + metrics.Ascent
	d := &font.Drawer{
		Dst:  subDst,
		Src:  image.Opaque,
		Face: face,
		Dot:  fixed.Point26_6{X: fixed.I(clip.Min.X), Y: baseline},
	}
	d.DrawString(s)
}

// itoa is a tiny strconv.Itoa shim kept local so the cache-key hot path does
// not depend on a package import that may be unused elsewhere.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
