"use strict";

const timecodeCopy = {
  ar: {
    eyebrow: "مزامنة العرض",
    title: "التايم كود ومزامنة العرض",
    subtitle: "مراقبة مصدر التايم كود، معدل الإطارات، الإزاحة، صحة الإشارة وارتباطات الكيو من الـRuntime Snapshot المنشور.",
    refresh: "تحديث",
    source: "المصدر",
    kind: "النوع",
    rate: "معدل الإطارات",
    offset: "الإزاحة",
    health: "حالة المصدر",
    lock: "قفل SHOW",
    last: "آخر تايم كود",
    frame: "الإطار",
    bindings: "ارتباطات الكيو",
    binding: "الارتباط",
    cue: "الكيو",
    target: "الإطار الهدف",
    expiry: "نافذة الانتهاء",
    enabled: "مفعّل",
    disabled: "معطّل",
    yes: "نعم",
    no: "لا",
    none: "لا توجد ارتباطات تايم كود في الـSnapshot الحالي.",
    noSnapshot: "لا يوجد Runtime Snapshot منشور يمكن قراءة إعدادات التايم كود منه.",
    safetyTitle: "سلوك الأمان",
    safety: "StageCore لا يشغّل أي كيو تلقائياً من التايم كود إذا كان المصدر مفقوداً أو قديماً أو غير مستقر أو حدثت قفزة/انقطاع. أثناء SHOW يبقى المصدر المحدد مقفولاً ولا يتم استبداله بصمت، والكيو المرتبط لا ينفذ إلا إذا كان هو الكيو المفعّل التالي.",
    configTitle: "طريقة الإعداد",
    config: "المصدر يُعرّف ضمن Configuration كـTarget من نوع TIMECODE_SOURCE، ووقت تشغيل الكيو يضاف داخل Execution policy للكيو تحت timecode. عند النشر تصبح هذه القيم جزءاً من الـRuntime Snapshot غير القابل للتغيير.",
    published: "Runtime Snapshot",
    stale: "قديم/متوقف",
    missing: "غير موجود",
    healthy: "سليم",
    unstable: "غير مستقر",
    jump: "قفزة",
    discontinuity: "انقطاع",
    drift: "انحراف",
  },
  en: {
    eyebrow: "SHOW SYNCHRONIZATION",
    title: "Timecode & Show Sync",
    subtitle: "Monitor the selected timecode source, frame rate, offset, signal health and cue bindings sealed in the published Runtime Snapshot.",
    refresh: "Refresh",
    source: "Source",
    kind: "Kind",
    rate: "Frame rate",
    offset: "Offset",
    health: "Source health",
    lock: "SHOW lock",
    last: "Last timecode",
    frame: "Frame",
    bindings: "Cue bindings",
    binding: "Binding",
    cue: "Cue",
    target: "Target frame",
    expiry: "Expiry window",
    enabled: "Enabled",
    disabled: "Disabled",
    yes: "Yes",
    no: "No",
    none: "No timecode cue bindings are present in the current Snapshot.",
    noSnapshot: "No published Runtime Snapshot is available for timecode configuration.",
    safetyTitle: "Safety behavior",
    safety: "StageCore never auto-fires a timecode cue while the selected source is missing, stale, unstable, jumping or discontinuous. During SHOW the selected source remains locked with no silent fallback, and a bound cue can execute only when it is the next enabled cue.",
    configTitle: "Configuration model",
    config: "Define the source in Configuration as a TIMECODE_SOURCE target and add cue timing under timecode in each Cue execution policy. Publishing seals both into the immutable Runtime Snapshot.",
    published: "Runtime Snapshot",
    stale: "Stale",
    missing: "Missing",
    healthy: "Healthy",
    unstable: "Unstable",
    jump: "Jump",
    discontinuity: "Discontinuity",
    drift: "Drift",
  },
};

function timecodeLanguage() {
  return el("languageSelect")?.value === "en" ? "en" : "ar";
}

