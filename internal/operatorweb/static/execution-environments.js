"use strict";

const stagecoreExecutionEnvironmentNavigateBase = navigate;
const f025Locale = localStorage.getItem("stagecore_locale") === "en" ? "en" : "ar-IQ";
const f025Strings = {
  "f025.nav": {"en":"Environments","ar-IQ":"بيئات التشغيل"},
  "f025.eyebrow": {"en":"EXECUTION ENVIRONMENTS","ar-IQ":"بيئات التشغيل الخارجية"},
  "f025.title": {"en":"External execution environments","ar-IQ":"بيئات التشغيل الخارجية"},
  "f025.summary": {"en":"Describe the workstation software and project assets required by this revision, then bind each environment to the Machine Role that owns it.","ar-IQ":"عرّف برنامج محطة العمل وملفات المشروع المطلوبة لهذه المسودة، ثم اربط كل بيئة بدور الجهاز المسؤول عنها."},
  "f025.guided": {"en":"Guided VDMX setup","ar-IQ":"إعداد VDMX الموجّه"},
  "f025.advanced": {"en":"Advanced manifest","ar-IQ":"Manifest متقدم"},
  "f025.environment_key": {"en":"Environment key","ar-IQ":"مفتاح البيئة"},
  "f025.name": {"en":"Environment name","ar-IQ":"اسم البيئة"},
  "f025.version": {"en":"VDMX version requirement","ar-IQ":"متطلب إصدار VDMX"},
  "f025.architecture": {"en":"Mac architecture","ar-IQ":"معمارية الـ Mac"},
  "f025.workspace_locator": {"en":"VDMX workspace path (optional)","ar-IQ":"مسار مشروع VDMX (اختياري)"},
  "f025.workspace_demo_hint": {"en":"Leave the workspace path blank for VDMX Demo / an unsaved workspace. StageCore can still open VDMX and capture the controls VDMX publishes through OSCQuery.","ar-IQ":"اترك مسار المشروع فارغاً لنسخة VDMX Demo أو لمشروع غير محفوظ. يبقى StageCore قادراً على فتح VDMX والتقاط عناصر التحكم التي ينشرها عبر OSCQuery."},
  "f025.oscquery_url": {"en":"VDMX OSCQuery URL","ar-IQ":"عنوان VDMX OSCQuery"},
  "f025.oscquery_hint": {"en":"Enable VDMX OSCQuery locally, then StageCore records the published control namespace and current observable values. This is a partial reconstruction snapshot, not a saved VDMX project.","ar-IQ":"فعّل OSCQuery محلياً داخل VDMX، ثم يسجل StageCore مساحة التحكم المنشورة والقيم الحالية القابلة للرصد. هذا Snapshot جزئي لإعادة البناء وليس ملف مشروع VDMX محفوظاً."},
  "f025.osc_go_hint": {"en":"Show GO control: send OSC /stagecore/go to 127.0.0.1:9010 on the Mac running StageCore Companion.","ar-IQ":"للتحكم GO أثناء العرض: أرسل OSC بالمسار /stagecore/go إلى 127.0.0.1:9010 على الماك الذي يشغل StageCore Companion."},
  "f025.capture_policy": {"en":"Asset policy","ar-IQ":"سياسة الملف"},
  "f025.reference_only": {"en":"Reference only — verify location, bytes are not backed up","ar-IQ":"مرجع فقط — تحقق من الموقع، والبايتات غير محفوظة كنسخة احتياطية"},
  "f025.content_bound": {"en":"Content bound — exact hash and size are already known","ar-IQ":"محتوى مرتبط — الهاش والحجم الدقيقان معروفان مسبقاً"},
  "f025.content_hash": {"en":"SHA-256 content hash","ar-IQ":"بصمة SHA-256 للمحتوى"},
  "f025.size_bytes": {"en":"Exact size in bytes","ar-IQ":"الحجم الدقيق بالبايت"},
  "f025.machine_role": {"en":"Machine Role binding","ar-IQ":"ربط دور الجهاز"},
  "f025.unbound_option": {"en":"Unbound — choose explicitly later","ar-IQ":"غير مربوط — اختر الدور لاحقاً بشكل صريح"},
  "f025.create": {"en":"Create environment","ar-IQ":"إنشاء بيئة التشغيل"},
  "f025.edit": {"en":"Edit environment","ar-IQ":"تعديل بيئة التشغيل"},
  "f025.update": {"en":"Update environment","ar-IQ":"تحديث بيئة التشغيل"},
  "f025.cancel_edit": {"en":"Cancel edit","ar-IQ":"إلغاء التعديل"},
  "f025.updated": {"en":"Execution environment updated.","ar-IQ":"تم تحديث بيئة التشغيل."},
  "f025.start_edit": {"en":"Start environment edit","ar-IQ":"بدء تعديل بيئات التشغيل"},
  "f025.refresh": {"en":"Refresh","ar-IQ":"تحديث"},
  "f025.unbound": {"en":"UNBOUND","ar-IQ":"غير مربوط"},
  "f025.portability_warning": {"en":"Reference-only assets are verified in place but are not portable backups.","ar-IQ":"الملفات المرجعية فقط تُفحص في مكانها لكنها ليست نسخاً احتياطية قابلة للنقل."},
  "f025.remove": {"en":"Remove","ar-IQ":"إزالة"},
  "f025.empty": {"en":"No execution environments are declared for this revision.","ar-IQ":"لا توجد بيئات تشغيل معرّفة لهذه المسودة."},
  "f025.read_only": {"en":"This revision is read-only. Start an environment edit to fork a new Draft before changing it.","ar-IQ":"هذه المسودة للقراءة فقط. ابدأ تعديل بيئات التشغيل لإنشاء Draft جديد قبل أي تغيير."},
  "f025.manifest_json": {"en":"Manifest JSON","ar-IQ":"Manifest بصيغة JSON"},
  "f025.created": {"en":"Execution environment created.","ar-IQ":"تم إنشاء بيئة التشغيل."},
  "f025.bound": {"en":"Machine Role binding updated.","ar-IQ":"تم تحديث ربط دور الجهاز."},
  "f025.removed": {"en":"Execution environment removed from the Draft.","ar-IQ":"تمت إزالة بيئة التشغيل من الـ Draft."},
  "f025.invalid_content_bound": {"en":"CONTENT_BOUND requires a 64-character SHA-256 hash and an exact non-negative byte size.","ar-IQ":"يتطلب CONTENT_BOUND بصمة SHA-256 من 64 محرفاً وحجماً دقيقاً غير سالب بالبايت."},
  "f025.delete_confirm": {"en":"Remove this execution environment from the Draft?","ar-IQ":"إزالة بيئة التشغيل هذه من الـ Draft؟"},
  "f025.identity": {"en":"Canonical identity","ar-IQ":"الهوية المعيارية"},
  "f025.reference_badge": {"en":"REFERENCE ONLY","ar-IQ":"مرجع فقط"},
  "f025.content_badge": {"en":"CONTENT BOUND","ar-IQ":"محتوى مرتبط"},
  "f025.readiness_note": {"en":"SHOW Preflight inspects the bound Companion through the authenticated read-only F-025 transport.","ar-IQ":"يفحص SHOW Preflight الجهاز المرافق المرتبط عبر مسار F-025 الموثق والمخصص للقراءة فقط."},
  "f025.snapshots": {"en":"Capture history","ar-IQ":"سجل اللقطات"},
  "f025.snapshots_loading": {"en":"Loading captured snapshots…","ar-IQ":"جارٍ تحميل اللقطات المسجلة…"},
  "f025.snapshots_empty": {"en":"No captured snapshots yet.","ar-IQ":"لا توجد لقطات مسجلة بعد."},
  "f025.snapshots_need_two": {"en":"Capture at least two snapshots to compare changes.","ar-IQ":"التقط Snapshotين على الأقل لمقارنة التغييرات."},
  "f025.compare": {"en":"Compare captures","ar-IQ":"مقارنة اللقطات"},
  "f025.before": {"en":"Before","ar-IQ":"قبل"},
  "f025.after": {"en":"After","ar-IQ":"بعد"},
  "f025.diff_identical": {"en":"No observable differences between these captures.","ar-IQ":"لا توجد فروقات مرصودة بين هاتين اللقطتين."},
  "f025.diff_changed": {"en":"Observable changes found.","ar-IQ":"تم العثور على تغييرات مرصودة."},
  "f025.diff_top_level": {"en":"Snapshot fields","ar-IQ":"حقول الـSnapshot"},
  "f025.diff_added": {"en":"Added items","ar-IQ":"عناصر مضافة"},
  "f025.diff_removed": {"en":"Removed items","ar-IQ":"عناصر محذوفة"},
  "f025.diff_items": {"en":"Changed items","ar-IQ":"عناصر متغيرة"},
  "f025.rebuild_plan": {"en":"Assisted rebuild plan","ar-IQ":"خطة إعادة البناء المساعدة"},
  "f025.view_rebuild_plan": {"en":"View rebuild plan","ar-IQ":"عرض خطة إعادة البناء"},
  "f025.rebuild_plan_empty": {"en":"This capture has no assisted rebuild plan.","ar-IQ":"هذه اللقطة لا تحتوي على خطة إعادة بناء مساعدة."},
  "f025.rebuild_fingerprint": {"en":"Reconstruction fingerprint","ar-IQ":"بصمة إعادة البناء"},
  "f025.rebuild_step": {"en":"Step","ar-IQ":"الخطوة"},
  "f025.rebuild_status": {"en":"Status","ar-IQ":"الحالة"},
  "f025.rebuild_provenance": {"en":"Provenance","ar-IQ":"المصدر"},
  "f025.retained_plan": {"en":"Retained editable rebuild plan","ar-IQ":"خطة إعادة البناء المحفوظة القابلة للتعديل"},
  "f025.retained_loading": {"en":"Loading retained rebuild plan…","ar-IQ":"جارٍ تحميل خطة إعادة البناء المحفوظة…"},
  "f025.retained_none": {"en":"No retained editable plan yet.","ar-IQ":"لا توجد خطة محفوظة قابلة للتعديل بعد."},
  "f025.seed_plan": {"en":"Seed editable plan from capture","ar-IQ":"إنشاء خطة قابلة للتعديل من Snapshot"},
  "f025.reseed_plan": {"en":"Reseed from capture","ar-IQ":"إعادة إنشاء الخطة من Snapshot"},
  "f025.reseed_confirm": {"en":"Replace the retained plan with a fresh copy from this capture? Existing manual edits will be discarded.","ar-IQ":"استبدال الخطة المحفوظة بنسخة جديدة من هذه اللقطة؟ سيتم حذف التعديلات اليدوية الحالية."},
  "f025.save_plan": {"en":"Save retained plan","ar-IQ":"حفظ الخطة"},
  "f025.add_step": {"en":"Add manual step","ar-IQ":"إضافة خطوة يدوية"},
  "f025.remove_step": {"en":"Remove step","ar-IQ":"حذف الخطوة"},
  "f025.delete_plan": {"en":"Delete retained plan","ar-IQ":"حذف الخطة المحفوظة"},
  "f025.delete_plan_confirm": {"en":"Delete the retained editable rebuild plan? The immutable captured Snapshot will not be changed.","ar-IQ":"حذف خطة إعادة البناء المحفوظة؟ لن يتم تغيير الـSnapshot الأصلية المحفوظة."},
  "f025.plan_action": {"en":"Action","ar-IQ":"الإجراء"},
  "f025.plan_notes": {"en":"Notes","ar-IQ":"الملاحظات"},
  "f025.plan_source": {"en":"Source capture","ar-IQ":"اللقطة المصدر"},
  "f025.plan_saved": {"en":"Retained rebuild plan saved.","ar-IQ":"تم حفظ خطة إعادة البناء."},
  "f025.plan_seeded": {"en":"Editable rebuild plan created from capture.","ar-IQ":"تم إنشاء خطة قابلة للتعديل من الـSnapshot."},
  "f025.plan_deleted": {"en":"Retained rebuild plan deleted.","ar-IQ":"تم حذف خطة إعادة البناء المحفوظة."},
  "f025.plan_required": {"en":"Every rebuild step needs an action and status.","ar-IQ":"كل خطوة في خطة إعادة البناء تحتاج إجراءً وحالة."}
};

