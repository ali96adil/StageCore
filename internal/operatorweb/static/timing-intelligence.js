"use strict";

const f028Copy = {
  "timing.nav": { en: "Timing", "ar-IQ": "التوقيت" },
  "timing.eyebrow": { en: "TIMING INTELLIGENCE", "ar-IQ": "ذكاء التوقيت" },
  "timing.title": { en: "Expected Next Cue", "ar-IQ": "توقع الكيو التالي" },
  "timing.subtitle": { en: "Learn from trusted rehearsal timing and compare the live Session with observed show pace. Predictions are advisory only and never trigger GO.", "ar-IQ": "التعلّم من توقيتات البروفات الموثوقة ومقارنة الجلسة الحالية بإيقاع العرض المرصود. التوقعات استشارية فقط ولا تشغّل GO أبداً." },
  "timing.refresh": { en: "Refresh", "ar-IQ": "تحديث" },
  "timing.advisory": { en: "Advisory only — never triggers GO", "ar-IQ": "استشاري فقط — لا يشغّل GO أبداً" },
  "timing.no_snapshot": { en: "No published Runtime Snapshot is available for timing analysis.", "ar-IQ": "لا توجد لقطة Runtime Snapshot منشورة متاحة لتحليل التوقيت." },
  "timing.next": { en: "Expected next Cue", "ar-IQ": "الكيو التالي المتوقع" },
  "timing.current": { en: "Current Cue", "ar-IQ": "الكيو الحالي" },
  "timing.expected": { en: "Expected at", "ar-IQ": "الوقت المتوقع" },
  "timing.window": { en: "Typical window", "ar-IQ": "النافذة المعتادة" },
  "timing.pace": { en: "Live pace", "ar-IQ": "إيقاع العرض الحالي" },
  "timing.confidence": { en: "Confidence", "ar-IQ": "الثقة" },
  "timing.reason": { en: "Why", "ar-IQ": "السبب" },
  "timing.early": { en: "Early", "ar-IQ": "مبكر" },
  "timing.normal": { en: "Normal", "ar-IQ": "طبيعي" },
  "timing.late": { en: "Late", "ar-IQ": "متأخر" },
  "timing.diverged": { en: "Diverged", "ar-IQ": "منحرف عن المسار" },
  "timing.unknown": { en: "Unknown", "ar-IQ": "غير معروف" },
  "timing.withheld": { en: "Withheld", "ar-IQ": "محجوبة" },
  "timing.low": { en: "Low", "ar-IQ": "منخفضة" },
  "timing.medium": { en: "Medium", "ar-IQ": "متوسطة" },
  "timing.high": { en: "High", "ar-IQ": "عالية" },
  "timing.no_live": { en: "No live Session has enough state for an Expected Next Cue projection.", "ar-IQ": "لا توجد جلسة حالية بحالة كافية لإظهار توقع الكيو التالي." },
  "timing.notes": { en: "Context notes", "ar-IQ": "ملاحظات سياقية" },
  "timing.notes_due": { en: "Notes are due now", "ar-IQ": "حان وقت إظهار الملاحظات" },
  "timing.notes_not_due": { en: "Notes will appear near the expected Cue time", "ar-IQ": "ستظهر الملاحظات قرب الوقت المتوقع للكيو" },
  "timing.lead": { en: "Note lead time", "ar-IQ": "وقت تقديم الملاحظات" },
  "timing.seconds": { en: "seconds", "ar-IQ": "ثانية" },
  "timing.rehearsals": { en: "Trusted rehearsals", "ar-IQ": "البروفات الموثوقة" },
  "timing.rehearsals_detail": { en: "AUTO uses completed or intentionally stopped rehearsals on the same immutable Snapshot content. INCLUDE/EXCLUDE changes only training selection; history remains intact.", "ar-IQ": "يستخدم AUTO البروفات المكتملة أو المتوقفة قصداً على نفس محتوى اللقطة غير القابل للتغيير. INCLUDE/EXCLUDE يغيّر اختيار التدريب فقط ويبقى السجل محفوظاً." },
  "timing.session": { en: "Rehearsal", "ar-IQ": "البروفة" },
  "timing.lifecycle": { en: "Lifecycle", "ar-IQ": "حالة الجلسة" },
  "timing.observations": { en: "Observations", "ar-IQ": "المشاهدات" },
  "timing.snapshot_match": { en: "Snapshot match", "ar-IQ": "تطابق اللقطة" },
  "timing.training": { en: "Training", "ar-IQ": "التدريب" },
  "timing.selection": { en: "Selection", "ar-IQ": "الاختيار" },
  "timing.auto": { en: "Auto", "ar-IQ": "تلقائي" },
  "timing.include": { en: "Include", "ar-IQ": "تضمين" },
  "timing.exclude": { en: "Exclude", "ar-IQ": "استبعاد" },
  "timing.used": { en: "Used", "ar-IQ": "مستخدمة" },
  "timing.not_used": { en: "Not used", "ar-IQ": "غير مستخدمة" },
  "timing.yes": { en: "Yes", "ar-IQ": "نعم" },
  "timing.no": { en: "No", "ar-IQ": "لا" },
  "timing.read_only": { en: "Selection is read-only for this role", "ar-IQ": "اختيار التدريب للقراءة فقط لهذا الدور" },
  "timing.transitions": { en: "Cue-to-Cue timing", "ar-IQ": "توقيت الانتقال بين الكيوات" },
  "timing.from": { en: "From", "ar-IQ": "من" },
  "timing.to": { en: "To", "ar-IQ": "إلى" },
  "timing.median": { en: "Median", "ar-IQ": "الوسيط" },
  "timing.range": { en: "Typical range", "ar-IQ": "المدى المعتاد" },
  "timing.samples": { en: "Trusted rehearsals", "ar-IQ": "البروفات الموثوقة" },
  "timing.no_transitions": { en: "No trusted sequential timing transitions are available for this Snapshot yet.", "ar-IQ": "لا توجد انتقالات توقيت متسلسلة موثوقة لهذه اللقطة حتى الآن." },
  "timing.section": { en: "Section timing", "ar-IQ": "توقيت المقطع" },
  "timing.section_detail": { en: "Measure a contiguous Cue section from real rehearsal paths. StageCore does not add unrelated transition medians together.", "ar-IQ": "قياس مقطع متصل من الكيوات اعتماداً على مسارات البروفة الفعلية. لا يجمع StageCore أوساط انتقالات منفصلة وغير مترابطة." },
  "timing.section_from": { en: "Section start", "ar-IQ": "بداية المقطع" },
  "timing.section_to": { en: "Section end", "ar-IQ": "نهاية المقطع" },
  "timing.calculate": { en: "Calculate section", "ar-IQ": "حساب المقطع" },
  "timing.clear": { en: "Clear", "ar-IQ": "مسح" },
  "timing.section_withheld": { en: "Not enough contiguous trusted rehearsal evidence for this section.", "ar-IQ": "لا توجد أدلة كافية من بروفات موثوقة ومتواصلة لهذا المقطع." },
  "timing.saved": { en: "Training selection updated.", "ar-IQ": "تم تحديث اختيار بيانات التدريب." },
  "timing.partial_truth": { en: "Partial rehearsals contribute only intervals that were actually observed; missing intervals are never invented.", "ar-IQ": "تساهم البروفات الجزئية فقط بالفترات التي تمت مشاهدتها فعلياً، ولا يتم اختراع الفترات المفقودة أبداً." },
};

