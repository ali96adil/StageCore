"use strict";

const F017_STORAGE_KEY = "stagecore_workspace_profiles_v1";
const F017_CONTRACT_VERSION = 1;
const F017_PROFILE_VERSION = 1;
const F017_DEFAULT_PRESET = "stage-manager";
const F017_MAX_CUSTOM_PROFILES = 24;
const F017_NAV_SIZES = ["compact", "normal", "wide"];
const F017_PAGES = ["dashboard", "configuration", "cues", "runtime", "preflight", "sessions", "notes"];

const f017Strings = {
  "workspace.title": { en: "Workspace Profiles", "ar-IQ": "ملفات مساحة العمل" },
  "workspace.profile": { en: "Workspace profile", "ar-IQ": "ملف مساحة العمل" },
  "workspace.manage": { en: "Manage", "ar-IQ": "إدارة" },
  "workspace.builtin": { en: "Built-in preset", "ar-IQ": "إعداد جاهز" },
  "workspace.custom": { en: "Custom profile", "ar-IQ": "ملف مخصص" },
  "workspace.create_copy": { en: "Create editable copy", "ar-IQ": "إنشاء نسخة قابلة للتعديل" },
  "workspace.name": { en: "Profile name", "ar-IQ": "اسم الملف" },
  "workspace.visible": { en: "Visible workspaces and priority", "ar-IQ": "مساحات العمل الظاهرة وترتيبها" },
  "workspace.default": { en: "Default workspace", "ar-IQ": "مساحة العمل الافتراضية" },
  "workspace.nav_size": { en: "Navigation size", "ar-IQ": "حجم شريط التنقل" },
  "workspace.compact": { en: "Compact", "ar-IQ": "مضغوط" },
  "workspace.normal": { en: "Normal", "ar-IQ": "عادي" },
  "workspace.wide": { en: "Wide", "ar-IQ": "واسع" },
  "workspace.save": { en: "Save profile", "ar-IQ": "حفظ الملف" },
  "workspace.delete": { en: "Delete profile", "ar-IQ": "حذف الملف" },
  "workspace.reset": { en: "Use Stage Manager default", "ar-IQ": "استخدام إعداد مدير المسرح" },
  "workspace.close": { en: "Close", "ar-IQ": "إغلاق" },
  "workspace.local_scope": { en: "Profiles are stored only on this browser/device in this foundation.", "ar-IQ": "تُحفظ الملفات على هذا المتصفح أو الجهاز فقط في هذا الأساس." },
  "workspace.presentation_only": { en: "Switching profiles changes presentation only and never changes the show.", "ar-IQ": "تبديل الملفات يغيّر العرض فقط ولا يغيّر تشغيل العرض المسرحي." },
  "workspace.locked": { en: "SHOW is active. Profile switching stays available, but structural profile editing is locked.", "ar-IQ": "العرض SHOW نشط. يبقى تبديل الملفات متاحاً، لكن تعديل بنية الملف مقفول." },
  "workspace.lock_unknown": { en: "StageCore could not confirm SHOW lock state, so profile editing is unavailable for this management action.", "ar-IQ": "تعذر على StageCore تأكيد حالة قفل SHOW، لذلك تعديل الملف غير متاح في عملية الإدارة الحالية." },
  "workspace.saved": { en: "Workspace profile saved.", "ar-IQ": "تم حفظ ملف مساحة العمل." },
  "workspace.deleted": { en: "Custom profile deleted.", "ar-IQ": "تم حذف الملف المخصص." },
  "workspace.invalid_last": { en: "Keep at least one workspace visible.", "ar-IQ": "أبقِ مساحة عمل واحدة ظاهرة على الأقل." },
  "workspace.move_up": { en: "Move up", "ar-IQ": "تحريك للأعلى" },
  "workspace.move_down": { en: "Move down", "ar-IQ": "تحريك للأسفل" },
  "workspace.stage_manager": { en: "Stage Manager", "ar-IQ": "مدير المسرح" },
  "workspace.video": { en: "Video", "ar-IQ": "الفيديو" },
  "workspace.lighting": { en: "Lighting", "ar-IQ": "الإضاءة" },
  "workspace.sound": { en: "Sound", "ar-IQ": "الصوت" },
  "workspace.rehearsal": { en: "Rehearsal", "ar-IQ": "البروفة" },
  "workspace.monitoring": { en: "Monitoring", "ar-IQ": "المراقبة" },
  "workspace.page.dashboard": { en: "Home", "ar-IQ": "الرئيسية" },
  "workspace.page.configuration": { en: "Setup", "ar-IQ": "الإعداد" },
  "workspace.page.cues": { en: "Cues", "ar-IQ": "الإشارات" },
  "workspace.page.runtime": { en: "Run", "ar-IQ": "التشغيل" },
  "workspace.page.preflight": { en: "Check", "ar-IQ": "الفحص" },
  "workspace.page.sessions": { en: "History", "ar-IQ": "السجل" },
  "workspace.page.notes": { en: "Notes", "ar-IQ": "الملاحظات" },
};