function timecodeHealthLabel(value, copy) {
  const key = String(value || "MISSING").toLowerCase();
  return copy[key] || value || copy.missing;
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
  const lang = timecodeLanguage();
  const copy = timecodeCopy[lang];
  const projectID = encodeURIComponent(state.project.project_id);

  let payload;
  try {
    payload = await api(`/api/v1/projects/${projectID}/timecode`);
  } catch (error) {
    content.innerHTML = `
      <div class="page-head">
        <div><p class="eyebrow">${esc(copy.eyebrow)}</p><h1>${esc(copy.title)}</h1><p>${esc(copy.subtitle)}</p></div>
        <button id="timecodeRefresh" class="button" type="button">${esc(copy.refresh)}</button>
      </div>
      <section class="card"><div class="empty">${esc(error.status === 404 ? copy.noSnapshot : errorMessage(error))}</div></section>`;
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
      <div><p class="eyebrow">${esc(copy.eyebrow)}</p><h1>${esc(copy.title)}</h1><p>${esc(copy.subtitle)}</p></div>
      <button id="timecodeRefresh" class="button" type="button">${esc(copy.refresh)}</button>
    </div>

    <div class="stat-grid">
      <article class="stat"><span class="label">${esc(copy.published)}</span><span class="value">${esc(summary.runtime_snapshot_id || "—")}</span><span class="sub mono">${esc(cfg.target_ref || "—")}</span></article>
      <article class="stat"><span class="label">${esc(copy.source)}</span><span class="value">${esc(source.source_id || (summary.enabled ? "—" : copy.disabled))}</span><span class="sub">${esc(copy.kind)} · ${esc(source.kind || "—")}</span></article>
      <article class="stat"><span class="label">${esc(copy.rate)}</span><span class="value">${esc(rate.name || "—")}</span><span class="sub">${rate.drop_frame ? "Drop-frame" : "Non-drop-frame"}</span></article>
      <article class="stat"><span class="label">${esc(copy.offset)}</span><span class="value">${esc(source.offset_frames ?? 0)} ${esc(copy.frame)}</span><span class="sub">${esc(copy.lock)} · ${summary.show_locked ? esc(copy.yes) : esc(copy.no)}</span></article>
      <article class="stat"><span class="label">${esc(copy.health)}</span><span class="value">${pill(timecodeHealthLabel(healthState, copy), timecodeHealthKind(healthState))}</span><span class="sub">${esc(health.detail || "")}</span></article>
      <article class="stat"><span class="label">${esc(copy.last)}</span><span class="value mono">${esc(formatTimecodeValue(sample?.timecode, sample?.rate || rate))}</span><span class="sub">${sample ? `${esc(copy.frame)} ${esc(sample.frame_number)} · ${esc(fmtDate(sample.observed_at))}` : "—"}</span></article>
    </div>

    <section class="card" style="margin-top:16px">
      <div class="section-title-row"><div><h2>${esc(copy.safetyTitle)}</h2><p class="muted">${esc(copy.safety)}</p></div></div>
    </section>

    <section class="card" style="margin-top:16px">
      <div class="section-title-row"><div><h2>${esc(copy.bindings)}</h2><p class="muted">${esc(copy.config)}</p></div></div>
      ${bindings.length ? `
        <div class="table-wrap" style="margin-top:12px">
          <table>
            <thead><tr><th>${esc(copy.binding)}</th><th>${esc(copy.cue)}</th><th>${esc(copy.target)}</th><th>${esc(copy.expiry)}</th><th>${esc(copy.enabled)}</th></tr></thead>
            <tbody>${bindings.map((binding) => `
              <tr>
                <td class="mono">${esc(binding.binding_id)}</td>
                <td class="mono">${esc(binding.cue_id)}</td>
                <td>${esc(binding.target_frame)}</td>
                <td>${esc(binding.expiry_frames)} ${esc(copy.frame)}</td>
                <td>${pill(binding.enabled ? copy.enabled : copy.disabled, binding.enabled ? "good" : "neutral")}</td>
              </tr>`).join("")}</tbody>
          </table>
        </div>` : `<div class="empty" style="margin-top:12px">${esc(copy.none)}</div>`}
    </section>

    <section class="card" style="margin-top:16px">
      <h2>${esc(copy.configTitle)}</h2>
      <p class="muted">${esc(copy.config)}</p>
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

el("languageSelect")?.addEventListener("change", () => {
  if (state.page === "timecode") {
    renderTimecodeWorkspace().catch((error) => setMessage(globalMessage, errorMessage(error), "error"));
  }
});
