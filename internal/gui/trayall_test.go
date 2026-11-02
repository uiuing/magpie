package gui

import (
	"encoding/json"
	"math"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/settings"
)

func TestQuotaAllFixtures(t *testing.T) {
	var cases []struct {
		Name              string
		Minimum           int
		Cards             []provider.SubscriptionQuota
		Accounts, Unknown int
		Windows           []map[string]any
	}
	b, err := os.ReadFile("testdata/quota-all.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(b, &cases); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			before, _ := json.Marshal(c.Cards)
			got := mergeQuotaAccounts(c.Cards, now, c.Minimum)
			if got.Accounts != c.Accounts || got.Unknown != c.Unknown || len(got.Windows) != len(c.Windows) {
				t.Fatalf("accounts %d unknown %d windows %+v; want %d %d %+v", got.Accounts, got.Unknown, got.Windows, c.Accounts, c.Unknown, c.Windows)
			}
			for i, w := range got.Windows {
				b, _ := json.Marshal(w.QuotaWindow)
				var fields map[string]any
				_ = json.Unmarshal(b, &fields)
				fields["accounts"], fields["unknown"], fields["held"] = float64(w.Accounts), float64(w.Unknown), float64(w.Held)
				fields["how"], fields["plan"] = w.How, w.Plan
				if w.FirstReset != nil {
					fields["firstReset"] = w.FirstReset.Format(time.RFC3339)
				}
				if w.How != "b" && w.Balance != nil {
					t.Fatal("a counted window retained a separate balance")
				}
				if w.ResetsAt != nil || w.StartsAt != nil || w.ResetSecs != 0 {
					t.Fatal("the aggregate invented a common reset")
				}
				for key, want := range c.Windows[i] {
					if n, ok := want.(float64); ok {
						v, ok := fields[key].(float64)
						if ok && math.Abs(n-v) < 1e-9 {
							continue
						}
					}
					if !reflect.DeepEqual(fields[key], want) {
						t.Errorf("window %d %s = %v, want %v", i, key, fields[key], want)
					}
				}
			}
			after, _ := json.Marshal(c.Cards)
			if string(before) != string(after) {
				t.Fatal("changed the original account readings")
			}
		})
	}
}

func TestTrayAllEligibilityFixtures(t *testing.T) {
	var cases []struct {
		Name, Provider, Plan, Figure string
		Show                         bool
		Members                      int
		Cards                        []provider.SubscriptionQuota
	}
	b, err := os.ReadFile("testdata/quota-all-eligibility.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &cases); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			for i := range c.Cards {
				c.Cards[i].Provider = c.Provider
			}
			got := trayAllCard(c.Cards, c.Provider+"|"+c.Plan, now, settings.Settings{Lang: "en"})
			if (got.Provider != "") != c.Show || c.Figure != "" && got.Balance != c.Figure {
				t.Fatalf("aggregate %+v; want shown=%v figure=%q", got, c.Show, c.Figure)
			}
			if c.Members > 0 && len(trayPlanAccounts(c.Cards, c.Provider, c.Plan)) != c.Members {
				t.Fatalf("aggregate counts a card outside the panel's allowance group")
			}
		})
	}
}

func TestTrayAllAccounts(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	five, week := now.Add(time.Hour), now.Add(3*24*time.Hour)
	acct := func(user string, used float64) provider.SubscriptionQuota {
		return provider.SubscriptionQuota{Provider: "claude", Name: "Claude Code", Icon: "claude-color", User: user, Plan: "Pro", Kind: "subscription", Windows: []provider.QuotaWindow{
			{Name: "5 hours", Used: used, Span: 5 * time.Hour, ResetsAt: &five},
			{Name: "7 days", Used: 57, Span: 7 * 24 * time.Hour, ResetsAt: &week},
		}}
	}
	cards := []provider.SubscriptionQuota{acct("a", 30)}
	for _, user := range []string{"b", "c", "d", "e", "f", "g"} {
		cards = append(cards, acct(user, 0))
	}
	s := settings.Settings{Lang: "en", QuotaLeft: true}
	q := trayAllCard(cards, "claude|Pro", now, s)
	cells, label, tip := trayUsageView([]provider.SubscriptionQuota{q}, now, true, true)
	if label != "96% · 43%" || len(cells) != 1 || cells[0].ID != "all:claude|Pro" || cells[0].Plain || len(cells[0].Icon) == 0 {
		t.Fatalf("all accounts: %q %+v", label, cells)
	}
	for _, want := range []string{"All 7 accounts", "95.7%", "43%", "7 accounts read", "first reset"} {
		if !strings.Contains(tip, want) {
			t.Errorf("tip lacks %q: %s", want, tip)
		}
	}
	if strings.Contains(tip, "pace") {
		t.Fatalf("aggregate has a single-account pace: %s", tip)
	}
	if tip != q.Name || cells[0].Tip != q.Name {
		t.Errorf("the card's prepared tooltip was added to: %q", tip)
	}
	if got := trayAllCard(cards[:1], "claude|Pro", now, s); got.Provider != "" {
		t.Errorf("one account produced an aggregate: %+v", got)
	}
	if got := trayAllCard(nil, "claude|Pro", now, s); got.Provider != "" {
		t.Errorf("no readings produced an aggregate: %+v", got)
	}

	t.Run("mixed plans and partial reads", func(t *testing.T) {
		other := acct("other", 80)
		other.Plan = "Max"
		got := trayAllCard([]provider.SubscriptionQuota{cards[0], other}, "claude|Pro", now, s)
		if got.Provider != "" {
			t.Fatalf("mixed plans produced an aggregate: %+v", got)
		}
		other.Plan = "Pro"
		other.Error = "offline"
		got = trayAllCard([]provider.SubscriptionQuota{cards[0], other, acct("c", 30)}, "claude|Pro", now, s)
		if got.Balance != "70% · 43%" || !strings.Contains(got.Name, "1 couldn't be read") {
			t.Fatalf("partial reading: %+v", got)
		}
		first := cards[0]
		first.Error = "offline"
		if got = trayAllCard([]provider.SubscriptionQuota{first, other}, "claude|Pro", now, s); got.Provider != "" {
			t.Fatalf("all unread: %+v", got)
		}
	})
	t.Run("held and stale windows", func(t *testing.T) {
		a, b := acct("a", 0), acct("b", 100)
		a.Windows[1].Used = 100
		got := trayAllCard([]provider.SubscriptionQuota{a, b}, "claude|Pro", now, s)
		if got.Balance != "0% · 22%" || !strings.Contains(got.Name, "1 held by a longer window") {
			t.Fatalf("held: %+v", got)
		}
		past := now.Add(-time.Hour)
		a.AsOf, a.Windows[0].ResetsAt = &past, &past
		b.User = "b-copy"
		got = trayAllCard([]provider.SubscriptionQuota{a, b, acct("b", 100)}, "claude|Pro", now, s)
		if !strings.HasPrefix(got.Balance, "0%") || !strings.Contains(got.Name, "1 expired reading left out") || !strings.Contains(got.Name, "1 account uses earlier readings") {
			t.Fatalf("stale: %+v", got)
		}
	})
	t.Run("a balance alone and another provider's card are no account", func(t *testing.T) {
		wallet := cards[0]
		wallet.Windows, wallet.Kind, wallet.Balance = nil, "balance", "$10"
		other := acct("a", 99)
		other.Provider = "other"
		in := append(slices.Clone(cards), wallet, other)
		if got := trayAllCard(in, "claude|Pro", now, s); got.Balance != q.Balance || got.Name != q.Name {
			t.Fatalf("counted a balance or crossed provider: %+v", got)
		}
	})
	if got := quotaView("all:claude|Pro"); got != "usage&tab=usage&provider=claude" {
		t.Errorf("window destination: %s", got)
	}
	if got := panelQuotaJS("all:claude|Pro"); got != `panelQuotaFocus("all:claude|Pro")` {
		t.Errorf("panel destination: %s", got)
	}
}