function f025T(key) {
  return f025Strings[key]?.[f025Locale] || f025Strings[key]?.en || key;
}

const f025OperationCapability = "execution.environment.operation";

function f025RoleSupportsOperations(role) {
  return (role?.required_capabilities || []).includes(f025OperationCapability);
}

function f025RoleOptions(roles, selected = "") {
  const options = (roles || []).filter((role) =>
    f025RoleSupportsOperations(role) || role.machine_role_id === selected
  ).map((role) => {
    const label = role.display_name || role.role_key || role.machine_role_id;
    const compatible = f025RoleSupportsOperations(role);
    const suffix = compatible ? "" : " · LEGACY / missing execution.environment.operation";
    return `<option value="${esc(role.machine_role_id)}" ${role.machine_role_id === selected ? "selected" : ""}>${esc(label)} · ${esc(role.role_key)}${esc(suffix)}</option>`;
  });
  return `<option value="">${esc(f025T("f025.unbound_option"))}</option>` + options.join("");
}

function f025CurrentRevisionID() {
  return state.project?.current_revision_id || "";
}

function f025CollectionPath(revisionID = f025CurrentRevisionID()) {
  return `/api/v1/projects/${encodeURIComponent(state.project.project_id)}/revisions/${encodeURIComponent(revisionID)}/execution-environments`;
}

