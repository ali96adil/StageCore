"use strict";

const f015ManagerStrings = {
  "extensions.nav": { en: "Extensions", "ar-IQ": "الإضافات" },
  "extensions.eyebrow": { en: "EXTENSION MANAGER", "ar-IQ": "مدير الإضافات" },
  "extensions.title": { en: "Plugins and Add-ons", "ar-IQ": "الإضافات والملحقات" },
  "extensions.summary": { en: "Review the local extension library, install verified packages, approve permissions, check readiness and control isolated Plugin runtimes.", "ar-IQ": "راجع مكتبة الإضافات المحلية وثبّت الحزم المتحقق منها وراجع الصلاحيات وافحص الجاهزية وتحكم بتشغيل الإضافات المعزول." },
  "extensions.refresh": { en: "Refresh", "ar-IQ": "تحديث" },
  "extensions.library": { en: "Local library", "ar-IQ": "المكتبة المحلية" },
  "extensions.installed": { en: "Installed", "ar-IQ": "المثبتة" },
  "extensions.ready": { en: "Ready", "ar-IQ": "جاهزة" },
  "extensions.running": { en: "Running", "ar-IQ": "قيد التشغيل" },
  "extensions.available_packages": { en: "Available packages", "ar-IQ": "الحزم المتاحة" },
  "extensions.installed_extensions": { en: "Installed extensions", "ar-IQ": "الإضافات المثبتة" },
  "extensions.no_packages": { en: "No extension packages are registered in the local library yet.", "ar-IQ": "لا توجد حزم إضافات مسجلة في المكتبة المحلية حالياً." },
  "extensions.no_installations": { en: "No extensions are installed on this Hub yet.", "ar-IQ": "لا توجد إضافات مثبتة على هذا الـ Hub حالياً." },
  "extensions.version": { en: "Version", "ar-IQ": "الإصدار" },
  "extensions.kind": { en: "Kind", "ar-IQ": "النوع" },
  "extensions.source": { en: "Source", "ar-IQ": "المصدر" },
  "extensions.compatibility": { en: "Compatibility", "ar-IQ": "التوافق" },
  "extensions.permissions": { en: "Permissions", "ar-IQ": "الصلاحيات" },
  "extensions.dependencies": { en: "Dependencies", "ar-IQ": "الاعتماديات" },
  "extensions.review_install": { en: "Review install", "ar-IQ": "مراجعة التثبيت" },
  "extensions.install_plan": { en: "Install plan", "ar-IQ": "خطة التثبيت" },
  "extensions.install": { en: "Install plan", "ar-IQ": "تنفيذ خطة التثبيت" },
  "extensions.already_installed": { en: "Already installed", "ar-IQ": "مثبتة مسبقاً" },
  "extensions.plan_ready": { en: "The install plan is ready.", "ar-IQ": "خطة التثبيت جاهزة." },
  "extensions.plan_dependencies": { en: "Required dependencies will be installed first in the verified order below.", "ar-IQ": "ستُثبت الاعتماديات المطلوبة أولاً حسب الترتيب المتحقق منه أدناه." },
  "extensions.plan_blocked": { en: "The install plan is blocked.", "ar-IQ": "خطة التثبيت محجوبة." },
  "extensions.blockers": { en: "Blockers", "ar-IQ": "الحواجب" },
  "extensions.warnings": { en: "Warnings", "ar-IQ": "التحذيرات" },
  "extensions.steps": { en: "Steps", "ar-IQ": "الخطوات" },
  "extensions.installing": { en: "Installing verified plan…", "ar-IQ": "جارٍ تنفيذ خطة التثبيت المتحقق منها…" },
  "extensions.install_complete": { en: "Install plan completed.", "ar-IQ": "اكتملت خطة التثبيت." },
  "extensions.permission_review": { en: "Permission review", "ar-IQ": "مراجعة الصلاحيات" },
  "extensions.approve": { en: "Approve", "ar-IQ": "موافقة" },
  "extensions.deny": { en: "Deny", "ar-IQ": "رفض" },
  "extensions.readiness": { en: "Readiness", "ar-IQ": "الجاهزية" },
  "extensions.runtime": { en: "Runtime", "ar-IQ": "التشغيل" },
  "extensions.enable": { en: "Enable", "ar-IQ": "تفعيل" },
  "extensions.disable": { en: "Disable", "ar-IQ": "تعطيل" },
  "extensions.runtime_not_applicable": { en: "This Add-on does not use the native Plugin runtime.", "ar-IQ": "هذا الملحق لا يستخدم تشغيل الإضافات الأصلي." },
  "extensions.manage_role": { en: "OWNER or TECHNICIAN authorization is required to change extensions.", "ar-IQ": "يلزم تفويض المالك أو الفني لتغيير الإضافات." },
  "extensions.show_note": { en: "Installation, permission changes and runtime enable/disable remain server-side blocked during an active SHOW.", "ar-IQ": "يبقى التثبيت وتغيير الصلاحيات وتفعيل أو تعطيل التشغيل محجوباً من الخادم أثناء العرض النشط." },
  "extensions.package_id": { en: "Package ID", "ar-IQ": "معرف الحزمة" },
  "extensions.installation_id": { en: "Installation ID", "ar-IQ": "معرف التثبيت" },
  "extensions.status": { en: "Status", "ar-IQ": "الحالة" },
  "extensions.error": { en: "Extension Manager request failed.", "ar-IQ": "فشل طلب مدير الإضافات." },
};

