"use strict";

const F008_LOCALE_KEY = "stagecore_locale";
const F008_RESUME_PROJECT_KEY = "stagecore_f008_resume_project";
const f008PageLocale = localStorage.getItem(F008_LOCALE_KEY) === "en" ? "en" : "ar";
let f008SelectedLocale = f008PageLocale;
let f008Step = 1;
let f008DismissedThisSession = false;
let f008ProjectsObserved = false;
let f008ResumeInFlight = false;
let f008Dialog = null;

const f008Strings = {
  "first_run.title": { en: "Finish first-run setup", "ar-IQ": "إكمال إعداد التشغيل الأول" },
  "first_run.summary": { en: "Confirm this Hub, choose local preferences, then create the first Project.", "ar-IQ": "تأكد من هذا الـ Hub واختر التفضيلات المحلية ثم أنشئ أول مشروع." },
  "first_run.step_identity": { en: "1 · Hub", "ar-IQ": "١ · الـ Hub" },
  "first_run.step_preferences": { en: "2 · Preferences", "ar-IQ": "٢ · التفضيلات" },
  "first_run.step_project": { en: "3 · First Project", "ar-IQ": "٣ · أول مشروع" },
  "first_run.identity_title": { en: "Secure OWNER access is ready", "ar-IQ": "دخول المالك الآمن جاهز" },
  "first_run.identity_detail": { en: "Check the local Hub identity before continuing with operator setup.", "ar-IQ": "تحقق من هوية الـ Hub المحلية قبل متابعة إعداد المشغّل." },
  "first_run.hub": { en: "Hub", "ar-IQ": "الـ Hub" },
  "first_run.fingerprint": { en: "Fingerprint", "ar-IQ": "البصمة" },
  "first_run.local_only": { en: "This setup stays on this Hub and browser. It does not require an Internet account and does not enter SHOW or run Cues.", "ar-IQ": "يبقى هذا الإعداد محلياً على الـ Hub وهذا المتصفح، ولا يحتاج حساب إنترنت ولا يدخل وضع العرض أو ينفذ إشارات." },
  "first_run.continue": { en: "Continue", "ar-IQ": "متابعة" },
  "first_run.back": { en: "Back", "ar-IQ": "رجوع" },
  "first_run.skip": { en: "Skip for now", "ar-IQ": "تخطي الآن" },
  "first_run.preferences_title": { en: "Choose this browser's operator preferences", "ar-IQ": "اختر تفضيلات المشغّل لهذا المتصفح" },
  "first_run.preferences_detail": { en: "Language and appearance are local presentation settings only. They never change show logic or safety state.", "ar-IQ": "اللغة والمظهر إعدادات عرض محلية فقط، ولا تغيّر منطق العرض أو حالة السلامة." },
  "first_run.language": { en: "Language", "ar-IQ": "اللغة" },
  "first_run.arabic": { en: "Arabic", "ar-IQ": "العربية" },
  "first_run.english": { en: "English", "ar-IQ": "الإنكليزية" },
  "first_run.appearance": { en: "Appearance", "ar-IQ": "المظهر" },
  "first_run.system": { en: "System", "ar-IQ": "تلقائي حسب الجهاز" },
  "first_run.light": { en: "Light", "ar-IQ": "فاتح" },
  "first_run.dark": { en: "Dark", "ar-IQ": "داكن" },
  "first_run.accent": { en: "Accent", "ar-IQ": "اللون المميز" },
  "first_run.blue": { en: "Blue", "ar-IQ": "أزرق" },
  "first_run.teal": { en: "Teal", "ar-IQ": "فيروزي" },
  "first_run.violet": { en: "Violet", "ar-IQ": "بنفسجي" },
  "first_run.amber": { en: "Amber", "ar-IQ": "كهرماني" },
  "first_run.project_title": { en: "Create the first Project", "ar-IQ": "أنشئ أول مشروع" },
  "first_run.project_detail": { en: "This creates the normal editable Draft through the same authenticated Project API used by the Operator workspace.", "ar-IQ": "ينشئ هذا مسودة عادية قابلة للتعديل عبر واجهة المشروع الموثقة نفسها التي تستخدمها مساحة عمل المشغّل." },
  "first_run.project_name": { en: "Project name", "ar-IQ": "اسم المشروع" },
  "first_run.project_description": { en: "Description (optional)", "ar-IQ": "الوصف (اختياري)" },
  "first_run.create_continue": { en: "Create and open Setup", "ar-IQ": "إنشاء وفتح الإعداد" },
  "first_run.creating": { en: "Creating the first Project…", "ar-IQ": "جارٍ إنشاء أول مشروع…" },
  "first_run.handoff": { en: "After creation, StageCore opens the guided Setup page for devices and targets. Preflight remains the authoritative readiness gate.", "ar-IQ": "بعد الإنشاء يفتح StageCore صفحة الإعداد الموجهة للأجهزة والأهداف، ويبقى الفحص المسبق هو بوابة الجاهزية المعتمدة." },
  "first_run.error": { en: "First-run setup could not continue.", "ar-IQ": "تعذر إكمال إعداد التشغيل الأول." },
};