async function f025LoadModel() {
  const revisionID = f025CurrentRevisionID();
  if (!revisionID) throw new Error("Current revision is unavailable.");
  return api(f025CollectionPath(revisionID));
}

function f025SnapshotCollectionPath(environmentID) {
  return `${f025CollectionPath()}/${encodeURIComponent(environmentID)}/snapshots`;
}

function f025RebuildPlanPath(environmentID) {
  return `${f025CollectionPath()}/${encodeURIComponent(environmentID)}/rebuild-plan`;
}

function f025SnapshotLabel(snapshot) {
  const shortID = String(snapshot.snapshot_id || "").slice(0, 8);
  return `${fmtDate(snapshot.created_at)} · ${snapshot.capture_status || "UNKNOWN"} · ${shortID}`;
}

function f025SnapshotDiffMarkup(diff) {
  if (diff?.identical) {
    return `<div class="message success">${esc(f025T("f025.diff_identical"))}</div>`;
  }
  const sections = [];
  if (diff?.top_level_fields?.length) {
    sections.push(`<p><strong>${esc(f025T("f025.diff_top_level"))}:</strong> ${esc(diff.top_level_fields.join(", "))}</p>`);
  }
  if (diff?.added_items?.length) {
    sections.push(`<p><strong>${esc(f025T("f025.diff_added"))}:</strong> ${esc(diff.added_items.join(", "))}</p>`);
  }
  if (diff?.removed_items?.length) {
    sections.push(`<p><strong>${esc(f025T("f025.diff_removed"))}:</strong> ${esc(diff.removed_items.join(", "))}</p>`);
  }
  if (diff?.changed_items?.length) {
    const changed = diff.changed_items.map((item) =>
      `${esc(item.key)} [${esc((item.changed_fields || []).join(", "))}]`
    ).join("<br>");
    sections.push(`<p><strong>${esc(f025T("f025.diff_items"))}:</strong><br>${changed}</p>`);
  }
  return `<div class="message warn"><strong>${esc(f025T("f025.diff_changed"))}</strong>${sections.join("")}</div>`;
}

function f025RebuildPlanMarkup(snapshot) {
  const plan = snapshot?.rebuild_plan || [];
  if (!plan.length) {
    return `<div class="message">${esc(f025T("f025.rebuild_plan_empty"))}</div>`;
  }
  const fingerprint = snapshot.reconstruction_fingerprint || "—";
  const rows = plan.map((step) => `
    <li>
      <strong>${esc(f025T("f025.rebuild_step"))} ${esc(step.step)} · ${esc(step.action)}</strong><br>
      <span class="muted">${esc(f025T("f025.rebuild_status"))}: ${esc(step.status)} · ${esc(f025T("f025.rebuild_provenance"))}: ${esc(step.provenance_class)}</span>
      ${step.notes ? `<br><span class="muted">${esc(step.notes)}</span>` : ""}
    </li>`).join("");
  return `
    <div class="message">
      <strong>${esc(f025T("f025.rebuild_plan"))}</strong><br>
      <span class="mono muted">${esc(f025T("f025.rebuild_fingerprint"))}: ${esc(fingerprint)}</span>
      <ol class="validation-list" style="margin-top:10px">${rows}</ol>
    </div>`;
}

async function f025LoadRebuildPlan(card, environmentID) {
  const select = card.querySelector(".f025-rebuild-snapshot");
  const result = card.querySelector(".f025-rebuild-plan-result");
  const snapshotID = select?.value || "";
  if (!snapshotID || !result) return;
  try {
    const payload = await api(
      `${f025SnapshotCollectionPath(environmentID)}/${encodeURIComponent(snapshotID)}`
    );
    result.innerHTML = f025RebuildPlanMarkup(payload.snapshot);
  } catch (error) {
    result.innerHTML = `<div class="message error">${esc(errorMessage(error))}</div>`;
  }
}