function f015ManagerLocale() {
  return localStorage.getItem("stagecore_locale") === "en" ? "en" : "ar-IQ";
}

function f015ManagerText(key) {
  const item = f015ManagerStrings[key];
  return item?.[f015ManagerLocale()] || item?.en || key;
}

function f015CanManage() {
  return state.user?.role === "OWNER" || state.user?.role === "TECHNICIAN";
}

function f015Kind(value) {
  if (["PASS", "READY", "READY_FOR_ACTIVATION", "APPROVED", "NOT_REQUIRED", "ENABLED"].includes(value)) return "good";
  if (["BLOCKED", "NOT_READY", "DENIED", "FAILED"].includes(value)) return "bad";
  if (["PENDING", "STARTING", "REQUIRES_DEPENDENCIES", "ADVISORY"].includes(value)) return "warn";
  return "neutral";
}

function f015Count(model, predicate) {
  return model.filter(predicate).length;
}

async function f015Optional(path, fallback = null) {
  try { return await api(path); }
  catch (error) {
    if (error?.status === 404) return fallback;
    return { _error: error };
  }
}

async function f015LoadModel() {
  const [libraryPayload, installationPayload] = await Promise.all([
    api("/api/v1/extensions"),
    api("/api/v1/extensions/installations"),
  ]);
  const packages = libraryPayload?.extensions || [];
  const installations = installationPayload?.installations || [];
  const detailEntries = await Promise.all(installations.map(async (installation) => {
    const id = encodeURIComponent(installation.installation_id);
    const [permissions, readiness, runtime] = await Promise.all([
      f015Optional(`/api/v1/extensions/installations/${id}/permission-review`, null),
      f015Optional(`/api/v1/extensions/installations/${id}/readiness`, null),
      installation.kind === "PLUGIN" ? f015Optional(`/api/v1/extensions/installations/${id}/runtime`, null) : Promise.resolve(null),
    ]);
    return [installation.installation_id, { permissions, readiness, runtime }];
  }));
  return { packages, installations, details: Object.fromEntries(detailEntries) };
}

function f015PackageName(pkg) {
  const locale = f015ManagerLocale();
  return pkg?.manifest?.name?.[locale] || pkg?.manifest?.name?.en || pkg?.manifest?.extension_id || pkg?.package_id || "Extension";
}

function f015PackageSummary(pkg) {
  const locale = f015ManagerLocale();
  return pkg?.manifest?.summary?.[locale] || pkg?.manifest?.summary?.en || "";
}

