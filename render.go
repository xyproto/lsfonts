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

// ASCII-only so fonts without extended coverage still preview usefully.
const sampleText = "AaBbCc 0123 The quick brown fox jumps over the lazy dog"

// stripCache: "path|index|pixW|pixH" -> *cardStrip.
var stripCache sync.Map

// cardStrip holds separate label/sample masks so the UI can tint each one
// independently (dim header, bright sample).
type cardStrip struct {
	Label  *image.Alpha
	Sample *image.Alpha
}

// parsedFontCache: "path|index" -> *opentype.Font. Parsing TTCs is slow.
var parsedFontCache sync.Map

// loadParsedFont parses entry's face (handles TTC), cached. nil on failure.
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

// renderStrip rasterizes a card preview (label on top, sample below) into
// two alpha masks of size pixW x pixH. nil when the font cannot be loaded.
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

	padX := pixH / 5
	padTop := pixH / 8
	padBot := pixH / 10
	gap := pixH / 14

	labelH := max(pixH*28/100, 12)
	sampleY0 := padTop + labelH + gap
	sampleH := pixH - padBot - sampleY0
	if sampleH < labelH {
		// Unusually short card: split evenly so neither row is squashed.
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

	// UI font keeps the label readable for symbol/icon faces. Falls back
	// to the entry's own face when no UI font is found.
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

// drawClippedString paints s into clip on dst. SubImage clips font.Drawer's
// per-glyph DrawMask call so overflow can't bleed outside the rect.
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

// itoa is a local strconv.Itoa shim used by the cache-key hot path.
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