function f025RetainedStepMarkup(step, editable) {
  const sourceStep = step.source_step == null ? "" : String(step.source_step);
  const provenance = step.provenance_class || "USER_DECLARED";
  if (!editable) {
    return `
      <li>
        <strong>${esc(f025T("f025.rebuild_step"))} ${esc(step.step)} · ${esc(step.action)}</strong><br>
        <span class="muted">${esc(f025T("f025.rebuild_status"))}: ${esc(step.status)} · ${esc(f025T("f025.rebuild_provenance"))}: ${esc(provenance)}</span>
        ${step.notes ? `<br><span class="muted">${esc(step.notes)}</span>` : ""}
      </li>`;
  }
  return `
    <div class="card f025-retained-step"
         data-source-step="${esc(sourceStep)}"
         data-provenance-class="${esc(provenance)}"
         style="margin-top:10px">
      <div class="section-title-row">
        <strong class="f025-retained-step-label">${esc(f025T("f025.rebuild_step"))} ${esc(step.step)}</strong>
        <span class="mono muted f025-retained-provenance">${esc(provenance)}</span>
      </div>
      <div class="form-grid two">
        <label>${esc(f025T("f025.plan_action"))}<input class="f025-retained-action" value="${esc(step.action || "")}" required></label>
        <label>${esc(f025T("f025.rebuild_status"))}<input class="f025-retained-status" value="${esc(step.status || "")}" required></label>
      </div>
      <label>${esc(f025T("f025.plan_notes"))}<textarea class="f025-retained-notes" rows="2">${esc(step.notes || "")}</textarea></label>
      <div class="toolbar" style="margin-top:8px">
        <button class="button danger f025-retained-remove-step" type="button">${esc(f025T("f025.remove_step"))}</button>
      </div>
    </div>`;
}

function f025RenumberRetainedSteps(target) {
  [...target.querySelectorAll(".f025-retained-step")].forEach((row, index) => {
    row.querySelector(".f025-retained-step-label")?.replaceChildren(
      document.createTextNode(`${f025T("f025.rebuild_step")} ${index + 1}`)
    );
  });
}

function f025MarkRetainedStepUserDeclared(row) {
  row.dataset.provenanceClass = "USER_DECLARED";
  row.querySelector(".f025-retained-provenance")?.replaceChildren(
    document.createTextNode("USER_DECLARED")
  );
}

function f025BindRetainedStepRow(row, target) {
  row.querySelectorAll(".f025-retained-action, .f025-retained-status, .f025-retained-notes").forEach((input) => {
    input.addEventListener("input", () => f025MarkRetainedStepUserDeclared(row));
  });
  row.querySelector(".f025-retained-remove-step")?.addEventListener("click", () => {
    row.remove();
    f025RenumberRetainedSteps(target);
  });
}

function f025BindRetainedStepEditor(target) {
  target.querySelectorAll(".f025-retained-step").forEach((row) => {
    f025BindRetainedStepRow(row, target);
  });
}

function f025CollectRetainedPlan(target, sourcePlan) {
  const rows = [...target.querySelectorAll(".f025-retained-step")];
  if (!rows.length) throw new Error(f025T("f025.plan_required"));
  const steps = rows.map((row, index) => {
    const action = row.querySelector(".f025-retained-action")?.value.trim() || "";
    const status = row.querySelector(".f025-retained-status")?.value.trim() || "";
    const notes = row.querySelector(".f025-retained-notes")?.value.trim() || "";
    if (!action || !status) throw new Error(f025T("f025.plan_required"));
    const sourceText = row.dataset.sourceStep || "";
    const step = {
      step: index + 1,
      action,
      status,
      provenance_class: row.dataset.provenanceClass || "USER_DECLARED",
    };
    if (sourceText) step.source_step = Number(sourceText);
    if (notes) step.notes = notes;
    return step;
  });
  return {
    schema_version: sourcePlan.schema_version,
    source_snapshot_sha256: sourcePlan.source_snapshot_sha256,
    source_reconstruction_fingerprint: sourcePlan.source_reconstruction_fingerprint,
    steps,
  };
}