const F017_PRESETS = {
  "stage-manager": {
    name_key: "workspace.stage_manager",
    visible_pages: ["dashboard", "runtime", "preflight", "cues", "sessions", "notes", "configuration"],
    page_order: ["dashboard", "runtime", "preflight", "cues", "sessions", "notes", "configuration"],
    default_page: "dashboard",
    navigation_size: "normal",
  },
  video: {
    name_key: "workspace.video",
    visible_pages: ["runtime", "cues", "configuration", "preflight", "dashboard", "sessions"],
    page_order: ["runtime", "cues", "configuration", "preflight", "dashboard", "sessions", "notes"],
    default_page: "runtime",
    navigation_size: "normal",
  },
  lighting: {
    name_key: "workspace.lighting",
    visible_pages: ["runtime", "cues", "configuration", "preflight", "dashboard"],
    page_order: ["runtime", "cues", "configuration", "preflight", "dashboard", "sessions", "notes"],
    default_page: "runtime",
    navigation_size: "compact",
  },
  sound: {
    name_key: "workspace.sound",
    visible_pages: ["runtime", "cues", "configuration", "preflight", "dashboard"],
    page_order: ["runtime", "cues", "configuration", "preflight", "dashboard", "sessions", "notes"],
    default_page: "runtime",
    navigation_size: "compact",
  },
  rehearsal: {
    name_key: "workspace.rehearsal",
    visible_pages: ["runtime", "cues", "notes", "sessions", "dashboard", "preflight", "configuration"],
    page_order: ["runtime", "cues", "notes", "sessions", "dashboard", "preflight", "configuration"],
    default_page: "runtime",
    navigation_size: "wide",
  },
  monitoring: {
    name_key: "workspace.monitoring",
    visible_pages: ["preflight", "dashboard", "runtime", "sessions", "notes"],
    page_order: ["preflight", "dashboard", "runtime", "sessions", "notes", "cues", "configuration"],
    default_page: "preflight",
    navigation_size: "compact",
  },
};

function f017Locale() {
  return localStorage.getItem("stagecore_locale") === "en" ? "en" : "ar-IQ";
}

function f017Text(key) {
  const value = f017Strings[key];
  return value?.[f017Locale()] || value?.en || key;
}

function f017UniqueKnownPages(values) {
  const result = [];
  for (const value of Array.isArray(values) ? values : []) {
    if (F017_PAGES.includes(value) && !result.includes(value)) result.push(value);
  }
  return result;
}

function f017PresetProfile(presetID) {
  const preset = F017_PRESETS[presetID] || F017_PRESETS[F017_DEFAULT_PRESET];
  const resolvedID = F017_PRESETS[presetID] ? presetID : F017_DEFAULT_PRESET;
  return {
    profile_version: F017_PROFILE_VERSION,
    profile_id: `preset:${resolvedID}`,
    name: f017Text(preset.name_key),
    base_preset: resolvedID,
    scope: "DEVICE_LOCAL",
    visible_pages: [...preset.visible_pages],
    page_order: [...preset.page_order],
    default_page: preset.default_page,
    navigation_size: preset.navigation_size,
    built_in: true,
    updated_at: null,
  };
}

