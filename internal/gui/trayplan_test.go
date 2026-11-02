package gui

import (
	"strings"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/settings"
)

// A plan pick stays on that plan as other plans arrive, including key plans
// and plugins. Counts still weigh each account's actual limit.
func TestTrayPlanSelection(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		provider, plan, other, kind string
		windows                     []provider.QuotaWindow
		want                        string
	}{
		{"codex", "plus", "prolite", "subscription", []provider.QuotaWindow{{Name: "5 hours", Used: 20}, {Name: "5 hours", Used: 60}}, "40%"},
		{"copilot", "Business", "Education", "subscription", []provider.QuotaWindow{{Name: "Premium requests", Amount: 200, Limit: 1000, Unit: "requests"}, {Name: "Premium requests", Amount: 150, Limit: 300, Unit: "requests"}}, "27%"},
		{"gemini", "Google AI Pro", "Google AI Ultra", "subscription", []provider.QuotaWindow{{Name: "Gemini Pro", Family: "Gemini", Model: "gemini-pro", Used: 20}, {Name: "Gemini Pro", Family: "Gemini", Model: "gemini-pro", Used: 60}}, "40%"},
		{"zhipu-plan", "pro", "max", "plan", []provider.QuotaWindow{{Name: "5 hours", Amount: 20, Limit: 100, Unit: "tokens"}, {Name: "5 hours", Amount: 60, Limit: 100, Unit: "tokens"}}, "40%"},
		{"fakeco", "Fake Pro", "Fake Team", "subscription", []provider.QuotaWindow{{Name: "Weekly", Used: 20}, {Name: "Weekly", Used: 60}}, "40%"},
		{"claude", "pro", "max", "subscription", []provider.QuotaWindow{{Name: "5 hours", Used: 20}, {Name: "5 hours", Used: 60}}, "40%"},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			cards := []provider.SubscriptionQuota{
				{Provider: tc.provider, Name: tc.provider, Plan: tc.plan, Kind: tc.kind, User: "a", Windows: tc.windows[:1]},
				{Provider: tc.provider, Name: tc.provider, Plan: tc.plan, Kind: tc.kind, User: "b", Windows: tc.windows[1:]},
			}
			if tc.kind == "plan" {
				cards[0].User = "" // a plan's last key need not have a user label
			}
			scope := tc.provider + "|" + tc.plan
			prefs := settings.Settings{Lang: "en"}
			before := trayAllCard(cards, scope, now, prefs)
			if before.Balance != tc.want || before.Provider != trayAll+scope || !strings.Contains(before.Name, "All 2 accounts") {
				t.Fatalf("selected plan: %+v", before)
			}
			other := provider.SubscriptionQuota{Provider: tc.provider, Name: tc.provider, Plan: tc.other, Kind: tc.kind, User: "other", Windows: []provider.QuotaWindow{{Name: tc.windows[0].Name, Used: 99}}}
			cards = append(cards, other)
			if got := trayAllCard(cards, scope, now, prefs); got.Provider != "" {
				t.Fatalf("mixed plans still produced an aggregate: %+v", got)
			}
			cards = cards[:2]
			unread := provider.SubscriptionQuota{Provider: tc.provider, Name: tc.provider, Plan: tc.plan, Kind: tc.kind, User: "unread", Error: "offline"}
			cards = append(cards, unread)
			if got := trayAllCard(cards, scope, now, prefs); got.Balance != tc.want || !strings.HasPrefix(got.Name, tc.provider+" · All 3 accounts\nSome accounts not counted\n") || !strings.Contains(got.Name, "1 couldn't be read") {
				t.Fatalf("the plan's unread account was lost: %+v", got)
			}
			cards[2].Plan = "" // Codex's expired token: the read fails before the plan is known
			if got := trayAllCard(cards, scope, now, prefs); got.Balance != tc.want || !strings.Contains(got.Name, "All 3 accounts") || !strings.Contains(got.Name, "1 couldn't be read") {
				t.Fatalf("an unread account with no plan hid the aggregate: %+v", got)
			}
			if got := trayAllCard([]provider.SubscriptionQuota{other}, scope, now, prefs); got.Provider != "" {
				t.Fatalf("a missing plan followed another one: %+v", got)
			}
		})
	}
}

func TestTrayAllRequiresOneSharedPlan(t *testing.T) {
	for _, pid := range []string{"codex", "copilot", "gemini", "zhipu-plan", "fakeco"} {
		for _, plans := range [][]string{nil, {"pro"}, {"free", "pro"}, {"pro", "pro", "free"}, {"", ""}, {"pro", ""}, {"pro", "pro"}} {
			var cards []provider.SubscriptionQuota
			for i, plan := range plans {
				cards = append(cards, provider.SubscriptionQuota{Provider: pid, Plan: plan, User: string(rune('a' + i)), Windows: []provider.QuotaWindow{{Name: "5 hours", Used: 30}}})
			}
			got := trayAllCard(cards, pid+"|pro", time.Now(), settings.Settings{})
			want := len(plans) == 2 && plans[0] == "pro" && plans[1] == "pro"
			if (got.Provider != "") != want {
				t.Errorf("%s plans=%v: aggregate present=%v, want %v", pid, plans, got.Provider != "", want)
			}
		}
	}
}

