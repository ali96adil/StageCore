"use strict";

const f018Copy = {
  "timecode.nav": { en: "Timecode", "ar-IQ": "التايم كود" },
  "timecode.eyebrow": { en: "SHOW SYNCHRONIZATION", "ar-IQ": "مزامنة العرض" },
  "timecode.title": { en: "Timecode & Show Sync", "ar-IQ": "التايم كود ومزامنة العرض" },
  "timecode.subtitle": { en: "Monitor the selected timecode source, frame rate, offset, signal health and cue bindings sealed in the published Runtime Snapshot.", "ar-IQ": "مراقبة مصدر التايم كود ومعدل الإطارات والإزاحة وصحة الإشارة وارتباطات الكيو المثبتة داخل Runtime Snapshot المنشور." },
  "timecode.refresh": { en: "Refresh", "ar-IQ": "تحديث" },
  "timecode.source": { en: "Source", "ar-IQ": "المصدر" },
  "timecode.kind": { en: "Kind", "ar-IQ": "النوع" },
  "timecode.rate": { en: "Frame rate", "ar-IQ": "معدل الإطارات" },
  "timecode.drop_frame": { en: "Drop-frame", "ar-IQ": "إسقاط الإطارات" },
  "timecode.non_drop_frame": { en: "Non-drop-frame", "ar-IQ": "بدون إسقاط إطارات" },
  "timecode.offset": { en: "Offset", "ar-IQ": "الإزاحة" },
  "timecode.health": { en: "Source health", "ar-IQ": "حالة المصدر" },
  "timecode.lock": { en: "SHOW lock", "ar-IQ": "قفل SHOW" },
  "timecode.last": { en: "Last timecode", "ar-IQ": "آخر تايم كود" },
  "timecode.frame": { en: "frame", "ar-IQ": "إطار" },
  "timecode.bindings": { en: "Cue bindings", "ar-IQ": "ارتباطات الكيو" },
  "timecode.binding": { en: "Binding", "ar-IQ": "الارتباط" },
  "timecode.cue": { en: "Cue", "ar-IQ": "الكيو" },
  "timecode.target": { en: "Target frame", "ar-IQ": "الإطار الهدف" },
  "timecode.expiry": { en: "Expiry window", "ar-IQ": "نافذة الانتهاء" },
  "timecode.enabled": { en: "Enabled", "ar-IQ": "مفعّل" },
  "timecode.disabled": { en: "Disabled", "ar-IQ": "معطّل" },
  "timecode.yes": { en: "Yes", "ar-IQ": "نعم" },
  "timecode.no": { en: "No", "ar-IQ": "لا" },
  "timecode.none": { en: "No timecode cue bindings are present in the current Snapshot.", "ar-IQ": "لا توجد ارتباطات تايم كود للكيو داخل الـSnapshot الحالي." },
  "timecode.no_snapshot": { en: "No published Runtime Snapshot is available for timecode configuration.", "ar-IQ": "لا يوجد Runtime Snapshot منشور يمكن قراءة إعدادات التايم كود منه." },
  "timecode.safety_title": { en: "Safety behavior", "ar-IQ": "سلوك الأمان" },
  "timecode.safety": { en: "StageCore never auto-fires a timecode cue while the selected source is missing, stale, unstable, jumping or discontinuous. During SHOW the selected source remains locked with no silent fallback, and a bound cue can execute only when it is the next enabled cue.", "ar-IQ": "StageCore لا يشغّل أي كيو تلقائياً من التايم كود إذا كان المصدر مفقوداً أو قديماً أو غير مستقر أو حدثت قفزة أو حالة انقطاع. أثناء SHOW يبقى المصدر المحدد مقفولاً بلا استبدال صامت، والكيو المرتبط لا ينفذ إلا إذا كان هو الكيو المفعّل التالي." },
  "timecode.config_title": { en: "Configuration model", "ar-IQ": "طريقة الإعداد" },
  "timecode.config": { en: "Define the source in Configuration as a TIMECODE_SOURCE target and add cue timing under timecode in each Cue execution policy. Publishing seals both into the immutable Runtime Snapshot.", "ar-IQ": "يُعرّف المصدر داخل Configuration كهدف من نوع TIMECODE_SOURCE، ويضاف توقيت الكيو تحت timecode في Execution policy. عند النشر تصبح القيمتان جزءاً من Runtime Snapshot غير القابل للتغيير." },
  "timecode.published": { en: "Runtime Snapshot", "ar-IQ": "لقطة Runtime Snapshot المنشورة" },
  "timecode.stale": { en: "Stale", "ar-IQ": "قديم أو متوقف" },
  "timecode.missing": { en: "Missing", "ar-IQ": "غير موجود" },
  "timecode.healthy": { en: "Healthy", "ar-IQ": "سليم" },
  "timecode.unstable": { en: "Unstable", "ar-IQ": "غير مستقر" },
  "timecode.jump": { en: "Jump", "ar-IQ": "قفزة" },
  "timecode.discontinuity": { en: "Discontinuity", "ar-IQ": "انقطاع" },
  "timecode.drift": { en: "Drift", "ar-IQ": "انحراف" },
  "timecode.authoring_title": { en: "Draft source setup", "ar-IQ": "إعداد مصدر المسودة" },
  "timecode.authoring_hint": { en: "Configure the single Timecode source for the next publish without raw JSON.", "ar-IQ": "اضبط مصدر التايم كود الوحيد للنشر القادم من دون JSON يدوي." },
  "timecode.target_name": { en: "Target name", "ar-IQ": "اسم الهدف" },
  "timecode.source_id": { en: "Source ID", "ar-IQ": "معرف المصدر" },
  "timecode.start": { en: "Start timecode", "ar-IQ": "تايم كود البداية" },
  "timecode.save_source": { en: "Save source", "ar-IQ": "حفظ المصدر" },
  "timecode.validate": { en: "Validate Draft", "ar-IQ": "فحص المسودة" },
  "timecode.validation": { en: "Draft validation", "ar-IQ": "فحص المسودة" },
  "timecode.validation_hint": { en: "Validation uses the same Hub rules that gate Publish.", "ar-IQ": "الفحص يستخدم نفس قواعد الـHub التي تمنع Publish عند وجود مشكلة." },
  "timecode.multiple_sources": { en: "Multiple TIMECODE_SOURCE targets exist. Resolve them in Routing & Machine Roles before publishing.", "ar-IQ": "يوجد أكثر من TIMECODE_SOURCE. عالجها من Routing & Machine Roles قبل النشر." },
  "timecode.no_source": { en: "No Draft Timecode source yet.", "ar-IQ": "لا يوجد مصدر Timecode في المسودة بعد." },
  "timecode.cue_authoring": { en: "Timecode Cue binding", "ar-IQ": "ربط الكيو بالتايم كود" },
  "timecode.binding_mode": { en: "Binding mode", "ar-IQ": "وضع الربط" },
  "timecode.none_mode": { en: "No binding", "ar-IQ": "بدون ربط" },
  "timecode.binding_id_hint": { en: "Binding ID (optional)", "ar-IQ": "معرف الربط (اختياري)" },
  "timecode.at": { en: "Fire at", "ar-IQ": "التشغيل عند" },
  "timecode.expiry_frames": { en: "Expiry frames", "ar-IQ": "إطارات انتهاء الصلاحية" },
  "timecode.advanced_policy": { en: "Raw execution policy stays under Advanced; these fields update only execution_policy.timecode.", "ar-IQ": "يبقى Execution policy الخام ضمن Advanced؛ هذه الحقول تعدّل execution_policy.timecode فقط." },
  "timecode.source_saved": { en: "Draft Timecode source saved.", "ar-IQ": "تم حفظ مصدر التايم كود للمسودة." },
};

