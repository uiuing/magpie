package gui

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/settings"
)

// trayAll names one provider's plan together: all:provider|plan.
const trayAll = "all:"

type quotaSumWindow struct {
	provider.QuotaWindow
	Accounts, Unknown, Held int
	How, Plan               string
	FirstReset              *time.Time
}

type quotaSum struct {
	Accounts, Unknown int
	Windows           []quotaSumWindow
}

// mergeQuotaAccounts follows quotaall.js's mergeQuotas. Both run the same
// fixtures: counts add, shares average within a plan, and an unread account
// is unknown. Visible aggregates require two readings of a window.
func mergeQuotaAccounts(subs []provider.SubscriptionQuota, now time.Time, minimum int) quotaSum {
	type key struct{ name, family, pool, unit, kind string }
	type merged struct {
		quotaSumWindow
		amount, limit, balance, used float64
	}
	out := quotaSum{}
	plans := map[string]bool{}
	by := map[key]*merged{}
	var order []*merged
	for _, q := range subs {
		if q.Error != "" || len(q.Windows) == 0 && q.Balance == "" {
			out.Unknown++
			continue
		}
		out.Accounts++
		plans[q.Plan] = true
		for _, w := range q.Windows {
			if w.Text {
				continue
			}
			kind := "p:" + q.Plan
			switch {
			case w.Unlimited:
				kind = "u"
			case w.Limit > 0:
				kind = "c"
			case w.Balance != nil:
				kind = "b"
			}
			k := key{w.Name, w.Family, w.Pool, w.Unit, kind}
			m := by[k]
			if m == nil {
				m = &merged{quotaSumWindow: quotaSumWindow{QuotaWindow: w, How: kind[:1], Plan: q.Plan}}
				// There is no common reset, pace, or count inferred from a
				// single account. Keep the span: the menu bar shows the
				// shortest first.
				m.ResetsAt, m.StartsAt, m.ResetSecs = nil, nil, 0
				m.Amount, m.Limit, m.Used, m.Balance = 0, 0, 0, nil
				m.Holds, m.HoldsWhy, m.HeldBy, m.Display = nil, "", "", ""
				by[k], order = m, append(order, m)
			}
			at := w.ResetsAt
			if q.AsOf != nil && at != nil && !at.After(now) {
				m.Unknown++
				continue
			}
			m.Accounts++
			held := w.HeldBy != "" && kind != "u" && kind != "b"
			if held {
				m.Held++
			}
			switch kind {
			case "c":
				amount := math.Max(0, w.Amount)
				if held {
					amount = w.Limit
				}
				m.amount, m.limit = m.amount+amount, m.limit+w.Limit
			case "b":
				m.balance += *w.Balance
			case "u":
			default:
				used := math.Max(0, math.Min(100, w.Used))
				if held {
					used = 100
				}
				m.used += used
			}
			if at != nil && at.After(now) && (m.FirstReset == nil || at.Before(*m.FirstReset)) {
				m.FirstReset = at
			}
		}
	}
	for _, m := range order {
		if m.Accounts < minimum || m.How == "u" && m.Accounts < out.Accounts {
			continue
		}
		switch m.How {
		case "c":
			m.Amount, m.Limit, m.Used = m.amount, m.limit, 100*m.amount/m.limit
		case "b":
			m.Balance = &m.balance
		case "p":
			m.Used = math.Round(10*m.used/float64(m.Accounts)) / 10
		}
		if m.How != "p" || len(plans) < 2 {
			m.Plan = ""
		}
		out.Windows = append(out.Windows, m.quotaSumWindow)
	}
	return out
}

// trayAllAccounts are a provider's cards the tray panel's strip counts as
// its accounts, as quotaall.js's quotaAccounts has them: each with
// windows, or whose read failed, but not a balance alone. Two keys of one
// name are two accounts.
func trayAllAccounts(cards []provider.SubscriptionQuota, pid string) []provider.SubscriptionQuota {
	var out []provider.SubscriptionQuota
	for _, q := range cards {
		if q.Provider == pid && (len(q.Windows) > 0 || q.Error != "" && q.Balance == "") {
			out = append(out, q)
		}
	}
	return out
}

// trayPlanAccounts is a provider's whole group on one known, selected plan.
// An account whose read failed before it named its plan (Codex's expired
// token) is one of them, unread, not another plan.
func trayPlanAccounts(cards []provider.SubscriptionQuota, pid, plan string) []provider.SubscriptionQuota {
	subs := trayAllAccounts(cards, pid)
	if plan == "" || len(subs) < 2 || slices.ContainsFunc(subs, func(q provider.SubscriptionQuota) bool { return q.Plan != plan && (q.Plan != "" || q.Error == "") }) {
		return nil
	}
	return subs
}

