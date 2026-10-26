#!/usr/bin/env node
import { chromium } from "playwright";
import { mkdir } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(__dirname, "../..");
const target = process.env.TARGET || "local";
const outDir = path.join(root, "tmp", "screenshots", target);

const baseURL = (process.env.BASE_URL || "http://127.0.0.1:19421").replace(/\/$/, "");
const password = process.env.PASSWORD || "preview";
// Uses the Playwright-managed Chromium unless CHROMIUM_PATH points to another build.
const chromiumPath = process.env.CHROMIUM_PATH || undefined;
const insecure = baseURL.startsWith("https://");

const staticPages = [
  { slug: "login", path: "/login" },
  { slug: "overview", path: "/" },
  { slug: "backups", path: "/backups" },
  { slug: "agents", path: "/agents" },
  { slug: "storage", path: "/storage" },
  { slug: "recovery", path: "/recovery" },
  { slug: "dr-testing", path: "/dr-testing" },
];

async function discoverDetailPaths(page) {
  const details = [];

  const backups = await page.evaluate(async () => {
    const r = await fetch("/api/backups");
    return r.ok ? r.json() : [];
  }).catch(() => []);
  if (backups.length) {
    details.push({ slug: "backup-detail", path: `/backups/${backups[0].id}` });
  }

  const agents = await page.evaluate(async () => {
    const r = await fetch("/api/agents");
    return r.ok ? r.json() : [];
  }).catch(() => []);
  if (agents.length) {
    details.push({ slug: "agent-detail", path: `/agents/${agents[0].name}` });
  }

  const repos = await page.evaluate(async () => {
    const r = await fetch("/api/repos");
    return r.ok ? r.json() : [];
  }).catch(() => []);
  if (repos.length) {
    details.push({ slug: "storage-detail", path: `/storage/${repos[0].name}` });
  }

  const snaps = await page.evaluate(async () => {
    const r = await fetch("/api/snapshots");
    return r.ok ? r.json() : [];
  }).catch(() => []);
  if (snaps.length) {
    const id = snaps[0].id || snaps[0].short_id;
    if (id) {
      details.push({ slug: "snapshot-detail", path: `/snapshots/${id}` });
    }
  }

  return details;
}

const waitUntil = "domcontentloaded";

async function login(page) {
  await page.goto(`${baseURL}/login`, { waitUntil, timeout: 30000 });
  await page.fill('input[name="password"]', password);
  const submit = page.locator('button.login-submit, form[action="/login"] button[type="submit"]');
  await submit.first().click();
  await page.waitForSelector(".sidebar", { timeout: 20000 });
}

async function capture(page, slug, viewport) {
  const suffix = viewport === "mobile" ? "-mobile" : "-desktop";
  const file = path.join(outDir, `${slug}${suffix}.png`);
  await page.setViewportSize(
    viewport === "mobile" ? { width: 390, height: 844 } : { width: 1280, height: 800 },
  );
  await page.waitForTimeout(300);
  await page.waitForSelector(".content, .login-shell", { timeout: 10000 });
  await page.screenshot({ path: file, fullPage: false });
  return file;
}

async function main() {
  await mkdir(outDir, { recursive: true });

  const browser = await chromium.launch({
    headless: true,
    executablePath: chromiumPath,
  });

  const context = await browser.newContext({
    viewport: { width: 1280, height: 800 },
    ignoreHTTPSErrors: insecure,
  });
  const page = await context.newPage();
  const saved = [];

  await login(page);
  const detailPages = await discoverDetailPaths(page);
  const authPages = [
    ...staticPages.filter((p) => p.slug !== "login"),
    ...detailPages,
  ];

  for (const entry of staticPages.filter((p) => p.slug === "login")) {
    await page.goto(`${baseURL}${entry.path}`, { waitUntil });
    saved.push(await capture(page, entry.slug, "desktop"));
    saved.push(await capture(page, entry.slug, "mobile"));
  }

  for (const entry of authPages) {
    await page.goto(`${baseURL}${entry.path}`, { waitUntil, timeout: 30000 });
    await page.waitForTimeout(500);
    saved.push(await capture(page, entry.slug, "desktop"));
    if (["overview", "backups", "agents"].includes(entry.slug)) {
      saved.push(await capture(page, entry.slug, "mobile"));
    }
  }

  await browser.close();
  for (const file of saved) {
    console.log(file);
  }
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});