"use strict";

Object.assign(f015ManagerStrings, {
  "extensions.show_note": {
    en: "Installation, permission changes, runtime enable/disable and uninstall remain server-side blocked during an active SHOW.",
    "ar-IQ": "يبقى التثبيت وتغيير الصلاحيات وتفعيل أو تعطيل التشغيل وإلغاء التثبيت محجوباً من الخادم أثناء العرض النشط.",
  },
  "extensions.removal": { en: "Removal", "ar-IQ": "الإزالة" },
  "extensions.uninstall": { en: "Uninstall", "ar-IQ": "إلغاء التثبيت" },
  "extensions.uninstall_ready": {
    en: "Removes this installation and its runtime state. The immutable package stays in the local library so it can be installed again later.",
    "ar-IQ": "يزيل هذا التثبيت وحالة تشغيله. تبقى الحزمة الثابتة في المكتبة المحلية حتى يمكن تثبيتها مرة أخرى لاحقاً.",
  },
  "extensions.uninstall_runtime_blocked": {
    en: "Disable the Plugin and wait until its runtime is STOPPED before uninstalling it.",
    "ar-IQ": "عطّل الإضافة وانتظر حتى تصبح حالة التشغيل STOPPED قبل إلغاء تثبيتها.",
  },
  "extensions.uninstall_runtime_unknown": {
    en: "Runtime status is unavailable. Refresh the page before uninstalling.",
    "ar-IQ": "حالة التشغيل غير متاحة. حدّث الصفحة قبل إلغاء التثبيت.",
  },
  "extensions.uninstall_confirm": {
    en: "Uninstall {name} v{version}? The package will remain in the local library for reinstall.",
    "ar-IQ": "إلغاء تثبيت {name} الإصدار {version}؟ ستبقى الحزمة في المكتبة المحلية لإعادة تثبيتها لاحقاً.",
  },
  "extensions.uninstalling": { en: "Uninstalling…", "ar-IQ": "جارٍ إلغاء التثبيت…" },
  "extensions.uninstall_complete": { en: "Extension uninstalled.", "ar-IQ": "تم إلغاء تثبيت الإضافة." },
  "extensions.uninstall_dependency_blocked": {
    en: "Uninstall is blocked because another installed extension still requires this extension",
    "ar-IQ": "إلغاء التثبيت محجوب لأن إضافة مثبتة أخرى ما زالت تعتمد على هذه الإضافة",
  },
  "extensions.uninstall_runtime_required": {
    en: "Disable the Plugin and wait until it is STOPPED before uninstalling.",
    "ar-IQ": "عطّل الإضافة وانتظر حتى تصبح STOPPED قبل إلغاء التثبيت.",
  },
  "extensions.uninstall_show_locked": {
    en: "Uninstall is blocked while a SHOW session is active.",
    "ar-IQ": "إلغاء التثبيت محجوب أثناء وجود جلسة SHOW نشطة.",
  },
  "extensions.uninstall_cleanup_warning": {
    en: "The installation was removed, but an inert payload file could not be cleaned up. Reinstall will verify it before reuse.",
    "ar-IQ": "تمت إزالة التثبيت، لكن تعذر تنظيف ملف حزمة غير نشط. ستتحقق إعادة التثبيت من الملف قبل إعادة استخدامه.",
  },
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
    if (result?.cleanup_warning) {
      setMessage(globalMessage, f015ManagerText("extensions.uninstall_cleanup_warning"), "warn");
    } else {
      setMessage(globalMessage, f015ManagerText("extensions.uninstall_complete"), "success");
    }
    await renderExtensions();
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
