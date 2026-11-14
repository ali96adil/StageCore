"use strict";

const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const test = require("node:test");

const source = fs.readFileSync("internal/operatorweb/static/project-files.js", "utf8");

async function renderProjectFiles(language) {
  const nav = {
    dataset: {},
    classList: {add() {}, remove() {}, contains() { return false; }},
    addEventListener() {},
  };
  const languageSelect = {value: language, addEventListener() {}};
  const content = {innerHTML: ""};
  const state = {
    page: "files",
    project: {project_id: "show-a"},
    user: {id: "operator-a"},
  };
  const payload = {
    scoped: true,
    upload_available: false,
    download_available: false,
    workspace: {
      project_id: "show-a",
      project_name: "Evening Show",
      entry_count: 12,
      total_bytes: 3 * 1024 * 1024 * 1024,
      files: [{
        media_asset_id: "asset-1",
        media_asset_name: "Opening audio",
        asset_policy: "MANAGED",
        content_version_id: "version-1",
        original_filename: "opening.wav",
        size_bytes: 1024,
        category: "AUDIO",
      }],
    },
  };
  const context = {
    document: {
      createElement: () => nav,
      querySelector: () => ({insertAdjacentElement() {}}),
    },
    URLSearchParams,
    state,
    content,
    globalMessage: {},
    el: (id) => id === "languageSelect" ? languageSelect : undefined,
    api: async () => payload,
    esc: (value) => String(value ?? "").replace(/[&<>"']/g, (character) => ({
      "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
    })[character]),
    pill: (label) => `<span>${label}</span>`,
    navigate() {},
    setMessage() {},
    errorMessage: () => "error",
  };
  await vm.runInNewContext(`${source}\nrenderScopedProjectFiles();`, context);
  return content.innerHTML;
}

test("Project Files usage summary distinguishes total entries and logical Vault bytes in English", async () => {
  const html = await renderProjectFiles("en");
  assert.match(html, /Showing 1 \/ 12 entries/);
  assert.match(html, /File-version size total: 3\.00 GiB/);
  assert.match(html, /shared Vault content may be physically stored once/);
});

test("Project Files usage summary is localized in Arabic", async () => {
  const html = await renderProjectFiles("ar-IQ");
  assert.match(html, /المعروض 1 \/ 12 عنصر/);
  assert.match(html, /إجمالي أحجام نسخ الملفات: 3\.00 GiB/);
  assert.match(html, /قد يُخزّن محتوى Vault المشترك فعلياً مرة واحدة/);
});
