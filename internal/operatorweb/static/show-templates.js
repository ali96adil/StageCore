"use strict";

const f011Copy = {
  "templates.nav": { en: "Templates", "ar-IQ": "القوالب" },
  "templates.eyebrow": { en: "SHOW TEMPLATES", "ar-IQ": "قوالب العرض" },
  "templates.title": { en: "Start from a reusable show template", "ar-IQ": "ابدأ من قالب عرض قابل لإعادة الاستخدام" },
  "templates.summary": { en: "Templates create an ordinary editable Project Draft. They never publish a Runtime Snapshot, enter SHOW, or trigger GO.", "ar-IQ": "القوالب تنشئ مسودة مشروع اعتيادية وقابلة للتعديل. لا تنشر Runtime Snapshot ولا تدخل وضع العرض ولا تشغّل GO." },
  "templates.refresh": { en: "Refresh", "ar-IQ": "تحديث" },
  "templates.official": { en: "Official starters", "ar-IQ": "قوالب البداية الرسمية" },
  "templates.use": { en: "Use template", "ar-IQ": "استخدم القالب" },
  "templates.read_only": { en: "This role can review templates but cannot create Projects.", "ar-IQ": "هذا الدور يستطيع مراجعة القوالب لكنه لا يستطيع إنشاء المشاريع." },
  "templates.version": { en: "Version", "ar-IQ": "الإصدار" },
  "templates.fields": { en: "Setup fields", "ar-IQ": "حقول الإعداد" },
  "templates.no_fields": { en: "No setup fields are required.", "ar-IQ": "لا توجد حقول إعداد مطلوبة." },
  "templates.project_name": { en: "Project name", "ar-IQ": "اسم المشروع" },
  "templates.project_description": { en: "Project description", "ar-IQ": "وصف المشروع" },
  "templates.default_hint": { en: "Leave blank to use the template default.", "ar-IQ": "اتركه فارغاً لاستخدام القيمة الافتراضية للقالب." },
  "templates.create": { en: "Create editable Draft", "ar-IQ": "إنشاء مسودة قابلة للتعديل" },
  "templates.creating": { en: "Creating Project Draft…", "ar-IQ": "جاري إنشاء مسودة المشروع…" },
  "templates.created": { en: "Project Draft created from template.", "ar-IQ": "تم إنشاء مسودة المشروع من القالب." },
  "templates.cancel": { en: "Cancel", "ar-IQ": "إلغاء" },
  "templates.safety": { en: "Normal validation, Publish, Preflight and SHOW authority still apply after creation.", "ar-IQ": "تبقى صلاحيات التحقق والنشر والفحص المسبق ووضع العرض الاعتيادية مطبقة بعد الإنشاء." },
  "templates.import_title": { en: "Import template document", "ar-IQ": "استيراد ملف قالب" },
  "templates.import_summary": { en: "Review compatibility first. Validation never mutates the Hub; materialization is a separate explicit action.", "ar-IQ": "راجع التوافق أولاً. التحقق لا يغيّر الـHub؛ إنشاء المشروع خطوة منفصلة وصريحة." },
  "templates.choose_file": { en: "Choose JSON template", "ar-IQ": "اختيار ملف قالب JSON" },
  "templates.validate": { en: "Validate import", "ar-IQ": "التحقق من الاستيراد" },
  "templates.validating": { en: "Validating template…", "ar-IQ": "جاري التحقق من القالب…" },
  "templates.compatible": { en: "Compatible", "ar-IQ": "متوافق" },
  "templates.incompatible": { en: "Incompatible", "ar-IQ": "غير متوافق" },
  "templates.import_ready": { en: "Validated. You may create a new editable Project Draft from this document.", "ar-IQ": "تم التحقق. يمكنك إنشاء مسودة مشروع جديدة وقابلة للتعديل من هذا الملف." },
  "templates.import_create": { en: "Create from imported template", "ar-IQ": "إنشاء من القالب المستورد" },
  "templates.export_title": { en: "Export current Project as template", "ar-IQ": "تصدير المشروع الحالي كقالب" },
  "templates.export_summary": { en: "Exports the current Project graph with symbolic references only. Runtime history, Sessions, Snapshot identity, presentation state and secret-like configuration are not exported.", "ar-IQ": "يصدر هيكل المشروع الحالي بمراجع رمزية فقط. لا يتم تصدير سجل التشغيل أو الجلسات أو هوية اللقطة أو حالة الواجهة أو الإعدادات الشبيهة بالأسرار." },
  "templates.no_project": { en: "Open a Project first to export it as a template.", "ar-IQ": "افتح مشروعاً أولاً لتصديره كقالب." },
  "templates.export": { en: "Export template JSON", "ar-IQ": "تصدير قالب JSON" },
  "templates.exported": { en: "Template export prepared.", "ar-IQ": "تم تجهيز تصدير القالب." },
  "templates.required": { en: "Required", "ar-IQ": "مطلوب" },
  "templates.optional": { en: "Optional", "ar-IQ": "اختياري" },
  "templates.string": { en: "Text", "ar-IQ": "نص" },
  "templates.int": { en: "Number", "ar-IQ": "رقم" },
  "templates.bool": { en: "On / Off", "ar-IQ": "تشغيل / إيقاف" },
  "templates.none": { en: "No templates available.", "ar-IQ": "لا توجد قوالب متاحة." },
};

