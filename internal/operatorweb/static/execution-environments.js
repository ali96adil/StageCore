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
  "f025.workspace_locator": {"en":"VDMX workspace path","ar-IQ":"مسار مشروع VDMX"},
  "f025.capture_policy": {"en":"Asset policy","ar-IQ":"سياسة الملف"},
  "f025.reference_only": {"en":"Reference only — verify location, bytes are not backed up","ar-IQ":"مرجع فقط — تحقق من الموقع، والبايتات غير محفوظة كنسخة احتياطية"},
  "f025.content_bound": {"en":"Content bound — exact hash and size are already known","ar-IQ":"محتوى مرتبط — الهاش والحجم الدقيقان معروفان مسبقاً"},
  "f025.content_hash": {"en":"SHA-256 content hash","ar-IQ":"بصمة SHA-256 للمحتوى"},
  "f025.size_bytes": {"en":"Exact size in bytes","ar-IQ":"الحجم الدقيق بالبايت"},
  "f025.machine_role": {"en":"Machine Role binding","ar-IQ":"ربط دور الجهاز"},
  "f025.unbound_option": {"en":"Unbound — choose explicitly later","ar-IQ":"غير مربوط — اختر الدور لاحقاً بشكل صريح"},
  "f025.create": {"en":"Create environment","ar-IQ":"إنشاء بيئة التشغيل"},
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
  "f025.readiness_note": {"en":"SHOW Preflight inspects the bound Companion through the authenticated read-only F-025 transport.","ar-IQ":"يفحص SHOW Preflight الجهاز المرافق المرتبط عبر مسار F-025 الموثق والمخصص للقراءة فقط."}
};

function f025T(key) {
  return f025Strings[key]?.[f025Locale] || f025Strings[key]?.en || key;
}

function f025RoleOptions(roles, selected = "") {
  return `<option value="">${esc(f025T("f025.unbound_option"))}</option>` + (roles || []).map((role) => {
    const label = role.display_name || role.role_key || role.machine_role_id;
    return `<option value="${esc(role.machine_role_id)}" ${role.machine_role_id === selected ? "selected" : ""}>${esc(label)} · ${esc(role.role_key)}</option>`;
  }).join("");
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
    ${editable ? `<div class="toolbar" style="margin-top:12px"><button class="button danger f025-remove" data-environment-id="${esc(environment.execution_environment_id)}" type="button">${esc(f025T("f025.remove"))}</button></div>` : ""}
  </article>`;
}

function f025GuidedManifest() {
  const policy = document.getElementById("f025CapturePolicy").value;
  const locator = document.getElementById("f025WorkspaceLocator").value.trim();
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
  return {
    schema_version: 1,
    environment_key: document.getElementById("f025EnvironmentKey").value.trim(),
    name: document.getElementById("f025EnvironmentName").value.trim(),
    adapter_key: "stagecore.adapter.vdmx",
    application: {
      key: "vdmx",
      name: "VDMX",
      vendor: "VIDVOX",
      version_constraint: document.getElementById("f025Version").value.trim(),
      hosts: [{os: "darwin", architecture: document.getElementById("f025Architecture").value}],
    },
    assets: [asset],
    launch: {kind: "ASSET", asset_key: "workspace"},
  };
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
        <form id="f025GuidedForm" style="margin-top:14px">
          <div class="form-grid two">
            <label>${esc(f025T("f025.environment_key"))}<input id="f025EnvironmentKey" value="video-main" ${disabled} required></label>
            <label>${esc(f025T("f025.name"))}<input id="f025EnvironmentName" value="Main video workstation" ${disabled} required></label>
            <label>${esc(f025T("f025.version"))}<input id="f025Version" value="8.x-tested" ${disabled} required></label>
            <label>${esc(f025T("f025.architecture"))}<select id="f025Architecture" ${disabled}><option value="arm64">Apple Silicon · arm64</option><option value="amd64">Intel · amd64</option></select></label>
            <label>${esc(f025T("f025.workspace_locator"))}<input id="f025WorkspaceLocator" placeholder="/Users/show/Stage.vdmx5" ${disabled} required></label>
            <label>${esc(f025T("f025.capture_policy"))}<select id="f025CapturePolicy" ${disabled}><option value="REFERENCE_ONLY">${esc(f025T("f025.reference_only"))}</option><option value="CONTENT_BOUND">${esc(f025T("f025.content_bound"))}</option></select></label>
            <label id="f025ContentHashLabel" class="hidden">${esc(f025T("f025.content_hash"))}<input id="f025ContentHash" class="mono" maxlength="64" ${disabled}></label>
            <label id="f025SizeBytesLabel" class="hidden">${esc(f025T("f025.size_bytes"))}<input id="f025SizeBytes" type="number" min="0" step="1" ${disabled}></label>
            <label>${esc(f025T("f025.machine_role"))}<select id="f025GuidedRole" ${disabled}>${f025RoleOptions(model.machine_roles || [])}</select></label>
          </div>
          <button class="button primary" type="submit" ${disabled}>${esc(f025T("f025.create"))}</button>
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