function f008LocaleTag() {
  return f008SelectedLocale === "en" ? "en" : "ar-IQ";
}

function f008Text(key) {
  const entry = f008Strings[key];
  if (!entry) return key;
  return entry[f008LocaleTag()] || entry.en || key;
}

function f008StoredMode() {
  return typeof f016StoredMode === "function" ? f016StoredMode() : "system";
}

function f008StoredAccent() {
  return typeof f016StoredAccent === "function" ? f016StoredAccent() : "blue";
}

function f008Eligible() {
  return f008ProjectsObserved &&
    !f008DismissedThisSession &&
    state.user?.role === "OWNER" &&
    state.hub?.bootstrap_state === "CLAIMED" &&
    Array.isArray(state.projects) &&
    state.projects.length === 0 &&
    !el("appView")?.classList.contains("hidden");
}

function f008Progress() {
  return `<div class="f008-progress" aria-label="${esc(f008Text("first_run.title"))}">
    <span class="${f008Step === 1 ? "active" : ""}">${esc(f008Text("first_run.step_identity"))}</span>
    <span class="${f008Step === 2 ? "active" : ""}">${esc(f008Text("first_run.step_preferences"))}</span>
    <span class="${f008Step === 3 ? "active" : ""}">${esc(f008Text("first_run.step_project"))}</span>
  </div>`;
}

function f008IdentityStep() {
  const hubName = state.hub?.display_name || "StageCore Hub";
  const fingerprint = state.hub?.fingerprint || "—";
  return `<section class="f008-step">
    <div>
      <h3>${esc(f008Text("first_run.identity_title"))}</h3>
      <p>${esc(f008Text("first_run.identity_detail"))}</p>
    </div>
    <dl class="f008-identity">
      <div><dt>${esc(f008Text("first_run.hub"))}</dt><dd>${esc(hubName)}</dd></div>
      <div><dt>${esc(f008Text("first_run.fingerprint"))}</dt><dd class="mono">${esc(fingerprint)}</dd></div>
    </dl>
    <div class="f008-note">${esc(f008Text("first_run.local_only"))}</div>
  </section>`;
}

