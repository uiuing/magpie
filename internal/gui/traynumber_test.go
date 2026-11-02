//go:build !nogui

package gui

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

// Windows' tray icon is the first card's figure (Benjaminliang_1 on X),
// the percentage as the Mac's menu bar shows it, used or left; the
// next card's when the first has none; the bird with no cards, off
// Windows, or with Settings › Tray icon number off.
func TestTrayIconFigure(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	cards := trayCards()
	view := func(ids []string, left bool, pick map[string][]string) []trayCell {
		cells, _, _ := trayCardsView("en", trayPick(cards, ids, now), now, left, false, pick)
		return cells
	}
	for _, tc := range []struct {
		name     string
		goos     string
		noNumber bool
		cells    []trayCell
		want     string
	}{
		{"the first card's first window", "windows", false, view([]string{"claude|a@b.c", "codex|x@y.z"}, false, nil), "42"},
		{"what is left, as Settings has it", "windows", false, view([]string{"claude|a@b.c"}, true, nil), "58"},
		{"the window picked for the menu bar", "windows", false, view([]string{"claude|a@b.c"}, false, map[string][]string{"claude": {"7d"}}), "18"},
		{"a full window", "windows", false, view([]string{"codex|x@y.z"}, false, map[string][]string{"codex": {"7d"}}), "100"},
		{"a balance first: the next card's", "windows", false, view([]string{"deepseek", "copilot"}, false, nil), "63"},
		{"a balance alone: the bird", "windows", false, view([]string{"deepseek"}, false, nil), ""},
		{"no cards: the bird", "windows", false, view(nil, false, nil), ""},
		{"turned off: the bird", "windows", true, view([]string{"claude|a@b.c"}, false, nil), ""},
		{"the Mac draws its cells", "darwin", false, view([]string{"claude|a@b.c"}, false, nil), ""},
		{"Linux shows the label", "linux", false, view([]string{"claude|a@b.c"}, false, nil), ""},
		{"with Pace beside it", "windows", false, []trayCell{{Rows: []string{"42% +3%"}}}, "42"},
		{"an aggregate with unread accounts", "windows", false, []trayCell{{ID: "all:claude|pro", Rows: []string{"70%"}}}, "70"},
		{"unavailable plan readings", "windows", false, []trayCell{{ID: "all:claude|pro", Rows: []string{"—"}}}, ""},
		{"a balance's own text", "windows", false, []trayCell{{Rows: []string{"1234%x"}}, {Rows: []string{"Σ1.2M"}}}, ""},
	} {
		if got := trayIconFigure(tc.goos, tc.noNumber, tc.cells); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}

// The icon's figure can be a plan's accounts together after a balance or
// a ledger card: its tooltip comes first, so what it says of accounts left
// out or read earlier is within what Windows shows of a tooltip (127
// UTF-16 units), with or without logos.
func TestTrayFigureKeepsAggregateStatus(t *testing.T) {
	for _, status := range []string{"Some accounts not counted", "Includes earlier readings"} {
		first := strings.Repeat("账", 150)
		allTip := "Plan · All 2 accounts\n" + status + "\n5 hours: 30%"
		last := "Claude Code\n5 hours 10% used"
		cells := []trayCell{{ID: "usage:today", Rows: []string{"Σ1M"}, Tip: first}, {ID: "all:plan", Rows: []string{"30%"}, Tip: allTip}, {ID: "claude|a", Rows: []string{"10%"}, Tip: last}}
		for _, view := range [][]trayCell{cells, trayPlain(cells)} {
			if trayFigure(view) != "30" {
				t.Fatal("aggregate's numeric icon disappeared")
			}
			tip := trayFigureTip(view, first+"\n\n"+allTip+"\n\n"+last)
			if want := allTip + "\n\n" + first + "\n\n" + last; tip != want {
				t.Fatalf("the others' tips moved or left a gap: %q", tip)
			}
			u := utf16.Encode([]rune(tip))
			short := string(utf16.Decode(u[:min(len(u), 127)]))
			if !strings.Contains(short, status) || !strings.Contains(short, "30%") || strings.Count(tip, allTip) != 1 {
				t.Fatalf("the icon lost its explanation: %q", short)
			}
		}
	}
}

// The figure is drawn at the size Windows asks for, in the taskbar's text
// colour on transparency, as tall as fits with room above and below, and
// "100" narrowed into the square, not cut.
func TestTrayNumberIcon(t *testing.T) {
	// off by more than the rounding of a half-covered pixel
	far := func(a, b uint8) bool { d := int(a) - int(b); return d < -4 || d > 4 }
	for _, size := range []int{16, 20, 24, 32} {
		for _, dark := range []bool{true, false} {
			for _, fig := range []string{"7", "42", "100"} {
				b := trayNumberIcon(fig, size, dark)
				img, err := png.Decode(bytes.NewReader(b))
				if err != nil {
					t.Fatalf("%s at %d: %v", fig, size, err)
				}
				if img.Bounds() != image.Rect(0, 0, size, size) {
					t.Fatalf("%s at %d: %v", fig, size, img.Bounds())
				}
				want := trayNumberColour(dark)
				// white on a dark taskbar, near black on a light one
				if lum := int(want.R) + int(want.G) + int(want.B); dark != (lum > 3*200) || !dark && lum > 3*64 || want.A != 0xff {
					t.Fatalf("dark=%v: drawn in %v", dark, want)
				}
				ink := image.Rectangle{}
				for y := range size {
					for x := range size {
						c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
						if c.A < 0x80 {
							continue
						}
						if far(c.R, want.R) || far(c.G, want.G) || far(c.B, want.B) {
							t.Fatalf("%s at %d dark=%v: (%d,%d) is %v, want %v", fig, size, dark, x, y, c, want)
						}
						ink = ink.Union(image.Rect(x, y, x+1, y+1))
					}
				}
				if ink.Empty() {
					t.Fatalf("%s at %d: nothing drawn", fig, size)
				}
				// legible: the digits most of the square's height, a pixel
				// or more clear of its top and bottom
				if h := ink.Dy(); h < size*6/10 || ink.Min.Y < 1 || ink.Max.Y > size-1 {
					t.Errorf("%s at %d: ink %v", fig, size, ink)
				}
				// centred, and three digits fill the width without leaving it
				if d := ink.Min.X - (size - ink.Max.X); d < -2 || d > 2 {
					t.Errorf("%s at %d: off centre, ink %v", fig, size, ink)
				}
				if fig == "100" && ink.Dx() < size*8/10 {
					t.Errorf("100 at %d: %d wide", size, ink.Dx())
				}
			}
		}
	}
	if bytes.Equal(trayNumberIcon("42", 16, true), trayNumberIcon("17", 16, true)) {
		t.Fatal("42 and 17 drawn alike")
	}
	if trayNumberIcon("", 16, true) != nil || trayNumberIcon("42", 0, true) != nil {
		t.Fatal("drawn with no figure or no size")
	}
}