let f011ImportedDocument = null;
let f011ImportedTemplate = null;

function f011Locale() {
  if (el("languageSelect")?.value === "en") return "en";
  return localStorage.getItem("stagecore_locale") === "en" ? "en" : "ar-IQ";
}
function f011T(key) { const entry = f011Copy[key]; return entry?.[f011Locale()] || entry?.en || key; }
function f011Text(value) { return f011Locale() === "en" ? (value?.en || value?.["ar-IQ"] || "") : (value?.["ar-IQ"] || value?.en || ""); }

function f011InstallNav() {
  let button = el("templatesNav");
  if (!button) {
    button = document.createElement("button");
    button.id = "templatesNav";
    button.type = "button";
    button.className = "nav-button";
    button.dataset.page = "templates";
    el("projectsNav")?.insertAdjacentElement("afterend", button);
    button.addEventListener("click", () => renderShowTemplatesWorkspace().catch((error) => setMessage(globalMessage, errorMessage(error), "error")));
  }
  button.textContent = f011T("templates.nav");
}

function f011FieldInput(field, prefix = "templateField") {
  const id = `${prefix}-${field.key}`;
  const label = `${f011Text(field.label)} · ${f011T(field.required ? "templates.required" : "templates.optional")}`;
  const help = f011Text(field.help);
  let control = "";
  if (field.type === "BOOL") {
    control = `<select id="${esc(id)}" data-template-field="${esc(field.key)}" data-template-type="BOOL"><option value="true">${esc(f011T("templates.bool"))} · ✓</option><option value="false">${esc(f011T("templates.bool"))} · ✕</option></select>`;
  } else {
    const type = field.type === "INT" ? "number" : "text";
    const min = field.min_int ?? "";
    const max = field.max_int ?? "";
    const maxLength = field.max_length || "";
    let defaultValue = "";
    if (field.default_value !== undefined && field.default_value !== null) defaultValue = field.default_value;
    control = `<input id="${esc(id)}" data-template-field="${esc(field.key)}" data-template-type="${esc(field.type)}" type="${type}" ${field.required ? "required" : ""} ${min !== "" ? `min="${esc(min)}"` : ""} ${max !== "" ? `max="${esc(max)}"` : ""} ${maxLength ? `maxlength="${esc(maxLength)}"` : ""} value="${esc(defaultValue)}">`;
  }
  return `<label>${esc(label)}${control}<small class="muted">${esc(help)}</small></label>`;
}

function f011ReadValues(container) {
  const values = {};
  container.querySelectorAll("[data-template-field]").forEach((input) => {
    const key = input.dataset.templateField;
    const type = input.dataset.templateType;
    if (type === "BOOL") values[key] = input.value === "true";
    else if (type === "INT") {
      if (input.value !== "") values[key] = Number(input.value);
    } else if (input.value !== "") values[key] = input.value;
  });
  return values;
}

function f011TemplateCards(templates) {
  if (!templates.length) return `<div class="empty">${esc(f011T("templates.none"))}</div>`;
  return `<div class="grid cards">${templates.map((template) => `
    <article class="card">
      <div class="section-title-row"><div><p class="eyebrow">${esc(template.source || "OFFICIAL")}</p><h2>${esc(f011Text(template.name))}</h2></div>${pill(`v${template.version}`, "neutral")}</div>
      <p class="muted">${esc(f011Text(template.summary))}</p>
      <div class="meta"><span>${esc(f011T("templates.fields"))}: ${esc(template.fields?.length || 0)}</span><span>${esc((template.tags || []).join(" · "))}</span></div>
      ${canEdit() ? `<button class="button primary template-use" data-template-id="${esc(template.template_id)}" type="button">${esc(f011T("templates.use"))}</button>` : `<p class="muted">${esc(f011T("templates.read_only"))}</p>`}
    </article>`).join("")}</div>`;
}