function f008PreferencesStep() {
  const mode = f008StoredMode();
  const accent = f008StoredAccent();
  return `<section class="f008-step">
    <div>
      <h3>${esc(f008Text("first_run.preferences_title"))}</h3>
      <p>${esc(f008Text("first_run.preferences_detail"))}</p>
    </div>
    <div class="f008-choice-grid">
      <div class="f008-choice-group">
        <label>${esc(f008Text("first_run.language"))}
          <select id="f008Language">
            <option value="ar" ${f008SelectedLocale === "ar" ? "selected" : ""}>${esc(f008Text("first_run.arabic"))}</option>
            <option value="en" ${f008SelectedLocale === "en" ? "selected" : ""}>${esc(f008Text("first_run.english"))}</option>
          </select>
        </label>
      </div>
      <div class="f008-choice-group">
        <label>${esc(f008Text("first_run.appearance"))}
          <select id="f008Mode">
            <option value="system" ${mode === "system" ? "selected" : ""}>${esc(f008Text("first_run.system"))}</option>
            <option value="light" ${mode === "light" ? "selected" : ""}>${esc(f008Text("first_run.light"))}</option>
            <option value="dark" ${mode === "dark" ? "selected" : ""}>${esc(f008Text("first_run.dark"))}</option>
          </select>
        </label>
      </div>
      <div class="f008-choice-group">
        <label>${esc(f008Text("first_run.accent"))}
          <select id="f008Accent">
            <option value="blue" ${accent === "blue" ? "selected" : ""}>${esc(f008Text("first_run.blue"))}</option>
            <option value="teal" ${accent === "teal" ? "selected" : ""}>${esc(f008Text("first_run.teal"))}</option>
            <option value="violet" ${accent === "violet" ? "selected" : ""}>${esc(f008Text("first_run.violet"))}</option>
            <option value="amber" ${accent === "amber" ? "selected" : ""}>${esc(f008Text("first_run.amber"))}</option>
          </select>
        </label>
      </div>
    </div>
  </section>`;
}

function f008ProjectStep() {
  return `<section class="f008-step">
    <div>
      <h3>${esc(f008Text("first_run.project_title"))}</h3>
      <p>${esc(f008Text("first_run.project_detail"))}</p>
    </div>
    <form id="f008ProjectForm" class="f008-project-form">
      <label>${esc(f008Text("first_run.project_name"))}
        <input id="f008ProjectName" required maxlength="160" autocomplete="off">
      </label>
      <label>${esc(f008Text("first_run.project_description"))}
        <textarea id="f008ProjectDescription" maxlength="500" rows="3"></textarea>
      </label>
    </form>
    <div class="f008-note">${esc(f008Text("first_run.handoff"))}</div>
    <div id="f008Status" class="f008-status" role="status"></div>
  </section>`;
}

function f008Render() {
  if (!f008Dialog) return;
  f008Dialog.lang = f008LocaleTag();
  f008Dialog.dir = f008SelectedLocale === "en" ? "ltr" : "rtl";
  const stepContent = f008Step === 1 ? f008IdentityStep() : f008Step === 2 ? f008PreferencesStep() : f008ProjectStep();
  f008Dialog.innerHTML = `<div class="f008-shell">
    <header class="f008-head">
      <div>
        <p class="eyebrow">F-008</p>
        <h2>${esc(f008Text("first_run.title"))}</h2>
        <p>${esc(f008Text("first_run.summary"))}</p>
      </div>
      <button id="f008Close" class="icon-button" type="button" aria-label="${esc(f008Text("first_run.skip"))}">×</button>
    </header>
    ${f008Progress()}
    <div class="f008-body">${stepContent}</div>
    <footer class="f008-actions">
      <button id="f008Skip" class="button ghost" type="button">${esc(f008Text("first_run.skip"))}</button>
      <div class="f008-actions-end">
        ${f008Step > 1 ? `<button id="f008Back" class="button ghost" type="button">${esc(f008Text("first_run.back"))}</button>` : ""}
        ${f008Step < 3 ? `<button id="f008Next" class="button primary" type="button">${esc(f008Text("first_run.continue"))}</button>` : `<button id="f008Create" class="button primary" type="submit" form="f008ProjectForm">${esc(f008Text("first_run.create_continue"))}</button>`}
      </div>
    </footer>
  </div>`;

  el("f008Close")?.addEventListener("click", f008Dismiss);
  el("f008Skip")?.addEventListener("click", f008Dismiss);
  el("f008Back")?.addEventListener("click", () => {
    f008Step = Math.max(1, f008Step - 1);
    f008Render();
  });
  el("f008Next")?.addEventListener("click", () => {
    f008Step = Math.min(3, f008Step + 1);
    f008Render();
  });
  el("f008Language")?.addEventListener("change", (event) => {
    f008SelectedLocale = event.target.value === "en" ? "en" : "ar";
    f008Render();
  });
  el("f008Mode")?.addEventListener("change", (event) => {
    if (typeof f016SetMode === "function") f016SetMode(event.target.value);
  });
  el("f008Accent")?.addEventListener("change", (event) => {
    if (typeof f016SetAccent === "function") f016SetAccent(event.target.value);
  });
  el("f008ProjectForm")?.addEventListener("submit", f008CreateProject);
}