function f028Locale() {
  if (el("languageSelect")?.value === "en") return "en";
  return localStorage.getItem("stagecore_locale") === "en" ? "en" : "ar-IQ";
}

function f028T(key) {
  const entry = f028Copy[key];
  return entry?.[f028Locale()] || entry?.en || key;
}

function f028UpdateNav() {
  const button = document.querySelector('[data-page="timing"]');
  if (button) button.textContent = f028T("timing.nav");
}

function f028CanEditSelection() {
  return ["OWNER", "TECHNICIAN"].includes(state.user?.role);
}

function f028ConfidenceLabel(value) {
  return f028T(`timing.${String(value || "WITHHELD").toLowerCase()}`);
}

function f028ConfidenceKind(value) {
  if (value === "HIGH") return "good";
  if (value === "MEDIUM") return "good";
  if (value === "LOW") return "warn";
  return "neutral";
}

function f028PaceLabel(value) {
  return f028T(`timing.${String(value || "UNKNOWN").toLowerCase()}`);
}

function f028PaceKind(value) {
  if (value === "NORMAL") return "good";
  if (["EARLY", "LATE"].includes(value)) return "warn";
  if (value === "DIVERGED") return "bad";
  return "neutral";
}

function f028Duration(us) {
  const value = Number(us || 0);
  if (!Number.isFinite(value) || value <= 0) return "—";
  const seconds = value / 1000000;
  if (seconds < 60) return `${seconds.toFixed(seconds < 10 ? 1 : 0)} s`;
  const minutes = Math.floor(seconds / 60);
  const remainder = Math.round(seconds % 60);
  return `${minutes}m ${String(remainder).padStart(2, "0")}s`;
}

