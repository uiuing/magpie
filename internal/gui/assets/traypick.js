// Settings › Allowances in the menu bar: the picker of what is shown beside
// the icon. app.js imports this when Settings draws it (renderTrayUsage),
// so it and quotaall.js, which tells a plan's accounts together, weigh
// nothing on the core.
import "./quotaall.js";

// all:<provider>|<plan>: a plan's accounts together, as the panel's strip
// has them (quotaAllSummary), the cell native trayAllCard draws
const trayPlanOf = (id) => id.startsWith(ALL_ACCOUNTS) && id.includes("|") ? id.slice(id.indexOf("|") + 1) : null;
const trayPlanID = (pid, plan) => ALL_ACCOUNTS + pid + "|" + plan;
const trayGroupText = (n) => t("All {n} accounts", { n });
// the accounts an all: id stands for now; none while its plan is mixed or
// down to one account
function trayScopeCards(list, id) {
  const all = quotaAllSummary(list.filter((q) => q.provider === trayProvider(id)));
  return all?.plan === trayPlanOf(id) ? all.subs : [];
}

// the menu bar's menu waiting on the allowances: the pill drawn last opens
// it, with what that drawing keeps. Opened at the pill a redraw took away
// (another setting saved meanwhile), it came up in the window's corner and
// saved the other setting back as it was.
let trayUsageLoading = null;
window.renderTrayPick = function renderTrayPick(s, keep, ids) {
  const pill = el("button", "proto pick" + (ids.length ? " set" : ""));
  pill.type = "button";
  const paint = () => {
    const id = ids[0] || "", all = id.startsWith(ALL_ACCOUNTS);
    const card = (quotas || []).find((q) => trayCardID(q) === id || q.provider + IN_USE === id || all && q.provider === trayProvider(id));
    const total = TRAY_TOTALS.find(([v]) => v === id);
    // an aggregate not there now (down to one account, or plans mixed) is
    // still the pick, and comes back with its accounts
    const n = all && quotas ? trayScopeCards(quotas, id).length : 0;
    pill.title = all ? quotaAllHint() : "";
    pill.replaceChildren(el("span", "", !ids.length ? t("Off")
      : ids.length > 1 ? ids.some((id) => id.startsWith(ALL_ACCOUNTS) || TRAY_TOTALS.some(([v]) => v === id)) ? t("{n} shown", { n: ids.length }) : t("{n} subscriptions", { n: ids.length })
      : total ? t(total[1]) : (card ? card.name : trayProvider(id)) + (all ? " · " + (n > 1 ? trayGroupText(n) : t("All accounts")) : "")), svg(CHEV, 11, 1.6));
  };
  paint();
  if (ids.length === 1 && !quotas) loadQuotas().then(paint);
  const open = () => {
    pill.classList.remove("busy");
    paint();
    if (!pill.isConnected) return;
    // the Usage page's cards (keyCards): a key's plan and its balance are
    // one, not two rows of one name ticked together (#1658, MizuIro-H). A
    // balance left a card of its own (a stale stand-in) whose id another
    // row has already is left out: the menu bar shows the first card of an
    // id, so its row could only tick the other.
    const cards = keyCards((quotas || []).filter((q) => !q.error && (q.windows?.length || q.balance)))
      .filter((q, i, all) => all.findIndex((c) => trayCardID(c) === trayCardID(q)) === i);
    const opts = [{ v: "", name: "Off", note: "" }], grouped = new Set();
    for (const q of cards) {
      // a plan's accounts together first, an unread one too, as the
      // panel's strip counts them (quotaAllSummary)
      if (!grouped.has(q.provider)) {
        grouped.add(q.provider);
        const group = quotaAllSummary((quotas || []).filter((c) => c.provider === q.provider));
        if (group) opts.push({ v: trayPlanID(q.provider, group.plan), name: q.name, note: trayGroupText(group.subs.length), title: quotaAllHint() });
      }
      // a subscription with several accounts: the one in use, whichever
      // it is now, before each by name
      if (q.user && !opts.some((o) => o.v === q.provider + IN_USE) && cards.filter((c) => c.provider === q.provider).length > 1)
        opts.push({ v: q.provider + IN_USE, name: q.name, note: "Account in use" });
      opts.push({ v: trayCardID(q), name: q.name, note: [q.plan, q.alias, q.user].filter(Boolean).join(" · ") });
    }
    // magpie's own count of every agent's requests, as the Usage page's
    // Overview has it: tokens over cost (#1984)
    for (const [v, name] of TRAY_TOTALS) opts.push({ v, name, note: "Every agent's, as the Usage page counts them" });
    // one ticked that isn't there now (signed out, not answering, or an
    // aggregate its plan no longer has) stays to be unticked
    for (const id of ids) if (!opts.some((o) => o.v === id)) {
      if (!id.startsWith(ALL_ACCOUNTS)) opts.push({ v: id, name: id.split("|")[0], note: id.endsWith(IN_USE) ? "Account in use" : id.split("|")[1] || "" });
      else opts.push({ v: id, name: (quotas || []).find((q) => q.provider === trayProvider(id))?.name || trayProvider(id), note: trayPlanOf(id) + " · " + t("All accounts") });
    }
    if (!cards.length) opts.push({ v: "\x00", name: "No subscriptions yet", note: "Sign in to one, or add a plan's key, and it shows on the Usage page" });
    openProtoMenu(pill, opts, ids, (trayUsages) => savePrefs({ ...keep, trayUsages }), "Shown beside the icon");
  };
  pill.onclick = async (e) => {
    e.stopPropagation();
    if (pill.classList.contains("open")) return closeProtoMenu();
    if (quotas) return open();
    const loading = trayUsageLoading = { open };
    pill.classList.add("busy");
    await loadQuotas();
    if (trayUsageLoading !== loading) return;
    trayUsageLoading = null;
    loading.open();
  };
  if (trayUsageLoading) { trayUsageLoading.open = open; pill.classList.add("busy"); }
  $("#trayUsagePick").replaceChildren(pill);
};