func TestTrayAllFamiliesUseMergedShares(t *testing.T) {
	cards := []provider.SubscriptionQuota{
		{Provider: "agy", Plan: "Standard", User: "a", Windows: []provider.QuotaWindow{
			{Name: "Model A", Model: "a", Family: "Gemini", Used: 90},
			{Name: "Model B", Model: "b", Family: "Gemini", Used: 10},
		}},
		{Provider: "agy", Plan: "Standard", User: "b", Windows: []provider.QuotaWindow{
			{Name: "Model A", Model: "a", Family: "Gemini", Used: 0},
			{Name: "Model B", Model: "b", Family: "Gemini", Used: 100},
		}},
	}
	got := trayAllCard(cards, "agy|Standard", time.Now(), settings.Settings{Lang: "en"})
	if got.Balance != "55%" || !strings.Contains(got.Name, "Model B: 55%") {
		t.Fatalf("picked the first account's tightest model, not the aggregate's: %+v", got)
	}
}

// The menu bar's cards for settings.TrayUsages: a plan's accounts together
// is made from the cards its accounts' cells are, in the order picked, and
// one with no such card now (here a plan none of them is on) is left out,
// as an account signed out is. With no logo its cell has the letter an
// account's has, its name's, and its tooltip starts with that name.
func TestTrayUsageCardsMakeThePlansCard(t *testing.T) {
	now := time.Now()
	acct := func(user string, used float64) provider.SubscriptionQuota {
		return provider.SubscriptionQuota{Provider: "zhipu-plan", Name: "GLM Coding Plan", User: user, Plan: "pro", Windows: []provider.QuotaWindow{{Name: "5 hours", Span: 5 * time.Hour, Used: used}}}
	}
	unread := acct("c", 0)
	unread.Windows, unread.Error = nil, "offline"
	cards := []provider.SubscriptionQuota{acct("a", 20), acct("b", 60), unread}
	s := settings.Settings{Lang: "en", TrayUsages: []string{"zhipu-plan|b", "all:zhipu-plan|pro", "all:zhipu-plan|max"}}
	got := trayPick(withTrayAll(slices.Clip(cards), s, now), s.TrayUsages, now)
	if len(got) != 2 || trayCardID(got[0]) != "zhipu-plan|b" || got[1].Provider != "all:zhipu-plan|pro" || got[1].Balance != "40%" {
		t.Fatalf("picked %+v", got)
	}
	cells, _, _ := trayUsageView(got, now, false, false)
	if len(cells) != 2 || cells[1].ID != "all:zhipu-plan|pro" || cells[0].Letter != "G" || cells[1].Letter != "G" ||
		!strings.HasPrefix(cells[1].Tip, "GLM Coding Plan · All 3 accounts\nSome accounts not counted\n") {
		t.Fatalf("cells %+v", cells)
	}
}

func TestTrayAllSettingKeepsItsScope(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	ids := []string{"all:claude|Pro", "claude|*", "claude|all", "usage:today"}
	if err := settings.Save(settings.Settings{TrayUsages: ids}); err != nil {
		t.Fatal(err)
	}
	if got := settings.Load(); !slices.Equal(got.TrayUsages, ids) || got.TrayUsage != ids[0] {
		t.Fatalf("lost aggregate selection: %+v", got.TrayUsages)
	}
}
