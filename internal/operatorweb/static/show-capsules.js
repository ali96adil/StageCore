"use strict";

const f019Copy = {
  "capsule.nav": { en: "Capsules", "ar-IQ": "حزم العرض" },
  "capsule.eyebrow": { en: "PORTABLE SHOW CAPSULE", "ar-IQ": "حزمة العرض المحمولة" },
  "capsule.title": { en: "Complete Environment Restore", "ar-IQ": "استعادة بيئة العرض" },
  "capsule.subtitle": { en: "Package one immutable show environment for transfer to a replacement StageCore Hub. Restore is identity-preserving, checksum-verified and fail-closed.", "ar-IQ": "حزّم بيئة عرض واحدة غير قابلة للتغيير لنقلها إلى جهاز StageCore بديل. الاستعادة تحافظ على الهوية وتتحقق من البصمات وتتوقف عند أي نقص." },
  "capsule.manifest_only": { en: "Manifest only", "ar-IQ": "بيان فقط" },
  "capsule.self_contained": { en: "Self-contained", "ar-IQ": "مكتفية ذاتياً" },
  "capsule.preview": { en: "Preview", "ar-IQ": "معاينة" },
  "capsule.export": { en: "Export capsule", "ar-IQ": "تصدير الحزمة" },
  "capsule.refresh": { en: "Refresh library", "ar-IQ": "تحديث المكتبة" },
  "capsule.snapshot": { en: "Runtime Snapshot", "ar-IQ": "لقطة التشغيل" },
  "capsule.media": { en: "Media requirements", "ar-IQ": "متطلبات الوسائط" },
  "capsule.environments": { en: "Execution environments", "ar-IQ": "بيئات التنفيذ" },
  "capsule.extensions": { en: "StageCore extensions", "ar-IQ": "إضافات StageCore" },
  "capsule.objects": { en: "Content objects", "ar-IQ": "ملفات المحتوى" },
  "capsule.warnings": { en: "Portability warnings", "ar-IQ": "تحذيرات النقل" },
  "capsule.library": { en: "Capsule library", "ar-IQ": "مكتبة الحزم" },
  "capsule.library_detail": { en: "Exports are created locally. To restore on another Hub, copy the complete capsule directory into that Hub's imports directory; StageCore only opens canonical capsule IDs inside this controlled root.", "ar-IQ": "تُنشأ الحزم محلياً. للاستعادة على جهاز آخر، انسخ مجلد الحزمة كاملاً إلى مجلد imports في ذلك الجهاز؛ يفتح StageCore فقط معرفات الحزم الصحيحة داخل هذا المسار المحمي." },
  "capsule.imports": { en: "Import", "ar-IQ": "استيراد" },
  "capsule.exports": { en: "Export", "ar-IQ": "تصدير" },
  "capsule.plan": { en: "Check readiness", "ar-IQ": "فحص الجاهزية" },
  "capsule.materialize": { en: "Restore project", "ar-IQ": "استعادة المشروع" },
  "capsule.materialization_ready": { en: "Safe to materialize", "ar-IQ": "آمنة للاستعادة" },
  "capsule.materialization_blocked": { en: "Materialization blocked", "ar-IQ": "الاستعادة محظورة" },
  "capsule.host_ready": { en: "Replacement host ready", "ar-IQ": "الجهاز البديل جاهز" },
  "capsule.host_not_ready": { en: "Replacement host needs work", "ar-IQ": "الجهاز البديل يحتاج إكمال" },
  "capsule.no_capsules": { en: "No verified Show Capsules were found in the controlled imports/exports directories.", "ar-IQ": "لا توجد حزم عرض موثقة في مجلدي imports/exports المحميين." },
  "capsule.owner_restore": { en: "Restore requires OWNER backup.restore permission and CSRF authorization.", "ar-IQ": "الاستعادة تتطلب صلاحية backup.restore لحساب OWNER وتفويض CSRF." },
  "capsule.no_overwrite": { en: "Identity collisions never overwrite an existing Project, Revision or Runtime Snapshot.", "ar-IQ": "أي تعارض بالهوية لا يستبدل مشروعاً أو مراجعة أو Runtime Snapshot موجودة." },
  "capsule.show_lock": { en: "Materialization is blocked while any SHOW session is active.", "ar-IQ": "تُحظر الاستعادة أثناء وجود أي جلسة SHOW فعّالة." },
  "capsule.extensions_review": { en: "Extension package bytes can transfer, but installation and permission review remain under F-015 and are never auto-enabled.", "ar-IQ": "يمكن نقل ملفات الإضافات، لكن التثبيت ومراجعة الصلاحيات تبقى ضمن F-015 ولا يتم تفعيلها تلقائياً." },
  "capsule.presentation_local": { en: "Theme and workspace preferences are device-local and intentionally excluded.", "ar-IQ": "الثيم وتفضيلات مساحة العمل محلية للجهاز ويتم استبعادها عمداً." },
  "capsule.exported": { en: "Show Capsule exported.", "ar-IQ": "تم تصدير حزمة العرض." },
  "capsule.restored": { en: "Project materialized from verified Show Capsule.", "ar-IQ": "تمت استعادة المشروع من حزمة عرض موثقة." },
  "capsule.read_only": { en: "This role can inspect portability but cannot export Project changes.", "ar-IQ": "هذا الدور يستطيع فحص قابلية النقل لكنه لا يستطيع تصدير تغييرات المشروع." },
};