function f028CueLabel(cue) {
  if (!cue) return "—";
  return `${cue.display_label ? `${cue.display_label} · ` : ""}${cue.name || cue.cue_id}`;
}

function f028LeadTime() {
  const stored = Number(localStorage.getItem("stagecore_timing_note_lead_seconds") || "30");
  if (!Number.isFinite(stored)) return 30;
  return Math.max(0, Math.min(3600, Math.round(stored)));
}

async function f028LoadReport(sectionFrom = "", sectionTo = "") {
  const params = new URLSearchParams();
  params.set("lead_time_seconds", String(f028LeadTime()));
  if (sectionFrom && sectionTo) {
    params.set("section_from_cue_id", sectionFrom);
    params.set("section_to_cue_id", sectionTo);
  }
  const projectID = encodeURIComponent(state.project.project_id);
  const payload = await api(`/api/v1/projects/${projectID}/timing-intelligence?${params}`);
  return payload.report;
}

function f028ProjectionCard(projection) {
  if (!projection || !projection.next_cue) {
    return `<section class="card"><div class="empty">${esc(f028T("timing.no_live"))}</div></section>`;
  }
  const notes = projection.context_notes || [];
  return `
    <section class="card">
      <div class="section-title-row">
        <div><p class="eyebrow">${esc(f028T("timing.next"))}</p><h2>${esc(f028CueLabel(projection.next_cue))}</h2></div>
        <div class="toolbar">${pill(f028PaceLabel(projection.pace), f028PaceKind(projection.pace))}${pill(f028ConfidenceLabel(projection.confidence), f028ConfidenceKind(projection.confidence))}</div>
      </div>
      <div class="stat-grid" style="margin-top:14px">
        <article class="stat"><span class="label">${esc(f028T("timing.current"))}</span><span class="value">${esc(f028CueLabel(projection.current_cue))}</span><span class="sub">${projection.current_cue_at ? esc(fmtDate(projection.current_cue_at)) : "—"}</span></article>
        <article class="stat"><span class="label">${esc(f028T("timing.expected"))}</span><span class="value">${projection.expected_at ? esc(fmtDate(projection.expected_at)) : "—"}</span><span class="sub">${projection.transition ? esc(f028Duration(projection.transition.median_us)) : "—"}</span></article>
        <article class="stat"><span class="label">${esc(f028T("timing.window"))}</span><span class="value">${projection.window_start_at && projection.window_end_at ? `${esc(fmtDate(projection.window_start_at))} → ${esc(fmtDate(projection.window_end_at))}` : "—"}</span><span class="sub">${projection.transition ? `${esc(f028Duration(projection.transition.lower_us))} → ${esc(f028Duration(projection.transition.upper_us))}` : "—"}</span></article>
        <article class="stat"><span class="label">${esc(f028T("timing.confidence"))}</span><span class="value">${pill(f028ConfidenceLabel(projection.confidence), f028ConfidenceKind(projection.confidence))}</span><span class="sub">${esc(projection.reason || "")}</span></article>
      </div>
      <div style="margin-top:14px">
        <div class="section-title-row"><div><h3>${esc(f028T("timing.notes"))}</h3><p class="muted">${esc(projection.context_notes_due ? f028T("timing.notes_due") : f028T("timing.notes_not_due"))}</p></div></div>
        ${projection.context_notes_due ? (notes.length ? `<div class="grid cards" style="margin-top:10px">${notes.map((note) => `<article class="action-editor"><p class="eyebrow">${esc(note.category || "NOTE")}</p><strong>${esc(note.body)}</strong></article>`).join("")}</div>` : `<div class="empty">—</div>`) : ""}
      </div>
    </section>`;
}