const f018AllRateNames = ["23.976", "24", "25", "29.97", "29.97 DF", "30", "59.94", "59.94 DF", "60"];
const f018MTCRateNames = ["24", "25", "29.97 DF", "30"];

function f018SourceTargets(configuration) {
  return (configuration?.targets || []).filter((target) =>
    String(target.logical_type || "").toUpperCase() === "TIMECODE_SOURCE");
}

function f018SourceConfig(target) {
  const raw = target?.configuration;
  if (!raw || typeof raw !== "object" || Array.isArray(raw)) return {};
  return raw;
}

function f018RateNamesForKind(kind) {
  return String(kind || "").toUpperCase() === "MTC" ? f018MTCRateNames : f018AllRateNames;
}

function f018RateOptions(kind, selected) {
  const values = f018RateNamesForKind(kind);
  const normalized = String(selected || "");
  return values.map((value) => `<option value="${esc(value)}" ${value === normalized ? "selected" : ""}>${esc(value)}</option>`).join("");
}

function f018Locale() {
  if (el("languageSelect")?.value === "en") return "en";
  return localStorage.getItem("stagecore_locale") === "en" ? "en" : "ar-IQ";
}

function f018T(key) {
  const entry = f018Copy[key];
  return entry?.[f018Locale()] || entry?.en || key;
}