function f008Dismiss() {
  f008DismissedThisSession = true;
  if (f008Dialog?.open) f008Dialog.close();
}

async function f008CreateProject(event) {
  event.preventDefault();
  const name = el("f008ProjectName")?.value.trim() || "";
  const description = el("f008ProjectDescription")?.value.trim() || "";
  const status = el("f008Status");
  const submit = el("f008Create");
  if (!name) return;

  if (submit) submit.disabled = true;
  if (status) {
    status.className = "f008-status";
    status.textContent = f008Text("first_run.creating");
  }

  try {
    const payload = await api("/api/v1/projects", {
      method: "POST",
      json: { name, description },
    });
    const projectID = payload?.project?.project_id;
    if (!projectID) throw new Error(f008Text("first_run.error"));

    localStorage.setItem(F008_LOCALE_KEY, f008SelectedLocale === "en" ? "en" : "ar");
    sessionStorage.setItem(F008_RESUME_PROJECT_KEY, projectID);
    window.location.reload();
  } catch (error) {
    if (submit) submit.disabled = false;
    if (status) {
      status.className = "f008-status error";
      status.textContent = errorMessage(error) || f008Text("first_run.error");
    }
  }
}

function f008Open() {
  if (!f008Dialog) {
    f008Dialog = document.createElement("dialog");
    f008Dialog.id = "firstRunDialog";
    f008Dialog.className = "dialog f008-dialog";
    f008Dialog.addEventListener("cancel", () => {
      f008DismissedThisSession = true;
    });
    document.body.appendChild(f008Dialog);
  }
  f008Step = 1;
  f008SelectedLocale = localStorage.getItem(F008_LOCALE_KEY) === "en" ? "en" : "ar";
  f008Render();
  if (!f008Dialog.open) f008Dialog.showModal();
}

async function f008ResumeProjectIfNeeded() {
  if (f008ResumeInFlight || !f008ProjectsObserved || !state.user) return false;
  const projectID = sessionStorage.getItem(F008_RESUME_PROJECT_KEY);
  if (!projectID) return false;
  const exists = Array.isArray(state.projects) && state.projects.some((project) => project.project_id === projectID);
  if (!exists) {
    sessionStorage.removeItem(F008_RESUME_PROJECT_KEY);
    return false;
  }

  f008ResumeInFlight = true;
  sessionStorage.removeItem(F008_RESUME_PROJECT_KEY);
  try {
    await openProject(projectID);
    await navigate("configuration");
  } finally {
    f008ResumeInFlight = false;
  }
  return true;
}

async function f008MaybeStart() {
  if (await f008ResumeProjectIfNeeded()) return;
  if (f008Eligible() && !f008Dialog?.open) f008Open();
}

const f008BaseRenderProjects = renderProjects;
renderProjects = function f008RenderProjectsWithFirstRun(...args) {
  const result = f008BaseRenderProjects.apply(this, args);
  f008ProjectsObserved = true;
  queueMicrotask(() => { f008MaybeStart().catch(() => {}); });
  return result;
};

// All normal bootstrap/login/navigation paths load Projects before rendering them.
// The wrapper above therefore uses authoritative Hub state rather than a browser-only
// "setup complete" flag. This fallback only catches an unusually fast initial render.
queueMicrotask(() => {
  if (state.user && Array.isArray(state.projects) && state.projects.length > 0) {
    f008ProjectsObserved = true;
    f008MaybeStart().catch(() => {});
  }
});
