// The native menu bar and the page run the same aggregation examples.
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");
const { test } = require("node:test");
const assets = path.resolve(__dirname, "../assets");
const app = fs.readFileSync(path.join(assets, "app.js"), "utf8");
const ctx = vm.createContext({});
ctx.window = ctx;
vm.runInContext(app.match(/function to100\([^\n]+/)[0] + "\n" + fs.readFileSync(path.join(assets, "quotaall.js"), "utf8"), ctx);
const cases = JSON.parse(fs.readFileSync(path.resolve(__dirname, "../testdata/quota-all.json"), "utf8"));
for (const c of cases) test(c.name, () => {
  const before = JSON.stringify(c.cards);
  const got = JSON.parse(JSON.stringify(ctx.mergeQuotas(c.cards, Date.parse("2026-11-02T12:00:00Z"), c.minimum)));
  assert.equal(got.accounts, c.accounts);
  assert.equal(got.unknown, c.unknown);
  assert.equal(got.windows.length, c.windows.length);
  c.windows.forEach((want, i) => {
    for (const [key, value] of Object.entries(want)) {
      if (key === "firstReset") assert.equal(Date.parse(got.windows[i][key]), Date.parse(value));
      else assert.deepEqual(got.windows[i][key], value, `window ${i} ${key}`);
    }
    assert.equal(got.windows[i].resetsAt, undefined);
    assert.equal(got.windows[i].startsAt, undefined);
  });
  assert.equal(JSON.stringify(c.cards), before, "original readings changed");
});
const eligibility = JSON.parse(fs.readFileSync(path.resolve(__dirname, "../testdata/quota-all-eligibility.json"), "utf8"));
for (const c of eligibility) test("eligible: " + c.name, () => {
  const cards = c.cards.map((q) => ({ ...q, provider: c.provider })), before = JSON.stringify(cards);
  const got = ctx.quotaAllSummary(cards, Date.parse("2026-11-02T12:00:00Z"));
  assert.equal(!!got, c.show);
  if (got) assert.equal(got.plan, c.plan);
  if (c.members) assert.equal(got.subs.length, c.members);
  assert.equal(JSON.stringify(cards), before, "eligibility changed the readings");
});
