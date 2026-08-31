"use strict";

const F016_MODE_KEY = "stagecore_appearance_mode";
const F016_ACCENT_KEY = "stagecore_appearance_accent";
const F016_DEFAULT_MODE = "system";
const F016_DEFAULT_ACCENT = "blue";
const F016_MODES = ["system", "light", "dark"];
const F016_ACCENTS = ["blue", "teal", "violet", "amber"];

const f016Strings = {
  "appearance.title": { en: "Appearance", "ar-IQ": "المظهر" },
  "appearance.mode": { en: "Mode", "ar-IQ": "الوضع" },
  "appearance.system": { en: "System", "ar-IQ": "تلقائي حسب الجهاز" },
  "appearance.system_detail": { en: "Follows this device automatically", "ar-IQ": "يتبع إعداد الجهاز تلقائياً" },
  "appearance.light": { en: "Light", "ar-IQ": "فاتح" },
  "appearance.light_detail": { en: "Bright high-clarity surfaces", "ar-IQ": "سطوح فاتحة عالية الوضوح" },
  "appearance.dark": { en: "Dark", "ar-IQ": "داكن" },
  "appearance.dark_detail": { en: "Suited to dark theatre environments", "ar-IQ": "مناسب لبيئات المسرح المظلمة" },
  "appearance.accent": { en: "Accent", "ar-IQ": "اللون المميز" },
  "appearance.blue": { en: "Blue", "ar-IQ": "أزرق" },
  "appearance.teal": { en: "Teal", "ar-IQ": "فيروزي" },
  "appearance.violet": { en: "Violet", "ar-IQ": "بنفسجي" },
  "appearance.amber": { en: "Amber", "ar-IQ": "كهرماني" },
  "appearance.preview": { en: "Safety color preview", "ar-IQ": "معاينة ألوان الحالة" },
  "appearance.ready": { en: "Ready", "ar-IQ": "جاهز" },
  "appearance.warning": { en: "Warning", "ar-IQ": "تحذير" },
  "appearance.blocker": { en: "Blocker", "ar-IQ": "حاجب" },
  "appearance.local_scope": { en: "This foundation stores appearance only on this browser/device. It never changes show logic or safety state.", "ar-IQ": "يحفظ هذا الأساس المظهر على هذا المتصفح أو الجهاز فقط، ولا يغيّر منطق العرض أو حالة السلامة." },
  "appearance.reset": { en: "Reset appearance", "ar-IQ": "إعادة المظهر الافتراضي" },
  "appearance.close": { en: "Close", "ar-IQ": "إغلاق" },
};

function f016Locale() {
  return localStorage.getItem("stagecore_locale") === "en" ? "en" : "ar-IQ";
}

function f016Text(key) {
  const value = f016Strings[key];
  if (!value) return key;
  return value[f016Locale()] || value.en || key;
}

function f016StoredMode() {
  const value = localStorage.getItem(F016_MODE_KEY) || F016_DEFAULT_MODE;
  return F016_MODES.includes(value) ? value : F016_DEFAULT_MODE;
}

function f016StoredAccent() {
  const value = localStorage.getItem(F016_ACCENT_KEY) || F016_DEFAULT_ACCENT;
  return F016_ACCENTS.includes(value) ? value : F016_DEFAULT_ACCENT;
}

function f016SystemTheme() {
  return window.matchMedia("(prefers-color-scheme: light)").matches ? "light" : "dark";
}

function f016ResolvedTheme(mode = f016StoredMode()) {
  return mode === "system" ? f016SystemTheme() : mode;
}

function f016ApplyTheme() {
  const mode = f016StoredMode();
  const accent = f016StoredAccent();
  const resolved = f016ResolvedTheme(mode);
  document.documentElement.dataset.themePreference = mode;
  document.documentElement.dataset.theme = resolved;
  document.documentElement.dataset.accent = accent;
  document.documentElement.style.colorScheme = resolved;
  const trigger = document.getElementById("appearanceTrigger");
  if (trigger) {
    trigger.setAttribute("aria-label", f016Text("appearance.title"));
    const label = trigger.querySelector(".appearance-label");
    if (label) label.textContent = f016Text("appearance.title");
  }
}

function f016SetMode(mode) {
  if (!F016_MODES.includes(mode)) return;
  localStorage.setItem(F016_MODE_KEY, mode);
  f016ApplyTheme();
}

function f016SetAccent(accent) {
  if (!F016_ACCENTS.includes(accent)) return;
  localStorage.setItem(F016_ACCENT_KEY, accent);
  f016ApplyTheme();
}

function f016Reset() {
  localStorage.removeItem(F016_MODE_KEY);
  localStorage.removeItem(F016_ACCENT_KEY);
  f016ApplyTheme();
  f016SyncControls();
}

function f016ModeOption(mode, titleKey, detailKey) {
  return `<label class="appearance-option">
    <input type="radio" name="appearanceMode" value="${mode}">
    <strong>${f016Text(titleKey)}</strong>
    <small>${f016Text(detailKey)}</small>
  </label>`;
}

