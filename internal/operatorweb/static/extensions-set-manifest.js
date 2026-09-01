"use strict";

Object.assign(f015ManagerStrings, {
  "extensions.set_title": { en: "Backup and restore", "ar-IQ": "النسخ والاستعادة" },
  "extensions.set_summary": { en: "Export the installed extension set or restore the same verified versions from packages already present in this Hub's local library.", "ar-IQ": "صدّر مجموعة الإضافات المثبتة أو استعد نفس الإصدارات المتحقق منها من الحزم الموجودة مسبقاً في المكتبة المحلية لهذا الـ Hub." },
  "extensions.set_export": { en: "Export extension set", "ar-IQ": "تصدير مجموعة الإضافات" },
  "extensions.set_exporting": { en: "Exporting…", "ar-IQ": "جارٍ التصدير…" },
  "extensions.set_export_failed": { en: "Extension set export failed.", "ar-IQ": "فشل تصدير مجموعة الإضافات." },
  "extensions.set_restore_file": { en: "Extension set file", "ar-IQ": "ملف مجموعة الإضافات" },
  "extensions.set_review": { en: "Review restore", "ar-IQ": "مراجعة الاستعادة" },
  "extensions.set_reviewing": { en: "Reviewing…", "ar-IQ": "جارٍ المراجعة…" },
  "extensions.set_execute": { en: "Restore verified set", "ar-IQ": "استعادة المجموعة المتحقق منها" },
  "extensions.set_restoring": { en: "Restoring…", "ar-IQ": "جارٍ الاستعادة…" },
  "extensions.set_complete": { en: "Extension set restored. New Plugins remain disabled; review permissions before enabling them.", "ar-IQ": "تمت استعادة مجموعة الإضافات. تبقى الإضافات الجديدة معطلة؛ راجع الصلاحيات قبل تفعيلها." },
  "extensions.set_noop": { en: "This Hub already has the exact installed extension set.", "ar-IQ": "هذا الـ Hub يحتوي مسبقاً على نفس مجموعة الإضافات المثبتة بالضبط." },
  "extensions.set_blocked": { en: "Restore is blocked until every exact required package is available and compatible in the local library.", "ar-IQ": "الاستعادة محجوبة إلى أن تتوفر كل الحزم المطلوبة المطابقة والمتوافقة في المكتبة المحلية." },
  "extensions.set_safety": { en: "Restore never imports permission approvals or runtime enable state. Newly restored Plugins stay DISABLED and require a fresh permission review.", "ar-IQ": "الاستعادة لا تستورد موافقات الصلاحيات ولا حالة التفعيل. تبقى الإضافات المستعادة حديثاً DISABLED وتحتاج مراجعة صلاحيات جديدة." },
  "extensions.set_choose_file": { en: "Choose a StageCore extension set JSON file first.", "ar-IQ": "اختر أولاً ملف JSON لمجموعة إضافات StageCore." },
  "extensions.set_invalid": { en: "The extension set file is invalid.", "ar-IQ": "ملف مجموعة الإضافات غير صالح." },
  "extensions.set_show_locked": { en: "Extension restore is blocked while a SHOW session is active.", "ar-IQ": "استعادة الإضافات محجوبة أثناء وجود جلسة SHOW نشطة." },
  "extensions.set_confirm": { en: "Restore this verified extension set? New Plugins will remain disabled and permission approvals will not be restored.", "ar-IQ": "استعادة مجموعة الإضافات المتحقق منها؟ ستبقى الإضافات الجديدة معطلة ولن تتم استعادة موافقات الصلاحيات." },
});

let f015ExtensionSetRaw = "";
let f015ExtensionSetPlan = null;

function f015SetManifestPanel() {
  return `<section class="card f015-set-manifest" id="f015SetManifestPanel">
    <div class="section-title-row">
      <div><p class="eyebrow">${esc(f015ManagerText("extensions.set_title"))}</p><h2>${esc(f015ManagerText("extensions.set_title"))}</h2><p class="muted">${esc(f015ManagerText("extensions.set_summary"))}</p></div>
      <button class="button" id="f015SetExport" type="button">${esc(f015ManagerText("extensions.set_export"))}</button>
    </div>
    <p class="message warn">${esc(f015ManagerText("extensions.set_safety"))}</p>
    ${f015CanManage() ? `<div class="form-grid two f015-set-controls">
      <label>${esc(f015ManagerText("extensions.set_restore_file"))}<input id="f015SetFile" type="file" accept="application/json,.json"></label>
      <div class="f015-actions"><button class="button primary" id="f015SetReview" type="button">${esc(f015ManagerText("extensions.set_review"))}</button></div>
    </div><div id="f015SetPlan"></div>` : ""}
  </section>`;
}

