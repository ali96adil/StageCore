"use strict";

Object.assign(f015ManagerStrings, {
  "extensions.show_note": { en: "Installation, permission changes, runtime enable/disable, update/rollback, repair and uninstall remain server-side blocked during an active SHOW.", "ar-IQ": "يبقى التثبيت وتغيير الصلاحيات وتفعيل أو تعطيل التشغيل والتحديث أو الرجوع والإصلاح وإلغاء التثبيت محجوباً من الخادم أثناء العرض النشط." },
  "extensions.maintenance": { en: "Maintenance", "ar-IQ": "الصيانة" },
  "extensions.change_version": { en: "Change version", "ar-IQ": "تغيير الإصدار" },
  "extensions.target_version": { en: "Target version", "ar-IQ": "الإصدار الهدف" },
  "extensions.review_change": { en: "Review change", "ar-IQ": "مراجعة التغيير" },
  "extensions.no_other_versions": { en: "No other compatible version is available in the local library.", "ar-IQ": "لا يتوفر إصدار متوافق آخر في المكتبة المحلية." },
  "extensions.version_reset_note": { en: "Changing version clears prior permission approvals and runtime enable intent. Review permissions again before enabling the replacement version.", "ar-IQ": "تغيير الإصدار يمسح موافقات الصلاحيات السابقة ونية التفعيل. راجع الصلاحيات من جديد قبل تفعيل الإصدار البديل." },
  "extensions.update_plan": { en: "Version change plan", "ar-IQ": "خطة تغيير الإصدار" },
  "extensions.update": { en: "Update", "ar-IQ": "تحديث" },
  "extensions.rollback": { en: "Rollback", "ar-IQ": "رجوع لإصدار سابق" },
  "extensions.install_dependencies": { en: "Install required dependencies", "ar-IQ": "تثبيت الاعتماديات المطلوبة" },
  "extensions.dependencies_installed": { en: "Required dependencies were installed. Review the version change again.", "ar-IQ": "تم تثبيت الاعتماديات المطلوبة. راجع تغيير الإصدار مرة أخرى." },
  "extensions.update_confirm": { en: "Update {name} from v{from} to v{to}? Permission approvals and runtime enable intent will be cleared.", "ar-IQ": "تحديث {name} من الإصدار {from} إلى {to}؟ سيتم مسح موافقات الصلاحيات ونية التفعيل." },
  "extensions.rollback_confirm": { en: "Rollback {name} from v{from} to v{to}? Permission approvals and runtime enable intent will be cleared.", "ar-IQ": "الرجوع بـ {name} من الإصدار {from} إلى {to}؟ سيتم مسح موافقات الصلاحيات ونية التفعيل." },
  "extensions.updating": { en: "Updating…", "ar-IQ": "جارٍ التحديث…" },
  "extensions.rolling_back": { en: "Rolling back…", "ar-IQ": "جارٍ الرجوع للإصدار السابق…" },
  "extensions.update_complete": { en: "Extension updated. Review permissions again before enabling it.", "ar-IQ": "تم تحديث الإضافة. راجع الصلاحيات من جديد قبل تفعيلها." },
  "extensions.rollback_complete": { en: "Extension rolled back. Review permissions again before enabling it.", "ar-IQ": "تم الرجوع إلى الإصدار السابق. راجع الصلاحيات من جديد قبل تفعيل الإضافة." },
  "extensions.update_cleanup_warning": { en: "The version changed, but an inert old payload could not be cleaned up. It will never be executed as the active installation.", "ar-IQ": "تم تغيير الإصدار، لكن تعذر تنظيف ملف قديم غير نشط. لن يتم تشغيله كتثبيت فعّال." },
  "extensions.update_runtime_required": { en: "Disable the Plugin and wait until it is STOPPED before changing version or repairing it.", "ar-IQ": "عطّل الإضافة وانتظر حتى تصبح STOPPED قبل تغيير الإصدار أو إصلاحها." },
  "extensions.update_show_locked": { en: "Extension maintenance is blocked while a SHOW session is active.", "ar-IQ": "صيانة الإضافة محجوبة أثناء وجود جلسة SHOW نشطة." },
  "extensions.update_dependencies_required": { en: "Install the required dependencies before changing this version.", "ar-IQ": "ثبّت الاعتماديات المطلوبة قبل تغيير هذا الإصدار." },
  "extensions.update_plan_blocked": { en: "This version change is blocked by compatibility or installed dependency requirements.", "ar-IQ": "تغيير هذا الإصدار محجوب بسبب التوافق أو متطلبات إضافات مثبتة أخرى." },
  "extensions.repair": { en: "Repair payload", "ar-IQ": "إصلاح ملف الإضافة" },
  "extensions.repair_ready": { en: "Rebuilds the installed payload from the immutable local package without changing version or approved permissions.", "ar-IQ": "يعيد بناء ملف الإضافة المثبتة من الحزمة المحلية الثابتة بدون تغيير الإصدار أو الصلاحيات الموافق عليها." },
  "extensions.repair_confirm": { en: "Repair {name} v{version} from the immutable local package?", "ar-IQ": "إصلاح {name} الإصدار {version} من الحزمة المحلية الثابتة؟" },
  "extensions.repairing": { en: "Repairing…", "ar-IQ": "جارٍ الإصلاح…" },
  "extensions.repair_complete": { en: "Installed payload repaired and verified.", "ar-IQ": "تم إصلاح ملف الإضافة المثبتة والتحقق منه." },
  "extensions.repair_healthy": { en: "Installed payload is already healthy and verified.", "ar-IQ": "ملف الإضافة المثبتة سليم ومتحقق منه أصلاً." },
  "extensions.maintenance_runtime_unknown": { en: "Runtime status is unavailable. Refresh before changing version or repairing this Plugin.", "ar-IQ": "حالة التشغيل غير متاحة. حدّث الصفحة قبل تغيير إصدار هذه الإضافة أو إصلاحها." },
});