function f017NormalizeProfile(raw, fallbackPreset = F017_DEFAULT_PRESET) {
  const fallback = f017PresetProfile(F017_PRESETS[fallbackPreset] ? fallbackPreset : F017_DEFAULT_PRESET);
  if (!raw || typeof raw !== "object") return fallback;

  const order = f017UniqueKnownPages(raw.page_order);
  for (const page of F017_PAGES) if (!order.includes(page)) order.push(page);

  let visible = f017UniqueKnownPages(raw.visible_pages);
  if (!visible.length) visible = [fallback.default_page];
  visible.sort((a, b) => order.indexOf(a) - order.indexOf(b));

  let defaultPage = F017_PAGES.includes(raw.default_page) && visible.includes(raw.default_page)
    ? raw.default_page
    : visible[0];
  if (!defaultPage) defaultPage = "dashboard";

  const preset = F017_PRESETS[raw.base_preset] ? raw.base_preset : fallback.base_preset;
  const profileID = typeof raw.profile_id === "string" && raw.profile_id.startsWith("custom:")
    ? raw.profile_id
    : fallback.profile_id;
  const isCustom = profileID.startsWith("custom:");
  const name = isCustom && typeof raw.name === "string" && raw.name.trim()
    ? raw.name.trim().slice(0, 80)
    : fallback.name;

  return {
    profile_version: F017_PROFILE_VERSION,
    profile_id: profileID,
    name,
    base_preset: preset,
    scope: "DEVICE_LOCAL",
    visible_pages: visible,
    page_order: order,
    default_page: defaultPage,
    navigation_size: F017_NAV_SIZES.includes(raw.navigation_size) ? raw.navigation_size : fallback.navigation_size,
    built_in: !isCustom,
    updated_at: isCustom && typeof raw.updated_at === "string" ? raw.updated_at : null,
  };
}

function f017DefaultContainer() {
  return {
    contract_version: F017_CONTRACT_VERSION,
    active_profile_id: `preset:${F017_DEFAULT_PRESET}`,
    custom_profiles: [],
    last_page_by_project_profile: {},
  };
}

function f017ReadContainer() {
  let parsed;
  try { parsed = JSON.parse(localStorage.getItem(F017_STORAGE_KEY) || "null"); }
  catch (_) { return f017DefaultContainer(); }
  if (!parsed || parsed.contract_version !== F017_CONTRACT_VERSION) return f017DefaultContainer();

  const customProfiles = [];
  const seen = new Set();
  for (const candidate of Array.isArray(parsed.custom_profiles) ? parsed.custom_profiles : []) {
    const normalized = f017NormalizeProfile(candidate, candidate?.base_preset);
    if (!normalized.profile_id.startsWith("custom:") || seen.has(normalized.profile_id)) continue;
    seen.add(normalized.profile_id);
    customProfiles.push(normalized);
    if (customProfiles.length >= F017_MAX_CUSTOM_PROFILES) break;
  }

  const knownIDs = new Set([
    ...Object.keys(F017_PRESETS).map((id) => `preset:${id}`),
    ...customProfiles.map((profile) => profile.profile_id),
  ]);
  const activeProfileID = knownIDs.has(parsed.active_profile_id)
    ? parsed.active_profile_id
    : `preset:${F017_DEFAULT_PRESET}`;

  const memory = {};
  if (parsed.last_page_by_project_profile && typeof parsed.last_page_by_project_profile === "object") {
    for (const [key, page] of Object.entries(parsed.last_page_by_project_profile)) {
      if (typeof key === "string" && key.length <= 260 && F017_PAGES.includes(page)) memory[key] = page;
    }
  }

  return {
    contract_version: F017_CONTRACT_VERSION,
    active_profile_id: activeProfileID,
    custom_profiles: customProfiles,
    last_page_by_project_profile: memory,
  };
}

let f017Container = f017ReadContainer();

function f017WriteContainer() {
  try { localStorage.setItem(F017_STORAGE_KEY, JSON.stringify(f017Container)); }
  catch (_) { /* presentation preference failure must never block show control */ }
}

function f017ProfileByID(profileID) {
  if (typeof profileID === "string" && profileID.startsWith("preset:")) {
    return f017PresetProfile(profileID.slice("preset:".length));
  }
  const custom = f017Container.custom_profiles.find((profile) => profile.profile_id === profileID);
  return custom ? f017NormalizeProfile(custom, custom.base_preset) : f017PresetProfile(F017_DEFAULT_PRESET);
}

function f017ActiveProfile() {
  return f017ProfileByID(f017Container.active_profile_id);
}

