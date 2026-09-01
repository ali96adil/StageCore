"use strict";

Object.assign(f015ManagerStrings, {
  "extensions.show_note": { en: "Installation, permission changes, runtime enable/disable and uninstall remain server-side blocked during an active SHOW.", "ar-IQ": "يبقى التثبيت وتغيير الصلاحيات وتفعيل أو تعطيل التشغيل وإلغاء التثبيت محجوباً من الخادم أثناء العرض النشط." },
  "extensions.removal": { en: "Removal", "ar-IQ": "الإزالة" },
  "extensions.uninstall": { en: "Uninstall", "ar-IQ": "إلغاء التثبيت" },
  "extensions.uninstall_ready": { en: "Removes this installation and its runtime state. The immutable package stays in the local library so it can be installed again later.", "ar-IQ": "يزيل هذا التثبيت وحالة تشغيله. تبقى الحزمة الثابتة في المكتبة المحلية حتى يمكن تثبيتها مرة أخرى لاحقاً." },
  "extensions.uninstall_runtime_blocked": { en: "Disable the Plugin and wait until its runtime is STOPPED before uninstalling it.", "ar-IQ": "عطّل الإضافة وانتظر حتى تصبح حالة التشغيل STOPPED قبل إلغاء تثبيتها." },
  "extensions.uninstall_runtime_unknown": { en: "Runtime status is unavailable. Refresh the page before uninstalling.", "ar-IQ": "حالة التشغيل غير متاحة. حدّث الصفحة قبل إلغاء التثبيت." },
  "extensions.uninstall_confirm": { en: "Uninstall {name} v{version}? The package will remain in the local library for reinstall.", "ar-IQ": "إلغاء تثبيت {name} الإصدار {version}؟ ستبقى الحزمة في المكتبة المحلية لإعادة تثبيتها لاحقاً." },
  "extensions.uninstalling": { en: "Uninstalling…", "ar-IQ": "جارٍ إلغاء التثبيت…" },
  "extensions.uninstall_complete": { en: "Extension uninstalled.", "ar-IQ": "تم إلغاء تثبيت الإضافة." },
  "extensions.uninstall_dependency_blocked": { en: "Uninstall is blocked because another installed extension still requires this extension", "ar-IQ": "إلغاء التثبيت محجوب لأن إضافة مثبتة أخرى ما زالت تعتمد على هذه الإضافة" },
  "extensions.uninstall_runtime_required": { en: "Disable the Plugin and wait until it is STOPPED before uninstalling.", "ar-IQ": "عطّل الإضافة وانتظر حتى تصبح STOPPED قبل إلغاء التثبيت." },
  "extensions.uninstall_show_locked": { en: "Uninstall is blocked while a SHOW session is active.", "ar-IQ": "إلغاء التثبيت محجوب أثناء وجود جلسة SHOW نشطة." },
  "extensions.uninstall_cleanup_warning": { en: "The installation was removed, but an inert payload file could not be cleaned up. Reinstall will verify it before reuse.", "ar-IQ": "تمت إزالة التثبيت، لكن تعذر تنظيف ملف حزمة غير نشط. ستتحقق إعادة التثبيت من الملف قبل إعادة استخدامه." },
});

function f015FormatManagerText(key, values = {}) {
  let text = f015ManagerText(key);
  Object.entries(values).forEach(([name, value]) => {
    text = text.replaceAll(`{${name}}`, String(value));
  });
  return text;
}

function f015UninstallAvailability(installation, runtime) {
  if (!f015CanManage()) return { allowed: false, reason: "" };
  if (installation.kind !== "PLUGIN") return { allowed: true, reason: f015ManagerText("extensions.uninstall_ready") };
  if (!runtime || runtime._error) {
    return { allowed: false, reason: f015ManagerText("extensions.uninstall_runtime_unknown") };
  }
  const stopped = runtime.desired_state === "DISABLED" && runtime.observed_state === "STOPPED";
  return {
    allowed: stopped,
    reason: stopped ? f015ManagerText("extensions.uninstall_ready") : f015ManagerText("extensions.uninstall_runtime_blocked"),
  };
}

