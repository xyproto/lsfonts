package main

import "testing"

// TestRenderSample verifies that the first discovered font produces a
// non-empty preview mask. Skipped when no fonts are installed.
func TestRenderSample(t *testing.T) {
	entries := DiscoverFonts()
	if len(entries) == 0 {
		t.Skip("no fonts on this host")
	}
	strip := renderStrip(entries[0], 1200, 96)
	if strip == nil {
		t.Fatalf("renderStrip returned nil for %q", entries[0].Display)
	}
	var labelCov, sampleCov int
	for _, a := range strip.Label.Pix {
		if a > 0 {
			labelCov++
		}
	}
	for _, a := range strip.Sample.Pix {
		if a > 0 {
			sampleCov++
		}
	}
	if labelCov == 0 {
		t.Fatalf("label mask for %q is fully transparent", entries[0].Display)
	}
	if sampleCov == 0 {
		t.Fatalf("sample mask for %q is fully transparent", entries[0].Display)
	}
}