async function renderShowTemplatesWorkspace() {
  setPage("templates");
  f011InstallNav();
  setMessage(globalMessage, "");
  const payload = await api("/api/v1/show-templates");
  const templates = payload.templates || [];
  content.innerHTML = `
    <div class="page-head"><div><p class="eyebrow">${esc(f011T("templates.eyebrow"))}</p><h1>${esc(f011T("templates.title"))}</h1><p>${esc(f011T("templates.summary"))}</p></div><button id="templateRefresh" class="button" type="button">${esc(f011T("templates.refresh"))}</button></div>
    <section class="card" style="margin-bottom:14px"><div class="section-title-row"><div><h2>${esc(f011T("templates.official"))}</h2><p class="muted">${esc(f011T("templates.safety"))}</p></div></div></section>
    ${f011TemplateCards(templates)}
    <section class="card" style="margin-top:14px">
      <div class="section-title-row"><div><h2>${esc(f011T("templates.import_title"))}</h2><p class="muted">${esc(f011T("templates.import_summary"))}</p></div></div>
      <div class="form-grid two" style="margin-top:12px"><label>${esc(f011T("templates.choose_file"))}<input id="templateImportFile" type="file" accept="application/json,.json"></label><div><button id="templateImportValidate" class="button" type="button">${esc(f011T("templates.validate"))}</button></div></div>
      <div id="templateImportResult" style="margin-top:12px"></div>
    </section>
    <section class="card" style="margin-top:14px">
      <div class="section-title-row"><div><h2>${esc(f011T("templates.export_title"))}</h2><p class="muted">${esc(f011T("templates.export_summary"))}</p></div></div>
      ${state.project ? `<p><strong>${esc(state.project.name)}</strong></p><button id="templateExportCurrent" class="button" type="button">${esc(f011T("templates.export"))}</button>` : `<div class="empty">${esc(f011T("templates.no_project"))}</div>`}
    </section>`;

  el("templateRefresh")?.addEventListener("click", () => renderShowTemplatesWorkspace().catch((error) => setMessage(globalMessage, errorMessage(error), "error")));
  content.querySelectorAll(".template-use").forEach((button) => button.addEventListener("click", () => f011RenderMaterialize(button.dataset.templateId).catch((error) => setMessage(globalMessage, errorMessage(error), "error"))));
  el("templateImportValidate")?.addEventListener("click", () => f011ValidateImport().catch((error) => setMessage(globalMessage, errorMessage(error), "error")));
  el("templateExportCurrent")?.addEventListener("click", () => f011ExportCurrent().catch((error) => setMessage(globalMessage, errorMessage(error), "error")));
}

async function f011RenderMaterialize(templateID) {
  const payload = await api(`/api/v1/show-templates/${encodeURIComponent(templateID)}`);
  const template = payload.template;
  content.innerHTML = `
    <div class="page-head"><div><p class="eyebrow">${esc(f011T("templates.eyebrow"))}</p><h1>${esc(f011Text(template.name))}</h1><p>${esc(f011Text(template.summary))}</p></div><button id="templateCancel" class="button" type="button">${esc(f011T("templates.cancel"))}</button></div>
    <section class="card"><form id="templateMaterializeForm">
      <div class="form-grid two"><label>${esc(f011T("templates.project_name"))}<input id="templateProjectName" maxlength="160" placeholder="${esc(f011Text(template.project?.default_name))}"><small class="muted">${esc(f011T("templates.default_hint"))}</small></label><label>${esc(f011T("templates.project_description"))}<input id="templateProjectDescription" maxlength="500" placeholder="${esc(f011Text(template.project?.default_description))}"><small class="muted">${esc(f011T("templates.default_hint"))}</small></label></div>
      <div class="section-title-row" style="margin-top:16px"><div><h3>${esc(f011T("templates.fields"))}</h3></div></div>
      ${(template.fields || []).length ? `<div class="form-grid two">${template.fields.map((field) => f011FieldInput(field)).join("")}</div>` : `<div class="empty">${esc(f011T("templates.no_fields"))}</div>`}
      <p class="muted">${esc(f011T("templates.safety"))}</p><button class="button primary" type="submit">${esc(f011T("templates.create"))}</button>
    </form></section>`;
  el("templateCancel")?.addEventListener("click", () => renderShowTemplatesWorkspace().catch((error) => setMessage(globalMessage, errorMessage(error), "error")));
  el("templateMaterializeForm")?.addEventListener("submit", async (event) => {
    event.preventDefault();
    setMessage(globalMessage, f011T("templates.creating"));
    try {
      const result = await api(`/api/v1/show-templates/${encodeURIComponent(templateID)}/materialize`, { method: "POST", json: { project_name: el("templateProjectName").value.trim(), project_description: el("templateProjectDescription").value.trim(), locale: f011Locale(), values: f011ReadValues(event.currentTarget) } });
      await f011OpenCreatedProject(result.result.project_id);
    } catch (error) { setMessage(globalMessage, errorMessage(error), "error"); }
  });
}