// trayAllCard prepares a provider|plan scope's rows and tooltip with this
// refresh's settings.
func trayAllCard(cards []provider.SubscriptionQuota, scope string, now time.Time, s settings.Settings) provider.SubscriptionQuota {
	pid, plan, ok := strings.Cut(scope, "|")
	if !ok || pid == "" || plan == "" {
		return provider.SubscriptionQuota{}
	}
	subs := trayPlanAccounts(cards, pid, plan)
	all, shown := trayAllWindows(subs, now)
	if all.Accounts < 2 || len(all.Windows) == 0 {
		return provider.SubscriptionQuota{}
	}
	q := provider.SubscriptionQuota{Provider: trayAll + scope, Name: subs[0].Name, Icon: subs[0].Icon}
	// in magpie's language, as an account's tooltip is (trayCardText)
	lang := trayLang(s.Lang, systemLang)
	text := func(key string, args ...any) string { return trayText(lang, key, args...) }
	// English and German say one apart from several
	count := func(n int, one, many string) string {
		if n == 1 {
			return text(one, n)
		}
		return text(many, n)
	}
	tip := []string{q.Name + " · " + text("All %d accounts", len(subs))}
	partialReading, earlier := all.Unknown > 0, false
	for _, sub := range subs {
		earlier = earlier || sub.AsOf != nil
	}
	var rows []string
	for _, c := range shown {
		// One row a window. Readings of it in units, or of kinds, that
		// don't add up stay apart in the tooltip.
		var group []quotaSumWindow
		for _, w := range all.Windows {
			if sameSumWindow(c, w.QuotaWindow) {
				group = append(group, w)
			}
		}
		row, read := "—", 0
		partial := all.Unknown > 0
		for _, w := range group {
			read += w.Accounts
			// a count's unit as its card says it (pageWord)
			w.Unit = pageWord(lang, w.Unit)
			figure := traySumFigure(w, s.QuotaLeft, false)
			if row == "—" {
				row = traySumFigure(w, s.QuotaLeft, true)
			}
			name := trayWindowName(lang, w.Name)
			if w.Pool != "" && !strings.HasPrefix(name, w.Pool) {
				name = w.Pool + " · " + name
			}
			if w.Plan != "" {
				name += " · " + w.Plan
			}
			if w.How == "p" || w.How == "c" {
				figure = text(map[bool]string{true: "%s left", false: "%s used"}[s.QuotaLeft], figure)
			}
			line := name + ": " + figure
			if n := w.Count(s.QuotaLeft); n != "" {
				line = name + ": " + n + " · " + figure
			}
			line += " · " + count(w.Accounts, "%d account read", "%d accounts read")
			if w.Unknown > 0 {
				partial = true
				line += " · " + count(w.Unknown, "%d expired reading left out", "%d expired readings left out")
			}
			if w.Held > 0 {
				line += " · " + text("%d held by a longer window: none left", w.Held)
			}
			if w.FirstReset != nil {
				line += " · " + text("first reset in %s", until(lang, w.FirstReset.Sub(now)))
			}
			tip = append(tip, line)
		}
		if len(group) > 1 {
			row = "—" // readings that don't add up: each is a line of the tooltip
		}
		if len(group) > 0 && (partial || read < all.Accounts) {
			partialReading = true
		}
		rows = append(rows, row)
	}
	if all.Unknown > 0 {
		tip = append(tip, text("%d couldn't be read: left out", all.Unknown))
	}
	cached := 0
	for _, sub := range subs {
		if sub.AsOf != nil {
			cached++
		}
	}
	if cached > 0 {
		tip = append(tip, count(cached, "%d account uses earlier readings", "%d accounts use earlier readings"))
	}
	if len(rows) == 0 {
		rows = []string{"—"}
		tip = append(tip, text("No comparable allowance reported"))
	}
	if len(rows) > 2 {
		rows = rows[:2]
	}
	var status []string
	if partialReading {
		status = append(status, text("Some accounts not counted"))
	}
	if earlier {
		status = append(status, text("Includes earlier readings"))
	}
	// under the name, as an account's tooltip starts with its name: within
	// what Windows shows of a tooltip (trayFigureTip)
	if len(status) > 0 {
		tip[0] += "\n" + strings.Join(status, " · ")
	}
	q.Balance, q.Name = strings.Join(rows, " · "), strings.Join(tip, "\n")
	return q
}

// trayAllWindows are the accounts' windows together (only those two of
// them read, as in the panel's strip) and, of them, the ones the cell
// shows: those an account's cell shows unpicked (trayWindows), the
// shortest first, or else what there is, a balance or an unlimited
// allowance alone, as the strip shows it.
func trayAllWindows(subs []provider.SubscriptionQuota, now time.Time) (quotaSum, []provider.QuotaWindow) {
	subs = provider.WithHeldWindows(subs, now)
	all := mergeQuotaAccounts(subs, now, 2)
	var windows []provider.QuotaWindow
	for _, sum := range all.Windows {
		if i := slices.IndexFunc(windows, func(w provider.QuotaWindow) bool { return sameSumWindow(w, sum.QuotaWindow) }); i >= 0 {
			if sum.Used > windows[i].Used {
				windows[i] = sum.QuotaWindow
			}
		} else {
			windows = append(windows, sum.QuotaWindow)
		}
	}
	shown := trayWindows(provider.SubscriptionQuota{Windows: windows})
	if len(shown) == 0 {
		shown = windows
	}
	return all, shown
}

// sumWindowName is a window's name without the pool or family trayWindows
// may have put before it.
func sumWindowName(w provider.QuotaWindow) string {
	n := strings.TrimPrefix(w.Name, w.Pool+" · ")
	return strings.TrimPrefix(n, w.Family+" · ")
}

// sameSumWindow is whether a and b are one window of the accounts', by its
// name, family and pool, whatever its unit or kind of reading.
func sameSumWindow(a, b provider.QuotaWindow) bool {
	return sumWindowName(a) == sumWindowName(b) && a.Family == b.Family && a.Pool == b.Pool
}

func traySumFigure(w quotaSumWindow, left, compact bool) string {
	if w.Unlimited {
		return "∞"
	}
	if w.Balance != nil {
		return strings.TrimSpace(fmt.Sprintf("%g %s", *w.Balance, w.Unit))
	}
	n := math.Max(0, math.Min(100, w.Used))
	if left {
		n = 100 - n
	}
	if compact {
		return fmt.Sprintf("%.0f%%", math.Round(n))
	}
	return fmt.Sprintf("%g%%", math.Round(n*10)/10)
}