function f017MemoryKey(projectID, profileID = f017Container.active_profile_id) {
  return `${String(projectID || "").slice(0, 180)}|${String(profileID || "").slice(0, 80)}`;
}

function f017RememberPage(projectID, page) {
  const profile = f017ActiveProfile();
  if (!projectID || !profile.visible_pages.includes(page)) return;
  f017Container.last_page_by_project_profile[f017MemoryKey(projectID)] = page;
  f017WriteContainer();
}

function f017PreferredPage(projectID) {
  const profile = f017ActiveProfile();
  const remembered = f017Container.last_page_by_project_profile[f017MemoryKey(projectID)];
  if (profile.visible_pages.includes(remembered)) return remembered;
  if (profile.visible_pages.includes(profile.default_page)) return profile.default_page;
  return profile.visible_pages[0] || "dashboard";
}

function f017KnownProfiles() {
  return [
    ...Object.keys(F017_PRESETS).map(f017PresetProfile),
    ...f017Container.custom_profiles.map((profile) => f017NormalizeProfile(profile, profile.base_preset)),
  ];
}

function f017ProfileLabel(profile) {
  if (profile.built_in) {
    const preset = F017_PRESETS[profile.base_preset];
    return preset ? f017Text(preset.name_key) : profile.name;
  }
  return profile.name;
}

function f017EnsureActiveProfileExists() {
  const ids = new Set(f017KnownProfiles().map((profile) => profile.profile_id));
  if (!ids.has(f017Container.active_profile_id)) {
    f017Container.active_profile_id = `preset:${F017_DEFAULT_PRESET}`;
    f017WriteContainer();
  }
}

function f017SetActiveProfile(profileID) {
  const exists = f017KnownProfiles().some((profile) => profile.profile_id === profileID);
  f017Container.active_profile_id = exists ? profileID : `preset:${F017_DEFAULT_PRESET}`;
  f017WriteContainer();
  f017ApplyProfile({ navigateIfNeeded: true });
}

function f017PageLabel(page) {
  return f017Text(`workspace.page.${page}`);
}

function f017ApplyProfile({ navigateIfNeeded = false } = {}) {
  f017EnsureActiveProfileExists();
  const profile = f017ActiveProfile();
  const nav = document.getElementById("workspaceNav");
  const app = document.getElementById("appView");

  if (app) app.dataset.workspaceNavSize = profile.navigation_size;
  if (nav) {
    const buttons = new Map([...nav.querySelectorAll("[data-page]")].map((button) => [button.dataset.page, button]));
    for (const page of profile.page_order) {
      const button = buttons.get(page);
      if (button) nav.appendChild(button);
    }
    for (const [page, button] of buttons.entries()) {
      button.classList.toggle("f017-profile-hidden", !profile.visible_pages.includes(page));
    }
  }

  f017RenderTopbarOptions();

  if (navigateIfNeeded && state?.project && F017_PAGES.includes(state.page) && !profile.visible_pages.includes(state.page)) {
    void navigate(f017PreferredPage(state.project.project_id));
  }
}