function f019Locale() {
  if (el("languageSelect")?.value === "en") return "en";
  return localStorage.getItem("stagecore_locale") === "en" ? "en" : "ar-IQ";
}

function f019T(key) {
  const entry = f019Copy[key];
  return entry?.[f019Locale()] || entry?.en || key;
}

function f019UpdateNav() {
  const button = document.querySelector('[data-page="capsules"]');
  if (button) button.textContent = f019T("capsule.nav");
}

function f019CanExport() {
  return ["OWNER", "TECHNICIAN"].includes(state.user?.role);
}

function f019CanRestore() {
  return state.user?.role === "OWNER";
}

function f019Count(value) {
  return Array.isArray(value) ? value.length : 0;
}

function f019CheckPill(check) {
  const kind = check.severity === "BLOCK" ? "bad" : check.severity === "WARN" ? "warn" : "good";
  return `<li class="validation-item">${pill(check.severity, kind)} <strong>${esc(check.code)}</strong><br><span class="muted">${esc(check.message)}</span></li>`;
}

async function f019Preview(mode) {
  const projectID = encodeURIComponent(state.project.project_id);
  const payload = await api(`/api/v1/projects/${projectID}/show-capsule/preview?mode=${encodeURIComponent(mode)}`);
  return payload.manifest;
}

async function f019List() {
  const payload = await api("/api/v1/show-capsules");
  return payload.capsules || [];
}

function f019PreviewCard(manifest, mode) {
  const warnings = manifest.warnings || [];
  return `
    <section class="card" style="margin-bottom:14px">
      <div class="section-title-row">
        <div><p class="eyebrow">${esc(mode)}</p><h2>${esc(state.project.name)}</h2></div>
        ${pill(mode === "SELF_CONTAINED" ? f019T("capsule.self_contained") : f019T("capsule.manifest_only"), mode === "SELF_CONTAINED" ? "good" : "neutral")}
      </div>
      <div class="stat-grid" style="margin-top:14px">
        <article class="stat"><span class="label">${esc(f019T("capsule.snapshot"))}</span><span class="value">v${esc(manifest.runtime_snapshot?.snapshot_version || "—")}</span><span class="sub mono">${esc(manifest.runtime_snapshot?.runtime_snapshot_id || "—")}</span></article>
        <article class="stat"><span class="label">${esc(f019T("capsule.media"))}</span><span class="value">${esc(f019Count(manifest.media))}</span><span class="sub">${esc(f019T("capsule.objects"))}: ${esc(f019Count(manifest.objects))}</span></article>
        <article class="stat"><span class="label">${esc(f019T("capsule.environments"))}</span><span class="value">${esc(f019Count(manifest.execution_environments))}</span><span class="sub">F-025</span></article>
        <article class="stat"><span class="label">${esc(f019T("capsule.extensions"))}</span><span class="value">${esc(f019Count(manifest.extensions))}</span><span class="sub">F-015</span></article>
      </div>
      ${warnings.length ? `<div style="margin-top:14px"><strong>${esc(f019T("capsule.warnings"))}</strong><ul class="validation-list">${warnings.map((warning) => `<li class="validation-item">${esc(warning)}</li>`).join("")}</ul></div>` : ""}
    </section>`;
}