let f015MaintenanceModel = { packages: [], installations: [], details: {} };
const f015BaseLoadModelForMaintenance = f015LoadModel;
f015LoadModel = async function f015LoadModelWithMaintenance() {
  const model = await f015BaseLoadModelForMaintenance();
  f015MaintenanceModel = model;
  return model;
};

function f015MaintenanceAvailability(installation, runtime) {
  if (!f015CanManage()) return { allowed: false, reason: "" };
  if (installation.kind !== "PLUGIN") return { allowed: true, reason: f015ManagerText("extensions.repair_ready") };
  if (!runtime || runtime._error) return { allowed: false, reason: f015ManagerText("extensions.maintenance_runtime_unknown") };
  const stopped = runtime.desired_state === "DISABLED" && runtime.observed_state === "STOPPED";
  return { allowed: stopped, reason: stopped ? f015ManagerText("extensions.repair_ready") : f015ManagerText("extensions.update_runtime_required") };
}

function f015MaintenanceCandidates(installation) {
  return (f015MaintenanceModel.packages || []).filter((pkg) => {
    const manifest = pkg.manifest || {};
    return pkg.package_id !== installation.package_id
      && pkg.compatible
      && manifest.extension_id === installation.extension_id
      && manifest.kind === installation.kind;
  });
}

function f015RenderMaintenance(installation, details, packageByID) {
  if (!f015CanManage()) return "";
  const pkg = packageByID.get(installation.package_id);
  const name = pkg ? f015PackageName(pkg) : installation.extension_id;
  const availability = f015MaintenanceAvailability(installation, details?.runtime);
  const candidates = f015MaintenanceCandidates(installation);
  const candidateOptions = candidates.map((candidate) => `<option value="${esc(candidate.package_id)}">v${esc(candidate.manifest?.version || "—")} · ${esc(candidate.manifest?.source || "—")}</option>`).join("");
  const changeReason = candidates.length
    ? f015ManagerText("extensions.version_reset_note")
    : f015ManagerText("extensions.no_other_versions");
  return `<div class="f015-detail f015-maintenance-zone" data-extension-name="${esc(name)}" data-current-version="${esc(installation.version)}">
    <div class="section-title-row"><strong>${esc(f015ManagerText("extensions.maintenance"))}</strong>${pill(availability.allowed ? "READY" : "BLOCKED", availability.allowed ? "good" : "warn")}</div>
    <p class="muted">${esc(availability.reason)}</p>
    <div class="f015-maintenance-block">
      <strong>${esc(f015ManagerText("extensions.change_version"))}</strong>
      <p class="muted">${esc(changeReason)}</p>
      ${candidates.length ? `<label class="f015-maintenance-target-label"><span>${esc(f015ManagerText("extensions.target_version"))}</span><select class="f015-maintenance-target" ${availability.allowed ? "" : "disabled"}>${candidateOptions}</select></label><div class="f015-actions"><button class="button f015-review-update" type="button" ${availability.allowed ? "" : "disabled"}>${esc(f015ManagerText("extensions.review_change"))}</button></div>` : ""}
      <div class="f015-maintenance-plan"></div>
    </div>
    <div class="f015-maintenance-block">
      <strong>${esc(f015ManagerText("extensions.repair"))}</strong>
      <p class="muted">${esc(f015ManagerText("extensions.repair_ready"))}</p>
      <div class="f015-actions"><button class="button ghost f015-repair" type="button" ${availability.allowed ? "" : "disabled"}>${esc(f015ManagerText("extensions.repair"))}</button></div>
    </div>
  </div>`;
}