// A balance or an unlimited allowance alone is the cell's figure, as the
// panel's strip has it; beside a window that stops the account it is left
// out, as an account's cell leaves it out.
func TestTrayAllNonBlockingWindows(t *testing.T) {
	balance := 15.0
	for _, tc := range []struct {
		name string
		w    provider.QuotaWindow
		want string
	}{
		{"balance", provider.QuotaWindow{Name: "Credits", Balance: &balance, Unit: "USD", Aside: true}, "30 USD"},
		{"unlimited", provider.QuotaWindow{Name: "Chat requests", Unlimited: true, Aside: true}, "∞"},
	} {
		cards := []provider.SubscriptionQuota{
			{Provider: "fakeco", Plan: "Pro", User: "a", Windows: []provider.QuotaWindow{tc.w}},
			{Provider: "fakeco", Plan: "Pro", User: "b", Windows: []provider.QuotaWindow{tc.w}},
		}
		if got := trayAllCard(cards, "fakeco|Pro", time.Now(), settings.Settings{}); got.Balance != tc.want {
			t.Errorf("%s: native aggregate %q, panel figure %q", tc.name, got.Balance, tc.want)
		}
		for i := range cards {
			cards[i].Windows = append(cards[i].Windows, provider.QuotaWindow{Name: "Weekly", Used: 40})
		}
		if got := trayAllCard(cards, "fakeco|Pro", time.Now(), settings.Settings{}); got.Balance != "40%" {
			t.Errorf("%s beside a weekly window: %q, want the weekly 40%%", tc.name, got.Balance)
		}
	}
}

func TestTrayUnknownPlanIsNotOnePlan(t *testing.T) {
	for _, pid := range []string{"ollama-plan", "minimax-plan", "fakeco"} {
		cards := []provider.SubscriptionQuota{
			{Provider: pid, Name: pid, Kind: "plan", User: "a", Windows: []provider.QuotaWindow{{Name: "Monthly", Amount: 10, Limit: 20, Unit: "USD"}}},
			{Provider: pid, Name: pid, Kind: "plan", User: "b", Windows: []provider.QuotaWindow{{Name: "Monthly", Amount: 10, Limit: 100, Unit: "USD"}}},
		}
		for _, named := range []bool{false, true} {
			if named {
				cards[0].Plan = "Pro"
			}
			if got := trayPlanAccounts(cards, pid, ""); len(got) != 0 {
				t.Errorf("%s named=%v: unknown plans combined: %+v", pid, named, got)
			}
			if named {
				if got := trayPlanAccounts(cards, pid, "Pro"); len(got) != 0 {
					t.Errorf("%s: known plan included an unknown account: %+v", pid, got)
				}
			}
		}
	}
}

// The windows picked for a provider's account cells (settings.TrayWindows)
// are theirs: the plan's cell beside one shows the windows an account's
// shows unpicked, the shortest first, whatever the account's pick.
func TestTrayPlanWindowsStayIndependent(t *testing.T) {
	now := time.Now()
	cards := []provider.SubscriptionQuota{
		{Provider: "codex", Plan: "plus", User: "a", Windows: []provider.QuotaWindow{{Name: "7 days", Span: 7 * 24 * time.Hour, Used: 20}, {Name: "5 hours", Span: 5 * time.Hour, Used: 10}}},
		{Provider: "codex", Plan: "plus", User: "b", Windows: []provider.QuotaWindow{{Name: "7 days", Span: 7 * 24 * time.Hour, Used: 20}, {Name: "5 hours", Span: 5 * time.Hour, Used: 10}}},
	}
	for _, tc := range []struct {
		picked  []string
		account string
	}{{nil, "10% · 20%"}, {[]string{"5h"}, "10%"}, {[]string{"7d"}, "20%"}} {
		prefs := settings.Settings{TrayWindows: map[string][]string{"codex": tc.picked}}
		all := trayAllCard(cards, "codex|plus", now, prefs)
		cells, _, _ := trayCardsView("en", []provider.SubscriptionQuota{all, cards[0]}, now, false, false, prefs.TrayWindows)
		if len(cells) != 2 || strings.Join(cells[0].Rows, " · ") != "10% · 20%" || strings.Join(cells[1].Rows, " · ") != tc.account {
			t.Fatalf("account windows %v: the plan's cell %+v", tc.picked, cells)
		}
	}
}

func TestTrayPlanNamesKeepTheirIdentity(t *testing.T) {
	for _, plan := range []string{"Team | Pro / α", "*"} {
		card := provider.SubscriptionQuota{Provider: "fakeco", Name: "FakeCo", Kind: "subscription", Plan: plan, User: "a", Windows: []provider.QuotaWindow{{Name: "Weekly", Used: 30}}}
		second := card
		second.User = "b"
		q := trayAllCard([]provider.SubscriptionQuota{card, second}, "fakeco|"+plan, time.Now(), settings.Settings{})
		id := "all:fakeco|" + plan
		picked := trayPick([]provider.SubscriptionQuota{q}, []string{id}, time.Now())
		if len(picked) != 1 || picked[0].Provider != id || picked[0].Balance != "30%" {
			t.Fatalf("plan %q lost its selection: %+v", plan, picked)
		}
		if got := quotaView(id); got != "usage&tab=usage&provider=fakeco" {
			t.Fatalf("plan %q became an account link: %s", plan, got)
		}
	}
}