function f015RenderUninstall(installation, details, packageByID) {
  if (!f015CanManage()) return "";
  const pkg = packageByID.get(installation.package_id);
  const availability = f015UninstallAvailability(installation, details?.runtime);
  const name = pkg ? f015PackageName(pkg) : installation.extension_id;
  return `<div class="f015-detail f015-uninstall-zone">
    <div class="section-title-row"><strong>${esc(f015ManagerText("extensions.removal"))}</strong>${pill(availability.allowed ? "READY" : "BLOCKED", availability.allowed ? "good" : "warn")}</div>
    <p class="muted">${esc(availability.reason)}</p>
    <div class="f015-actions"><button class="button danger f015-uninstall" type="button" data-extension-name="${esc(name)}" data-extension-version="${esc(installation.version)}" ${availability.allowed ? "" : "disabled"}>${esc(f015ManagerText("extensions.uninstall"))}</button></div>
  </div>`;
}

const f015BaseRenderInstallationForUninstall = f015RenderInstallation;
f015RenderInstallation = function f015RenderInstallationWithUninstall(installation, details, packageByID) {
  const rendered = f015BaseRenderInstallationForUninstall(installation, details, packageByID);
  const removal = f015RenderUninstall(installation, details, packageByID);
  return rendered.replace(/<\/article>\s*$/, `${removal}</article>`);
};

function f015UninstallErrorMessage(error) {
  const code = error?.payload?.error_code;
  if (code === "EXTENSION_RUNTIME_MUST_BE_DISABLED") return f015ManagerText("extensions.uninstall_runtime_required");
  if (code === "SHOW_CONFIGURATION_LOCKED") return f015ManagerText("extensions.uninstall_show_locked");
  if (code === "EXTENSION_REQUIRED_BY_INSTALLED") {
    const blockers = error?.payload?.blockers || [];
    const names = blockers.map((item) => `${item.required_by}${item.required_by_version ? ` v${item.required_by_version}` : ""}`);
    const suffix = names.length ? `: ${names.join(", ")}` : ".";
    return `${f015ManagerText("extensions.uninstall_dependency_blocked")}${suffix}`;
  }
  return errorMessage(error) || f015ManagerText("extensions.error");
}

async function f015ExecuteUninstall(button) {
  const card = button.closest("[data-installation-id]");
  const installationID = card?.dataset.installationId;
  if (!installationID || !f015CanManage() || button.disabled) return;

  const name = button.dataset.extensionName || installationID;
  const version = button.dataset.extensionVersion || "—";
  const confirmed = globalThis.confirm(f015FormatManagerText("extensions.uninstall_confirm", { name, version }));
  if (!confirmed) return;

  const originalText = button.textContent;
  button.disabled = true;
  button.textContent = f015ManagerText("extensions.uninstalling");
  setMessage(globalMessage, "");
  try {
    const result = await api(`/api/v1/extensions/installations/${encodeURIComponent(installationID)}`, { method: "DELETE" });
    const notice = result?.cleanup_warning
      ? { text: f015ManagerText("extensions.uninstall_cleanup_warning"), kind: "warn" }
      : { text: f015ManagerText("extensions.uninstall_complete"), kind: "success" };
    await renderExtensions();
    setMessage(globalMessage, notice.text, notice.kind);
  } catch (error) {
    button.disabled = false;
    button.textContent = originalText;
    setMessage(globalMessage, f015UninstallErrorMessage(error), "error");
  }
}

const f015BaseRenderExtensionsForUninstall = renderExtensions;
renderExtensions = async function f015RenderExtensionsWithUninstall() {
  await f015BaseRenderExtensionsForUninstall();
  content.querySelectorAll(".f015-uninstall").forEach((button) => {
    button.addEventListener("click", () => f015ExecuteUninstall(button));
  });
};

