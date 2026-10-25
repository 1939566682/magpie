// Run with Node's test runner and Playwright on the module path; see README.md.
// A window's count reads as "used / of unit · used N%", or as the count alone
// when the vendor names no used count of its own (UsedUnknown: 千问办公,
// MiniMax and AutoClaw give only what is left, so a "0% used" would be a
// figure nobody gave). Where credits run out before the window's reset, only
// the loss is said — "14.23 credits expire tomorrow 13:49" — not the cycle's
// reset as well, the same day twice (codearts says both).
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");

const assets = path.resolve(__dirname, "../assets");

function serve(quotas, lang = "en") {
  return async (route) => {
    const req = route.request(), url = new URL(req.url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:false};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json({ agents: [], profiles: [], settings: { lang, theme: "light" } });
    if (url.pathname === "/api/providers") return json({ providers: [{
      id: "workbuddy", name: "WorkBuddy", icon: "workbuddy-color", models: [], agents: [], fallback: [], headers: {}, keyList: [],
      account: { agent: "plugin", agentName: "WorkBuddy", user: "{{EMAIL_y2x0jdqk}}", plan: "pro",
        logins: [{ user: "{{EMAIL_y2x0jdqk}}", plan: "pro", active: true, on: true }] },
    }], presets: [], excluded: [], gateway: { running: true, window: true } });
    if (url.pathname === "/api/login/usage") return json(Object.fromEntries(quotas.map((q) => [q.user, q])));
    if (url.pathname === "/api/usage/quotas") return json(quotas);
    if (url.pathname === "/api/groups") return json({ groups: [], models: [] });
    if (url.pathname === "/api/plugins") return json({ plugins: [] });
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    try { await route.fulfill({ body: await fs.readFile(file), contentType }); } catch { await route.fulfill({ status: 404, body: "" }); }
  };
}

const tomorrow = () => new Date(Date.now() + 24 * 3600 * 1000).toISOString();

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const lang of ["en", "zh"]) {
    test(`${engine} ${lang}: a window with no used count says its count alone`, async (t) => {
      const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
      t.after(() => browser.close());
      const page = await browser.newPage({ viewport: { width: 900, height: 700 }, reducedMotion: "reduce" });
      page.setDefaultTimeout(5000);
      const errors = [];
      page.on("pageerror", (e) => errors.push(e.message));
      // 千问办公's 当日 + 长期: only what is left is known
      const quota = { provider: "qwenwork-community", name: "千问办公", user: "1933563689@qq.com", windows: [
        { name: "长期积分", used: 0, amount: 0, limit: 2000, unit: "credits", usedUnknown: true, display: "剩余 2,000 积分" },
        { name: "当日积分", used: 0, amount: 0, limit: 98.6336, unit: "credits", usedUnknown: true, display: "剩余 98.6336 积分" },
      ] };
      await page.route("**/*", serve([quota], lang));
      await page.goto("http://magpie.test/?view=usage");
      const rows = page.locator(".subscription-card .quota-n");
      await rows.first().waitFor();
      assert.match(await rows.nth(0).innerText(), /0 \/ 2,000/);
      assert.match(await rows.nth(1).innerText(), /0 \/ 98\.63/);
      // no share is invented from them
      for (const text of await rows.allInnerTexts()) assert.doesNotMatch(text, /used|left|已用|剩余\s*\d/);
      assert.deepEqual(errors, []);
    });

    test(`${engine} ${lang}: credits that run out are said, not the cycle's reset too`, async (t) => {
      const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
      t.after(() => browser.close());
      const page = await browser.newPage({ viewport: { width: 900, height: 700 }, reducedMotion: "reduce" });
      page.setDefaultTimeout(5000);
      const errors = [];
      page.on("pageerror", (e) => errors.push(e.message));
      // codearts says both a cycle end and a package lost before it
      const quota = { provider: "codearts", name: "CodeArts", user: "dingtalk_d8edff95-2868-495a-b51d-344e072e7125@dingtalk.local", windows: [
        { name: "总积分", used: 11.2, amount: 122.73, limit: 1100, unit: "credits",
          resetsAt: new Date(Date.now() + 27 * 24 * 3600 * 1000).toISOString(),
          runsOutAt: tomorrow(), runsOut: 1000 },
      ] };
      await page.route("**/*", serve([quota], lang));
      await page.goto("http://magpie.test/?view=usage");
      const card = page.locator(".subscription-card");
      await card.first().waitFor();
      const reset = card.locator(".quota-reset");
      assert.match(await reset.innerText(), /1,000 credits expire|1,000 积分 将于/);
      // the cycle's own day is not said as well
      assert.doesNotMatch(await reset.innerText(), /Resets|重置于|重置于/);
      // the share still reads
      assert.match(await card.locator(".quota-n").innerText(), /122\.73 \/ 1,100/);
      assert.match(await card.locator(".quota-n").innerText(), /11\.2%|已用/);
      assert.deepEqual(errors, []);
    });
  }
}
