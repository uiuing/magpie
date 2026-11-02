package gui

import (
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/settings"
)

func TestTrayAllReviewEdges(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	prefs := settings.Settings{Lang: "en"}
	account := func(user string, ws ...provider.QuotaWindow) provider.SubscriptionQuota {
		return provider.SubscriptionQuota{Provider: "plan", Name: "Plan", User: user, Kind: "subscription", Plan: "Pro", Windows: ws}
	}
	window := func(name string, used float64, span time.Duration) provider.QuotaWindow {
		reset := now.Add(span)
		return provider.QuotaWindow{Name: name, Used: used, Span: span, ResetsAt: &reset}
	}
	t.Run("a plan key needs no user label", func(t *testing.T) {
		q := account("", window("5 hours", 30, 5*time.Hour))
		q.Kind = "plan"
		got := trayAllCard([]provider.SubscriptionQuota{q, account("b", window("5 hours", 30, 5*time.Hour))}, "plan|Pro", now, prefs)
		if got.Balance != "30%" || strings.Contains(got.Name, "0 accounts") {
			t.Fatalf("the remaining plan disappeared: %+v", got)
		}
	})
	t.Run("finite readings survive an unlimited first account", func(t *testing.T) {
		unlimited := provider.QuotaWindow{Name: "Chat requests", Aside: true, Unlimited: true}
		finite := provider.QuotaWindow{Name: "Chat requests", Aside: true, Used: 30}
		cards := []provider.SubscriptionQuota{account("paid", unlimited), account("finite-a", finite), account("finite-b", finite)}
		for range 2 {
			if got := trayAllCard(cards, "plan|Pro", now, prefs); got.Balance != "30%" {
				t.Errorf("account order lost the finite window: %+v", got)
			}
			slices.Reverse(cards)
		}
	})
	t.Run("missing readings do not prove zero accounts", func(t *testing.T) {
		got := trayAllCard(nil, "plan|Pro", now, prefs)
		if strings.Contains(got.Name, "0 accounts") || got.Provider != "" {
			t.Fatalf("unavailable list declared empty: %+v", got)
		}
	})
	t.Run("Windows keeps the incomplete notice before truncation", func(t *testing.T) {
		q := account("good", window("5 hours", 30, 5*time.Hour), window("7 days", 50, 7*24*time.Hour))
		bad := account("unread")
		bad.Error = "offline"
		second := q
		second.User = "good-b"
		got := trayAllCard([]provider.SubscriptionQuota{q, second, bad}, "plan|Pro", now, prefs)
		u := utf16.Encode([]rune(got.Name))
		short := string(utf16.Decode(u[:min(len(u), 127)]))
		if !strings.HasPrefix(short, "Plan · All 3 accounts\nSome accounts not counted\n") {
			t.Fatalf("Windows hides the incomplete reading: %s", short)
		}
	})
	t.Run("standard windows use the selected language", func(t *testing.T) {
		q := account("a", window("5 hours", 20, 5*time.Hour))
		for lang, want := range map[string]string{"zh": "5 小时", "zh-TW": "5 小時", "ja": "5 時間", "de": "5 Stunden"} {
			s := prefs
			s.Lang = lang
			second := q
			second.User = "b"
			got := trayAllCard([]provider.SubscriptionQuota{q, second}, "plan|Pro", now, s)
			if !strings.Contains(got.Name, want) {
				t.Errorf("%s has no %q: %s", lang, want, got.Name)
			}
		}
		// every window's name as an account's tooltip says it, which is as
		// its card does (trayWindowName); a name the page has no word for
		// stays the vendor's
		for _, lang := range []string{"zh", "zh-TW", "ja", "de"} {
			s := prefs
			s.Lang = lang
			for _, name := range []string{"Month", "MCP · Month", "90 days", "12 hours", "15 minutes", "Allowance", "Vendor's Special Limit"} {
				one := account("a", window(name, 20, 0))
				two := one
				two.User = "b"
				got := trayAllCard([]provider.SubscriptionQuota{one, two}, "plan|Pro", now, s)
				_, tip := trayCardText(lang, one, now, false, false, nil)
				said := trayWindowName(lang, name)
				if !strings.Contains(got.Name, said+": ") || !strings.Contains(tip, said+" ") {
					t.Errorf("%s %q: the cell says %q, an account %q, want %q in both", lang, name, got.Name, tip, said)
				}
			}
		}
	})
}