function f018UpdateNav() {
  const button = document.querySelector('[data-page="timecode"]');
  if (button) button.textContent = f018T("timecode.nav");
}

function timecodeHealthLabel(value) {
  const key = String(value || "MISSING").toLowerCase();
  return f018T(`timecode.${key}`);
}

function timecodeHealthKind(value) {
  if (value === "HEALTHY") return "good";
  if (["MISSING", "STALE"].includes(value)) return "warn";
  return "bad";
}

function formatTimecodeValue(value, rate) {
  if (!value || typeof value !== "object") return "—";
  const pad = (n) => String(Number(n || 0)).padStart(2, "0");
  const separator = rate?.drop_frame ? ";" : ":";
  return `${pad(value.hours)}:${pad(value.minutes)}:${pad(value.seconds)}${separator}${pad(value.frames)}`;
}

async function renderTimecodeWorkspace() {
  if (!state.project) return;
  setPage("timecode");
  setMessage(globalMessage, "");
  f018UpdateNav();
  const projectID = encodeURIComponent(state.project.project_id);

  const [configurationResult, publishedResult] = await Promise.all([
    api(`/api/v1/projects/${projectID}/configuration`).catch((error) => ({ __error: error })),
    api(`/api/v1/projects/${projectID}/timecode`).catch((error) => ({ __error: error })),
  ]);
  if (configurationResult?.__error) {
    content.innerHTML = `
      <div class="page-head">
        <div><p class="eyebrow">${esc(f018T("timecode.eyebrow"))}</p><h1>${esc(f018T("timecode.title"))}</h1></div>
        <button id="timecodeRefresh" class="button" type="button">${esc(f018T("timecode.refresh"))}</button>
      </div>
      <section class="card"><div class="empty">${esc(errorMessage(configurationResult.__error))}</div></section>`;
    el("timecodeRefresh")?.addEventListener("click", renderTimecodeWorkspace);
    return;
  }

  const configuration = configurationResult || {};
  const targets = f018SourceTargets(configuration);
  const draftTarget = targets.length === 1 ? targets[0] : null;
  const draftConfig = f018SourceConfig(draftTarget);
  const sourceKind = String(draftConfig.kind || "INTERNAL").toUpperCase();
  const sourceRate = String(draftConfig.rate || "30");
  const editable = canEdit() && targets.length <= 1;
  const validation = state.f018TimecodeValidation || null;

  const publishedError = publishedResult?.__error || null;
  const payload = publishedError ? null : publishedResult;
  const summary = payload?.summary || {};
  const cfg = summary.configuration || {};
  const source = cfg.source || {};
  const rate = source.rate || {};
  const health = summary.health || {};
  const sample = summary.last_sample || null;
  const bindings = cfg.bindings || [];
  const healthState = health.state || "MISSING";

  const validationFindings = validation?.findings || [];
  content.innerHTML = `
    <div class="page-head">
      <div><p class="eyebrow">${esc(f018T("timecode.eyebrow"))}</p><h1>${esc(f018T("timecode.title"))}</h1><p>${esc(f018T("timecode.subtitle"))}</p></div>
      <button id="timecodeRefresh" class="button" type="button">${esc(f018T("timecode.refresh"))}</button>
    </div>

    <section class="card">
      <div class="section-title-row">
        <div><p class="eyebrow">DRAFT</p><h2>${esc(f018T("timecode.authoring_title"))}</h2><p class="muted">${esc(f018T("timecode.authoring_hint"))}</p></div>
        ${targets.length === 1 ? pill("1 TIMECODE_SOURCE", "good") : targets.length > 1 ? pill(`${targets.length} TIMECODE_SOURCE`, "bad") : pill(f018T("timecode.no_source"), "neutral")}
      </div>
      ${targets.length > 1 ? `<div class="message error">${esc(f018T("timecode.multiple_sources"))}</div>` : ""}
      <form id="f018SourceForm" data-alias-id="${esc(draftTarget?.alias_id || "")}" style="margin-top:14px">
        <div class="form-grid two">
          <label>${esc(f018T("timecode.target_name"))}<input id="f018TargetName" value="${esc(draftTarget?.logical_name || "TIMECODE")}" ${draftTarget ? "disabled" : ""} ${editable ? "" : "disabled"} required></label>
          <label>${esc(f018T("timecode.source_id"))}<input id="f018SourceID" value="${esc(draftConfig.source_id || draftTarget?.logical_name || "show-clock")}" ${editable ? "" : "disabled"} required></label>
          <label>${esc(f018T("timecode.kind"))}
            <select id="f018SourceKind" ${editable ? "" : "disabled"}>
              ${["INTERNAL", "MTC", "LTC"].map((kind) => `<option value="${kind}" ${kind === sourceKind ? "selected" : ""}>${kind}</option>`).join("")}
            </select>
          </label>
          <label>${esc(f018T("timecode.rate"))}<select id="f018SourceRate" ${editable ? "" : "disabled"}>${f018RateOptions(sourceKind, sourceRate)}</select></label>
          <label>${esc(f018T("timecode.offset"))} (${esc(f018T("timecode.frame"))})<input id="f018OffsetFrames" type="number" step="1" value="${esc(draftConfig.offset_frames ?? 0)}" ${editable ? "" : "disabled"}></label>
          <label>${esc(f018T("timecode.start"))}<input id="f018StartTimecode" class="mono" value="${esc(draftConfig.start_timecode || (sourceRate.includes("DF") ? "00:00:00;00" : "00:00:00:00"))}" ${editable ? "" : "disabled"}></label>
        </div>
        <div class="row-actions">
          <button class="button primary" type="submit" ${editable ? "" : "disabled"}>${esc(f018T("timecode.save_source"))}</button>
          <button id="f018ValidateDraft" class="button" type="button">${esc(f018T("timecode.validate"))}</button>
          <button id="f018OpenCues" class="button ghost" type="button">Cues</button>
        </div>
      </form>
      <div style="margin-top:14px">
        <div class="section-title-row"><div><h3>${esc(f018T("timecode.validation"))}</h3><p class="muted">${esc(f018T("timecode.validation_hint"))}</p></div>${validation ? pill(validation.valid ? "PASS" : "BLOCK", validation.valid ? "good" : "bad") : pill("NOT RUN", "neutral")}</div>
        ${validationFindings.length ? `<ul class="validation-list">${validationFindings.map((finding) => `<li class="validation-item"><strong>${esc(finding.code || finding.severity || "Finding")}</strong>${esc(finding.message || "Validation finding")}</li>`).join("")}</ul>` : validation ? `<p class="muted">No blocking findings.</p>` : ""}
      </div>
    </section>

    ${payload ? `
      <div class="stat-grid" style="margin-top:16px">
        <article class="stat"><span class="label">${esc(f018T("timecode.published"))}</span><span class="value">${esc(summary.runtime_snapshot_id || "—")}</span><span class="sub mono">${esc(cfg.target_ref || "—")}</span></article>
        <article class="stat"><span class="label">${esc(f018T("timecode.source"))}</span><span class="value">${esc(source.source_id || (summary.enabled ? "—" : f018T("timecode.disabled")))}</span><span class="sub">${esc(f018T("timecode.kind"))} · ${esc(source.kind || "—")}</span></article>
        <article class="stat"><span class="label">${esc(f018T("timecode.rate"))}</span><span class="value">${esc(rate.name || "—")}</span><span class="sub">${esc(rate.drop_frame ? f018T("timecode.drop_frame") : f018T("timecode.non_drop_frame"))}</span></article>
        <article class="stat"><span class="label">${esc(f018T("timecode.offset"))}</span><span class="value">${esc(source.offset_frames ?? 0)} ${esc(f018T("timecode.frame"))}</span><span class="sub">${esc(f018T("timecode.lock"))} · ${summary.show_locked ? esc(f018T("timecode.yes")) : esc(f018T("timecode.no"))}</span></article>
        <article class="stat"><span class="label">${esc(f018T("timecode.health"))}</span><span class="value">${pill(timecodeHealthLabel(healthState), timecodeHealthKind(healthState))}</span><span class="sub">${esc(health.detail || "")}</span></article>
        <article class="stat"><span class="label">${esc(f018T("timecode.last"))}</span><span class="value mono">${esc(formatTimecodeValue(sample?.timecode, sample?.rate || rate))}</span><span class="sub">${sample ? `${esc(f018T("timecode.frame"))} ${esc(sample.frame_number)} · ${esc(fmtDate(sample.observed_at))}` : "—"}</span></article>
      </div>

      <section class="card" style="margin-top:16px">
        <div class="section-title-row"><div><h2>${esc(f018T("timecode.safety_title"))}</h2><p class="muted">${esc(f018T("timecode.safety"))}</p></div></div>
      </section>

      <section class="card" style="margin-top:16px">
        <div class="section-title-row"><div><h2>${esc(f018T("timecode.bindings"))}</h2><p class="muted">Published immutable bindings. Edit future Cue timing from the guided Cue editor.</p></div></div>
        ${bindings.length ? `
          <div class="table-wrap" style="margin-top:12px">
            <table>
              <thead><tr><th>${esc(f018T("timecode.binding"))}</th><th>${esc(f018T("timecode.cue"))}</th><th>${esc(f018T("timecode.target"))}</th><th>${esc(f018T("timecode.expiry"))}</th><th>${esc(f018T("timecode.enabled"))}</th></tr></thead>
              <tbody>${bindings.map((binding) => `
                <tr>
                  <td class="mono">${esc(binding.binding_id)}</td>
                  <td class="mono">${esc(binding.cue_id)}</td>
                  <td>${esc(binding.target_frame)}</td>
                  <td>${esc(binding.expiry_frames)} ${esc(f018T("timecode.frame"))}</td>
                  <td>${pill(binding.enabled ? f018T("timecode.enabled") : f018T("timecode.disabled"), binding.enabled ? "good" : "neutral")}</td>
                </tr>`).join("")}</tbody>
            </table>
          </div>` : `<div class="empty" style="margin-top:12px">${esc(f018T("timecode.none"))}</div>`}
      </section>` : `
      <section class="card" style="margin-top:16px"><div class="empty">${esc(publishedError?.status === 404 ? f018T("timecode.no_snapshot") : errorMessage(publishedError))}</div></section>`}
  `;

  const kind = el("f018SourceKind");
  const rateSelect = el("f018SourceRate");
  const start = el("f018StartTimecode");
  const syncRateChoices = () => {
    const previous = rateSelect?.value || "30";
    const options = f018RateNamesForKind(kind?.value);
    const selected = options.includes(previous) ? previous : options[0];
    if (rateSelect) {
      rateSelect.innerHTML = options.map((value) => `<option value="${esc(value)}">${esc(value)}</option>`).join("");
      rateSelect.value = selected;
    }
    if (start && (!start.value || start.value === "00:00:00:00" || start.value === "00:00:00;00")) {
      start.value = selected.includes("DF") ? "00:00:00;00" : "00:00:00:00";
    }
  };
  kind?.addEventListener("change", syncRateChoices);
  rateSelect?.addEventListener("change", () => {
    if (start && (!start.value || start.value === "00:00:00:00" || start.value === "00:00:00;00")) {
      start.value = rateSelect.value.includes("DF") ? "00:00:00;00" : "00:00:00:00";
    }
  });

  el("f018SourceForm")?.addEventListener("submit", async (event) => {
    event.preventDefault();
    const targetName = el("f018TargetName").value.trim();
    const sourceID = el("f018SourceID").value.trim();
    const offsetFrames = Number.parseInt(el("f018OffsetFrames").value || "0", 10);
    if (!targetName || !sourceID || !Number.isSafeInteger(offsetFrames)) {
      setMessage(globalMessage, "Target name, source ID and integer offset are required.", "error");
      return;
    }
    const sourceConfiguration = {
      source_id: sourceID,
      kind: el("f018SourceKind").value,
      rate: el("f018SourceRate").value,
      offset_frames: offsetFrames,
      start_timecode: el("f018StartTimecode").value.trim(),
    };
    try {
      const aliasID = el("f018SourceForm").dataset.aliasId || "";
      if (aliasID) {
        await api(`/api/v1/projects/${projectID}/targets/${encodeURIComponent(aliasID)}/configuration`, {
          method: "PUT",
          json: { configuration: sourceConfiguration },
        });
      } else {
        await api(`/api/v1/projects/${projectID}/targets`, {
          method: "POST",
          json: {
            logical_name: targetName,
            logical_type: "TIMECODE_SOURCE",
            configuration: sourceConfiguration,
          },
        });
      }
      state.f018TimecodeValidation = await api(`/api/v1/projects/${projectID}/validation`).catch(() => null);
      setMessage(globalMessage, f018T("timecode.source_saved"), "success");
      await renderTimecodeWorkspace();
    } catch (error) {
      setMessage(globalMessage, errorMessage(error), "error");
    }
  });

  el("f018ValidateDraft")?.addEventListener("click", async () => {
    try {
      state.f018TimecodeValidation = await api(`/api/v1/projects/${projectID}/validation`);
      await renderTimecodeWorkspace();
    } catch (error) {
      setMessage(globalMessage, errorMessage(error), "error");
    }
  });
  el("f018OpenCues")?.addEventListener("click", () => navigate("cues"));
  el("timecodeRefresh")?.addEventListener("click", renderTimecodeWorkspace);
}

