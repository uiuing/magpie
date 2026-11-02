// Settings › Allowances' All N accounts is one pick, not a tick of each
// account, and the menu bar cell it makes opens its provider's strip in the
// panel, no account unfolded.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");
const { openPart } = require("./reader.cjs");
const assets = path.resolve(__dirname, "../assets");
const words = {
  en: (n) => `All ${n} accounts`,
  zh: (n) => `全部 ${n} 个账号`,
  "zh-TW": (n) => `全部 ${n} 個帳號`,
  ja: (n) => `全 ${n} アカウント`,
  de: (n) => `Alle ${n} Konten`,
};
// an aggregate picked but not there now: its row in the menu, to untick
const gone = { en: "All accounts", zh: "全部账号", "zh-TW": "全部帳號", ja: "すべてのアカウント", de: "Alle Konten" };
function fixture(lang) {
  const five = { name: "5 hours", used: 30, trayKind: "5h" };
  const cards = [
    { provider: "claude", name: "Claude Code", icon: "claude-color", kind: "subscription", plan: "Pro", user: "one@example.com", windows: [five, { name: "7 days", used: 60, trayKind: "7d" }] },
    { provider: "claude", name: "Claude Code", icon: "claude-color", kind: "subscription", plan: "Pro", user: "two@example.com", windows: [{ ...five, used: 70 }, { name: "7 days", used: 60, trayKind: "7d" }] },
    { provider: "claude", name: "Claude Code", icon: "claude-color", kind: "subscription", plan: "Pro", user: "unread@example.com", error: "offline", windows: [] },
  ];
  const state = { cards, posts: [], settings: {
    lang, theme: "light", tray: "panel", trayUsages: [], trayUsageEvery: 3,
    quotaLeft: true, currency: "usd", version: "dev", visionModels: [], imageGenModels: [],
    fx: { rate: 7.2, stale: false }, panelUsageHidden: ["claude"],
  } };
  state.serve = async (route) => {
    const req = route.request(), u = new URL(req.url());
    const json = (data) => route.fulfill({ json: data });
    if (u.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = ${JSON.stringify({ lang, theme: "light", web: false })};` });
    if (u.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (u.pathname === "/api/state") return json({ agents: [], profiles: [], settings: state.settings });
    if (u.pathname === "/api/settings") {
      if (req.method() === "POST") {
        const body = req.postDataJSON();
        state.posts.push(body);
        state.settings = { ...state.settings, ...body, trayUsage: body.trayUsages?.[0] || "" };
      }
      return json(state.settings);
    }
    if (u.pathname === "/api/usage/quotas") return json(state.cards);
    if (u.pathname === "/api/providers") return json({ providers: [], presets: [], excluded: [], gateway: { running: true, window: true } });
    if (u.pathname === "/api/groups") return json({ groups: [], models: [] });
    if (u.pathname === "/api/plugins") return json({ plugins: [] });
    if (u.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, u.pathname === "/" ? "index.html" : u.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    try { await route.fulfill({ body: await fs.readFile(file), contentType }); } catch { await route.fulfill({ status: 404, body: "" }); }
  };
  return state;
}
// Keep the refresh promise on the page while waiting on its resulting state.
// CDP holds evaluate's returned promises weakly; a collection during a refresh
// must not end the test before the new cards and Settings have settled.
async function refresh(page, cards) {
  await page.waitForFunction(() => !quotasLoading);
  await page.evaluate(() => {
    window.__trayReadDone = false;
    window.__trayRead = loadQuotas().then(() => { renderSettings(); window.__trayReadDone = true; });
  });
  await page.waitForFunction((expected) => window.__trayReadDone && JSON.stringify(quotas) === expected, JSON.stringify(cards));
}
const shot = async (page, name) => {
  if (!process.env.ARTIFACT_DIR) return;
  await fs.mkdir(process.env.ARTIFACT_DIR, { recursive: true });
  await page.screenshot({ path: path.join(process.env.ARTIFACT_DIR, name + ".png") });
};
for (const engine of process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"]) {
  for (const [lang, w] of Object.entries(words)) {
    test(`${engine} ${lang}: aggregate choice survives account changes and opens the group`, async (t) => {
      const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
      t.after(() => browser.close());
      const context = await browser.newContext({ viewport: { width: 900, height: 760 }, reducedMotion: "reduce" });
      const page = await context.newPage(), f = fixture(lang), errors = [];
      page.on("pageerror", (e) => errors.push(e.message));
      page.setDefaultTimeout(8000);
      page.setDefaultNavigationTimeout(60000);
      await page.route("**/*", f.serve);
      await page.goto("http://magpie.test/?view=usage");
      await page.locator("#prefs").click();
      await openPart(page, "usage");
      const pill = page.locator("#trayUsagePick button");
      await pill.waitFor();
      await pill.scrollIntoViewIfNeeded();
      const top = await page.locator("#view-settings").evaluate((e) => e.scrollTop);
      await pill.click();
      const all = page.locator(".proto-menu .pm-item").filter({ has: page.locator(".pm-note", { hasText: w(3) }) });
      await all.waitFor();
      assert.equal(await all.count(), 1);
      assert.equal(await all.getAttribute("title"), await page.evaluate(() => quotaAll(quotas).querySelector(".quota-all-head").title));
      await shot(page, `all-menu-${engine}-${lang}`);
      await all.click();
      await page.keyboard.press("Escape");
      await page.waitForFunction((label) => document.querySelector("#trayUsagePick button")?.textContent.includes(label), w(3));
      assert.deepEqual(f.posts.at(-1).trayUsages, ["all:claude|Pro"]);
      assert.equal(await page.locator("#view-settings").evaluate((e) => e.scrollTop), top, "saving scrolled Settings");
      // An account ticked beside it is its own cell; All never stands for
      // each account's id.
      await pill.click();
      assert.equal(await all.getAttribute("aria-checked"), "true");
      await page.locator(".proto-menu .pm-item").filter({ hasText: "one@example.com" }).click();
      await page.keyboard.press("Escape");
      await page.waitForFunction(() => prefs.trayUsages.length === 2);
      assert.deepEqual(f.posts.at(-1).trayUsages, ["all:claude|Pro", "claude|one@example.com"]);
      // the windows picker is the account's alone: the cell of them all
      // shows the windows an account's shows unpicked
      await page.locator("#trayWindowsPick button").waitFor();
      assert.equal(await page.locator("#trayWindowsPick button").count(), 1);
      await pill.click();
      await page.locator(".proto-menu .pm-item").filter({ hasText: "one@example.com" }).click();
      await page.keyboard.press("Escape");
      await page.waitForFunction(() => prefs.trayUsages.length === 1);
      for (const count of [1, 0, 3]) {
        f.cards = fixture(lang).cards.slice(0, count);
        if (count === 1) {
          f.cards[0].user = ""; f.cards[0].kind = "plan";
          f.cards.push({ provider: "claude", name: "Claude Code", kind: "balance", error: "offline", windows: [] });
        }
        await refresh(page, f.cards);
        await pill.click();
        const pick = page.locator(".proto-menu .pm-item").filter({ has: page.getByText(w(count), { exact: true }) });
        if (count > 1) assert.equal(await pick.getAttribute("aria-checked"), "true", "the eligible aggregate lost its selection");
        else {
          assert.equal(await pick.count(), 0, "a single or missing account still has an aggregate choice");
          const kept = page.locator(".proto-menu .pm-item").filter({ has: page.getByText(`Pro · ${gone[lang]}`, { exact: true }) });
          assert.equal(await kept.getAttribute("aria-checked"), "true", "the pick not there now can't be unticked");
        }
        await page.keyboard.press("Escape");
      }
      await shot(page, `all-settings-${engine}-${lang}`);

      // Other providers put the requested group below the viewport. Their
      // lazy aggregate strips must settle before the native click scrolls.
      const earlier = Array.from({ length: 4 }, (_, i) => [0, 1].map((j) => ({
        provider: `earlier-${i}`, name: `Earlier ${i}`, user: `account-${j}`, plan: "Pro", icon: "generic",
        windows: [{ name: "5 hours", used: 20 }],
      }))).flat();
      f.cards = [...earlier, ...fixture(lang).cards];
      const panel = await context.newPage();
      panel.setDefaultTimeout(8000);
      panel.setDefaultNavigationTimeout(60000);
      panel.on("pageerror", (e) => errors.push(e.message));
      await panel.setViewportSize({ width: 360, height: 760 });
      await panel.route("**/*", f.serve);
      await panel.addInitScript(() => {
        window.__scrolls = [];
        const scroll = Element.prototype.scrollIntoView;
        Element.prototype.scrollIntoView = function (opts) { window.__scrolls.push(this.closest("[data-provider]")?.dataset.provider); scroll.call(this, opts); };
      });
      await panel.goto("http://magpie.test/?mode=panel");
      await panel.waitForFunction((n) => typeof panelQuotaFocus === "function" && quotas?.length === n && state.settings, f.cards.length);
      const before = await panel.evaluate(() => localStorage.getItem("magpie.usageAccounts"));
      await panel.evaluate(() => panelQuotaFocus("all:claude|Pro"));
      await panel.waitForFunction(() => window.__scrolls.includes("claude"));
      const group = panel.locator('#panelQuota .pq-group[data-provider="claude"]');
      await group.waitFor();
      await panel.waitForFunction(() => {
        const heading = document.querySelector('#panelQuota [data-provider="claude"] .quota-all');
        if (!heading) return false;
        const r = heading.getBoundingClientRect(), v = heading.closest(".view").getBoundingClientRect();
        return r.top >= v.top - 1 && r.bottom <= v.bottom + 1;
      });
      assert.equal(await panel.evaluate(() => localStorage.getItem("magpie.usageAccounts")), before, "All expanded individual accounts");
      assert.deepEqual(await panel.evaluate(() => state.settings.panelUsageHidden), ["claude"], "peek rewrote hidden groups");
      assert.ok(await panel.evaluate(() => document.documentElement.scrollWidth <= innerWidth), "panel spills sideways");
      await shot(panel, `all-panel-${engine}-${lang}`);
      assert.deepEqual(errors, []);
    });

    test(`${engine} ${lang}: a cell's click opens the strip, drawn late or drawn again`, async (t) => {
      const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
      t.after(() => browser.close());
      // no reduced motion: it makes the flash a frame long (tray-cell-click)
      const context = await browser.newContext({ viewport: { width: 360, height: 300 } });
      const page = await context.newPage(), f = fixture(lang), errors = [];
      page.on("pageerror", (e) => errors.push(e.message));
      page.setDefaultTimeout(8000);
      page.setDefaultNavigationTimeout(60000);
      // others above it, and its accounts open: taller than the panel
      f.cards = [...Array.from({ length: 4 }, (_, i) => [0, 1].map((j) => ({
        provider: `earlier-${i}`, name: `Earlier ${i}`, user: `account-${j}`, plan: "Pro", icon: "generic",
        windows: [{ name: "5 hours", used: 20 }],
      }))).flat(), ...f.cards];
      await page.addInitScript(() => localStorage.setItem("magpie.usageAccounts", JSON.stringify({ "claude\none@example.com": true, "claude\ntwo@example.com": true })));
      // every strip held back, so the click comes before any is there
      let release;
      const held = new Promise((resolve) => { release = resolve; });
      await page.route("**/*", f.serve);
      await page.route("**/quotaall.js", async (route) => { await held; await route.fallback(); });
      await page.goto("http://magpie.test/?mode=panel");
      await page.waitForFunction((n) => typeof panelQuotaFocus === "function" && quotas?.length === n && state.settings && !!quotaAllJS.p, f.cards.length);
      const group = '#panelQuota .pq-group[data-provider="claude"]', strip = group + " .quota-all";
      const inView = () => page.waitForFunction((s) => {
        const e = document.querySelector(s);
        if (!e) return false;
        const r = e.getBoundingClientRect(), v = e.closest(".view").getBoundingClientRect();
        return r.top >= v.top - 1 && r.bottom <= v.bottom + 1;
      }, strip);
      await page.evaluate(() => panelQuotaFocus("all:claude|Pro"));
      await page.evaluate(() => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r))));
      assert.equal(await page.locator("#panelQuota .flash").count(), 0, "the click was spent before the strip was there");
      release();
      // the highlight held where it is, whatever the machine's speed
      await page.waitForFunction((s) => {
        const e = document.querySelector(s), a = e?.getAnimations().find((x) => x.animationName === "flash");
        if (!a) return false;
        a.pause();
        a.currentTime = 600;
        window.__strip = e;
        return true;
      }, strip);
      await inView();
      // the strip is lit in its own corners, not the group, which has none
      assert.equal(await page.locator("#panelQuota .flash").count(), 1, "more than the strip is lit");
      assert(await page.locator(strip).evaluate((e) => parseFloat(getComputedStyle(e).borderTopLeftRadius) > 0), "the strip lost its corners");
      // a focus refresh draws the cards again: the highlight and the place go on
      const top = await page.locator(group).evaluate((e) => e.closest(".view").scrollTop);
      await page.evaluate(() => renderPanelQuota());
      assert(await page.locator(strip).evaluate((e) => e !== window.__strip && e.getAnimations().find((x) => x.animationName === "flash")?.currentTime >= 600), "the redraw lost the highlight");
      assert.equal(await page.locator(group).evaluate((e) => e.closest(".view").scrollTop), top, "the redraw moved the panel");
      // past the head of a group taller than the panel: the strip comes back,
      // not the group's nearest edge
      assert(await page.locator(group).evaluate((e) => e.offsetHeight > e.closest(".view").clientHeight), "the group fits, so this tests nothing");
      await page.locator(group).evaluate((e) => { const v = e.closest(".view"); v.scrollTop = v.scrollHeight; });
      assert(await page.locator(strip).evaluate((e) => e.getBoundingClientRect().bottom < e.closest(".view").getBoundingClientRect().top), "the strip is still in view, so this tests nothing");
      await page.evaluate(() => panelQuotaFocus("all:claude|Pro"));
      await inView();
      // the provider's id alone finds the group too, and lights the same
      await page.evaluate(() => {
        for (const e of document.querySelectorAll("#panelQuota .flash")) e.classList.remove("flash");
        panelQuotaFocus("claude");
      });
      await page.waitForSelector(strip + ".flash", { state: "attached" });
      // a read failed since the cell was drawn, so no strip: the group's first
      // card is lit, held there whatever the machine's speed
      f.cards = f.cards.map((q) => q.provider === "claude" && q.user !== "one@example.com" ? { ...q, error: "offline", windows: [] } : q);
      await page.evaluate((cards) => { quotas = cards; panelQuotaFocus("all:claude|Pro"); }, f.cards);
      await page.waitForFunction((g) => {
        const e = document.querySelector(g + " .flash");
        e?.getAnimations().find((x) => x.animationName === "flash")?.pause();
        return e?.matches(".pq-card");
      }, group);
      assert.equal(await page.locator(strip).count(), 0, "the strip is still there, so this tests nothing");
      assert.equal(await page.locator("#panelQuota .flash").count(), 1, "more than the first card is lit");
      assert(await page.locator(group + " .pq-card").first().evaluate((e) => e.classList.contains("flash")), "not the group's first card");
      assert.deepEqual(errors, []);
    });

    test(`${engine} ${lang}: plan choices stay scoped across providers`, async (t) => {
      const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
      t.after(() => browser.close());
      const labels = {
        en: ["All 2 accounts", "2 accounts", "1 account"],
        zh: ["全部 2 个账号", "2 个账号", "1 个账号"],
        "zh-TW": ["全部 2 個帳號", "2 個帳號", "1 個帳號"],
        ja: ["全 2 アカウント", "2 個のアカウント", "1 アカウント"],
        de: ["Alle 2 Konten", "2 Konten", "1 Konto"],
      }[lang];
      const scenarios = [
        { provider: "codex", name: "Codex", plan: "plus", other: "prolite", kind: "subscription", windows: [{ name: "5 hours", trayKind: "5h", used: 20 }, { name: "7 days", trayKind: "7d", used: 40 }] },
        { provider: "copilot", name: "GitHub Copilot", plan: "Business", other: "Education", kind: "subscription", windows: [{ name: "Premium requests", trayKind: "month", amount: 150, limit: 300, unit: "requests", used: 50 }, { name: "Chat requests", trayKind: "name:chat requests", unlimited: true }] },
        { provider: "gemini", name: "Gemini CLI", plan: "Google AI Pro", other: "Google AI Ultra", kind: "subscription", windows: [{ name: "Gemini Pro", model: "gemini-pro", family: "Gemini", trayKind: "name:gemini · gemini pro", used: 60 }, { name: "Gemini Flash", model: "gemini-flash", family: "Gemini", trayKind: "name:gemini · gemini flash", used: 20 }] },
        { provider: "zhipu-plan", name: "GLM Coding Plan", plan: "pro", other: "max", kind: "plan", windows: [{ name: "5 hours", trayKind: "5h", used: 20 }, { name: "7 days", trayKind: "7d", used: 40 }] },
        { provider: "fakeco-plugin", name: "FakeCo", plan: "Fake Pro | 团队", other: "Fake Team", kind: "subscription", windows: [{ name: "Daily credits", trayKind: "1d", amount: 20, limit: 100, unit: "credits", used: 20 }, { name: "Weekly credits", trayKind: "7d", amount: 40, limit: 100, unit: "credits", used: 40 }] },
      ];
      const card = (s, user, plan = s.plan, windows = s.windows) => ({
        provider: s.provider, name: s.name, icon: "generic", kind: s.kind, user, plan, windows: windows.map((w) => ({ ...w })),
      });
      const open = async (f) => {
        const context = await browser.newContext({ viewport: { width: 900, height: 760 }, reducedMotion: "reduce" });
        const page = await context.newPage();
        page.setDefaultTimeout(8000);
        page.setDefaultNavigationTimeout(60000);
        await page.route("**/*", f.serve);
        await page.goto("http://magpie.test/?view=usage");
        await page.locator("#prefs").click();
        await openPart(page, "usage");
        await page.locator("#trayUsagePick button").waitFor();
        return { context, page };
      };
      for (const s of scenarios) await t.test(s.provider, async (t) => {
        const f = fixture(lang), scope = `all:${s.provider}|${s.plan}`;
        f.cards = [card(s, s.kind === "plan" ? "" : "first@example.com")];
        const { context, page } = await open(f);
        t.after(() => context.close());
        const errors = [];
        page.on("pageerror", (e) => errors.push(e.message));
        const pill = page.locator("#trayUsagePick button");
        const option = (text) => page.locator(".proto-menu .pm-item").filter({ has: page.getByText(text, { exact: true }) });
        await pill.click();
        assert.equal(await page.locator(".proto-menu .pm-item").count(), 4, "a single account gained an aggregate choice");
        assert.equal(await page.evaluate(() => quotaAll(quotas) !== null), false, "the panel disagrees about a single account");
        await page.keyboard.press("Escape");

        f.cards.push(card(s, "second@example.com"));
        await refresh(page, f.cards);
        await pill.click();
        assert.equal(await page.evaluate(() => quotaAll(quotas) !== null), true, "the panel has no corresponding aggregate");
        await option(w(2)).click();
        await page.keyboard.press("Escape");
        await page.waitForFunction((id) => prefs.trayUsages?.includes(id), scope);
        assert.deepEqual(f.posts.at(-1).trayUsages, [scope]);

        f.cards.push({ ...card(s, "unread@example.com"), error: "offline", windows: [] });
        await refresh(page, f.cards);
        await pill.click();
        assert.equal(await option(w(3)).getAttribute("aria-checked"), "true");
        assert.equal(await page.evaluate(() => quotaAll(quotas).querySelector(".quota-all-head").textContent), w(3));
        await page.keyboard.press("Escape");
        f.cards = f.cards.slice(0, 2);

        const onlyOther = { name: "Other plan only", trayKind: "name:other plan only", used: 90 };
        f.cards.push(card(s, "other@example.com", s.other, [s.windows[0], onlyOther]));
        const posts = f.posts.length;
        await refresh(page, f.cards);
        await pill.click();
        assert.equal(await option(w(2)).count(), 0, "two same-plan accounts remained selectable inside mixed plans");
        assert.equal(await page.locator(".proto-menu .pm-item").count(), 8, "mixed plans gained an aggregate, or lost the one picked");
        assert.equal(await option(`${s.plan} · ${gone[lang]}`).getAttribute("aria-checked"), "true", "the picked aggregate can't be unticked");
        // the Usage page's strip is as it was: mixed plans, each plan's share apart
        assert.equal(await page.evaluate(() => quotaAll(quotas) !== null), true, "the Usage page lost its strip for mixed plans");
        await page.keyboard.press("Escape");
        assert.equal(f.posts.length, posts, "a refresh or opening the menu rewrote the pick");
        assert.deepEqual(f.settings.trayUsages, [scope]);
        assert.equal(await page.locator("#trayWindowsPick button").count(), 0);
        // ticking another meanwhile keeps the aggregate for when its plan is one again
        await pill.click();
        await page.locator(".proto-menu .pm-item").filter({ hasText: "other@example.com" }).click();
        await page.keyboard.press("Escape");
        await page.waitForFunction(() => prefs.trayUsages?.length === 2);
        assert.ok(f.posts.at(-1).trayUsages.includes(scope), "ticking another dropped the aggregate");
        await pill.click();
        await page.locator(".proto-menu .pm-item").filter({ hasText: "other@example.com" }).click();
        await page.keyboard.press("Escape");
        await page.waitForFunction(() => prefs.trayUsages?.length === 1);
        assert.deepEqual(f.posts.at(-1).trayUsages, [scope]);

        f.cards = f.cards.slice(0, 2);
        await refresh(page, f.cards);
        await pill.click();
        assert.equal(await option(w(2)).getAttribute("aria-checked"), "true");
        await page.keyboard.press("Escape");

        f.cards = f.cards.slice(0, 1);
        await refresh(page, f.cards);
        await pill.click();
        assert.equal(await option(w(1)).count(), 0, "a selected aggregate was offered for one account");
        assert.equal(await page.locator(".proto-menu .pm-item").count(), 5, "off, the account, the two totals and the aggregate picked");
        await page.locator(".proto-menu .pm-item").first().click();
        await page.waitForFunction(() => prefs.trayUsages?.length === 0);
        assert.deepEqual(f.posts.at(-1).trayUsages, [], "Off cannot clear an aggregate that is no longer eligible");
        assert.deepEqual(errors, []);
      });
      for (const provider of ["ollama-plan", "minimax-plan", "fakeco-plugin"]) await t.test(`${provider}: unknown plans`, async (t) => {
        const f = fixture(lang), s = { provider, name: provider, kind: "plan", plan: "", windows: [{ name: "Monthly", trayKind: "month", amount: 10, limit: 100, unit: "credits", used: 10 }] };
        f.cards = [card(s, ""), card(s, "another-key")];
        const { context, page } = await open(f);
        t.after(() => context.close());
        await page.locator("#trayUsagePick button").click();
        const notes = await page.locator(".proto-menu .pm-note").allTextContents();
        assert.ok(!notes.includes(labels[0]), "missing plan names were treated as the same plan");
        assert.equal(await page.locator(".proto-menu .pm-item").count(), 6, "unknown plans gained an aggregate option");
      });
      await t.test("the windows picker stays the accounts'", async (t) => {
        const f = fixture(lang), s = scenarios[0];
        f.cards = [card(s, "a"), card(s, "b")];
        f.settings.trayUsages = ["all:codex|plus", "codex|a"];
        f.settings.trayWindows = { codex: ["5h"] };
        const { context, page } = await open(f);
        t.after(() => context.close());
        const errors = [];
        page.on("pageerror", (e) => errors.push(e.message));
        const picks = page.locator("#trayWindowsPick button");
        await picks.first().waitFor();
        assert.equal(await picks.count(), 1, "the plan's accounts together gained a windows picker");
        assert.match(await picks.first().textContent(), /^Codex · /);
        assert.deepEqual(errors, []);
      });
      await t.test("Codex free and pro have no aggregate choice", async (t) => {
        const f = fixture(lang), s = { ...scenarios[0], plan: "free", other: "pro" };
        f.cards = [card(s, "a"), card(s, "b", s.other)];
        const { context, page } = await open(f);
        t.after(() => context.close());
        await page.locator("#trayUsagePick button").click();
        const rows = page.locator(".proto-menu .pm-item");
        assert.equal(await rows.count(), 6, "mixed plans must offer only in-use, individual accounts, Off and totals");
        assert.equal(await rows.filter({ has: page.getByText(`free · ${labels[2]}`, { exact: true }) }).count(), 0);
        assert.equal(await rows.filter({ has: page.getByText(`pro · ${labels[2]}`, { exact: true }) }).count(), 0);
        await page.keyboard.press("Escape");
        assert.deepEqual(f.posts, []);
      });
    });
  }
}
