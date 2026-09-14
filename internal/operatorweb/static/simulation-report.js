"use strict";

(() => {
  let observer = null;
  let rendering = false;
  let currentReport = null;

  function ar() {
    return (document.documentElement.lang || "ar").toLowerCase().startsWith("ar");
  }

  function t(arText, enText) {
    return ar() ? arText : enText;
  }

  function projectID() {
    return state.project?.project_id || "";
  }

  function storageKey() {
    return `stagecore_last_simulation_session_${projectID()}`;
  }

  function rememberSession(sessionID) {
    if (!projectID() || !sessionID) return;
    localStorage.setItem(storageKey(), sessionID);
  }

  function rememberedSession() {
    if (!projectID()) return "";
    return localStorage.getItem(storageKey()) || "";
  }

  async function activeSessionID() {
    if (!projectID()) return "";
    try {
      const status = await api(`/api/v1/projects/${encodeURIComponent(projectID())}/simulation`);
      const id = status?.session?.session_id || status?.session?.id || "";
      if (id) rememberSession(id);
      return id || rememberedSession();
    } catch (_) {
      return rememberedSession();
    }
  }

  function badge(count, kind = "") {
    return `<span class="phase4-pulse ${kind}">${esc(count)}</span>`;
  }

  function renderMappings(items) {
    if (!items?.length) return `<div class="phase4-empty">${t("لا توجد mappings مفقودة.", "No missing mappings.")}</div>`;
    return `<div class="table-wrap"><table><thead><tr><th>${t("النوع", "Kind")}</th><th>Target</th><th>Capability</th><th>${t("السبب", "Reason")}</th></tr></thead><tbody>${items.map((item) => `<tr><td>${esc(item.kind)}</td><td class="mono">${esc(item.target_ref)}</td><td class="mono">${esc(item.capability)}</td><td>${esc(item.reason_code)}</td></tr>`).join("")}</tbody></table></div>`;
  }

  function renderTiming(items) {
    if (!items?.length) return `<div class="phase4-empty">${t("لا توجد timing risks ضمن أدلة المحاكاة الحالية.", "No timing risks in current simulation evidence.")}</div>`;
    return `<div class="table-wrap"><table><thead><tr><th>Target</th><th>Capability</th><th>${t("النتيجة", "Result")}</th><th>Latency</th><th>Timeout</th><th>${t("الخطر", "Risk")}</th></tr></thead><tbody>${items.map((item) => `<tr><td class="mono">${esc(item.target_ref)}</td><td class="mono">${esc(item.capability)}</td><td>${esc(item.result)}</td><td>${esc(item.latency_ms ?? "—")} ms</td><td>${esc(item.timeout_ms)} ms</td><td>${esc(item.reason_code)} · ${esc(item.severity)}</td></tr>`).join("")}</tbody></table></div>`;
  }

  function renderFailures(items) {
    if (!items?.length) return `<div class="phase4-empty">${t("لا توجد failures غير مسترجعة.", "No unrecovered simulated failures.")}</div>`;
    return `<div class="table-wrap"><table><thead><tr><th>Target</th><th>Capability</th><th>${t("النتيجة", "Result")}</th><th>Error</th><th>on_error</th></tr></thead><tbody>${items.map((item) => `<tr><td class="mono">${esc(item.target_ref)}</td><td class="mono">${esc(item.capability)}</td><td>${esc(item.result)}</td><td class="mono">${esc(item.error_code || item.reason_code)}</td><td>${esc(item.on_error)}</td></tr>`).join("")}</tbody></table></div>`;
  }

  function renderStage(items) {
    if (!items?.length) return `<div class="phase4-empty">${t("لا توجد targets قابلة للمقارنة.", "No targets available for stage comparison.")}</div>`;
    return `<div class="table-wrap"><table><thead><tr><th>Target</th><th>${t("المقارنة", "Comparison")}</th><th>Simulation</th><th>${t("المرحلة الفعلية", "Observed stage")}</th><th>${t("الدليل", "Evidence")}</th></tr></thead><tbody>${items.map((item) => {
      const sim = item.simulated_online === undefined ? "UNKNOWN" : (item.simulated_online ? "ONLINE" : "OFFLINE");
      const actual = item.actual_connection || "NOT_OBSERVED";
      const kind = item.comparison === "MATCH" ? "ready" : item.comparison === "DIFFERENT" ? "blocker" : "unknown";
      return `<tr><td class="mono">${esc(item.target_ref)}</td><td>${badge(item.comparison, kind)}</td><td>${esc(sim)}</td><td>${esc(actual)}${item.actual_readiness ? ` · ${esc(item.actual_readiness)}` : ""}</td><td>${esc(item.reason_code)}</td></tr>`;
    }).join("")}</tbody></table></div>`;
  }

  function renderReport(report, sessionID) {
    currentReport = report;
    const summary = report.summary || {};
    return `<section id="simulationReportPanel" class="phase4-form">
      <div class="phase4-card-head">
        <div><p class="eyebrow">F-024 · SIMULATION REPORT</p><h2>${t("تقرير المحاكاة", "Simulation Report")}</h2><p class="muted">${t("تقرير مشتق من Runtime Snapshot والتنفيذات وFlight Recorder وDigital Twin ومشاهدات Stage الفعلية. القيم غير المرصودة تبقى UNKNOWN.", "Derived from the Runtime Snapshot, execution history, Flight Recorder, Digital Twin and observed Stage evidence. Unobserved values remain UNKNOWN.")}</p></div>
        <span class="phase4-pulse warning">EVIDENCE-BASED · READ ONLY</span>
      </div>
      <dl class="phase4-kv">
        <div><dt>Session</dt><dd class="mono">${esc(sessionID)}</dd></div>
        <div><dt>Runtime Snapshot</dt><dd class="mono">${esc(report.runtime_snapshot_id)}</dd></div>
        <div><dt>Snapshot SHA-256</dt><dd class="mono">${esc(report.snapshot_content_hash)}</dd></div>
        <div><dt>${t("وقت التقرير", "Generated")}</dt><dd>${esc(fmtDate(report.generated_at))}</dd></div>
      </dl>
      <div class="phase4-actions">
        <button id="simReportRefresh" class="button" type="button">${t("تحديث التقرير", "Refresh report")}</button>
        <button id="simReportExport" class="button" type="button">${t("تصدير JSON", "Export JSON")}</button>
      </div>
      <div class="stat-grid">
        <article class="stat"><span class="label">Missing mappings</span><span class="value">${esc(summary.missing_mappings || 0)}</span></article>
        <article class="stat"><span class="label">Timing risks</span><span class="value">${esc(summary.timing_risks || 0)}</span></article>
        <article class="stat"><span class="label">Unrecovered failures</span><span class="value">${esc(summary.unhandled_failures || 0)}</span></article>
        <article class="stat"><span class="label">Stage differences</span><span class="value">${esc(summary.stage_differences || 0)}</span><span class="sub">${esc(summary.stage_unknown || 0)} UNKNOWN</span></article>
      </div>
      <div class="section-title-row"><div><h3>${t("Mappings مفقودة", "Missing mappings")}</h3></div>${badge(summary.missing_mappings || 0, summary.missing_mappings ? "blocker" : "ready")}</div>
      ${renderMappings(report.missing_mappings)}
      <div class="section-title-row"><div><h3>${t("مخاطر التوقيت", "Timing risks")}</h3></div>${badge(summary.timing_risks || 0, summary.timing_risks ? "warning" : "ready")}</div>
      ${renderTiming(report.timing_risks)}
      <div class="section-title-row"><div><h3>${t("Failures غير مسترجعة", "Unrecovered failures")}</h3></div>${badge(summary.unhandled_failures || 0, summary.unhandled_failures ? "blocker" : "ready")}</div>
      ${renderFailures(report.unhandled_failures)}
      <div class="section-title-row"><div><h3>${t("Simulation مقابل Stage المرصود", "Simulation vs observed Stage")}</h3></div></div>
      ${renderStage(report.stage_differences)}
      <p class="muted">${t("ملاحظة: latency ونتائج التنفيذ أعلاه Simulation evidence وليست قياسات أداء فعلية للأجهزة.", "Note: latency and execution results above are simulation evidence, not measured physical-device performance.")}</p>
    </section>`;
  }

  function renderUnavailable(sessionID, message) {
    return `<section id="simulationReportPanel" class="phase4-form">
      <div class="phase4-card-head"><div><p class="eyebrow">F-024 · SIMULATION REPORT</p><h2>${t("تقرير المحاكاة", "Simulation Report")}</h2></div><span class="phase4-pulse unknown">UNKNOWN</span></div>
      <p>${esc(message)}</p>
      ${sessionID ? `<p class="mono">${esc(sessionID)}</p><button id="simReportRefresh" class="button" type="button">${t("إعادة المحاولة", "Retry")}</button>` : ""}
    </section>`;
  }

  async function loadReport() {
    if (rendering || state.page !== "simulation" || !projectID()) return;
    rendering = true;
    try {
      const sessionID = await activeSessionID();
      const old = el("simulationReportPanel");
      if (!sessionID) {
        if (!old) content.insertAdjacentHTML("beforeend", renderUnavailable("", t("ابدأ جلسة Simulation حتى يتوفر تقرير قائم على الأدلة.", "Start a Simulation Session to produce an evidence-based report.")));
        return;
      }
      const payload = await api(`/api/v1/projects/${encodeURIComponent(projectID())}/simulation/report/${encodeURIComponent(sessionID)}`);
      const html = renderReport(payload.report, sessionID);
      if (old) old.outerHTML = html;
      else content.insertAdjacentHTML("beforeend", html);
      bindReportControls(sessionID);
    } catch (error) {
      const sessionID = rememberedSession();
      const old = el("simulationReportPanel");
      const html = renderUnavailable(sessionID, errorMessage(error));
      if (old) old.outerHTML = html;
      else content.insertAdjacentHTML("beforeend", html);
      if (sessionID) bindReportControls(sessionID);
    } finally {
      rendering = false;
    }
  }

  function bindReportControls(sessionID) {
    el("simReportRefresh")?.addEventListener("click", () => loadReport());
    el("simReportExport")?.addEventListener("click", () => {
      if (!currentReport) return;
      const blob = new Blob([JSON.stringify(currentReport, null, 2)], { type: "application/json" });
      const url = URL.createObjectURL(blob);
      const anchor = document.createElement("a");
      anchor.href = url;
      anchor.download = `stagecore-simulation-report-${sessionID}.json`;
      document.body.appendChild(anchor);
      anchor.click();
      anchor.remove();
      URL.revokeObjectURL(url);
    });
  }

  function installObserver() {
    if (observer || !content) return;
    observer = new MutationObserver(() => {
      if (state.page === "simulation" && !el("simulationReportPanel")) loadReport().catch(() => {});
    });
    observer.observe(content, { childList: true, subtree: false });
  }

  installObserver();
  setInterval(() => {
    if (state.page === "simulation") loadReport().catch(() => {});
  }, 5000);
})();