function f028TransitionTable(transitions) {
  if (!transitions.length) return `<div class="empty">${esc(f028T("timing.no_transitions"))}</div>`;
  return `<div class="table-wrap"><table>
    <thead><tr><th>${esc(f028T("timing.from"))}</th><th>${esc(f028T("timing.to"))}</th><th>${esc(f028T("timing.median"))}</th><th>${esc(f028T("timing.range"))}</th><th>${esc(f028T("timing.samples"))}</th><th>${esc(f028T("timing.confidence"))}</th></tr></thead>
    <tbody>${transitions.map((item) => `<tr>
      <td>${esc(f028CueLabel(item.from))}</td>
      <td>${esc(f028CueLabel(item.to))}</td>
      <td>${esc(f028Duration(item.statistics.median_us))}</td>
      <td>${esc(f028Duration(item.statistics.lower_us))} → ${esc(f028Duration(item.statistics.upper_us))}</td>
      <td>${esc(item.statistics.trusted_session_count)}</td>
      <td>${pill(f028ConfidenceLabel(item.statistics.confidence), f028ConfidenceKind(item.statistics.confidence))}</td>
    </tr>`).join("")}</tbody>
  </table></div>`;
}

function f028SessionTable(sessions) {
  if (!sessions.length) return `<div class="empty">—</div>`;
  const editable = f028CanEditSelection();
  return `<div class="table-wrap"><table>
    <thead><tr><th>${esc(f028T("timing.session"))}</th><th>${esc(f028T("timing.lifecycle"))}</th><th>${esc(f028T("timing.observations"))}</th><th>${esc(f028T("timing.snapshot_match"))}</th><th>${esc(f028T("timing.training"))}</th><th>${esc(f028T("timing.selection"))}</th></tr></thead>
    <tbody>${sessions.map((session) => `<tr data-timing-session="${esc(session.session_id)}">
      <td><strong>${esc(session.name || session.session_id)}</strong><div class="mono muted">${esc(session.session_id)}</div>${session.reason ? `<div class="muted">${esc(session.reason)}</div>` : ""}</td>
      <td>${pill(session.lifecycle_state, session.eligible ? "good" : "neutral")}</td>
      <td>${esc(session.observation_count)}</td>
      <td>${pill(session.snapshot_match ? f028T("timing.yes") : f028T("timing.no"), session.snapshot_match ? "good" : "warn")}</td>
      <td>${pill(session.effective ? f028T("timing.used") : f028T("timing.not_used"), session.effective ? "good" : "neutral")}</td>
      <td>${editable ? `<select class="timing-selection" data-session-id="${esc(session.session_id)}" ${session.eligible ? "" : "disabled"}>
        <option value="AUTO" ${session.selection_mode === "AUTO" ? "selected" : ""}>${esc(f028T("timing.auto"))}</option>
        <option value="INCLUDE" ${session.selection_mode === "INCLUDE" ? "selected" : ""}>${esc(f028T("timing.include"))}</option>
        <option value="EXCLUDE" ${session.selection_mode === "EXCLUDE" ? "selected" : ""}>${esc(f028T("timing.exclude"))}</option>
      </select>` : esc(f028T("timing.read_only"))}</td>
    </tr>`).join("")}</tbody>
  </table></div>`;
}