function f015PackageMeta(pkg) {
  const manifest = pkg.manifest || {};
  return `<div class="f015-meta">
    <div><span>${esc(f015ManagerText("extensions.version"))}</span><strong>${esc(manifest.version || "—")}</strong></div>
    <div><span>${esc(f015ManagerText("extensions.kind"))}</span><strong>${esc(manifest.kind || "—")}</strong></div>
    <div><span>${esc(f015ManagerText("extensions.source"))}</span><strong>${esc(manifest.source || "—")}</strong></div>
    <div><span>${esc(f015ManagerText("extensions.compatibility"))}</span><strong>${esc(pkg.compatible ? "PASS" : "BLOCKED")}</strong></div>
    <div><span>${esc(f015ManagerText("extensions.permissions"))}</span><strong>${esc((manifest.permissions || []).length)}</strong></div>
    <div><span>${esc(f015ManagerText("extensions.dependencies"))}</span><strong>${esc((manifest.dependencies || []).filter((item) => !item.optional).length)}</strong></div>
  </div>`;
}

function f015RenderPackage(pkg, installedByPackage) {
  const installed = installedByPackage.has(pkg.package_id);
  return `<article class="card f015-card" data-package-id="${esc(pkg.package_id)}">
    <div class="f015-card-head">
      <div><p class="eyebrow">${esc(pkg.manifest?.extension_id || "EXTENSION")}</p><h2>${esc(f015PackageName(pkg))}</h2></div>
      ${pill(installed ? f015ManagerText("extensions.already_installed") : (pkg.compatible ? "PASS" : "BLOCKED"), installed || pkg.compatible ? "good" : "bad")}
    </div>
    <p class="muted">${esc(f015PackageSummary(pkg))}</p>
    ${f015PackageMeta(pkg)}
    <div><span class="muted">${esc(f015ManagerText("extensions.package_id"))}</span><div class="mono">${esc(pkg.package_id)}</div></div>
    <div class="f015-actions">
      <button class="button ${installed ? "ghost" : "primary"} f015-review-plan" type="button" ${installed ? "disabled" : ""}>${esc(installed ? f015ManagerText("extensions.already_installed") : f015ManagerText("extensions.review_install"))}</button>
    </div>
  </article>`;
}

function f015ReviewStatus(detail) {
  if (!detail) return "—";
  if (detail._error) return "ERROR";
  return detail.status || detail.observed_state || "—";
}

function f015RenderPermissions(installation, review) {
  if (!review || review._error) return `<div class="f015-detail"><strong>${esc(f015ManagerText("extensions.permission_review"))}</strong><p class="muted">${esc(f015ReviewStatus(review))}</p></div>`;
  const items = review.items || [];
  return `<div class="f015-detail">
    <div class="section-title-row"><strong>${esc(f015ManagerText("extensions.permission_review"))}</strong>${pill(review.status || "—", f015Kind(review.status))}</div>
    <div class="f015-permissions">
      ${items.length ? items.map((item) => `<div class="f015-permission" data-permission="${esc(item.permission)}">
        <div><strong class="mono">${esc(item.permission)}</strong><small class="muted">${esc(item.decision || "PENDING")}</small></div>
        ${f015CanManage() ? `<div class="f015-permission-actions"><button class="button f015-permission-decision" data-decision="APPROVED" type="button">${esc(f015ManagerText("extensions.approve"))}</button><button class="button ghost f015-permission-decision" data-decision="DENIED" type="button">${esc(f015ManagerText("extensions.deny"))}</button></div>` : ""}
      </div>`).join("") : `<div class="muted">${esc(review.status || "NOT_REQUIRED")}</div>`}
    </div>
  </div>`;
}

function f015RenderReadiness(readiness) {
  if (!readiness || readiness._error) return `<div class="f015-detail"><strong>${esc(f015ManagerText("extensions.readiness"))}</strong><p class="muted">${esc(f015ReviewStatus(readiness))}</p></div>`;
  return `<div class="f015-detail">
    <div class="section-title-row"><strong>${esc(f015ManagerText("extensions.readiness"))}</strong>${pill(readiness.status || "—", f015Kind(readiness.status))}</div>
    <div class="f015-checks">${(readiness.checks || []).map((check) => `<div class="f015-check"><div><strong>${esc(check.code || check.id)}</strong><small>${esc(check.detail || "")}</small></div>${pill(check.status || "—", f015Kind(check.status))}</div>`).join("")}</div>
  </div>`;
}