function f016AccentOption(accent, titleKey) {
  return `<label class="appearance-option">
    <input type="radio" name="appearanceAccent" value="${accent}">
    <span class="appearance-swatch ${accent}" aria-hidden="true"></span>
    <strong>${f016Text(titleKey)}</strong>
  </label>`;
}

function f016BuildDialog() {
  const dialog = document.createElement("dialog");
  dialog.id = "appearanceDialog";
  dialog.className = "dialog appearance-dialog";
  dialog.innerHTML = `<form method="dialog">
    <div class="dialog-head">
      <div>
        <p class="eyebrow">F-016</p>
        <h2>${f016Text("appearance.title")}</h2>
      </div>
      <button id="appearanceCloseIcon" class="icon-button" type="button" aria-label="${f016Text("appearance.close")}">×</button>
    </div>

    <section class="appearance-choice-group" aria-labelledby="appearanceModeHeading">
      <h3 id="appearanceModeHeading">${f016Text("appearance.mode")}</h3>
      <div class="appearance-options">
        ${f016ModeOption("system", "appearance.system", "appearance.system_detail")}
        ${f016ModeOption("light", "appearance.light", "appearance.light_detail")}
        ${f016ModeOption("dark", "appearance.dark", "appearance.dark_detail")}
      </div>
    </section>

    <section class="appearance-choice-group" aria-labelledby="appearanceAccentHeading">
      <h3 id="appearanceAccentHeading">${f016Text("appearance.accent")}</h3>
      <div class="appearance-options appearance-accent-options">
        ${f016AccentOption("blue", "appearance.blue")}
        ${f016AccentOption("teal", "appearance.teal")}
        ${f016AccentOption("violet", "appearance.violet")}
        ${f016AccentOption("amber", "appearance.amber")}
      </div>
    </section>

    <section class="appearance-choice-group" aria-labelledby="appearancePreviewHeading">
      <h3 id="appearancePreviewHeading">${f016Text("appearance.preview")}</h3>
      <div class="appearance-preview" aria-label="${f016Text("appearance.preview")}">
        <span class="ready">${f016Text("appearance.ready")}</span>
        <span class="warning">${f016Text("appearance.warning")}</span>
        <span class="blocker">${f016Text("appearance.blocker")}</span>
      </div>
      <p class="appearance-scope">${f016Text("appearance.local_scope")}</p>
    </section>

    <div class="dialog-actions">
      <button id="appearanceReset" class="button ghost" type="button">${f016Text("appearance.reset")}</button>
      <button class="button primary" value="close" type="submit">${f016Text("appearance.close")}</button>
    </div>
  </form>`;
  document.body.appendChild(dialog);
  return dialog;
}

function f016SyncControls() {
  const mode = f016StoredMode();
  const accent = f016StoredAccent();
  document.querySelectorAll('input[name="appearanceMode"]').forEach((input) => {
    input.checked = input.value === mode;
  });
  document.querySelectorAll('input[name="appearanceAccent"]').forEach((input) => {
    input.checked = input.value === accent;
  });
}

function f016InitControls() {
  const topbar = document.querySelector(".topbar");
  if (!topbar || document.getElementById("appearanceTrigger")) return;

  const trigger = document.createElement("button");
  trigger.id = "appearanceTrigger";
  trigger.className = "button ghost appearance-trigger";
  trigger.type = "button";
  trigger.setAttribute("aria-label", f016Text("appearance.title"));
  trigger.innerHTML = `<span class="appearance-icon" aria-hidden="true">◐</span><span class="appearance-label">${f016Text("appearance.title")}</span>`;

  const userArea = document.getElementById("userArea");
  if (userArea) topbar.insertBefore(trigger, userArea);
  else topbar.appendChild(trigger);

  const dialog = f016BuildDialog();
  f016SyncControls();

  trigger.addEventListener("click", () => {
    f016SyncControls();
    dialog.showModal();
  });
  document.getElementById("appearanceCloseIcon")?.addEventListener("click", () => dialog.close());
  document.getElementById("appearanceReset")?.addEventListener("click", f016Reset);
  dialog.querySelectorAll('input[name="appearanceMode"]').forEach((input) => {
    input.addEventListener("change", () => f016SetMode(input.value));
  });
  dialog.querySelectorAll('input[name="appearanceAccent"]').forEach((input) => {
    input.addEventListener("change", () => f016SetAccent(input.value));
  });
}

const f016SystemMedia = window.matchMedia("(prefers-color-scheme: light)");
f016SystemMedia.addEventListener?.("change", () => {
  if (f016StoredMode() === "system") f016ApplyTheme();
});

/* Apply before the document paints when this script is loaded from <head>. */
f016ApplyTheme();

if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", f016InitControls, { once: true });
} else {
  f016InitControls();
}