function f028SectionCard(report, cues, selectedFrom = "", selectedTo = "") {
  const section = report.section;
  const options = cues.map((cue) => `<option value="${esc(cue.cue_id)}">${esc(`${cue.display_label || ""} · ${cue.name}`)}</option>`).join("");
  return `<section class="card">
    <div class="section-title-row"><div><h2>${esc(f028T("timing.section"))}</h2><p class="muted">${esc(f028T("timing.section_detail"))}</p></div></div>
    <div class="form-grid two" style="margin-top:12px">
      <label>${esc(f028T("timing.section_from"))}<select id="timingSectionFrom"><option value="">—</option>${options}</select></label>
      <label>${esc(f028T("timing.section_to"))}<select id="timingSectionTo"><option value="">—</option>${options}</select></label>
    </div>
    <div class="toolbar" style="margin-top:12px"><button id="timingSectionCalculate" class="button" type="button">${esc(f028T("timing.calculate"))}</button><button id="timingSectionClear" class="button ghost" type="button">${esc(f028T("timing.clear"))}</button></div>
    ${section ? `<div class="stat-grid" style="margin-top:14px">
      <article class="stat"><span class="label">${esc(f028T("timing.median"))}</span><span class="value">${esc(f028Duration(section.statistics.median_us))}</span><span class="sub">${esc(f028CueLabel(section.from))} → ${esc(f028CueLabel(section.to))}</span></article>
      <article class="stat"><span class="label">${esc(f028T("timing.range"))}</span><span class="value">${esc(f028Duration(section.statistics.lower_us))} → ${esc(f028Duration(section.statistics.upper_us))}</span><span class="sub">${esc(section.statistics.trusted_session_count)} ${esc(f028T("timing.samples"))}</span></article>
      <article class="stat"><span class="label">${esc(f028T("timing.confidence"))}</span><span class="value">${pill(f028ConfidenceLabel(section.statistics.confidence), f028ConfidenceKind(section.statistics.confidence))}</span><span class="sub">${section.statistics.confidence === "WITHHELD" ? esc(f028T("timing.section_withheld")) : ""}</span></article>
    </div>` : ""}
  </section>`;
}

