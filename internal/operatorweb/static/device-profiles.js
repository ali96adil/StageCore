"use strict";

const f021BaseRenderConfiguration = renderConfiguration;

const f021Copy = {
  "device_profiles.title": { en: "Add a device from a profile", "ar-IQ": "إضافة جهاز من ملف تعريف" },
  "device_profiles.summary": { en: "Choose a known device profile and fill in simple connection fields. StageCore builds the target configuration for you.", "ar-IQ": "اختر ملف تعريف جهاز معروف واملأ حقول الاتصال البسيطة، وسيبني StageCore إعداد الهدف تلقائياً." },
  "device_profiles.profile": { en: "Device profile", "ar-IQ": "ملف تعريف الجهاز" },
  "device_profiles.logical_name": { en: "Project device name", "ar-IQ": "اسم الجهاز داخل المشروع" },
  "device_profiles.capabilities": { en: "Capabilities", "ar-IQ": "القدرات" },
  "device_profiles.protocol": { en: "Tested protocol", "ar-IQ": "البروتوكول المختبر" },
  "device_profiles.review": { en: "Review & validate", "ar-IQ": "مراجعة وتحقق" },
  "device_profiles.save": { en: "Add device to Project", "ar-IQ": "إضافة الجهاز إلى المشروع" },
  "device_profiles.review_note": { en: "Review validates the profile fields and builds a preview. It does not send a live command to the device.", "ar-IQ": "تتحقق المراجعة من حقول ملف التعريف وتبني معاينة، ولا ترسل أمراً حياً إلى الجهاز." },
  "device_profiles.preview": { en: "Configuration preview", "ar-IQ": "معاينة الإعداد" },
  "device_profiles.preview_ready": { en: "Validated and ready to save", "ar-IQ": "تم التحقق وهو جاهز للحفظ" },
  "device_profiles.preview_changed": { en: "Connection fields changed. Review again before saving.", "ar-IQ": "تغيّرت حقول الاتصال. أعد المراجعة قبل الحفظ." },
  "device_profiles.advanced": { en: "Advanced / manual target setup", "ar-IQ": "إعداد هدف متقدم / يدوي" },
  "device_profiles.saved": { en: "Device target added to the Project.", "ar-IQ": "تمت إضافة هدف الجهاز إلى المشروع." },
  "device_profiles.unavailable": { en: "Device profiles are unavailable. Advanced target setup is still available.", "ar-IQ": "ملفات تعريف الأجهزة غير متاحة حالياً. يبقى إعداد الهدف المتقدم متاحاً." },
  "device_profiles.required": { en: "Complete the required device fields before review.", "ar-IQ": "أكمل حقول الجهاز المطلوبة قبل المراجعة." },
};

function f021Locale() {
  return localStorage.getItem("stagecore_locale") === "en" ? "en" : "ar-IQ";
}

function f021T(key) {
  const entry = f021Copy[key];
  return entry?.[f021Locale()] || entry?.en || key;
}

function f021ProfileText(value) {
  if (!value) return "";
  return value[f021Locale()] || value.en || value["ar-IQ"] || "";
}

function f021DefaultValue(field) {
  if (field.default_value === undefined || field.default_value === null) return "";
  return field.default_value;
}

