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
  "timecode.published": { en: "Runtime Snapshot", "ar-IQ": "Runtime Snapshot" },
  "timecode.stale": { en: "Stale", "ar-IQ": "قديم أو متوقف" },
  "timecode.missing": { en: "Missing", "ar-IQ": "غير موجود" },
  "timecode.healthy": { en: "Healthy", "ar-IQ": "سليم" },
  "timecode.unstable": { en: "Unstable", "ar-IQ": "غير مستقر" },
  "timecode.jump": { en: "Jump", "ar-IQ": "قفزة" },
  "timecode.discontinuity": { en: "Discontinuity", "ar-IQ": "انقطاع" },
  "timecode.drift": { en: "Drift", "ar-IQ": "انحراف" },
};

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

  let payload;
  try {
    payload = await api(`/api/v1/projects/${projectID}/timecode`);
  } catch (error) {
    content.innerHTML = `
      <div class="page-head">
        <div><p class="eyebrow">${esc(f018T("timecode.eyebrow"))}</p><h1>${esc(f018T("timecode.title"))}</h1><p>${esc(f018T("timecode.subtitle"))}</p></div>
        <button id="timecodeRefresh" class="button" type="button">${esc(f018T("timecode.refresh"))}</button>
      </div>
      <section class="card"><div class="empty">${esc(error.status === 404 ? f018T("timecode.no_snapshot") : errorMessage(error))}</div></section>`;
    el("timecodeRefresh")?.addEventListener("click", renderTimecodeWorkspace);
    return;
  }

  const summary = payload.summary || {};
  const cfg = summary.configuration || {};
  const source = cfg.source || {};
  const rate = source.rate || {};
  const health = summary.health || {};
  const sample = summary.last_sample || null;
  const bindings = cfg.bindings || [];
  const healthState = health.state || "MISSING";

  content.innerHTML = `
    <div class="page-head">
      <div><p class="eyebrow">${esc(f018T("timecode.eyebrow"))}</p><h1>${esc(f018T("timecode.title"))}</h1><p>${esc(f018T("timecode.subtitle"))}</p></div>
      <button id="timecodeRefresh" class="button" type="button">${esc(f018T("timecode.refresh"))}</button>
    </div>

    <div class="stat-grid">
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
      <div class="section-title-row"><div><h2>${esc(f018T("timecode.bindings"))}</h2><p class="muted">${esc(f018T("timecode.config"))}</p></div></div>
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
    </section>

    <section class="card" style="margin-top:16px">
      <h2>${esc(f018T("timecode.config_title"))}</h2>
      <p class="muted">${esc(f018T("timecode.config"))}</p>
      <pre class="mono" style="white-space:pre-wrap">TIMECODE_SOURCE
{
  "source_id": "show-clock",
  "kind": "INTERNAL | MTC | LTC",
  "rate": "29.97 DF",
  "offset_frames": 0,
  "start_timecode": "00:00:00;00"
}

execution_policy.timecode
{
  "binding_id": "scene-1-go",
  "at": "00:01:12;10",
  "expiry_frames": 2,
  "enabled": true
}</pre>
    </section>`;

  el("timecodeRefresh")?.addEventListener("click", renderTimecodeWorkspace);
}

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