async function renderTimingIntelligenceWorkspace(sectionFrom = "", sectionTo = "") {
  if (!state.project) return;
  setPage("timing");
  setMessage(globalMessage, "");
  f028UpdateNav();

  let report;
  let cuePayload;
  try {
    [report, cuePayload] = await Promise.all([
      f028LoadReport(sectionFrom, sectionTo),
      api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/cues`),
    ]);
  } catch (error) {
    content.innerHTML = `<div class="page-head"><div><p class="eyebrow">${esc(f028T("timing.eyebrow"))}</p><h1>${esc(f028T("timing.title"))}</h1><p>${esc(f028T("timing.subtitle"))}</p></div><button id="timingRefresh" class="button" type="button">${esc(f028T("timing.refresh"))}</button></div><section class="card"><div class="empty">${esc(error.status === 404 ? f028T("timing.no_snapshot") : errorMessage(error))}</div></section>`;
    el("timingRefresh")?.addEventListener("click", () => renderTimingIntelligenceWorkspace());
    return;
  }

  const cues = (cuePayload.cues || []).filter((cue) => cue.enabled !== false).sort((a, b) => (a.order_index ?? 0) - (b.order_index ?? 0));
  const leadTime = f028LeadTime();
  content.innerHTML = `
    <div class="page-head">
      <div><p class="eyebrow">${esc(f028T("timing.eyebrow"))}</p><h1>${esc(f028T("timing.title"))}</h1><p>${esc(f028T("timing.subtitle"))}</p></div>
      <div class="toolbar">${pill(f028T("timing.advisory"), "neutral")}<button id="timingRefresh" class="button" type="button">${esc(f028T("timing.refresh"))}</button></div>
    </div>
    <section class="card" style="margin-bottom:16px">
      <div class="form-grid two">
        <label>${esc(f028T("timing.lead"))}<input id="timingLeadSeconds" type="number" min="0" max="3600" value="${esc(leadTime)}"></label>
        <div><p class="muted">${esc(f028T("timing.partial_truth"))}</p><p class="mono muted">Snapshot ${esc(report.runtime_snapshot_id)}</p></div>
      </div>
    </section>
    ${f028ProjectionCard(report.projection)}
    <section class="card" style="margin-top:16px"><div class="section-title-row"><div><h2>${esc(f028T("timing.transitions"))}</h2></div></div><div style="margin-top:12px">${f028TransitionTable(report.transitions || [])}</div></section>
    <section class="card" style="margin-top:16px"><div class="section-title-row"><div><h2>${esc(f028T("timing.rehearsals"))}</h2><p class="muted">${esc(f028T("timing.rehearsals_detail"))}</p></div></div><div style="margin-top:12px">${f028SessionTable(report.sessions || [])}</div></section>
    <div style="margin-top:16px">${f028SectionCard(report, cues, sectionFrom, sectionTo)}</div>`;

  const fromSelect = el("timingSectionFrom");
  const toSelect = el("timingSectionTo");
  if (fromSelect) fromSelect.value = sectionFrom;
  if (toSelect) toSelect.value = sectionTo;
  el("timingRefresh")?.addEventListener("click", () => renderTimingIntelligenceWorkspace(sectionFrom, sectionTo));
  el("timingLeadSeconds")?.addEventListener("change", () => {
    const value = Math.max(0, Math.min(3600, Number(el("timingLeadSeconds").value || 30)));
    localStorage.setItem("stagecore_timing_note_lead_seconds", String(Math.round(value)));
    renderTimingIntelligenceWorkspace(sectionFrom, sectionTo).catch((error) => setMessage(globalMessage, errorMessage(error), "error"));
  });
  el("timingSectionCalculate")?.addEventListener("click", () => {
    const from = fromSelect.value;
    const to = toSelect.value;
    if (!from || !to || from === to) return;
    renderTimingIntelligenceWorkspace(from, to).catch((error) => setMessage(globalMessage, errorMessage(error), "error"));
  });
  el("timingSectionClear")?.addEventListener("click", () => renderTimingIntelligenceWorkspace().catch((error) => setMessage(globalMessage, errorMessage(error), "error")));

  content.querySelectorAll(".timing-selection").forEach((select) => {
    select.addEventListener("change", async () => {
      const previous = (report.sessions || []).find((item) => item.session_id === select.dataset.sessionId)?.selection_mode || "AUTO";
      select.disabled = true;
      try {
        await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/timing-intelligence/sessions/${encodeURIComponent(select.dataset.sessionId)}`, {
          method: "PUT", json: { mode: select.value },
        });
        setMessage(globalMessage, f028T("timing.saved"), "success");
        await renderTimingIntelligenceWorkspace(sectionFrom, sectionTo);
      } catch (error) {
        select.value = previous;
        select.disabled = false;
        setMessage(globalMessage, errorMessage(error), "error");
      }
    });
  });
}

const timingNav = document.querySelector('[data-page="timing"]');
timingNav?.addEventListener("click", (event) => {
  event.preventDefault();
  renderTimingIntelligenceWorkspace().catch((error) => setMessage(globalMessage, errorMessage(error), "error"));
});
f028UpdateNav();

el("languageSelect")?.addEventListener("change", () => {
  f028UpdateNav();
  if (state.page === "timing") {
    renderTimingIntelligenceWorkspace().catch((error) => setMessage(globalMessage, errorMessage(error), "error"));
  }
});