Object.assign(f015ManagerStrings, {
  "extensions.add_packages": { en: "Add extension packages", "ar-IQ": "إضافة حزم إضافات" },
  "extensions.offline_bundle": { en: "Offline bundle", "ar-IQ": "حزمة محلية" },
  "extensions.offline_bundle_detail": { en: "Choose a .scext bundle from this computer. Local uploads remain LOCAL or COMMUNITY and never gain OFFICIAL trust.", "ar-IQ": "اختر حزمة .scext من هذا الحاسوب. الملفات المرفوعة محلياً تبقى LOCAL أو COMMUNITY ولا تحصل أبداً على ثقة OFFICIAL." },
  "extensions.choose_bundle": { en: "Choose .scext bundle", "ar-IQ": "اختيار حزمة .scext" },
  "extensions.import_bundle": { en: "Import bundle", "ar-IQ": "استيراد الحزمة" },
  "extensions.importing_bundle": { en: "Importing and verifying…", "ar-IQ": "جارٍ الاستيراد والتحقق…" },
  "extensions.import_complete": { en: "Offline bundle verified and added to the local library.", "ar-IQ": "تم التحقق من الحزمة المحلية وإضافتها إلى المكتبة." },
  "extensions.import_existing": { en: "This exact bundle is already registered in the local library.", "ar-IQ": "هذه الحزمة نفسها مسجلة مسبقاً في المكتبة المحلية." },
  "extensions.official_catalog": { en: "Official catalog", "ar-IQ": "الكتالوج الرسمي" },
  "extensions.official_catalog_detail": { en: "Sync OFFICIAL bundles only from the StageCore-owned trusted catalog on this Hub. The browser cannot choose or override that path.", "ar-IQ": "زامن حزم OFFICIAL فقط من كتالوج StageCore الموثوق على هذا الـ Hub. المتصفح لا يستطيع اختيار هذا المسار أو تغييره." },
  "extensions.sync_catalog": { en: "Sync official catalog", "ar-IQ": "مزامنة الكتالوج الرسمي" },
  "extensions.syncing_catalog": { en: "Syncing trusted catalog…", "ar-IQ": "جارٍ مزامنة الكتالوج الموثوق…" },
  "extensions.catalog_complete": { en: "Official catalog synchronized. {count} bundle(s) checked.", "ar-IQ": "تمت مزامنة الكتالوج الرسمي. تم فحص {count} حزمة." },
  "extensions.offline_show_locked": { en: "Package import and catalog sync are blocked while a SHOW session is active.", "ar-IQ": "استيراد الحزم ومزامنة الكتالوج محجوبان أثناء وجود جلسة SHOW نشطة." },
  "extensions.offline_source_forbidden": { en: "A browser upload cannot claim OFFICIAL provenance. Use the trusted StageCore catalog for official bundles.", "ar-IQ": "لا يمكن لملف مرفوع من المتصفح ادعاء مصدر OFFICIAL. استخدم كتالوج StageCore الموثوق للحزم الرسمية." },
  "extensions.offline_integrity_failed": { en: "Bundle verification failed because its payload hash or size does not match.", "ar-IQ": "فشل التحقق من الحزمة لأن بصمة الملف أو حجمه لا يطابق البيانات المعلنة." },
  "extensions.offline_invalid": { en: "The selected file is not a valid StageCore extension bundle.", "ar-IQ": "الملف المحدد ليس حزمة إضافات StageCore صالحة." },
  "extensions.offline_too_large": { en: "The selected extension bundle exceeds the supported size limit.", "ar-IQ": "حزمة الإضافة المحددة تتجاوز الحجم المدعوم." },
  "extensions.catalog_unavailable": { en: "The trusted official catalog is unavailable on this Hub.", "ar-IQ": "الكتالوج الرسمي الموثوق غير متاح على هذا الـ Hub." },
});

function f015OfflineErrorMessage(error) {
  const code = error?.payload?.error_code;
  if (code === "SHOW_CONFIGURATION_LOCKED") return f015ManagerText("extensions.offline_show_locked");
  if (code === "EXTENSION_BUNDLE_SOURCE_FORBIDDEN") return f015ManagerText("extensions.offline_source_forbidden");
  if (code === "EXTENSION_BUNDLE_INTEGRITY_FAILED") return f015ManagerText("extensions.offline_integrity_failed");
  if (code === "EXTENSION_BUNDLE_INVALID" || code === "EXTENSION_BUNDLE_IMPORT_FAILED") return f015ManagerText("extensions.offline_invalid");
  if (code === "EXTENSION_BUNDLE_TOO_LARGE") return f015ManagerText("extensions.offline_too_large");
  if (code === "EXTENSION_CATALOG_UNAVAILABLE") return f015ManagerText("extensions.catalog_unavailable");
  return errorMessage(error) || f015ManagerText("extensions.error");
}