function f017NewCustomID() {
  if (globalThis.crypto?.randomUUID) return `custom:${globalThis.crypto.randomUUID()}`;
  return `custom:${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

function f017CreateEditableCopy(source = f017ActiveProfile()) {
  const base = f017NormalizeProfile(source, source.base_preset);
  const suffix = f017Locale() === "ar-IQ" ? " — مخصص" : " — Custom";
  const profile = {
    ...base,
    profile_id: f017NewCustomID(),
    name: `${f017ProfileLabel(base)}${suffix}`.slice(0, 80),
    built_in: false,
    updated_at: new Date().toISOString(),
  };
  f017Container.custom_profiles.unshift(profile);
  f017Container.custom_profiles = f017Container.custom_profiles.slice(0, F017_MAX_CUSTOM_PROFILES);
  f017Container.active_profile_id = profile.profile_id;
  f017WriteContainer();
  f017ApplyProfile({ navigateIfNeeded: true });
  return profile;
}

function f017SaveCustom(profile) {
  const normalized = f017NormalizeProfile({ ...profile, updated_at: new Date().toISOString() }, profile.base_preset);
  if (!normalized.profile_id.startsWith("custom:")) return false;
  const index = f017Container.custom_profiles.findIndex((item) => item.profile_id === normalized.profile_id);
  if (index < 0) f017Container.custom_profiles.unshift(normalized);
  else f017Container.custom_profiles[index] = normalized;
  f017Container.active_profile_id = normalized.profile_id;
  f017WriteContainer();
  f017ApplyProfile({ navigateIfNeeded: true });
  return true;
}

function f017DeleteCustom(profileID) {
  if (!String(profileID).startsWith("custom:")) return;
  f017Container.custom_profiles = f017Container.custom_profiles.filter((profile) => profile.profile_id !== profileID);
  for (const key of Object.keys(f017Container.last_page_by_project_profile)) {
    if (key.endsWith(`|${profileID}`)) delete f017Container.last_page_by_project_profile[key];
  }
  f017Container.active_profile_id = `preset:${F017_DEFAULT_PRESET}`;
  f017WriteContainer();
  f017ApplyProfile({ navigateIfNeeded: true });
}

function f017Reset() {
  f017Container.active_profile_id = `preset:${F017_DEFAULT_PRESET}`;
  f017WriteContainer();
  f017ApplyProfile({ navigateIfNeeded: true });
}

async function f017ProfileEditPolicy() {
  if (!state?.project?.project_id) return { allowed: true, locked: false, reason: "NO_PROJECT" };
  try {
    const payload = await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/configuration/lock`);
    const lock = payload?.show_configuration_lock || { locked: false };
    return { allowed: !lock.locked, locked: !!lock.locked, reason: lock.reason || "" };
  } catch (error) {
    return { allowed: false, locked: false, unknown: true, error };
  }
}

function f017RenderTopbarOptions() {
  const select = document.getElementById("workspaceProfileSelect");
  if (!select) return;
  const previous = f017Container.active_profile_id;
  select.innerHTML = f017KnownProfiles().map((profile) => {
    const prefix = profile.built_in ? "" : "★ ";
    return `<option value="${esc(profile.profile_id)}">${esc(prefix + f017ProfileLabel(profile))}</option>`;
  }).join("");
  select.value = previous;
  const label = document.querySelector('label[for="workspaceProfileSelect"]');
  if (label) label.textContent = f017Text("workspace.profile");
  const manage = document.getElementById("workspaceProfileManage");
  if (manage) {
    manage.textContent = f017Text("workspace.manage");
    manage.setAttribute("aria-label", f017Text("workspace.title"));
  }
}

function f017ProfileEditorRows(profile, editable) {
  return profile.page_order.map((page, index) => {
    const visible = profile.visible_pages.includes(page);
    return `<li class="f017-workspace-row" data-f017-page="${esc(page)}">
      <label class="f017-workspace-toggle"><input class="f017-visible" type="checkbox" ${visible ? "checked" : ""} ${editable ? "" : "disabled"}><span>${esc(f017PageLabel(page))}</span></label>
      <div class="f017-row-actions">
        <button class="icon-button f017-up" type="button" aria-label="${esc(f017Text("workspace.move_up"))}" ${!editable || index === 0 ? "disabled" : ""}>↑</button>
        <button class="icon-button f017-down" type="button" aria-label="${esc(f017Text("workspace.move_down"))}" ${!editable || index === profile.page_order.length - 1 ? "disabled" : ""}>↓</button>
      </div>
    </li>`;
  }).join("");
}

function f017DefaultOptions(profile) {
  return profile.page_order.filter((page) => profile.visible_pages.includes(page)).map((page) =>
    `<option value="${esc(page)}" ${profile.default_page === page ? "selected" : ""}>${esc(f017PageLabel(page))}</option>`
  ).join("");
}

