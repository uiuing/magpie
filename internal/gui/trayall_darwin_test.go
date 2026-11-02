//go:build darwin && cgo

package gui

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/settings"
)

func TestTrayAllImage(t *testing.T) {
	cards := []provider.SubscriptionQuota{
		{Provider: "claude", Name: "Claude Code", Icon: "claude-color", Plan: "Pro", User: "one", Windows: []provider.QuotaWindow{{Name: "5 hours", Used: 10}, {Name: "7 days", Used: 60}}},
		{Provider: "claude", Name: "Claude Code", Icon: "claude-color", Plan: "Pro", User: "two", Windows: []provider.QuotaWindow{{Name: "5 hours", Used: 30}, {Name: "7 days", Used: 40}}},
		{Provider: "claude", Name: "Claude Code", Icon: "claude-color", Plan: "Pro", User: "three", Windows: []provider.QuotaWindow{{Name: "5 hours", Used: 30}, {Name: "7 days", Used: 40}}},
		{Provider: "claude", Name: "Claude Code", Icon: "claude-color", Plan: "Pro", User: "four", Windows: []provider.QuotaWindow{{Name: "5 hours", Used: 30}, {Name: "7 days", Used: 40}}},
	}
	bird, err := os.ReadFile("tray.png")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"all", "partial", "separate"} {
		cards[1].Error, cards[1].Windows[0].Unit, cards[2].Windows[0].Unit = "", "", ""
		switch name {
		case "partial":
			cards[1].Error = "offline"
		case "separate":
			cards[1].Windows[0].Unit, cards[2].Windows[0].Unit = "credits", "credits"
		}
		q := trayAllCard(cards, "claude|Pro", time.Now(), settings.Settings{Lang: "zh", QuotaLeft: true})
		cells, _, _ := trayUsageView([]provider.SubscriptionQuota{q}, time.Now(), true, false)
		// readings that don't add up are no figure, and no word in the cell
		if name == "separate" && cells[0].Rows[0] != "—" {
			t.Fatalf("separate readings: %q", cells[0].Rows)
		}
		for _, dark := range []bool{false, true} {
			b, w, h := trayImagePNG(cells, bird, 22, 2, dark, true)
			img, err := png.Decode(bytes.NewReader(b))
			if err != nil || h != 44 || w < 60 || w > 250 || img.Bounds().Dx() != w {
				t.Fatalf("%s dark=%v: %dx%d %v", name, dark, w, h, err)
			}
			textX := int((trayImageCellStart(cells, 22, 0) + 17) * 2)
			if inkIn(img, image.Rect(textX, 0, w, 1), dark) != 0 || inkIn(img, image.Rect(textX, h-1, w, h), dark) != 0 {
				t.Fatalf("%s dark=%v: text touches the image's edge", name, dark)
			}
			for row := range cells[0].Rows {
				n := len(cells[0].Rows)
				if inkIn(img, image.Rect(textX, row*h/n, w, (row+1)*h/n), dark) == 0 {
					t.Fatalf("%s dark=%v: row %d has no text", name, dark, row)
				}
			}
			if out := os.Getenv("MAGPIE_TRAY_PREVIEW"); out != "" {
				file := fmt.Sprintf("all-native-%s-%t.png", name, dark)
				if err := os.WriteFile(filepath.Join(out, file), b, 0o644); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}
