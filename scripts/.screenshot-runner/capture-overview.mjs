#!/usr/bin/env node
import { chromium } from "playwright";
import path from "node:path";

const baseURL = (process.env.BASE_URL || "http://127.0.0.1:19421").replace(/\/$/, "");
const password = process.env.PASSWORD || "preview";
const outfile = process.env.OUTFILE || path.join(process.cwd(), "tmp/screenshots/brand-audit/overview-before.png");
// Uses the Playwright-managed Chromium unless CHROMIUM_PATH points to another build.
const chromiumPath = process.env.CHROMIUM_PATH || undefined;

const insecure = baseURL.startsWith("https://");
const browser = await chromium.launch({ headless: true, executablePath: chromiumPath });
const page = await (await browser.newContext({
  viewport: { width: 1280, height: 800 },
  ignoreHTTPSErrors: insecure,
})).newPage();
await page.goto(`${baseURL}/login`, { waitUntil: "domcontentloaded" });
await page.fill('input[name="password"]', password);
await page.click("button.login-submit, form[action='/login'] button[type='submit']");
await page.waitForSelector(".sidebar", { timeout: 20000 });
await page.waitForTimeout(600);
await page.screenshot({ path: outfile });
await browser.close();
console.log(outfile);