function f017RenderDialog(dialog, policy, draft = null, message = "", messageKind = "success") {
  const active = draft || f017ActiveProfile();
  const editable = !active.built_in && policy.allowed;
  const policyMessage = policy.locked ? f017Text("workspace.locked") : policy.unknown ? f017Text("workspace.lock_unknown") : "";

  dialog.innerHTML = `<form method="dialog" class="f017-dialog-form">
    <div class="dialog-head">
      <div><p class="eyebrow">F-017</p><h2>${esc(f017Text("workspace.title"))}</h2><p class="muted">${esc(f017Text("workspace.presentation_only"))}</p></div>
      <button class="icon-button f017-close" type="button" aria-label="${esc(f017Text("workspace.close"))}">×</button>
    </div>
    ${policyMessage ? `<div class="message warn">${esc(policyMessage)}</div>` : ""}
    ${message ? `<div class="message ${esc(messageKind)}">${esc(message)}</div>` : ""}

    <section class="f017-profile-summary">
      <div><span>${esc(active.built_in ? f017Text("workspace.builtin") : f017Text("workspace.custom"))}</span><strong>${esc(f017ProfileLabel(active))}</strong></div>
      <p>${esc(f017Text("workspace.local_scope"))}</p>
    </section>

    ${active.built_in ? `<button class="button primary f017-copy" type="button" ${policy.allowed ? "" : "disabled"}>${esc(f017Text("workspace.create_copy"))}</button>` : `
      <label>${esc(f017Text("workspace.name"))}<input id="f017ProfileName" maxlength="80" value="${esc(active.name)}" ${editable ? "" : "disabled"}></label>`}

    <section class="f017-editor-section">
      <h3>${esc(f017Text("workspace.visible"))}</h3>
      <ul class="f017-workspace-list">${f017ProfileEditorRows(active, editable)}</ul>
    </section>

    <div class="form-grid two">
      <label>${esc(f017Text("workspace.default"))}<select id="f017DefaultPage" ${editable ? "" : "disabled"}>${f017DefaultOptions(active)}</select></label>
      <label>${esc(f017Text("workspace.nav_size"))}<select id="f017NavSize" ${editable ? "" : "disabled"}>
        <option value="compact" ${active.navigation_size === "compact" ? "selected" : ""}>${esc(f017Text("workspace.compact"))}</option>
        <option value="normal" ${active.navigation_size === "normal" ? "selected" : ""}>${esc(f017Text("workspace.normal"))}</option>
        <option value="wide" ${active.navigation_size === "wide" ? "selected" : ""}>${esc(f017Text("workspace.wide"))}</option>
      </select></label>
    </div>

    <div class="dialog-actions f017-dialog-actions">
      <button class="button ghost f017-reset" type="button">${esc(f017Text("workspace.reset"))}</button>
      ${!active.built_in ? `<button class="button danger f017-delete" type="button" ${editable ? "" : "disabled"}>${esc(f017Text("workspace.delete"))}</button><button class="button primary f017-save" type="button" ${editable ? "" : "disabled"}>${esc(f017Text("workspace.save"))}</button>` : ""}
      <button class="button f017-close" value="close" type="submit">${esc(f017Text("workspace.close"))}</button>
    </div>
  </form>`;

  const working = f017NormalizeProfile(active, active.base_preset);
  working.profile_id = active.profile_id;
  working.name = active.name;
  working.built_in = active.built_in;

  const rerender = (nextMessage = "", nextKind = "success") => f017RenderDialog(dialog, policy, working, nextMessage, nextKind);
  const refreshWritePolicy = async () => {
    const currentPolicy = await f017ProfileEditPolicy();
    if (!currentPolicy.allowed) {
      f017RenderDialog(dialog, currentPolicy, working);
      return null;
    }
    return currentPolicy;
  };

  dialog.querySelectorAll(".f017-close").forEach((button) => button.addEventListener("click", () => dialog.close()));
  dialog.querySelector(".f017-copy")?.addEventListener("click", async () => {
    const currentPolicy = await refreshWritePolicy();
    if (!currentPolicy) return;
    const created = f017CreateEditableCopy(active);
    f017RenderDialog(dialog, currentPolicy, created);
  });
  dialog.querySelector(".f017-reset")?.addEventListener("click", () => {
    f017Reset();
    f017RenderDialog(dialog, policy, f017ActiveProfile());
  });

  if (!editable) return;

  dialog.querySelectorAll(".f017-visible").forEach((checkbox) => {
    checkbox.addEventListener("change", () => {
      const page = checkbox.closest("[data-f017-page]").dataset.f017Page;
      const next = new Set(working.visible_pages);
      checkbox.checked ? next.add(page) : next.delete(page);
      if (!next.size) {
        checkbox.checked = true;
        rerender(f017Text("workspace.invalid_last"), "warn");
        return;
      }
      working.visible_pages = working.page_order.filter((item) => next.has(item));
      if (!working.visible_pages.includes(working.default_page)) working.default_page = working.visible_pages[0];
      rerender();
    });
  });

  dialog.querySelectorAll(".f017-up, .f017-down").forEach((button) => {
    button.addEventListener("click", () => {
      const page = button.closest("[data-f017-page]").dataset.f017Page;
      const from = working.page_order.indexOf(page);
      const to = from + (button.classList.contains("f017-up") ? -1 : 1);
      if (from < 0 || to < 0 || to >= working.page_order.length) return;
      [working.page_order[from], working.page_order[to]] = [working.page_order[to], working.page_order[from]];
      working.visible_pages.sort((a, b) => working.page_order.indexOf(a) - working.page_order.indexOf(b));
      rerender();
    });
  });

  dialog.querySelector("#f017DefaultPage")?.addEventListener("change", (event) => { working.default_page = event.target.value; });
  dialog.querySelector("#f017NavSize")?.addEventListener("change", (event) => { working.navigation_size = event.target.value; });
  dialog.querySelector(".f017-save")?.addEventListener("click", async () => {
    const currentPolicy = await refreshWritePolicy();
    if (!currentPolicy) return;
    working.name = (dialog.querySelector("#f017ProfileName")?.value || working.name).trim().slice(0, 80) || working.name;
    f017SaveCustom(working);
    f017RenderDialog(dialog, currentPolicy, f017ActiveProfile(), f017Text("workspace.saved"));
  });
  dialog.querySelector(".f017-delete")?.addEventListener("click", async () => {
    const currentPolicy = await refreshWritePolicy();
    if (!currentPolicy) return;
    f017DeleteCustom(working.profile_id);
    f017RenderDialog(dialog, currentPolicy, f017ActiveProfile(), f017Text("workspace.deleted"));
  });
}

