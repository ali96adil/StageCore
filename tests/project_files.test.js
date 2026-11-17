"use strict";

const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const test = require("node:test");

const source = fs.readFileSync("internal/operatorweb/static/project-files.js", "utf8");

async function renderProjectFiles(language, search = "") {
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
      match_count: 12,
      logical_bytes: 3 * 1024 * 1024 * 1024,
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
  const requests = [];
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
    api: async (url) => {
      requests.push(url);
      const query = new URLSearchParams(String(url).split("?")[1] || "");
      payload.workspace.match_count = query.has("q") ? 2 : 12;
      return payload;
    },
    esc: (value) => String(value ?? "").replace(/[&<>"']/g, (character) => ({
      "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
    })[character]),
    pill: (label) => `<span>${label}</span>`,
    navigate() {},
    setMessage() {},
    errorMessage: () => "error",
  };
  await vm.runInNewContext(`${source}\nrenderScopedProjectFiles("", ${JSON.stringify(search)});`, context);
  return {html: content.innerHTML, requests};
}

test("Project Files usage summary distinguishes total entries and logical Vault bytes in English", async () => {
  const {html} = await renderProjectFiles("en");
  assert.match(html, /Showing 1 \/ 12 entries/);
  assert.match(html, /File-version size total: 3\.00 GiB/);
  assert.match(html, /Types on this page:/);
  assert.match(html, /shared Vault content may be physically stored once/);
  assert.match(html, /Name or original filename/);
});

test("Project Files usage summary is localized in Arabic", async () => {
  const {html} = await renderProjectFiles("ar-IQ");
  assert.match(html, /المعروض 1 \/ 12 عنصر/);
  assert.match(html, /إجمالي أحجام نسخ الملفات: 3\.00 GiB/);
  assert.match(html, /الأنواع بهذه الصفحة:/);
  assert.match(html, /قد يُخزّن محتوى Vault المشترك فعلياً مرة واحدة/);
  assert.match(html, /اسم الملف أو الاسم الأصلي/);
});

test("Project Files preserves server search and reports matching result count", async () => {
  const {html, requests} = await renderProjectFiles("en", "Opening");
  assert.match(requests[0], /[?&]q=Opening(?:&|$)/);
  assert.match(html, /Showing 1 \/ 2 entries/);
  assert.match(html, /value="Opening"/);
  assert.match(html, /maxlength="128"/);
});