function f018CuePolicyObject(cue) {
  const raw = cue?.execution_policy;
  if (!raw) return {};
  if (typeof raw === "object" && !Array.isArray(raw)) {
    return JSON.parse(JSON.stringify(raw));
  }
  try {
    const parsed = JSON.parse(String(raw));
    return parsed && typeof parsed === "object" && !Array.isArray(parsed) ? parsed : {};
  } catch (_) {
    return {};
  }
}

function f018EnsureCueAuthoring() {
  if (el("f018CueTimecode")) return;
  const rawPolicy = el("cueExecutionPolicy");
  const rawLabel = rawPolicy?.closest("label");
  if (!rawLabel?.parentNode) return;

  const panel = document.createElement("section");
  panel.id = "f018CueTimecode";
  panel.className = "card";
  panel.style.margin = "12px 0";
  panel.innerHTML = `
    <div class="section-title-row">
      <div><p class="eyebrow">TIMECODE</p><h3>${esc(f018T("timecode.cue_authoring"))}</h3><p class="muted">${esc(f018T("timecode.advanced_policy"))}</p></div>
    </div>
    <div class="form-grid two">
      <label>${esc(f018T("timecode.binding_mode"))}
        <select id="f018CueTimecodeMode">
          <option value="NONE">${esc(f018T("timecode.none_mode"))}</option>
          <option value="ENABLED">${esc(f018T("timecode.enabled"))}</option>
          <option value="DISABLED">${esc(f018T("timecode.disabled"))}</option>
        </select>
      </label>
      <label>${esc(f018T("timecode.binding_id_hint"))}<input id="f018CueBindingID" placeholder="Defaults to cue:<cue id>"></label>
      <label>${esc(f018T("timecode.at"))}<input id="f018CueAt" class="mono" placeholder="00:01:12:10"></label>
      <label>${esc(f018T("timecode.expiry_frames"))}<input id="f018CueExpiry" type="number" min="0" step="1" value="0"></label>
    </div>
    <p class="muted">Use HH:MM:SS:FF for non-drop and HH:MM:SS;FF for drop-frame. The Hub validates the configured source rate before Publish.</p>
  `;
  rawLabel.parentNode.insertBefore(panel, rawLabel);
  el("f018CueTimecodeMode")?.addEventListener("change", f018SyncCueBindingEnabled);
}