const f015BaseRenderInstallationForMaintenance = f015RenderInstallation;
f015RenderInstallation = function f015RenderInstallationWithMaintenance(installation, details, packageByID) {
  const rendered = f015BaseRenderInstallationForMaintenance(installation, details, packageByID);
  const maintenance = f015RenderMaintenance(installation, details, packageByID);
  if (!maintenance) return rendered;
  const removalMarker = `<div class="f015-detail f015-uninstall-zone">`;
  if (rendered.includes(removalMarker)) return rendered.replace(removalMarker, `${maintenance}${removalMarker}`);
  return rendered.replace(/<\/article>\s*$/, `${maintenance}</article>`);
};

function f015RenderUpdatePlan(plan) {
  const blocked = plan.status === "BLOCKED" || (plan.blockers || []).length > 0;
  const requiresDependencies = plan.status === "REQUIRES_DEPENDENCIES";
  const actionLabel = plan.direction === "ROLLBACK" ? f015ManagerText("extensions.rollback") : f015ManagerText("extensions.update");
  return `<article class="f015-plan f015-inline-plan">
    <div class="section-title-row"><div><strong>${esc(f015ManagerText("extensions.update_plan"))}</strong><p class="muted">v${esc(plan.current_version || "—")} → v${esc(plan.target_version || "—")}</p></div>${pill(plan.status || "—", f015Kind(plan.status))}</div>
    ${(plan.blockers || []).length ? `<h4>${esc(f015ManagerText("extensions.blockers"))}</h4><div class="f015-plan-list">${plan.blockers.map((item) => `<div class="f015-plan-step"><strong>${esc(item.code)}</strong><span class="muted">${esc(item.detail || item.required_by || item.extension_id || "")}</span></div>`).join("")}</div>` : ""}
    ${(plan.warnings || []).length ? `<h4>${esc(f015ManagerText("extensions.warnings"))}</h4><div class="f015-plan-list">${plan.warnings.map((item) => `<div class="f015-plan-step"><strong>${esc(item.code)}</strong><span class="muted">${esc(item.extension_id || "")}</span></div>`).join("")}</div>` : ""}
    ${(plan.steps || []).length ? `<h4>${esc(f015ManagerText("extensions.steps"))}</h4><div class="f015-plan-list">${plan.steps.map((step) => `<div class="f015-plan-step"><strong>${esc(step.order)}. ${esc(step.extension_id)}</strong><span>v${esc(step.version)} · ${esc(step.source)}</span></div>`).join("")}</div>` : ""}
    <div class="f015-actions">
      ${requiresDependencies ? `<button class="button f015-update-dependencies" type="button">${esc(f015ManagerText("extensions.install_dependencies"))}</button>` : ""}
      ${!blocked && !requiresDependencies && plan.status === "READY" ? `<button class="button primary f015-execute-update" type="button" data-target-package-id="${esc(plan.target_package_id)}" data-direction="${esc(plan.direction || "UPDATE")}" data-target-version="${esc(plan.target_version || "—")}">${esc(actionLabel)}</button>` : ""}
    </div>
  </article>`;
}

async function f015ShowMaintenancePlan(button) {
  const card = button.closest("[data-installation-id]");
  const installationID = card?.dataset.installationId;
  const target = card?.querySelector(".f015-maintenance-target")?.value;
  const area = card?.querySelector(".f015-maintenance-plan");
  if (!installationID || !target || !area || button.disabled) return;
  button.disabled = true;
  setMessage(globalMessage, "");
  try {
    const plan = await api(`/api/v1/extensions/installations/${encodeURIComponent(installationID)}/update-plan?target_package_id=${encodeURIComponent(target)}`);
    area.innerHTML = f015RenderUpdatePlan(plan);
    area.querySelector(".f015-update-dependencies")?.addEventListener("click", (dependencyButton) => f015InstallMaintenanceDependencies(dependencyButton.currentTarget, plan));
    area.querySelector(".f015-execute-update")?.addEventListener("click", (event) => f015ExecuteMaintenanceUpdate(event.currentTarget));
  } catch (error) {
    setMessage(globalMessage, f015MaintenanceErrorMessage(error), "error");
  } finally {
    button.disabled = false;
  }
}