function f021FieldControl(field, disabled) {
  const id = `f021Field_${String(field.key).replace(/[^A-Za-z0-9_-]/g, "_")}`;
  const required = field.required ? "required" : "";
  const disabledAttr = disabled ? "disabled" : "";
  const help = f021ProfileText(field.help);
  const label = f021ProfileText(field.label) || field.key;
  const value = f021DefaultValue(field);
  let control = "";
  if (field.type === "BOOL") {
    control = `<input id="${esc(id)}" data-f021-field="${esc(field.key)}" type="checkbox" ${value === true ? "checked" : ""} ${disabledAttr}>`;
  } else if (field.type === "INT") {
    const min = field.min_int === undefined || field.min_int === null ? "" : `min="${esc(field.min_int)}"`;
    const max = field.max_int === undefined || field.max_int === null ? "" : `max="${esc(field.max_int)}"`;
    control = `<input id="${esc(id)}" data-f021-field="${esc(field.key)}" type="number" value="${esc(value)}" ${min} ${max} ${required} ${disabledAttr}>`;
  } else {
    const type = field.type === "SECRET" ? "password" : "text";
    control = `<input id="${esc(id)}" data-f021-field="${esc(field.key)}" type="${type}" value="${esc(value)}" autocomplete="off" ${required} ${disabledAttr}>`;
  }
  return `<label>${esc(label)}${control}${help ? `<small class="muted">${esc(help)}</small>` : ""}</label>`;
}

function f021ReadValues(profile, form) {
  const values = {};
  for (const field of profile.connection_fields || []) {
    const input = form.querySelector(`[data-f021-field="${CSS.escape(field.key)}"]`);
    if (!input) continue;
    if (field.type === "BOOL") {
      values[field.key] = input.checked;
    } else if (field.type === "INT") {
      if (input.value === "") {
        if (field.required) throw new Error(f021T("device_profiles.required"));
        continue;
      }
      values[field.key] = Number(input.value);
    } else {
      const value = input.value.trim();
      if (!value && field.required) throw new Error(f021T("device_profiles.required"));
      if (value) values[field.key] = value;
    }
  }
  return values;
}

function f021ProfileSummary(profile) {
  const capabilities = (profile.capabilities || []).map((item) => f021ProfileText(item.name) || item.capability_key).filter(Boolean);
  const protocols = profile.tested_protocol_versions || [];
  return `<div class="f002-detail-grid">
    <div><span>${esc(f021T("device_profiles.capabilities"))}</span><strong>${esc(capabilities.join(", ") || "—")}</strong></div>
    <div><span>${esc(f021T("device_profiles.protocol"))}</span><strong>${esc(protocols.join(", ") || "—")}</strong></div>
  </div>`;
}