async function f025HydrateRetainedPlan(card, environmentID, snapshots, editable) {
  const target = card.querySelector(".f025-retained-plan");
  if (!target) return;
  const path = f025RebuildPlanPath(environmentID);
  const options = (selectedID) => snapshots.map((snapshot) =>
    `<option value="${esc(snapshot.snapshot_id)}" ${snapshot.snapshot_id === selectedID ? "selected" : ""}>${esc(f025SnapshotLabel(snapshot))}</option>`
  ).join("");
  try {
    const payload = await api(path);
    const plan = payload.plan || {};
    const steps = plan.steps || [];
    if (!editable) {
      target.innerHTML = `
        <p class="muted">${esc(f025T("f025.plan_source"))}: <span class="mono">${esc(payload.source_snapshot_id || "—")}</span></p>
        <ol class="validation-list">${steps.map((step) => f025RetainedStepMarkup(step, false)).join("")}</ol>`;
      return;
    }

    target.innerHTML = `
      <p class="muted">${esc(f025T("f025.plan_source"))}: <span class="mono">${esc(payload.source_snapshot_id || "—")}</span></p>
      <div class="f025-retained-steps">${steps.map((step) => f025RetainedStepMarkup(step, true)).join("")}</div>
      <div class="toolbar" style="margin-top:10px">
        <button class="button f025-retained-add-step" type="button">${esc(f025T("f025.add_step"))}</button>
        <button class="button primary f025-retained-save" type="button">${esc(f025T("f025.save_plan"))}</button>
      </div>
      <div class="form-grid two" style="margin-top:12px">
        <label>${esc(f025T("f025.plan_source"))}<select class="f025-retained-reseed-snapshot">${options(payload.source_snapshot_id)}</select></label>
      </div>
      <div class="toolbar" style="margin-top:10px">
        <button class="button f025-retained-reseed" type="button">${esc(f025T("f025.reseed_plan"))}</button>
        <button class="button danger f025-retained-delete" type="button">${esc(f025T("f025.delete_plan"))}</button>
      </div>`;

    f025BindRetainedStepEditor(target);

    target.querySelector(".f025-retained-add-step")?.addEventListener("click", () => {
      const stepsTarget = target.querySelector(".f025-retained-steps");
      if (!stepsTarget) return;
      const next = stepsTarget.querySelectorAll(".f025-retained-step").length + 1;
      stepsTarget.insertAdjacentHTML("beforeend", f025RetainedStepMarkup({
        step: next,
        action: "",
        status: "MANUAL",
        provenance_class: "USER_DECLARED",
      }, true));
      const row = stepsTarget.lastElementChild;
      if (row) {
        row.dataset.provenanceClass = "USER_DECLARED";
        f025BindRetainedStepRow(row, target);
        row.querySelector(".f025-retained-action")?.focus();
      }
      f025RenumberRetainedSteps(target);
    });

    target.querySelector(".f025-retained-save")?.addEventListener("click", async () => {
      try {
        const editedPlan = f025CollectRetainedPlan(target, plan);
        await api(path, {
          method: "PUT",
          json: {source_snapshot_id: payload.source_snapshot_id, plan: editedPlan},
        });
        await renderExecutionEnvironments(f025T("f025.plan_saved"));
      } catch (error) { f025Error(error); }
    });

    target.querySelector(".f025-retained-reseed")?.addEventListener("click", async () => {
      if (!confirm(f025T("f025.reseed_confirm"))) return;
      const sourceSnapshotID = target.querySelector(".f025-retained-reseed-snapshot")?.value || "";
      if (!sourceSnapshotID) return;
      try {
        await api(`${path}/seed?replace=true`, {
          method: "POST",
          json: {source_snapshot_id: sourceSnapshotID},
        });
        await renderExecutionEnvironments(f025T("f025.plan_seeded"));
      } catch (error) { f025Error(error); }
    });

    target.querySelector(".f025-retained-delete")?.addEventListener("click", async () => {
      if (!confirm(f025T("f025.delete_plan_confirm"))) return;
      try {
        await api(`${path}?confirm=true`, {method: "DELETE"});
        await renderExecutionEnvironments(f025T("f025.plan_deleted"));
      } catch (error) { f025Error(error); }
    });
  } catch (error) {
    if (error.status !== 404) {
      target.innerHTML = `<div class="message error">${esc(errorMessage(error))}</div>`;
      return;
    }
    if (!editable) {
      target.innerHTML = `<p class="muted">${esc(f025T("f025.retained_none"))}</p>`;
      return;
    }
    const latest = snapshots[snapshots.length - 1];
    target.innerHTML = `
      <p class="muted">${esc(f025T("f025.retained_none"))}</p>
      <div class="form-grid two">
        <label>${esc(f025T("f025.plan_source"))}<select class="f025-retained-seed-snapshot">${options(latest?.snapshot_id || "")}</select></label>
      </div>
      <div class="toolbar" style="margin-top:10px">
        <button class="button primary f025-retained-seed" type="button">${esc(f025T("f025.seed_plan"))}</button>
      </div>`;
    target.querySelector(".f025-retained-seed")?.addEventListener("click", async () => {
      const sourceSnapshotID = target.querySelector(".f025-retained-seed-snapshot")?.value || "";
      if (!sourceSnapshotID) return;
      try {
        await api(`${path}/seed`, {
          method: "POST",
          json: {source_snapshot_id: sourceSnapshotID},
        });
        await renderExecutionEnvironments(f025T("f025.plan_seeded"));
      } catch (seedError) { f025Error(seedError); }
    });
  }
}

async function f025CompareSnapshots(card, environmentID) {
  const before = card.querySelector(".f025-snapshot-before")?.value || "";
  const after = card.querySelector(".f025-snapshot-after")?.value || "";
  const result = card.querySelector(".f025-snapshot-diff-result");
  if (!before || !after || !result) return;
  try {
    const query = new URLSearchParams({
      before_snapshot_id: before,
      after_snapshot_id: after,
    });
    const payload = await api(`${f025SnapshotCollectionPath(environmentID)}/diff?${query}`);
    result.innerHTML = f025SnapshotDiffMarkup(payload.diff);
  } catch (error) {
    result.innerHTML = `<div class="message error">${esc(errorMessage(error))}</div>`;
  }
}