function f015RenderRuntime(installation, runtime) {
  if (installation.kind !== "PLUGIN") return `<div class="f015-detail"><strong>${esc(f015ManagerText("extensions.runtime"))}</strong><p class="muted">${esc(f015ManagerText("extensions.runtime_not_applicable"))}</p></div>`;
  if (!runtime || runtime._error) return `<div class="f015-detail"><strong>${esc(f015ManagerText("extensions.runtime"))}</strong><p class="muted">${esc(f015ReviewStatus(runtime))}</p></div>`;
  const enabled = runtime.desired_state === "ENABLED";
  return `<div class="f015-detail">
    <div class="section-title-row"><div><strong>${esc(f015ManagerText("extensions.runtime"))}</strong><p class="muted">${esc(runtime.desired_state || "—")} · ${esc(runtime.observed_state || "—")} · generation ${esc(runtime.generation ?? "—")}</p></div>${pill(runtime.observed_state || runtime.desired_state || "—", f015Kind(runtime.observed_state || runtime.desired_state))}</div>
    ${runtime.last_error_code ? `<p class="message error f015-warning">${esc(runtime.last_error_code)} · ${esc(runtime.last_error_message || "")}</p>` : ""}
    ${f015CanManage() ? `<div class="f015-actions"><button class="button ${enabled ? "ghost" : "primary"} f015-runtime-transition" data-transition="${enabled ? "disable" : "enable"}" type="button">${esc(enabled ? f015ManagerText("extensions.disable") : f015ManagerText("extensions.enable"))}</button></div>` : ""}
  </div>`;
}

function f015RenderInstallation(installation, details, packageByID) {
  const pkg = packageByID.get(installation.package_id);
  return `<article class="card f015-card" data-installation-id="${esc(installation.installation_id)}">
    <div class="f015-card-head"><div><p class="eyebrow">${esc(installation.extension_id)}</p><h2>${esc(pkg ? f015PackageName(pkg) : installation.extension_id)}</h2><p class="muted">v${esc(installation.version)} · ${esc(installation.kind)}</p></div>${pill(details?.readiness?.status || installation.lifecycle_state, f015Kind(details?.readiness?.status || installation.lifecycle_state))}</div>
    <div class="f015-meta"><div><span>${esc(f015ManagerText("extensions.installation_id"))}</span><strong class="mono">${esc(installation.installation_id)}</strong></div><div><span>${esc(f015ManagerText("extensions.package_id"))}</span><strong class="mono">${esc(installation.package_id)}</strong></div></div>
    ${f015RenderPermissions(installation, details?.permissions)}
    ${f015RenderReadiness(details?.readiness)}
    ${f015RenderRuntime(installation, details?.runtime)}
  </article>`;
}