async function f015InstallMaintenanceDependencies(button, plan) {
  if (!f015CanManage() || button.disabled) return;
  button.disabled = true;
  setMessage(globalMessage, "");
  try {
    for (const step of (plan.steps || [])) {
      await api(`/api/v1/extensions/packages/${encodeURIComponent(step.package_id)}/install`, { method: "POST" });
    }
    await renderExtensions();
    setMessage(globalMessage, f015ManagerText("extensions.dependencies_installed"), "success");
  } catch (error) {
    button.disabled = false;
    setMessage(globalMessage, f015MaintenanceErrorMessage(error), "error");
  }
}

async function f015ExecuteMaintenanceUpdate(button) {
  const card = button.closest("[data-installation-id]");
  const zone = card?.querySelector(".f015-maintenance-zone");
  const installationID = card?.dataset.installationId;
  const targetPackageID = button.dataset.targetPackageId;
  const direction = button.dataset.direction || "UPDATE";
  const targetVersion = button.dataset.targetVersion || "—";
  const name = zone?.dataset.extensionName || installationID;
  const currentVersion = zone?.dataset.currentVersion || "—";
  if (!installationID || !targetPackageID || !f015CanManage() || button.disabled) return;
  const confirmKey = direction === "ROLLBACK" ? "extensions.rollback_confirm" : "extensions.update_confirm";
  if (!globalThis.confirm(f015FormatManagerText(confirmKey, { name, from: currentVersion, to: targetVersion }))) return;

  const originalText = button.textContent;
  button.disabled = true;
  button.textContent = f015ManagerText(direction === "ROLLBACK" ? "extensions.rolling_back" : "extensions.updating");
  setMessage(globalMessage, "");
  try {
    const result = await api(`/api/v1/extensions/installations/${encodeURIComponent(installationID)}/update`, { method: "POST", json: { target_package_id: targetPackageID } });
    const notice = result?.cleanup_warning
      ? { text: f015ManagerText("extensions.update_cleanup_warning"), kind: "warn" }
      : { text: f015ManagerText(result?.direction === "ROLLBACK" ? "extensions.rollback_complete" : "extensions.update_complete"), kind: "success" };
    await renderExtensions();
    setMessage(globalMessage, notice.text, notice.kind);
  } catch (error) {
    button.disabled = false;
    button.textContent = originalText;
    setMessage(globalMessage, f015MaintenanceErrorMessage(error), "error");
  }
}

async function f015ExecuteRepair(button) {
  const card = button.closest("[data-installation-id]");
  const zone = card?.querySelector(".f015-maintenance-zone");
  const installationID = card?.dataset.installationId;
  const name = zone?.dataset.extensionName || installationID;
  const version = zone?.dataset.currentVersion || "—";
  if (!installationID || !f015CanManage() || button.disabled) return;
  if (!globalThis.confirm(f015FormatManagerText("extensions.repair_confirm", { name, version }))) return;

  const originalText = button.textContent;
  button.disabled = true;
  button.textContent = f015ManagerText("extensions.repairing");
  setMessage(globalMessage, "");
  try {
    const result = await api(`/api/v1/extensions/installations/${encodeURIComponent(installationID)}/repair`, { method: "POST" });
    await renderExtensions();
    setMessage(globalMessage, f015ManagerText(result?.already_healthy ? "extensions.repair_healthy" : "extensions.repair_complete"), "success");
  } catch (error) {
    button.disabled = false;
    button.textContent = originalText;
    setMessage(globalMessage, f015MaintenanceErrorMessage(error), "error");
  }
}

function f015MaintenanceErrorMessage(error) {
  const code = error?.payload?.error_code;
  if (code === "EXTENSION_RUNTIME_MUST_BE_DISABLED") return f015ManagerText("extensions.update_runtime_required");
  if (code === "SHOW_CONFIGURATION_LOCKED") return f015ManagerText("extensions.update_show_locked");
  if (code === "EXTENSION_UPDATE_DEPENDENCIES_REQUIRED") return f015ManagerText("extensions.update_dependencies_required");
  if (code === "EXTENSION_UPDATE_PLAN_BLOCKED") return f015ManagerText("extensions.update_plan_blocked");
  return errorMessage(error) || f015ManagerText("extensions.error");
}

const f015BaseRenderExtensionsForMaintenance = renderExtensions;
renderExtensions = async function f015RenderExtensionsWithMaintenance() {
  await f015BaseRenderExtensionsForMaintenance();
  content.querySelectorAll(".f015-review-update").forEach((button) => button.addEventListener("click", () => f015ShowMaintenancePlan(button)));
  content.querySelectorAll(".f015-repair").forEach((button) => button.addEventListener("click", () => f015ExecuteRepair(button)));
};