async function f011OpenCreatedProject(projectID) {
  await loadProjects();
  const loaded = await api(`/api/v1/projects/${encodeURIComponent(projectID)}`);
  state.project = loaded.project;
  updateWorkspaceProject();
  setMessage(globalMessage, f011T("templates.created"), "success");
  await navigate("dashboard");
}

async function f011ValidateImport() {
  const input = el("templateImportFile");
  const file = input?.files?.[0];
  if (!file) throw new Error(f011T("templates.choose_file"));
  if (file.size > 2 * 1024 * 1024) throw new Error("SHOW_TEMPLATE_DOCUMENT_INVALID");
  setMessage(globalMessage, f011T("templates.validating"));
  const text = await file.text();
  const document = JSON.parse(text);
  const payload = await api("/api/v1/show-templates/import/validate", { method: "POST", json: { template: document } });
  f011ImportedDocument = document;
  f011ImportedTemplate = payload.template;
  const compatibility = payload.compatibility || { compatible: false, reasons: [] };
  const result = el("templateImportResult");
  result.innerHTML = `<div class="section-title-row"><div><h3>${esc(f011Text(payload.template.name) || payload.template.template_id)}</h3><p class="muted">${esc(f011Text(payload.template.summary))}</p></div>${pill(compatibility.compatible ? f011T("templates.compatible") : f011T("templates.incompatible"), compatibility.compatible ? "good" : "bad")}</div>${(compatibility.reasons || []).length ? `<ul class="validation-list">${compatibility.reasons.map((reason) => `<li class="validation-item">${esc(reason)}</li>`).join("")}</ul>` : ""}${compatibility.compatible && canEdit() ? `<p class="muted">${esc(f011T("templates.import_ready"))}</p><button id="templateImportCreate" class="button primary" type="button">${esc(f011T("templates.import_create"))}</button>` : ""}`;
  setMessage(globalMessage, "");
  el("templateImportCreate")?.addEventListener("click", () => f011RenderImportedMaterialize());
}

function f011RenderImportedMaterialize() {
  const template = f011ImportedTemplate;
  if (!template || !f011ImportedDocument) return;
  content.innerHTML = `<div class="page-head"><div><p class="eyebrow">${esc(f011T("templates.import_title"))}</p><h1>${esc(f011Text(template.name) || template.template_id)}</h1></div><button id="templateCancel" class="button" type="button">${esc(f011T("templates.cancel"))}</button></div><section class="card"><form id="templateImportedForm"><div class="form-grid two"><label>${esc(f011T("templates.project_name"))}<input id="templateProjectName" maxlength="160" placeholder="${esc(f011Text(template.project?.default_name))}"></label><label>${esc(f011T("templates.project_description"))}<input id="templateProjectDescription" maxlength="500" placeholder="${esc(f011Text(template.project?.default_description))}"></label></div>${(template.fields || []).length ? `<div class="form-grid two">${template.fields.map((field) => f011FieldInput(field, "importField")).join("")}</div>` : `<div class="empty">${esc(f011T("templates.no_fields"))}</div>`}<p class="muted">${esc(f011T("templates.safety"))}</p><button class="button primary" type="submit">${esc(f011T("templates.import_create"))}</button></form></section>`;
  el("templateCancel")?.addEventListener("click", () => renderShowTemplatesWorkspace().catch((error) => setMessage(globalMessage, errorMessage(error), "error")));
  el("templateImportedForm")?.addEventListener("submit", async (event) => { event.preventDefault(); try { const payload = await api("/api/v1/show-templates/import/materialize", { method: "POST", json: { template: f011ImportedDocument, project_name: el("templateProjectName").value.trim(), project_description: el("templateProjectDescription").value.trim(), locale: f011Locale(), values: f011ReadValues(event.currentTarget) } }); await f011OpenCreatedProject(payload.result.project_id); } catch (error) { setMessage(globalMessage, errorMessage(error), "error"); } });
}

async function f011ExportCurrent() {
  if (!state.project) return;
  const payload = await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/template-export`);
  const text = JSON.stringify(payload.template, null, 2);
  const blob = new Blob([text], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = `${String(state.project.name || "stagecore-template").replace(/[^A-Za-z0-9._-]+/g, "-")}.stagecore-template.json`;
  document.body.appendChild(link); link.click(); link.remove(); URL.revokeObjectURL(url);
  setMessage(globalMessage, f011T("templates.exported"), "success");
}

f011InstallNav();
el("languageSelect")?.addEventListener("change", () => { f011InstallNav(); if (state.page === "templates") renderShowTemplatesWorkspace().catch((error) => setMessage(globalMessage, errorMessage(error), "error")); });