async function f025HydrateSnapshotDiffs(editable) {
  const cards = [...content.querySelectorAll("[data-f025-snapshot-diff]")];
  await Promise.all(cards.map(async (card) => {
    const environmentID = card.dataset.environmentId;
    if (!environmentID) return;
    try {
      const payload = await api(f025SnapshotCollectionPath(environmentID));
      const snapshots = payload.snapshots || [];
      if (!snapshots.length) {
        card.innerHTML = `<p class="eyebrow">${esc(f025T("f025.snapshots"))}</p><p class="muted">${esc(f025T("f025.snapshots_empty"))}</p>`;
        return;
      }

      const options = (selectedID) => snapshots.map((snapshot) =>
        `<option value="${esc(snapshot.snapshot_id)}" ${snapshot.snapshot_id === selectedID ? "selected" : ""}>${esc(f025SnapshotLabel(snapshot))}</option>`
      ).join("");
      const latest = snapshots[snapshots.length - 1];
      let compare = `<p class="muted">${esc(f025T("f025.snapshots_need_two"))}</p>`;
      if (snapshots.length >= 2) {
        const before = snapshots[snapshots.length - 2];
        compare = `
          <div class="form-grid two">
            <label>${esc(f025T("f025.before"))}<select class="f025-snapshot-before">${options(before.snapshot_id)}</select></label>
            <label>${esc(f025T("f025.after"))}<select class="f025-snapshot-after">${options(latest.snapshot_id)}</select></label>
          </div>
          <div class="toolbar" style="margin-top:10px">
            <button class="button f025-snapshot-compare" type="button">${esc(f025T("f025.compare"))}</button>
          </div>
          <div class="f025-snapshot-diff-result" style="margin-top:10px"></div>`;
      }

      card.innerHTML = `
        <p class="eyebrow">${esc(f025T("f025.snapshots"))}</p>
        ${compare}
        <details style="margin-top:12px">
          <summary>${esc(f025T("f025.rebuild_plan"))}</summary>
          <div class="form-grid two" style="margin-top:10px">
            <label>${esc(f025T("f025.snapshots"))}<select class="f025-rebuild-snapshot">${options(latest.snapshot_id)}</select></label>
          </div>
          <div class="toolbar" style="margin-top:10px">
            <button class="button f025-rebuild-plan-load" type="button">${esc(f025T("f025.view_rebuild_plan"))}</button>
          </div>
          <div class="f025-rebuild-plan-result" style="margin-top:10px"></div>
        </details>
        <details style="margin-top:12px">
          <summary>${esc(f025T("f025.retained_plan"))}</summary>
          <div class="f025-retained-plan" style="margin-top:10px">
            <p class="muted">${esc(f025T("f025.retained_loading"))}</p>
          </div>
        </details>`;

      card.querySelector(".f025-snapshot-compare")?.addEventListener("click", () =>
        f025CompareSnapshots(card, environmentID)
      );
      card.querySelector(".f025-rebuild-plan-load")?.addEventListener("click", () =>
        f025LoadRebuildPlan(card, environmentID)
      );
      await f025HydrateRetainedPlan(card, environmentID, snapshots, editable);
    } catch (error) {
      card.innerHTML = `<p class="eyebrow">${esc(f025T("f025.snapshots"))}</p><div class="message error">${esc(errorMessage(error))}</div>`;
    }
  }));
}

function f025HasReferenceOnly(environment) {
  return (environment.manifest?.assets || []).some((asset) => asset.capture_policy === "REFERENCE_ONLY");
}

function f025EnvironmentCard(environment, roles, editable) {
  const roleID = environment.machine_role_id || "";
  const referenceOnly = f025HasReferenceOnly(environment);
  const policies = (environment.manifest?.assets || []).map((asset) => asset.capture_policy).filter(Boolean);
  return `<article class="card">
    <div class="section-title-row">
      <div>
        <p class="eyebrow">${esc(environment.environment_key)}</p>
        <h2>${esc(environment.name)}</h2>
        <p class="muted">${esc(environment.adapter_key)} · ${esc(environment.application_key)}</p>
      </div>
      <div class="toolbar">
        ${roleID ? pill("BOUND", "good") : pill(f025T("f025.unbound"), "warn")}
        ${policies.includes("CONTENT_BOUND") ? pill(f025T("f025.content_badge"), "good") : ""}
        ${referenceOnly ? pill(f025T("f025.reference_badge"), "warn") : ""}
      </div>
    </div>
    ${referenceOnly ? `<div class="message warn">${esc(f025T("f025.portability_warning"))}</div>` : ""}
    <div class="form-grid two" style="margin-top:14px">
      <label>${esc(f025T("f025.machine_role"))}
        <select class="f025-role-binding" data-environment-id="${esc(environment.execution_environment_id)}" ${editable ? "" : "disabled"}>
          ${f025RoleOptions(roles, roleID)}
        </select>
      </label>
      <div>
        <span class="label">${esc(f025T("f025.identity"))}</span>
        <p class="mono muted">${esc(environment.content_sha256)}</p>
      </div>
    </div>
    <details style="margin-top:12px"><summary>${esc(f025T("f025.manifest_json"))}</summary><pre class="mono muted">${esc(JSON.stringify(environment.manifest, null, 2))}</pre></details>
    <div data-f025-snapshot-diff data-environment-id="${esc(environment.execution_environment_id)}" style="margin-top:14px">
      <p class="eyebrow">${esc(f025T("f025.snapshots"))}</p>
      <p class="muted">${esc(f025T("f025.snapshots_loading"))}</p>
    </div>
    ${editable ? `<div class="toolbar" style="margin-top:12px"><button class="button f025-edit" data-environment-id="${esc(environment.execution_environment_id)}" type="button">${esc(f025T("f025.edit"))}</button><button class="button danger f025-remove" data-environment-id="${esc(environment.execution_environment_id)}" type="button">${esc(f025T("f025.remove"))}</button></div>` : ""}
  </article>`;
}