function f015RenderSetPlan(plan) {
  const blocked = plan?.status === "BLOCKED" || (plan?.blockers || []).length > 0;
  const noop = plan?.status === "NOOP";
  const summary = blocked ? f015ManagerText("extensions.set_blocked") : noop ? f015ManagerText("extensions.set_noop") : f015ManagerText("extensions.plan_ready");
  return `<article class="f015-plan f015-inline-plan">
    <div class="section-title-row"><div><strong>${esc(f015ManagerText("extensions.set_review"))}</strong><p class="muted">${esc(summary)}</p></div>${pill(plan?.status || "—", f015Kind(plan?.status))}</div>
    ${(plan?.blockers || []).length ? `<h4>${esc(f015ManagerText("extensions.blockers"))}</h4><div class="f015-plan-list">${plan.blockers.map((item) => `<div class="f015-plan-step"><strong>${esc(item.code)}</strong><span class="muted">${esc(item.extension_id || item.detail || "")}${item.detail && item.extension_id ? ` · ${esc(item.detail)}` : ""}</span></div>`).join("")}</div>` : ""}
    ${(plan?.steps || []).length ? `<h4>${esc(f015ManagerText("extensions.steps"))}</h4><div class="f015-plan-list">${plan.steps.map((step) => `<div class="f015-plan-step"><strong>${esc(step.order)}. ${esc(step.extension_id)}</strong><span>v${esc(step.version)} · ${esc(step.action)}</span></div>`).join("")}</div>` : ""}
    <p class="message warn">${esc(f015ManagerText("extensions.set_safety"))}</p>
    ${!blocked && !noop && plan?.status === "READY" ? `<div class="f015-actions"><button class="button primary" id="f015SetExecute" type="button">${esc(f015ManagerText("extensions.set_execute"))}</button></div>` : ""}
  </article>`;
}

async function f015ExportExtensionSet(button) {
  const original = button.textContent;
  button.disabled = true;
  button.textContent = f015ManagerText("extensions.set_exporting");
  setMessage(globalMessage, "");
  try {
    const response = await fetch("/api/v1/extensions/set-manifest", { credentials: "same-origin", cache: "no-store" });
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    const blob = await response.blob();
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = "stagecore-extension-set.json";
    document.body.appendChild(link);
    link.click();
    link.remove();
    setTimeout(() => URL.revokeObjectURL(url), 0);
  } catch (error) {
    setMessage(globalMessage, f015ManagerText("extensions.set_export_failed"), "error");
  } finally {
    button.disabled = false;
    button.textContent = original;
  }
}

async function f015ReviewExtensionSet(button) {
  if (!f015CanManage() || button.disabled) return;
  const file = el("f015SetFile")?.files?.[0];
  if (!file) {
    setMessage(globalMessage, f015ManagerText("extensions.set_choose_file"), "warn");
    return;
  }
  const original = button.textContent;
  button.disabled = true;
  button.textContent = f015ManagerText("extensions.set_reviewing");
  setMessage(globalMessage, "");
  try {
    f015ExtensionSetRaw = await file.text();
    f015ExtensionSetPlan = await api("/api/v1/extensions/set-manifest/restore-plan", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: f015ExtensionSetRaw,
    });
    const area = el("f015SetPlan");
    if (area) area.innerHTML = f015RenderSetPlan(f015ExtensionSetPlan);
    el("f015SetExecute")?.addEventListener("click", (event) => f015ExecuteExtensionSet(event.currentTarget));
  } catch (error) {
    const code = error?.payload?.error_code;
    setMessage(globalMessage, code === "EXTENSION_SET_INVALID" ? f015ManagerText("extensions.set_invalid") : errorMessage(error), "error");
    f015ExtensionSetPlan = null;
  } finally {
    button.disabled = false;
    button.textContent = original;
  }
}

async function f015ExecuteExtensionSet(button) {
  if (!f015CanManage() || !f015ExtensionSetRaw || f015ExtensionSetPlan?.status !== "READY" || button.disabled) return;
  if (!globalThis.confirm(f015ManagerText("extensions.set_confirm"))) return;
  const original = button.textContent;
  button.disabled = true;
  button.textContent = f015ManagerText("extensions.set_restoring");
  setMessage(globalMessage, "");
  try {
    await api("/api/v1/extensions/set-manifest/restore", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: f015ExtensionSetRaw,
    });
    f015ExtensionSetRaw = "";
    f015ExtensionSetPlan = null;
    await renderExtensions();
    setMessage(globalMessage, f015ManagerText("extensions.set_complete"), "success");
  } catch (error) {
    const code = error?.payload?.error_code;
    let message = errorMessage(error);
    if (code === "SHOW_CONFIGURATION_LOCKED") message = f015ManagerText("extensions.set_show_locked");
    if (code === "EXTENSION_SET_INVALID") message = f015ManagerText("extensions.set_invalid");
    if (code === "EXTENSION_SET_RESTORE_BLOCKED") message = f015ManagerText("extensions.set_blocked");
    setMessage(globalMessage, message, "error");
    button.disabled = false;
    button.textContent = original;
  }
}

function f015AttachExtensionSetPanel() {
  if (el("f015SetManifestPanel")) return;
  const summary = content.querySelector(".f015-summary-grid");
  if (!summary) return;
  summary.insertAdjacentHTML("beforebegin", f015SetManifestPanel());
  el("f015SetExport")?.addEventListener("click", (event) => f015ExportExtensionSet(event.currentTarget));
  el("f015SetReview")?.addEventListener("click", (event) => f015ReviewExtensionSet(event.currentTarget));
}

const f015BaseRenderExtensionsForSetManifest = renderExtensions;
renderExtensions = async function f015RenderExtensionsWithSetManifest() {
  await f015BaseRenderExtensionsForSetManifest();
  f015AttachExtensionSetPanel();
};
