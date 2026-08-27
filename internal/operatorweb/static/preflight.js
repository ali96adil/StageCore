"use strict";

const stagecoreNavigateBase = navigate;
const stagecoreDashboardBase = renderDashboard;

function preflightKind(status) {
  if (status === "PASS") return "good";
  if (status === "WARN") return "warn";
  if (status === "BLOCK") return "bad";
  return "neutral";
}

function bytesLabel(value) {
  const bytes = Number(value || 0);
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  let amount = bytes;
  let index = 0;
  while (amount >= 1024 && index < units.length - 1) {
    amount /= 1024;
    index += 1;
  }
  return `${amount >= 10 || index === 0 ? amount.toFixed(0) : amount.toFixed(1)} ${units[index]}`;
}

async function loadPreflight() {
  if (!state.project) throw new Error("Open a Project before running Preflight.");
  return api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/preflight`);
}

function preflightCheckRows(report) {
  if (!report.checks?.length) return `<div class="empty">No Preflight checks were returned.</div>`;
  return report.checks.map((check) => `
    <article class="card">
      <div class="section-title-row">
        <div>
          <p class="eyebrow">${esc(check.category || "CHECK")}</p>
          <h3>${esc(check.summary)}</h3>
        </div>
        ${pill(check.status, preflightKind(check.status))}
      </div>
      ${check.detail ? `<p class="muted" style="margin-top:10px">${esc(check.detail)}</p>` : ""}
      ${check.entity_id ? `<p class="mono muted" style="margin-top:8px">${esc(check.entity_id)}</p>` : ""}
    </article>`).join("");
}

function preflightRoleRows(report) {
  if (!report.roles?.length) return `<div class="empty">No required Machine Roles in this Runtime Snapshot.</div>`;
  return report.roles.map((role) => `
    <article class="card">
      <div class="section-title-row">
        <div><p class="eyebrow">MACHINE ROLE</p><h3>${esc(role.role_key || role.machine_role_id)}</h3></div>
        ${pill(role.status, preflightKind(role.status))}
      </div>
      <div class="meta" style="margin-top:10px">
        <span>State ${esc(role.role_state || "UNASSIGNED")}</span>
        <span>Agent ${esc(role.readiness || "UNKNOWN")}</span>
        <span>${role.connected ? "Connected" : "Offline"}</span>
      </div>
      <p class="muted" style="margin-top:10px">${esc(role.summary || "")}</p>
      ${role.companion_name ? `<p style="margin-top:8px">Companion: <strong>${esc(role.companion_name)}</strong></p>` : ""}
      ${role.applied_runtime_snapshot_id ? `<p class="mono muted">Applied ${esc(role.applied_runtime_snapshot_id)}</p>` : ""}
    </article>`).join("");
}

function preflightMediaRows(report) {
  if (!report.media?.length) return `<div class="empty">No required media in this Runtime Snapshot.</div>`;
  return report.media.map((media) => `
    <article class="card">
      <div class="section-title-row">
        <div><p class="eyebrow">MEDIA · ${esc(media.role_key || "ROLE")}</p><h3>${esc(media.media_asset_id)}</h3></div>
        ${pill(media.status, preflightKind(media.status))}
      </div>
      <p class="muted" style="margin-top:10px">${esc(media.summary || "")}</p>
      <div class="meta" style="margin-top:8px"><span>${esc(bytesLabel(media.size_bytes))}</span><span>${media.required ? "Required" : "Optional"}</span></div>
      <p class="mono muted" style="margin-top:8px">SHA-256 ${esc(media.content_hash || "—")}</p>
    </article>`).join("");
}

function storageCard(report) {
  const storage = report.storage || {};
  return `
    <article class="card">
      <div class="section-title-row">
        <div><p class="eyebrow">AUTHORITATIVE STORAGE</p><h3>${esc(storage.state || "UNKNOWN")}</h3></div>
        ${pill(storage.status || "BLOCK", preflightKind(storage.status || "BLOCK"))}
      </div>
      <div class="stat-grid" style="margin-top:12px">
        <div class="stat"><span class="label">Free</span><span class="value">${esc(bytesLabel(storage.free_bytes))}</span><span class="sub">${Number(storage.free_percent || 0).toFixed(1)}%</span></div>
        <div class="stat"><span class="label">Runtime reserve</span><span class="value">${esc(bytesLabel(storage.reserve_bytes))}</span><span class="sub">${storage.writable ? "Writable" : "Not writable"}</span></div>
      </div>
      ${storage.reason ? `<p class="muted" style="margin-top:10px">${esc(storage.reason)}</p>` : ""}
    </article>`;
}

async function renderPreflight() {
  const report = await loadPreflight();
  const blockers = (report.checks || []).filter((check) => check.status === "BLOCK").length;
  const warnings = (report.checks || []).filter((check) => check.status === "WARN").length;
  content.innerHTML = `
    <div class="page-head">
      <div>
        <p class="eyebrow">PREFLIGHT</p>
        <h1>${esc(state.project.name)} readiness</h1>
        <p>Authoritative Snapshot, Companion, media and storage checks used by SHOW entry.</p>
      </div>
      <div class="toolbar">${pill(report.status, preflightKind(report.status))}<button id="refreshPreflight" class="button" type="button">Refresh</button></div>
    </div>
    <div class="stat-grid">
      <article class="stat"><span class="label">Runtime Snapshot</span><span class="value">${report.runtime_snapshot_id ? `v${esc(report.snapshot_version)}` : "Not Published"}</span><span class="sub mono">${esc(report.runtime_snapshot_id || "—")}</span></article>
      <article class="stat"><span class="label">Blocking checks</span><span class="value">${esc(blockers)}</span><span class="sub">SHOW requires zero blockers</span></article>
      <article class="stat"><span class="label">Warnings</span><span class="value">${esc(warnings)}</span><span class="sub">Warnings remain visible but do not block SHOW</span></article>
      <article class="stat"><span class="label">Evaluated</span><span class="value">${esc(fmtDate(report.evaluated_at))}</span><span class="sub">Live Hub state</span></article>
    </div>
    <div class="section-title-row" style="margin-top:20px"><div><p class="eyebrow">CHECKS</p><h2>PASS / WARN / BLOCK</h2></div></div>
    <div class="grid cards">${preflightCheckRows(report)}</div>
    <div class="section-title-row" style="margin-top:20px"><div><p class="eyebrow">COMPANIONS</p><h2>Machine Role readiness</h2></div></div>
    <div class="grid cards">${preflightRoleRows(report)}</div>
    <div class="section-title-row" style="margin-top:20px"><div><p class="eyebrow">MEDIA</p><h2>Required media readiness</h2></div></div>
    <div class="grid cards">${preflightMediaRows(report)}</div>
    <div class="section-title-row" style="margin-top:20px"><div><p class="eyebrow">STORAGE</p><h2>Runtime reserve</h2></div></div>
    ${storageCard(report)}`;
  el("refreshPreflight").addEventListener("click", async () => {
    try { await renderPreflight(); }
    catch (error) { setMessage(globalMessage, errorMessage(error), "error"); }
  });
}

navigate = async function stagecoreNavigate(page) {
  if (page !== "preflight") return stagecoreNavigateBase(page);
  if (!state.project) return;
  setPage(page);
  setMessage(globalMessage, "");
  try { await renderPreflight(); }
  catch (error) { setMessage(globalMessage, errorMessage(error), "error"); }
};

renderDashboard = async function stagecoreDashboard() {
  await stagecoreDashboardBase();
  if (!state.project) return;
  try {
    const report = await loadPreflight();
    const summary = document.createElement("section");
    summary.className = "card";
    summary.style.marginTop = "16px";
    const blockers = (report.checks || []).filter((check) => check.status === "BLOCK").length;
    const warnings = (report.checks || []).filter((check) => check.status === "WARN").length;
    summary.innerHTML = `
      <div class="section-title-row">
        <div><p class="eyebrow">LIVE PREFLIGHT</p><h2>Readiness ${esc(report.status)}</h2><p class="muted">${esc(blockers)} blockers · ${esc(warnings)} warnings</p></div>
        <div class="toolbar">${pill(report.status, preflightKind(report.status))}<button id="dashboardPreflight" class="button" type="button">Open Preflight</button></div>
      </div>`;
    content.appendChild(summary);
    el("dashboardPreflight").addEventListener("click", () => navigate("preflight"));
  } catch (error) {
    const summary = document.createElement("section");
    summary.className = "card";
    summary.style.marginTop = "16px";
    summary.innerHTML = `<p class="eyebrow">LIVE PREFLIGHT</p><h2>Readiness unavailable</h2><p class="muted">${esc(errorMessage(error))}</p>`;
    content.appendChild(summary);
  }
};