function f025GuidedManifest(baseManifest = null) {
  const policy = document.getElementById("f025CapturePolicy").value;
  const locator = document.getElementById("f025WorkspaceLocator").value.trim();
  const oscQueryURL = document.getElementById("f025OSCQueryURL").value.trim();
  const manifest = baseManifest ? JSON.parse(JSON.stringify(baseManifest)) : {};
  manifest.schema_version = 1;
  manifest.environment_key = document.getElementById("f025EnvironmentKey").value.trim();
  manifest.name = document.getElementById("f025EnvironmentName").value.trim();
  manifest.adapter_key = manifest.adapter_key || "stagecore.adapter.vdmx";

  const application = manifest.application || {};
  const otherHosts = (application.hosts || []).filter((host) => String(host.os || "").toLowerCase() !== "darwin");
  manifest.application = {
    ...application,
    key: application.key || "vdmx",
    name: application.name || "VDMX",
    vendor: application.vendor || "VIDVOX",
    version_constraint: document.getElementById("f025Version").value.trim(),
    hosts: [...otherHosts, {os: "darwin", architecture: document.getElementById("f025Architecture").value}],
  };

  const otherAssets = (manifest.assets || []).filter((asset) => asset.key !== "workspace");
  if (locator) {
    const asset = {
      key: "workspace",
      kind: "PROJECT_FILE",
      name: "VDMX workspace",
      capture_policy: policy,
      locator,
    };
    if (policy === "CONTENT_BOUND") {
      const contentHash = document.getElementById("f025ContentHash").value.trim().toLowerCase();
      const sizeText = document.getElementById("f025SizeBytes").value.trim();
      const sizeBytes = Number(sizeText);
      if (!/^[a-f0-9]{64}$/.test(contentHash) || sizeText === "" || !Number.isSafeInteger(sizeBytes) || sizeBytes < 0) {
        throw new Error(f025T("f025.invalid_content_bound"));
      }
      asset.content_hash = contentHash;
      asset.size_bytes = sizeBytes;
    }
    manifest.assets = [...otherAssets, asset];
    if (!baseManifest || !manifest.launch || manifest.launch.asset_key === "workspace") {
      manifest.launch = {kind: "ASSET", asset_key: "workspace"};
    }
  } else {
    manifest.assets = otherAssets;
    if (!manifest.assets.length) delete manifest.assets;
    if (manifest.launch?.asset_key === "workspace") delete manifest.launch;
  }

  const otherBindings = (manifest.bindings || []).filter((binding) => binding.key !== "oscquery");
  if (oscQueryURL) {
    manifest.bindings = [...otherBindings, {
      key: "oscquery",
      kind: "NETWORK",
      name: "VDMX OSCQuery",
      external_ref: oscQueryURL,
      required: false,
    }];
  } else {
    manifest.bindings = otherBindings;
    if (!manifest.bindings.length) delete manifest.bindings;
  }
  return manifest;
}

function f025AdvancedTemplate() {
  return JSON.stringify({
    schema_version: 1,
    environment_key: "video-secondary",
    name: "Secondary execution workstation",
    adapter_key: "stagecore.adapter.vdmx",
    application: {
      key: "vdmx",
      name: "VDMX",
      vendor: "VIDVOX",
      version_constraint: "8.x-tested",
      hosts: [{os: "darwin", architecture: "arm64"}],
    },
    assets: [{
      key: "workspace",
      kind: "PROJECT_FILE",
      name: "VDMX workspace",
      capture_policy: "REFERENCE_ONLY",
      locator: "/Users/show/Secondary.vdmx5",
    }],
    bindings: [{
      key: "oscquery",
      kind: "NETWORK",
      name: "VDMX OSCQuery",
      external_ref: "http://127.0.0.1:8080/",
      required: false,
    }],
    launch: {kind: "ASSET", asset_key: "workspace"},
  }, null, 2);
}

async function f025Create(manifest, machineRoleID) {
  const created = await api(f025CollectionPath(), {method: "POST", json: {manifest}});
  if (machineRoleID) {
    await api(`${f025CollectionPath()}/${encodeURIComponent(created.execution_environment_id)}/machine-role`, {
      method: "PUT",
      json: {machine_role_id: machineRoleID},
    });
  }
}