async function f021EnhanceConfiguration() {
  const targetForm = el("targetForm");
  if (!targetForm || el("f021ProfileGuide")) return;

  let payload;
  try {
    payload = await api("/api/v1/device-profiles");
  } catch (_) {
    const warning = document.createElement("div");
    warning.className = "message warn";
    warning.textContent = f021T("device_profiles.unavailable");
    targetForm.parentNode.insertBefore(warning, targetForm);
    return;
  }

  const profiles = (payload.profiles || []).filter((profile) => profile.target && (profile.connection_fields || []).length > 0);
  if (!profiles.length) return;

  const editable = !el("targetName")?.disabled;
  const guide = document.createElement("section");
  guide.id = "f021ProfileGuide";
  guide.className = "f002-builder";
  guide.innerHTML = `
    <div class="f002-builder-head">
      <div><strong>${esc(f021T("device_profiles.title"))}</strong><span>${esc(f021T("device_profiles.summary"))}</span></div>
    </div>
    <form id="f021ProfileForm">
      <div class="form-grid two">
        <label>${esc(f021T("device_profiles.profile"))}<select id="f021ProfileSelect" ${editable ? "" : "disabled"}>${profiles.map((profile) => `<option value="${esc(profile.profile_id)}">${esc(f021ProfileText(profile.name) || profile.profile_id)}</option>`).join("")}</select></label>
        <label>${esc(f021T("device_profiles.logical_name"))}<input id="f021LogicalName" placeholder="PROJECTOR-MAIN" required ${editable ? "" : "disabled"}></label>
      </div>
      <p id="f021ProfileDescription" class="muted"></p>
      <div id="f021ProfileFacts"></div>
      <div id="f021Fields" class="form-grid two"></div>
      <p class="muted">${esc(f021T("device_profiles.review_note"))}</p>
      <div class="toolbar">
        <button id="f021Review" class="button" type="button" ${editable ? "" : "disabled"}>${esc(f021T("device_profiles.review"))}</button>
        <button id="f021Save" class="button primary" type="submit" disabled>${esc(f021T("device_profiles.save"))}</button>
      </div>
      <div id="f021Preview" class="message hidden"></div>
    </form>`;

  const advanced = document.createElement("details");
  advanced.className = "f002-advanced";
  advanced.innerHTML = `<summary>${esc(f021T("device_profiles.advanced"))}</summary>`;

  targetForm.parentNode.insertBefore(guide, targetForm);
  targetForm.parentNode.insertBefore(advanced, targetForm);
  advanced.appendChild(targetForm);

  const form = el("f021ProfileForm");
  const selector = el("f021ProfileSelect");
  const fields = el("f021Fields");
  const description = el("f021ProfileDescription");
  const facts = el("f021ProfileFacts");
  const preview = el("f021Preview");
  const save = el("f021Save");
  let reviewed = null;

  function selectedProfile() {
    return profiles.find((profile) => profile.profile_id === selector.value) || profiles[0];
  }

  function invalidateReview(showMessage = false) {
    reviewed = null;
    save.disabled = true;
    if (showMessage && !preview.classList.contains("hidden")) {
      preview.className = "message warn";
      preview.textContent = f021T("device_profiles.preview_changed");
    } else if (!showMessage) {
      preview.className = "message hidden";
      preview.textContent = "";
    }
  }

  function renderProfile() {
    const profile = selectedProfile();
    description.textContent = f021ProfileText(profile.summary);
    facts.innerHTML = f021ProfileSummary(profile);
    fields.innerHTML = (profile.connection_fields || []).map((field) => f021FieldControl(field, !editable)).join("");
    fields.querySelectorAll("input, select").forEach((input) => input.addEventListener("input", () => invalidateReview(true)));
    invalidateReview(false);
  }

  selector.addEventListener("change", renderProfile);
  el("f021LogicalName").addEventListener("input", () => invalidateReview(true));
  renderProfile();

  el("f021Review").addEventListener("click", async () => {
    try {
      const profile = selectedProfile();
      const values = f021ReadValues(profile, form);
      const result = await api(`/api/v1/device-profiles/${encodeURIComponent(profile.profile_id)}/materialize`, { method: "POST", json: { values } });
      reviewed = {
        profileID: profile.profile_id,
        logicalName: el("f021LogicalName").value.trim(),
        values: JSON.stringify(values),
        target: result.target,
      };
      if (!reviewed.logicalName) throw new Error(f021T("device_profiles.required"));
      preview.className = "message good";
      preview.innerHTML = `<strong>${esc(f021T("device_profiles.preview"))}</strong><br>${esc(f021T("device_profiles.preview_ready"))} · ${esc(reviewed.logicalName)} · ${esc(result.target.logical_type)}`;
      save.disabled = !editable;
    } catch (error) {
      reviewed = null;
      save.disabled = true;
      preview.className = "message error";
      preview.textContent = errorMessage(error);
    }
  });

  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    if (!reviewed || !editable) return;
    try {
      const profile = selectedProfile();
      const values = f021ReadValues(profile, form);
      const logicalName = el("f021LogicalName").value.trim();
      if (reviewed.profileID !== profile.profile_id || reviewed.logicalName !== logicalName || reviewed.values !== JSON.stringify(values)) {
        invalidateReview(true);
        return;
      }
      await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/targets`, {
        method: "POST",
        json: {
          logical_name: logicalName,
          logical_type: reviewed.target.logical_type,
          configuration: reviewed.target.configuration,
        },
      });
      await refreshProjectAndConfiguration();
      setMessage(globalMessage, f021T("device_profiles.saved"), "good");
    } catch (error) {
      configurationError(error);
    }
  });
}

renderConfiguration = async function f021RenderConfiguration(...args) {
  await f021BaseRenderConfiguration(...args);
  await f021EnhanceConfiguration();
};