function f015RenderOfflineImportPanel() {
  if (!f015CanManage()) return "";
  return `<section class="f015-section f015-package-import">
    <div class="section-title-row"><div><p class="eyebrow">F-015</p><h2>${esc(f015ManagerText("extensions.add_packages"))}</h2></div></div>
    <div class="f015-grid">
      <article class="card f015-card">
        <div><strong>${esc(f015ManagerText("extensions.offline_bundle"))}</strong><p class="muted">${esc(f015ManagerText("extensions.offline_bundle_detail"))}</p></div>
        <input class="f015-bundle-file" type="file" accept=".scext,application/vnd.stagecore.extension-bundle" aria-label="${esc(f015ManagerText("extensions.choose_bundle"))}">
        <div class="f015-actions"><button class="button primary f015-import-bundle" type="button" disabled>${esc(f015ManagerText("extensions.import_bundle"))}</button></div>
      </article>
      <article class="card f015-card">
        <div><strong>${esc(f015ManagerText("extensions.official_catalog"))}</strong><p class="muted">${esc(f015ManagerText("extensions.official_catalog_detail"))}</p></div>
        <div class="f015-actions"><button class="button f015-sync-catalog" type="button">${esc(f015ManagerText("extensions.sync_catalog"))}</button></div>
      </article>
    </div>
    <p class="muted f015-warning">${esc(f015ManagerText("extensions.offline_show_locked"))}</p>
  </section>`;
}

async function f015ExecuteOfflineImport(input, button) {
  const file = input?.files?.[0];
  if (!file || !f015CanManage() || button.disabled) return;
  const original = button.textContent;
  button.disabled = true;
  button.textContent = f015ManagerText("extensions.importing_bundle");
  setMessage(globalMessage, "");
  try {
    const result = await api("/api/v1/extensions/import-bundle", {
      method: "POST",
      headers: { "Content-Type": "application/vnd.stagecore.extension-bundle" },
      body: file,
    });
    const notice = result?.already_registered ? f015ManagerText("extensions.import_existing") : f015ManagerText("extensions.import_complete");
    await renderExtensions();
    setMessage(globalMessage, notice, "success");
  } catch (error) {
    button.disabled = false;
    button.textContent = original;
    setMessage(globalMessage, f015OfflineErrorMessage(error), "error");
  }
}

async function f015ExecuteCatalogSync(button) {
  if (!f015CanManage() || button.disabled) return;
  const original = button.textContent;
  button.disabled = true;
  button.textContent = f015ManagerText("extensions.syncing_catalog");
  setMessage(globalMessage, "");
  try {
    const result = await api("/api/v1/extensions/catalog/sync", { method: "POST" });
    const count = (result?.imported || []).length;
    await renderExtensions();
    setMessage(globalMessage, f015FormatManagerText("extensions.catalog_complete", { count }), "success");
  } catch (error) {
    button.disabled = false;
    button.textContent = original;
    setMessage(globalMessage, f015OfflineErrorMessage(error), "error");
  }
}

const f015BaseRenderExtensionsForOfflineImport = renderExtensions;
renderExtensions = async function f015RenderExtensionsWithOfflineImport() {
  await f015BaseRenderExtensionsForOfflineImport();
  if (!f015CanManage()) return;
  const firstSection = content.querySelector(".f015-section");
  const panel = f015RenderOfflineImportPanel();
  if (firstSection && panel) firstSection.insertAdjacentHTML("beforebegin", panel);
  const input = content.querySelector(".f015-bundle-file");
  const importButton = content.querySelector(".f015-import-bundle");
  input?.addEventListener("change", () => {
    if (importButton) importButton.disabled = !input.files?.length;
  });
  importButton?.addEventListener("click", () => f015ExecuteOfflineImport(input, importButton));
  const syncButton = content.querySelector(".f015-sync-catalog");
  syncButton?.addEventListener("click", () => f015ExecuteCatalogSync(syncButton));
};