async function f017OpenManager() {
  let dialog = document.getElementById("workspaceProfileDialog");
  if (!dialog) {
    dialog = document.createElement("dialog");
    dialog.id = "workspaceProfileDialog";
    dialog.className = "dialog f017-dialog";
    document.body.appendChild(dialog);
  }
  const policy = await f017ProfileEditPolicy();
  f017RenderDialog(dialog, policy);
  dialog.showModal();
}

function f017InstallTopbar() {
  if (document.getElementById("workspaceProfileControl")) return;
  const topbar = document.querySelector(".topbar");
  const userArea = document.getElementById("userArea");
  if (!topbar) return;

  const control = document.createElement("div");
  control.id = "workspaceProfileControl";
  control.className = "f017-profile-control";
  control.innerHTML = `<label for="workspaceProfileSelect">${esc(f017Text("workspace.profile"))}</label><select id="workspaceProfileSelect" aria-label="${esc(f017Text("workspace.profile"))}"></select><button id="workspaceProfileManage" class="button ghost" type="button">${esc(f017Text("workspace.manage"))}</button>`;
  if (userArea) topbar.insertBefore(control, userArea);
  else topbar.appendChild(control);

  document.getElementById("workspaceProfileSelect").addEventListener("change", (event) => f017SetActiveProfile(event.target.value));
  document.getElementById("workspaceProfileManage").addEventListener("click", f017OpenManager);
  f017RenderTopbarOptions();
}

/* Hook existing navigation without changing runtime/API semantics. */
const f017BaseNavigate = navigate;
navigate = async function f017Navigate(page, ...args) {
  const result = await f017BaseNavigate(page, ...args);
  if (state?.project?.project_id && F017_PAGES.includes(page)) f017RememberPage(state.project.project_id, page);
  return result;
};

const f017BaseOpenProject = openProject;
openProject = async function f017OpenProject(projectID, ...args) {
  const preferred = f017PreferredPage(projectID);
  const result = await f017BaseOpenProject(projectID, ...args);
  f017ApplyProfile();
  if (preferred !== "dashboard") await navigate(preferred);
  return result;
};

const f017BaseCreateProject = createProject;
createProject = async function f017CreateProject(event, ...args) {
  const result = await f017BaseCreateProject(event, ...args);
  f017ApplyProfile();
  if (state?.project?.project_id) {
    const preferred = f017PreferredPage(state.project.project_id);
    if (preferred !== "dashboard") await navigate(preferred);
  }
  return result;
};

function f017Init() {
  f017InstallTopbar();
  f017ApplyProfile();
}

if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", f017Init, { once: true });
else f017Init();