function f018SyncCueBindingEnabled() {
  const enabled = el("f018CueTimecodeMode")?.value !== "NONE";
  for (const id of ["f018CueBindingID", "f018CueAt", "f018CueExpiry"]) {
    const control = el(id);
    if (control) control.disabled = !enabled;
  }
}

function f018PopulateCueTimecode(cue) {
  f018EnsureCueAuthoring();
  const policy = f018CuePolicyObject(cue);
  const binding = policy.timecode && typeof policy.timecode === "object" ? policy.timecode : null;
  el("f018CueTimecodeMode").value = !binding ? "NONE" : binding.enabled === false ? "DISABLED" : "ENABLED";
  el("f018CueBindingID").value = binding?.binding_id || "";
  el("f018CueAt").value = binding?.at || "";
  el("f018CueExpiry").value = Number.isSafeInteger(binding?.expiry_frames) ? binding.expiry_frames : 0;
  f018SyncCueBindingEnabled();
}

function f018SyncCueTimecodePolicy(event) {
  try {
    f018EnsureCueAuthoring();
    const mode = el("f018CueTimecodeMode")?.value || "NONE";
    const policy = parseJSONField(el("cueExecutionPolicy").value, "Cue execution policy");
    if (!policy || typeof policy !== "object" || Array.isArray(policy)) {
      throw new Error("Cue execution policy must be a JSON object.");
    }
    if (mode === "NONE") {
      delete policy.timecode;
    } else {
      const at = el("f018CueAt").value.trim();
      const expiryFrames = Number.parseInt(el("f018CueExpiry").value || "0", 10);
      const bindingID = el("f018CueBindingID").value.trim();
      if (!/^\d{2}:\d{2}:\d{2}[:;]\d{2}$/.test(at)) {
        throw new Error("Timecode Cue binding must use HH:MM:SS:FF or HH:MM:SS;FF.");
      }
      if (!Number.isSafeInteger(expiryFrames) || expiryFrames < 0) {
        throw new Error("Timecode expiry frames must be a non-negative integer.");
      }
      policy.timecode = {
        ...(bindingID ? { binding_id: bindingID } : {}),
        at,
        expiry_frames: expiryFrames,
        enabled: mode === "ENABLED",
      };
    }
    el("cueExecutionPolicy").value = JSON.stringify(policy, null, 2);
  } catch (error) {
    event.preventDefault();
    event.stopImmediatePropagation();
    setMessage(globalMessage, error.message || errorMessage(error), "error");
  }
}

const f018BaseOpenCueEditor = openCueEditor;
openCueEditor = function f018OpenCueEditor(cue) {
  f018BaseOpenCueEditor(cue);
  f018PopulateCueTimecode(cue);
};

f018EnsureCueAuthoring();
cueForm.addEventListener("submit", f018SyncCueTimecodePolicy, true);

const timecodeNav = document.querySelector('[data-page="timecode"]');
timecodeNav?.addEventListener("click", (event) => {
  event.preventDefault();
  renderTimecodeWorkspace().catch((error) => setMessage(globalMessage, errorMessage(error), "error"));
});
f018UpdateNav();

el("languageSelect")?.addEventListener("change", () => {
  f018UpdateNav();
  if (state.page === "timecode") {
    renderTimecodeWorkspace().catch((error) => setMessage(globalMessage, errorMessage(error), "error"));
  }
});