async function renderExtensions() {
  setPage("extensions");
  document.querySelectorAll(".nav-button").forEach((button) => button.classList.remove("active"));
  el("extensionsNav")?.classList.add("active");
  setMessage(globalMessage, "");

  const model = await f015LoadModel();
  const packageByID = new Map(model.packages.map((item) => [item.package_id, item]));
  const installedByPackage = new Set(model.installations.map((item) => item.package_id));
  const readyCount = f015Count(model.installations, (item) => model.details[item.installation_id]?.readiness?.status === "READY_FOR_ACTIVATION");
  const runningCount = f015Count(model.installations, (item) => model.details[item.installation_id]?.runtime?.observed_state === "READY");

  content.innerHTML = `<div class="page-head"><div><p class="eyebrow" data-i18n="extensions.eyebrow">${esc(f015ManagerText("extensions.eyebrow"))}</p><h1 data-i18n="extensions.title">${esc(f015ManagerText("extensions.title"))}</h1><p data-i18n="extensions.summary">${esc(f015ManagerText("extensions.summary"))}</p></div><div class="toolbar"><button id="f015Refresh" class="button" type="button" data-i18n="extensions.refresh">${esc(f015ManagerText("extensions.refresh"))}</button></div></div>
    <p class="message warn" data-i18n="extensions.show_note">${esc(f015ManagerText("extensions.show_note"))}</p>
    ${!f015CanManage() ? `<p class="message" data-i18n="extensions.manage_role">${esc(f015ManagerText("extensions.manage_role"))}</p>` : ""}
    <div class="f015-summary-grid">
      <div class="f015-summary"><span data-i18n="extensions.library">${esc(f015ManagerText("extensions.library"))}</span><strong>${esc(model.packages.length)}</strong></div>
      <div class="f015-summary"><span data-i18n="extensions.installed">${esc(f015ManagerText("extensions.installed"))}</span><strong>${esc(model.installations.length)}</strong></div>
      <div class="f015-summary"><span data-i18n="extensions.ready">${esc(f015ManagerText("extensions.ready"))}</span><strong>${esc(readyCount)}</strong></div>
      <div class="f015-summary"><span data-i18n="extensions.running">${esc(f015ManagerText("extensions.running"))}</span><strong>${esc(runningCount)}</strong></div>
    </div>
    <section class="f015-section"><div class="section-title-row"><div><p class="eyebrow">LIBRARY</p><h2 data-i18n="extensions.available_packages">${esc(f015ManagerText("extensions.available_packages"))}</h2></div></div><div id="f015PlanArea"></div><div class="f015-grid">${model.packages.length ? model.packages.map((pkg) => f015RenderPackage(pkg, installedByPackage)).join("") : `<div class="f015-empty" data-i18n="extensions.no_packages">${esc(f015ManagerText("extensions.no_packages"))}</div>`}</div></section>
    <section class="f015-section"><div class="section-title-row"><div><p class="eyebrow">INSTALLED</p><h2 data-i18n="extensions.installed_extensions">${esc(f015ManagerText("extensions.installed_extensions"))}</h2></div></div><div class="f015-grid">${model.installations.length ? model.installations.map((item) => f015RenderInstallation(item, model.details[item.installation_id] || {}, packageByID)).join("") : `<div class="f015-empty" data-i18n="extensions.no_installations">${esc(f015ManagerText("extensions.no_installations"))}</div>`}</div></section>`;

  el("f015Refresh")?.addEventListener("click", () => renderExtensions().catch(f015Error));
  content.querySelectorAll(".f015-review-plan").forEach((button) => button.addEventListener("click", () => {
    const packageID = button.closest("[data-package-id]")?.dataset.packageId;
    if (packageID) f015ShowInstallPlan(packageID).catch(f015Error);
  }));
  content.querySelectorAll(".f015-permission-decision").forEach((button) => button.addEventListener("click", async () => {
    const installationID = button.closest("[data-installation-id]")?.dataset.installationId;
    const permission = button.closest("[data-permission]")?.dataset.permission;
    if (!installationID || !permission || !f015CanManage()) return;
    try {
      await api(`/api/v1/extensions/installations/${encodeURIComponent(installationID)}/permissions/${encodeURIComponent(permission)}`, { method: "PUT", json: { decision: button.dataset.decision } });
      await renderExtensions();
    } catch (error) { f015Error(error); }
  }));
  content.querySelectorAll(".f015-runtime-transition").forEach((button) => button.addEventListener("click", async () => {
    const installationID = button.closest("[data-installation-id]")?.dataset.installationId;
    if (!installationID || !f015CanManage()) return;
    try {
      await api(`/api/v1/extensions/installations/${encodeURIComponent(installationID)}/${button.dataset.transition}`, { method: "POST" });
      await renderExtensions();
    } catch (error) { f015Error(error); }
  }));
}