function f019LibraryTable(capsules) {
  if (!capsules.length) return `<div class="empty">${esc(f019T("capsule.no_capsules"))}</div>`;
  return `<div class="table-wrap"><table>
    <thead><tr><th>Project</th><th>Snapshot</th><th>Mode</th><th>Location</th><th>Created</th><th></th></tr></thead>
    <tbody>${capsules.map((item) => `
      <tr>
        <td><strong>${esc(item.project_name)}</strong><br><small class="mono">${esc(item.capsule_id)}</small></td>
        <td class="mono">${esc(item.runtime_snapshot_id)}</td>
        <td>${pill(item.export_mode, item.export_mode === "SELF_CONTAINED" ? "good" : "neutral")}</td>
        <td>${pill(item.location === "imports" ? f019T("capsule.imports") : f019T("capsule.exports"), item.location === "imports" ? "good" : "neutral")}</td>
        <td>${esc(fmtDate(item.created_at))}</td>
        <td>${item.location === "imports" ? `<button class="button capsule-plan" data-id="${esc(item.capsule_id)}" type="button">${esc(f019T("capsule.plan"))}</button>` : ""}</td>
      </tr>`).join("")}</tbody>
  </table></div>`;
}

async function renderShowCapsuleWorkspace(mode = "MANIFEST_ONLY", selectedPlan = null) {
  if (!state.project) return;
  setPage("capsules");
  f019UpdateNav();
  const [manifest, capsules] = await Promise.all([f019Preview(mode), f019List()]);
  const plan = selectedPlan;
  content.innerHTML = `
    <div class="page-head">
      <div><p class="eyebrow">${esc(f019T("capsule.eyebrow"))}</p><h1>${esc(f019T("capsule.title"))}</h1><p>${esc(f019T("capsule.subtitle"))}</p></div>
      <div class="toolbar">
        <button id="capsuleManifestMode" class="button ${mode === "MANIFEST_ONLY" ? "primary" : ""}" type="button">${esc(f019T("capsule.manifest_only"))}</button>
        <button id="capsuleSelfMode" class="button ${mode === "SELF_CONTAINED" ? "primary" : ""}" type="button">${esc(f019T("capsule.self_contained"))}</button>
      </div>
    </div>
    <section class="card" style="margin-bottom:14px">
      <div class="section-title-row"><div><h2>${esc(f019T("capsule.preview"))}</h2><p class="muted">${esc(f019T("capsule.no_overwrite"))}</p></div>
        ${f019CanExport() ? `<button id="capsuleExport" class="button primary" type="button">${esc(f019T("capsule.export"))}</button>` : pill(f019T("capsule.read_only"), "neutral")}
      </div>
    </section>
    ${f019PreviewCard(manifest, mode)}
    <section class="card" style="margin-bottom:14px">
      <div class="section-title-row"><div><h2>${esc(f019T("capsule.library"))}</h2><p class="muted">${esc(f019T("capsule.library_detail"))}</p></div><button id="capsuleRefresh" class="button" type="button">${esc(f019T("capsule.refresh"))}</button></div>
      <div style="margin-top:14px">${f019LibraryTable(capsules)}</div>
    </section>
    ${plan ? f019PlanCard(plan) : ""}
    <section class="card">
      <div class="grid cards">
        <article class="action-editor"><strong>${esc(f019T("capsule.show_lock"))}</strong></article>
        <article class="action-editor"><strong>${esc(f019T("capsule.owner_restore"))}</strong></article>
        <article class="action-editor"><strong>${esc(f019T("capsule.extensions_review"))}</strong></article>
        <article class="action-editor"><strong>${esc(f019T("capsule.presentation_local"))}</strong></article>
      </div>
    </section>`;

  el("capsuleManifestMode")?.addEventListener("click", () => renderShowCapsuleWorkspace("MANIFEST_ONLY").catch(f019ShowError));
  el("capsuleSelfMode")?.addEventListener("click", () => renderShowCapsuleWorkspace("SELF_CONTAINED").catch(f019ShowError));
  el("capsuleRefresh")?.addEventListener("click", () => renderShowCapsuleWorkspace(mode).catch(f019ShowError));
  el("capsuleExport")?.addEventListener("click", async () => {
    try {
      await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/show-capsules`, { method: "POST", json: { mode } });
      setMessage(globalMessage, f019T("capsule.exported"), "success");
      await renderShowCapsuleWorkspace(mode);
    } catch (error) { f019ShowError(error); }
  });
  content.querySelectorAll(".capsule-plan").forEach((button) => button.addEventListener("click", async () => {
    try {
      const payload = await api(`/api/v1/show-capsules/imports/${encodeURIComponent(button.dataset.id)}/plan`);
      await renderShowCapsuleWorkspace(mode, payload.plan);
    } catch (error) { f019ShowError(error); }
  }));
  el("capsuleMaterialize")?.addEventListener("click", async () => {
    if (!plan?.capsule_id) return;
    try {
      const payload = await api(`/api/v1/show-capsules/imports/${encodeURIComponent(plan.capsule_id)}/materialize`, { method: "POST" });
      setMessage(globalMessage, f019T("capsule.restored"), "success");
      await loadProjects();
      await renderShowCapsuleWorkspace(mode, payload.result?.plan || plan);
    } catch (error) { f019ShowError(error); }
  });
}

function f019PlanCard(plan) {
  const materialKind = plan.materialization_ready ? "good" : "bad";
  const hostKind = plan.replacement_host_ready ? "good" : "warn";
  return `<section class="card" style="margin-bottom:14px">
    <div class="section-title-row">
      <div><p class="eyebrow">RESTORE PLAN</p><h2>${esc(plan.manifest?.project?.name || plan.project_id)}</h2><p class="mono muted">${esc(plan.capsule_id)}</p></div>
      <div class="toolbar">${pill(plan.materialization_ready ? f019T("capsule.materialization_ready") : f019T("capsule.materialization_blocked"), materialKind)}${pill(plan.replacement_host_ready ? f019T("capsule.host_ready") : f019T("capsule.host_not_ready"), hostKind)}</div>
    </div>
    <div class="stat-grid" style="margin-top:14px">
      <article class="stat"><span class="label">Included</span><span class="value">${esc(f019Count(plan.included_objects))}</span><span class="sub">capsule objects</span></article>
      <article class="stat"><span class="label">Reusable</span><span class="value">${esc(f019Count(plan.reusable_objects))}</span><span class="sub">local Vault objects</span></article>
      <article class="stat"><span class="label">Missing</span><span class="value">${esc(f019Count(plan.missing_objects))}</span><span class="sub">requirements</span></article>
    </div>
    <ul class="validation-list" style="margin-top:14px">${(plan.checks || []).map(f019CheckPill).join("")}</ul>
    ${f019CanRestore() && plan.materialization_ready ? `<div class="toolbar" style="margin-top:14px"><button id="capsuleMaterialize" class="button primary" type="button">${esc(f019T("capsule.materialize"))}</button></div>` : `<p class="muted" style="margin-top:14px">${esc(f019T("capsule.owner_restore"))}</p>`}
  </section>`;
}

function f019ShowError(error) {
  setMessage(globalMessage, errorMessage(error), "error");
}

const capsuleNav = document.querySelector('[data-page="capsules"]');
capsuleNav?.addEventListener("click", (event) => {
  event.preventDefault();
  renderShowCapsuleWorkspace().catch(f019ShowError);
});
f019UpdateNav();

el("languageSelect")?.addEventListener("change", () => {
  f019UpdateNav();
  if (state.page === "capsules") renderShowCapsuleWorkspace().catch(f019ShowError);
});