async function renderExecutionEnvironments(message = "") {
  const model = await f025LoadModel();
  const roleCanEdit = canEdit();
  const editable = roleCanEdit && model.revision.status === "DRAFT";
  const disabled = editable ? "" : "disabled";
  const startEdit = roleCanEdit && !editable ? `<button id="f025StartEdit" class="button primary" type="button">${esc(f025T("f025.start_edit"))}</button>` : "";
  content.innerHTML = `
    <div class="page-head">
      <div><p class="eyebrow">${esc(f025T("f025.eyebrow"))}</p><h1>${esc(f025T("f025.title"))}</h1><p>${esc(f025T("f025.summary"))}</p></div>
      <div class="toolbar">${pill(model.revision.status, model.revision.status === "DRAFT" ? "warn" : "good")}${startEdit}<button id="f025Refresh" class="button" type="button">${esc(f025T("f025.refresh"))}</button></div>
    </div>
    ${message ? `<div class="message success">${esc(message)}</div>` : ""}
    ${roleCanEdit && !editable ? `<div class="message warn">${esc(f025T("f025.read_only"))}</div>` : ""}
    <div class="message">${esc(f025T("f025.readiness_note"))}</div>

    ${roleCanEdit ? `<div class="grid cards" style="margin-top:14px">
      <article class="card">
        <p class="eyebrow">VDMX</p><h2>${esc(f025T("f025.guided"))}</h2>
        <form id="f025GuidedForm" data-environment-id="" style="margin-top:14px">
          <div class="form-grid two">
            <label>${esc(f025T("f025.environment_key"))}<input id="f025EnvironmentKey" value="video-main" ${disabled} required></label>
            <label>${esc(f025T("f025.name"))}<input id="f025EnvironmentName" value="Main video workstation" ${disabled} required></label>
            <label>${esc(f025T("f025.version"))}<input id="f025Version" value="8.x-tested" ${disabled} required></label>
            <label>${esc(f025T("f025.architecture"))}<select id="f025Architecture" ${disabled}><option value="arm64">Apple Silicon · arm64</option><option value="amd64">Intel · amd64</option></select></label>
            <label>${esc(f025T("f025.workspace_locator"))}<input id="f025WorkspaceLocator" placeholder="/Users/show/Stage.vdmx5" ${disabled}><span class="muted">${esc(f025T("f025.workspace_demo_hint"))}</span></label>\n            <label>${esc(f025T("f025.oscquery_url"))}<input id="f025OSCQueryURL" value="http://127.0.0.1:8080/" inputmode="url" dir="ltr" ${disabled}><span class="muted">${esc(f025T("f025.oscquery_hint"))}</span></label>
            <label>${esc(f025T("f025.capture_policy"))}<select id="f025CapturePolicy" ${disabled}><option value="REFERENCE_ONLY">${esc(f025T("f025.reference_only"))}</option><option value="CONTENT_BOUND">${esc(f025T("f025.content_bound"))}</option></select></label>
            <label id="f025ContentHashLabel" class="hidden">${esc(f025T("f025.content_hash"))}<input id="f025ContentHash" class="mono" maxlength="64" ${disabled}></label>
            <label id="f025SizeBytesLabel" class="hidden">${esc(f025T("f025.size_bytes"))}<input id="f025SizeBytes" type="number" min="0" step="1" ${disabled}></label>
            <label>${esc(f025T("f025.machine_role"))}<select id="f025GuidedRole" ${disabled}>${f025RoleOptions(model.machine_roles || [])}</select></label>
          </div>
          <div class="message">${esc(f025T("f025.osc_go_hint"))}</div>\n          <div class="toolbar"><button id="f025GuidedSubmit" class="button primary" type="submit" ${disabled}>${esc(f025T("f025.create"))}</button><button id="f025GuidedCancel" class="button ghost hidden" type="button" ${disabled}>${esc(f025T("f025.cancel_edit"))}</button></div>
        </form>
      </article>

      <article class="card">
        <p class="eyebrow">MANIFEST V1</p><h2>${esc(f025T("f025.advanced"))}</h2>
        <form id="f025AdvancedForm" style="margin-top:14px">
          <label>${esc(f025T("f025.manifest_json"))}<textarea id="f025AdvancedManifest" class="mono" rows="18" ${disabled}>${esc(f025AdvancedTemplate())}</textarea></label>
          <label>${esc(f025T("f025.machine_role"))}<select id="f025AdvancedRole" ${disabled}>${f025RoleOptions(model.machine_roles || [])}</select></label>
          <button class="button primary" type="submit" ${disabled}>${esc(f025T("f025.create"))}</button>
        </form>
      </article>
    </div>` : ""}

    <div class="grid cards" style="margin-top:14px">
      ${(model.execution_environments || []).length ? (model.execution_environments || []).map((environment) => f025EnvironmentCard(environment, model.machine_roles || [], editable)).join("") : `<div class="empty">${esc(f025T("f025.empty"))}</div>`}
    </div>`;

  document.querySelector('[data-page="environments"]')?.replaceChildren(document.createTextNode(f025T("f025.nav")));
  document.getElementById("f025Refresh")?.addEventListener("click", () => renderExecutionEnvironments().catch(f025Error));
  document.getElementById("f025StartEdit")?.addEventListener("click", f025StartEdit);
  await f025HydrateSnapshotDiffs(editable);
  if (!editable) return;

  const policy = document.getElementById("f025CapturePolicy");
  const syncPolicy = () => {
    const contentBound = policy.value === "CONTENT_BOUND";
    document.getElementById("f025ContentHashLabel")?.classList.toggle("hidden", !contentBound);
    document.getElementById("f025SizeBytesLabel")?.classList.toggle("hidden", !contentBound);
  };
  policy?.addEventListener("change", syncPolicy);
  syncPolicy();

  document.getElementById("f025GuidedForm")?.addEventListener("submit", async (event) => {
    event.preventDefault();
    try {
      await f025Create(f025GuidedManifest(), document.getElementById("f025GuidedRole").value);
      await renderExecutionEnvironments(f025T("f025.created"));
    } catch (error) { f025Error(error); }
  });
  document.getElementById("f025AdvancedForm")?.addEventListener("submit", async (event) => {
    event.preventDefault();
    try {
      const manifest = JSON.parse(document.getElementById("f025AdvancedManifest").value);
      await f025Create(manifest, document.getElementById("f025AdvancedRole").value);
      await renderExecutionEnvironments(f025T("f025.created"));
    } catch (error) { f025Error(error); }
  });
  content.querySelectorAll(".f025-role-binding").forEach((select) => {
    select.addEventListener("change", async () => {
      try {
        await api(`${f025CollectionPath()}/${encodeURIComponent(select.dataset.environmentId)}/machine-role`, {
          method: "PUT",
          json: {machine_role_id: select.value || null},
        });
        await renderExecutionEnvironments(f025T("f025.bound"));
      } catch (error) { f025Error(error); }
    });
  });
  content.querySelectorAll(".f025-remove").forEach((button) => {
    button.addEventListener("click", async () => {
      if (!confirm(f025T("f025.delete_confirm"))) return;
      try {
        await api(`${f025CollectionPath()}/${encodeURIComponent(button.dataset.environmentId)}?confirm=true`, {method: "DELETE"});
        await renderExecutionEnvironments(f025T("f025.removed"));
      } catch (error) { f025Error(error); }
    });
  });
}

async function f025StartEdit() {
  try {
    await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/configuration/draft`, {method: "POST"});
    const projectPayload = await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}`);
    state.project = projectPayload.project;
    updateWorkspaceProject();
    await renderExecutionEnvironments();
  } catch (error) { f025Error(error); }
}

function f025Error(error) {
  setMessage(globalMessage, errorMessage(error), "error");
}

document.querySelector('[data-page="environments"]')?.replaceChildren(document.createTextNode(f025T("f025.nav")));

navigate = async function stagecoreExecutionEnvironmentNavigate(page) {
  if (page !== "environments") return stagecoreExecutionEnvironmentNavigateBase(page);
  if (!state.project) return;
  setPage(page);
  setMessage(globalMessage, "");
  try { await renderExecutionEnvironments(); }
  catch (error) { f025Error(error); }
};