async function f015ShowInstallPlan(packageID) {
  const plan = await api(`/api/v1/extensions/packages/${encodeURIComponent(packageID)}/install-plan`);
  const area = el("f015PlanArea");
  if (!area) return;
  const blocked = plan.status === "BLOCKED" || (plan.blockers || []).length > 0;
  const steps = plan.steps || [];
  area.innerHTML = `<article class="card f015-plan"><div class="section-title-row"><div><p class="eyebrow" data-i18n="extensions.install_plan">${esc(f015ManagerText("extensions.install_plan"))}</p><h3>${esc(plan.root_extension_id)} · v${esc(plan.root_version)}</h3><p class="muted">${esc(blocked ? f015ManagerText("extensions.plan_blocked") : plan.status === "REQUIRES_DEPENDENCIES" ? f015ManagerText("extensions.plan_dependencies") : f015ManagerText("extensions.plan_ready"))}</p></div>${pill(plan.status || "—", f015Kind(plan.status))}</div>
    ${(plan.blockers || []).length ? `<h4 data-i18n="extensions.blockers">${esc(f015ManagerText("extensions.blockers"))}</h4><div class="f015-plan-list">${plan.blockers.map((item) => `<div class="f015-plan-step"><span>${esc(item.code)}</span><span class="muted">${esc(item.detail || item.extension_id || "")}</span></div>`).join("")}</div>` : ""}
    ${(plan.warnings || []).length ? `<h4 data-i18n="extensions.warnings">${esc(f015ManagerText("extensions.warnings"))}</h4><div class="f015-plan-list">${plan.warnings.map((item) => `<div class="f015-plan-step"><span>${esc(item.code)}</span><span class="muted">${esc(item.extension_id || "")}</span></div>`).join("")}</div>` : ""}
    ${steps.length ? `<h4 data-i18n="extensions.steps">${esc(f015ManagerText("extensions.steps"))}</h4><div class="f015-plan-list">${steps.map((step) => `<div class="f015-plan-step"><strong>${esc(step.order)}. ${esc(step.extension_id)}</strong><span>v${esc(step.version)} · ${esc(step.source)}</span></div>`).join("")}</div>` : ""}
    <div id="f015InstallStatus" class="muted"></div>
    ${f015CanManage() && !blocked && !plan.root_already_installed && steps.length ? `<div class="f015-actions"><button id="f015InstallPlan" class="button primary" type="button" data-i18n="extensions.install">${esc(f015ManagerText("extensions.install"))}</button></div>` : ""}
  </article>`;
  el("f015InstallPlan")?.addEventListener("click", () => f015ExecuteInstallPlan(steps).catch(f015Error));
  area.scrollIntoView({ behavior: "smooth", block: "start" });
}

async function f015ExecuteInstallPlan(steps) {
  if (!f015CanManage()) return;
  const status = el("f015InstallStatus");
  const button = el("f015InstallPlan");
  if (button) button.disabled = true;
  if (status) status.textContent = f015ManagerText("extensions.installing");
  try {
    for (const step of steps) {
      await api(`/api/v1/extensions/packages/${encodeURIComponent(step.package_id)}/install`, { method: "POST" });
    }
    if (status) status.textContent = f015ManagerText("extensions.install_complete");
    await renderExtensions();
  } catch (error) {
    if (button) button.disabled = false;
    throw error;
  }
}

function f015Error(error) {
  setMessage(globalMessage, errorMessage(error) || f015ManagerText("extensions.error"), "error");
}

const f015BaseShowApp = showApp;
const f015BaseShowLogin = showLogin;
showApp = function f015ShowApp() {
  f015BaseShowApp();
  el("extensionsNav")?.classList.remove("hidden");
};
showLogin = function f015ShowLogin(message = "") {
  el("extensionsNav")?.classList.add("hidden");
  f015BaseShowLogin(message);
};

const f015Nav = el("extensionsNav");
if (f015Nav) {
  f015Nav.textContent = f015ManagerText("extensions.nav");
  if (state.user) f015Nav.classList.remove("hidden");
  f015Nav.addEventListener("click", () => renderExtensions().catch(f015Error));
}
