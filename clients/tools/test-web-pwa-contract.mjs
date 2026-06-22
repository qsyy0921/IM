import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";

const repoRoot = path.resolve(import.meta.dirname, "..");
const webRoot = path.join(repoRoot, "web");

const indexHTML = readText(path.join(webRoot, "index.html"));
const manifestText = readText(path.join(webRoot, "public", "manifest.webmanifest"));
const serviceWorker = readText(path.join(webRoot, "public", "nexusim-sw.js"));
const icon = readText(path.join(webRoot, "public", "pwa-icon.svg"));
const pwaSource = readText(path.join(webRoot, "src", "pwa.ts"));
const mainSource = readText(path.join(webRoot, "src", "main.tsx"));

assert(indexHTML.includes('rel="manifest" href="/manifest.webmanifest"'), "manifest link missing");
assert(indexHTML.includes('rel="icon" href="/pwa-icon.svg"'), "PWA icon link missing");
assert(indexHTML.includes('name="theme-color" content="#0f766e"'), "theme color missing");

const manifest = JSON.parse(manifestText);
assert.equal(manifest.name, "NexusIM");
assert.equal(manifest.short_name, "NexusIM");
assert.equal(manifest.start_url, "/");
assert.equal(manifest.scope, "/");
assert.equal(manifest.display, "standalone");
assert.equal(manifest.theme_color, "#0f766e");
assert(Array.isArray(manifest.icons), "manifest icons must be present");
assert(manifest.icons.some(icon => icon.src === "/pwa-icon.svg" && icon.purpose.includes("maskable")));
assertNoSensitiveWords("manifest", manifestText);

assert(icon.includes("<svg"), "PWA icon must be SVG");
assert(!icon.includes("href="), "PWA icon must not reference external assets");

assert(serviceWorker.includes("nexusim-browser-shell-v1"), "cache name missing");
assert(serviceWorker.includes('request.method !== "GET"'), "non-GET bypass missing");
assert(serviceWorker.includes("url.origin !== self.location.origin"), "same-origin guard missing");
assert(serviceWorker.includes('"/api/"'), "API bypass missing");
assert(serviceWorker.includes('"/ws"'), "WebSocket bypass missing");
assert(serviceWorker.includes('"/nexusim-shell-config.js"'), "shell config bypass missing");
assert(serviceWorker.includes('"/nexusim-sw.js"'), "service worker self bypass missing");
assert(!serviceWorker.includes("localStorage"), "service worker must not use localStorage");
assert(!serviceWorker.includes("IndexedDB"), "service worker must not use IndexedDB");
assertNoSensitiveWords("service worker", serviceWorker);

assert(pwaSource.includes("serviceWorker"), "PWA registration must use serviceWorker");
assert(pwaSource.includes('register("/nexusim-sw.js"'), "service worker path mismatch");
assert(pwaSource.includes("shell?.target && shell.target !== \"browser\""), "WebView target skip missing");
assert(pwaSource.includes("windows-desktop") && pwaSource.includes("android"), "target union missing");
assertNoSensitiveWords("PWA registration", pwaSource);

assert(mainSource.includes("registerBrowserPWA"), "main.tsx must register browser PWA");

console.log("web PWA contract ok");

function readText(filePath) {
  return fs.readFileSync(filePath, "utf8");
}

function assertNoSensitiveWords(label, content) {
  const lower = content.toLowerCase();
  for (const word of ["token", "secret", "password", "credential", "private_key"]) {
    assert(!lower.includes(word), `${label} must not contain sensitive word: ${word}`);
  }
}
