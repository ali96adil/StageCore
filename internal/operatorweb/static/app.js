"use strict";

const state = {
  csrf: sessionStorage.getItem("stagecore_csrf") || "",
  user: null,
  hub: null,
  projects: [],
  project: null,
  page: "projects",
  cues: [],
  validation: null,
  runtimeTimer: null,
  runtimeForceExitAvailable: false,
};

const el = (id) => document.getElementById(id);
const loginView = el("loginView");
const appView = el("appView");
const content = el("content");
const loginMessage = el("loginMessage");
const globalMessage = el("globalMessage");
const cueDialog = el("cueDialog");
const cueForm = el("cueForm");
const actionsEditor = el("actionsEditor");

function esc(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

function jsonText(value) {
  if (value === null || value === undefined || value === "") return "{}";
  if (typeof value === "string") {
    try { return JSON.stringify(JSON.parse(value), null, 2); } catch (_) { return value; }
  }
  return JSON.stringify(value, null, 2);
}

function parseJSONField(text, label) {
  try { return JSON.parse(text || "{}"); }
  catch (_) { throw new Error(`${label} must contain valid JSON.`); }
}

function fmtDate(value) {
  if (!value) return "—";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "—" : date.toLocaleString();
}

function requestID() {
  if (globalThis.crypto?.randomUUID) return globalThis.crypto.randomUUID();
  const bytes = new Uint8Array(16);
  globalThis.crypto?.getRandomValues?.(bytes);
  bytes[6] = (bytes[6] & 0x0f) | 0x40;
  bytes[8] = (bytes[8] & 0x3f) | 0x80;
  const hex = [...bytes].map((b) => b.toString(16).padStart(2, "0")).join("");
  return `${hex.slice(0,8)}-${hex.slice(8,12)}-${hex.slice(12,16)}-${hex.slice(16,20)}-${hex.slice(20)}`;
}

function setMessage(target, text, kind = "") {
  if (!text) {
    target.textContent = "";
    target.className = "message hidden";
    return;
  }
  target.textContent = text;
  target.className = `message ${kind}`.trim();
}

function errorMessage(error) {
  if (!error) return "Request failed.";
  if (error.payload?.result?.error?.message) return error.payload.result.error.message;
  if (error.payload?.error?.message) return error.payload.error.message;
  if (error.payload?.error_code) return error.payload.error_code.replaceAll("_", " ");
  return error.message || "Request failed.";
}

async function api(path, options = {}) {
  const method = (options.method || "GET").toUpperCase();
  const headers = new Headers(options.headers || {});
  if (!["GET", "HEAD", "OPTIONS"].includes(method)) {
    if (!state.csrf) throw new Error("Sign in again to restore state-changing controls.");
    headers.set("X-StageCore-CSRF", state.csrf);
  }
  if (options.json !== undefined) {
    headers.set("Content-Type", "application/json");
    options.body = JSON.stringify(options.json);
  }
  const response = await fetch(path, {
    ...options,
    method,
    headers,
    credentials: "same-origin",
    cache: "no-store",
  });
  let payload = null;
  if (response.status !== 204) {
    const text = await response.text();
    if (text) {
      try { payload = JSON.parse(text); }
      catch (_) { payload = { message: text }; }
    }
  }
  if (!response.ok) {
    const error = new Error(`HTTP ${response.status}`);
    error.status = response.status;
    error.payload = payload;
    if (response.status === 401) showLogin("Your session is no longer valid. Sign in again.");
    throw error;
  }
  return payload;
}

function canEdit() {
  return ["OWNER", "TECHNICIAN"].includes(state.user?.role);
}

function canRuntime() {
  return ["OWNER", "TECHNICIAN", "OPERATOR"].includes(state.user?.role);
}

function showLogin(message = "") {
  stopRuntimePolling();
  state.user = null;
  state.project = null;
  loginView.classList.remove("hidden");
  appView.classList.add("hidden");
  el("userArea").classList.add("hidden");
  setMessage(loginMessage, message, message ? "warn" : "");
}

function showApp() {
  loginView.classList.add("hidden");
  appView.classList.remove("hidden");
  el("userArea").classList.remove("hidden");
  el("userLabel").textContent = `${state.user.username} · ${state.user.role}`;
  el("roleBadge").textContent = state.user.role;
}

function updateHubIdentity() {
  if (!state.hub) return;
  const shortFingerprint = state.hub.fingerprint || "fingerprint unavailable";
  el("hubBadge").textContent = `${state.hub.display_name} · ${shortFingerprint}`;
  el("hubName").textContent = state.hub.display_name || "StageCore Hub";
  el("hubFingerprint").textContent = shortFingerprint;
}

async function boot() {
  try {
    state.hub = await api("/api/v1/auth/status");
    updateHubIdentity();
  } catch (error) {
    if (error.status === 426) {
      showLogin("Secure transport is required for this Stage LAN connection.");
      el("hubBadge").textContent = "Secure transport required";
      return;
    }
    el("hubBadge").textContent = "Hub identity unavailable";
  }

  try {
    const me = await api("/api/v1/auth/me");
    if (!state.csrf) {
      showLogin("A browser session exists, but control authorization must be refreshed. Sign in again.");
      return;
    }
    state.user = me.user;
    showApp();
    await loadProjects();
    renderProjects();
  } catch (_) {
    showLogin();
  }
}

el("loginForm").addEventListener("submit", async (event) => {
  event.preventDefault();
  setMessage(loginMessage, "Signing in…");
  try {
    const payload = await apiLogin(el("username").value, el("password").value);
    state.user = payload.user;
    state.csrf = payload.csrf_token;
    sessionStorage.setItem("stagecore_csrf", state.csrf);
    el("password").value = "";
    showApp();
    await loadProjects();
    renderProjects();
    setMessage(loginMessage, "");
  } catch (error) {
    setMessage(loginMessage, errorMessage(error), "error");
  }
});

async function apiLogin(username, password) {
  const response = await fetch("/api/v1/auth/login", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    credentials: "same-origin",
    cache: "no-store",
    body: JSON.stringify({ username, password }),
  });
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    const error = new Error(`HTTP ${response.status}`);
    error.status = response.status;
    error.payload = payload;
    throw error;
  }
  return payload;
}

el("logoutButton").addEventListener("click", async () => {
  try { await api("/api/v1/auth/logout", { method: "POST" }); }
  catch (_) { /* local state is cleared even if the server already revoked it */ }
  state.csrf = "";
  sessionStorage.removeItem("stagecore_csrf");
  showLogin("Signed out.");
});

el("projectsNav").addEventListener("click", async () => {
  setPage("projects");
  await loadProjects();
  renderProjects();
});

document.querySelectorAll("[data-page]").forEach((button) => {
  button.addEventListener("click", () => navigate(button.dataset.page));
});

function setPage(page) {
  state.page = page;
  document.querySelectorAll(".nav-button").forEach((button) => button.classList.remove("active"));
  if (page === "projects") el("projectsNav").classList.add("active");
  else document.querySelector(`[data-page="${page}"]`)?.classList.add("active");
  if (page !== "runtime") stopRuntimePolling();
}

async function navigate(page) {
  if (!state.project) return;
  setPage(page);
  setMessage(globalMessage, "");
  try {
    if (page === "dashboard") await renderDashboard();
    if (page === "cues") await renderCues();
    if (page === "runtime") await renderRuntime(true);
  } catch (error) {
    setMessage(globalMessage, errorMessage(error), "error");
  }
}

async function loadProjects() {
  const payload = await api("/api/v1/projects");
  state.projects = payload.projects || [];
  if (state.project) {
    state.project = state.projects.find((item) => item.project_id === state.project.project_id) || state.project;
  }
}

function renderProjects() {
  setPage("projects");
  el("workspaceNav").classList.toggle("hidden", !state.project);
  content.innerHTML = `
    <div class="page-head">
      <div><p class="eyebrow">PROJECTS</p><h1>StageCore Projects</h1><p>Local show-control projects on this Hub.</p></div>
    </div>
    ${canEdit() ? `
      <section class="card" style="margin-bottom:14px">
        <div class="section-title-row"><div><h2>Create Project</h2><p class="muted">Creates an initial editable Draft revision.</p></div></div>
        <form id="createProjectForm" class="form-grid two" style="margin-top:14px">
          <label>Name<input id="newProjectName" required maxlength="160"></label>
          <label>Description<input id="newProjectDescription" maxlength="500"></label>
          <div><button class="button primary" type="submit">Create Project</button></div>
        </form>
      </section>` : ""}
    <div class="grid cards">
      ${state.projects.length ? state.projects.map((project) => `
        <article class="card project-card">
          <div><h2>${esc(project.name)}</h2><p class="muted">${esc(project.description || "No description")}</p></div>
          <div class="meta"><span>${esc(project.lifecycle_state)}</span><span>Updated ${esc(fmtDate(project.updated_at))}</span></div>
          <div class="row-actions">
            <button class="button primary open-project" data-project-id="${esc(project.project_id)}" type="button">Open Project</button>
            ${canEdit() ? `<button class="button duplicate-project" data-project-id="${esc(project.project_id)}" type="button">Duplicate Project</button>` : ""}
          </div>
        </article>`).join("") : `<div class="empty">No Projects yet.</div>`}
    </div>`;

  content.querySelectorAll(".open-project").forEach((button) => {
    button.addEventListener("click", () => openProject(button.dataset.projectId));
  });
  content.querySelectorAll(".duplicate-project").forEach((button) => {
    button.addEventListener("click", () => duplicateProject(button.dataset.projectId, button));
  });
  el("createProjectForm")?.addEventListener("submit", createProject);
}

async function duplicateProject(sourceID, button) {
  const source = state.projects.find((project) => project.project_id === sourceID);
  if (!source || !canEdit() || button.disabled) return;
  const name = prompt(`Name for the duplicate of “${source.name}”:`, `${source.name} Copy`);
  if (name === null) return;
  const cleanName = name.trim();
  if (!cleanName || cleanName.length > 160) {
    setMessage(globalMessage, "Duplicate Project name must be 1–160 characters.", "error");
    return;
  }
  if (!confirm(`Duplicate “${source.name}” as “${cleanName}”? Cues, notes and authoring settings will be copied. The new Project starts as an unpublished Draft; physical Stage Devices, Machine Role assignments and running Sessions are NOT transferred.`)) return;
  button.disabled = true;
  try {
    const result = await api(`/api/v1/projects/${encodeURIComponent(sourceID)}/duplicate`, {
      method: "POST",
      json: { name: cleanName },
    });
    await loadProjects();
    state.project = result.project;
    updateWorkspaceProject();
    await navigate("dashboard");
    setMessage(globalMessage, "Project duplicated. Review device bindings and Publish a new Snapshot before starting a Session.", "success");
  } catch (error) {
    setMessage(globalMessage, errorMessage(error), "error");
  } finally {
    button.disabled = false;
  }
}

async function createProject(event) {
  event.preventDefault();
  try {
    const payload = await api("/api/v1/projects", {
      method: "POST",
      json: { name: el("newProjectName").value.trim(), description: el("newProjectDescription").value.trim() },
    });
    await loadProjects();
    state.project = payload.project;
    updateWorkspaceProject();
    setMessage(globalMessage, "Project created.", "success");
    await navigate("dashboard");
  } catch (error) {
    setMessage(globalMessage, errorMessage(error), "error");
  }
}

async function openProject(projectID) {
  try {
    const payload = await api(`/api/v1/projects/${encodeURIComponent(projectID)}`);
    state.project = payload.project;
    updateWorkspaceProject();
    await navigate("dashboard");
  } catch (error) {
    setMessage(globalMessage, errorMessage(error), "error");
  }
}

function updateWorkspaceProject() {
  el("workspaceNav").classList.remove("hidden");
  el("workspaceProjectName").textContent = state.project?.name || "Project";
}

function pill(text, kind = "neutral") {
  return `<span class="pill ${kind}">${esc(text)}</span>`;
}

async function renderDashboard() {
  const projectID = encodeURIComponent(state.project.project_id);
  const [dashboard, runtime] = await Promise.all([
    api(`/api/v1/projects/${projectID}/dashboard`),
    api(`/api/v1/projects/${projectID}/runtime`).catch(() => null),
  ]);
  state.project = dashboard.project;
  updateWorkspaceProject();
  const published = dashboard.published_snapshot;
  const draft = dashboard.draft_revision;
  const publicationKind = dashboard.unpublished_changes ? "warn" : (published ? "good" : "bad");
  const readinessKind = dashboard.readiness?.status === "PASS" ? "good" : "warn";
  const dashboardRuntimeCues = runtime?.cues || [];
  const dashboardCueParents = cueParentMapFor(dashboardRuntimeCues);
  const dashboardCurrentCue = dashboard.current_cue
    ? (cueInList(dashboardRuntimeCues, dashboard.current_cue.cue_id) || dashboard.current_cue)
    : null;
  const dashboardNextCue = dashboard.next_cue
    ? (cueInList(dashboardRuntimeCues, dashboard.next_cue.cue_id) || dashboard.next_cue)
    : null;
  content.innerHTML = `
    <div class="page-head">
      <div><p class="eyebrow">DASHBOARD</p><h1>${esc(dashboard.project.name)}</h1><p>${esc(dashboard.project.description || "No description")}</p></div>
      <div class="toolbar">${pill(dashboard.mode, dashboard.mode === "SHOW" ? "bad" : dashboard.mode === "REHEARSAL" ? "good" : "neutral")}${pill(dashboard.unpublished_changes ? "Unpublished Changes" : dashboard.publication_state, publicationKind)}</div>
    </div>
    <div class="stat-grid">
      <article class="stat"><span class="label">Draft Revision</span><span class="value">r${esc(draft.revision_number)} · ${esc(draft.status)}</span><span class="sub mono">${esc(draft.revision_id)}</span></article>
      <article class="stat"><span class="label">Published Runtime</span><span class="value">${published ? `Snapshot v${esc(published.snapshot_version)}` : "Not Published"}</span><span class="sub mono">${esc(published?.runtime_snapshot_id || "—")}</span></article>
      <article class="stat">
        <span class="label">Current Cue</span>
        <span class="value">${dashboard.current_cue ? `${esc(dashboard.current_cue.display_label)} · ${esc(dashboard.current_cue.name)}` : "—"}</span>
        ${dashboardCurrentCue && renderCueRelationship(dashboardCurrentCue, dashboardRuntimeCues, dashboardCueParents, true)
          ? `<div class="runtime-cue-links dashboard-cue-links">${renderCueRelationship(dashboardCurrentCue, dashboardRuntimeCues, dashboardCueParents, true)}</div>`
          : ""}
        <span class="sub">Mode ${esc(dashboard.mode)}</span>
      </article>
      <article class="stat">
        <span class="label">Next Cue</span>
        <span class="value">${dashboard.next_cue ? `${esc(dashboard.next_cue.display_label)} · ${esc(dashboard.next_cue.name)}` : "—"}</span>
        ${dashboardNextCue && renderCueRelationship(dashboardNextCue, dashboardRuntimeCues, dashboardCueParents, true)
          ? `<div class="runtime-cue-links dashboard-cue-links">${renderCueRelationship(dashboardNextCue, dashboardRuntimeCues, dashboardCueParents, true)}</div>`
          : ""}
        <span class="sub">${dashboard.active_session ? `Session ${esc(dashboard.active_session.type)}` : "No active Session"}</span>
      </article>
      <article class="stat"><span class="label">Readiness</span><span class="value">${pill(dashboard.readiness?.status || "NOT_EVALUATED", readinessKind)}</span><span class="sub">${esc(dashboard.readiness?.note || "")}</span></article>
      <article class="stat"><span class="label">Runtime Results</span><span class="value">${esc(dashboard.runtime_error_count)} errors</span><span class="sub">${esc(dashboard.runtime_warning_count)} warnings</span></article>
    </div>
    <div class="toolbar" style="margin-top:16px">
      <button class="button" id="dashboardCues" type="button">Open Cues</button>
      <button class="button primary" id="dashboardRuntime" type="button">Open Runtime</button>
    </div>`;
  el("dashboardCues").addEventListener("click", () => navigate("cues"));
  el("dashboardRuntime").addEventListener("click", () => navigate("runtime"));
}

async function loadCues() {
  const payload = await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/cues`);
  state.cues = payload.cues || [];
  return payload;
}

async function renderCues(message = "", messageKind = "success") {
  const payload = await loadCues();
  const hasDraft = payload.revision?.status === "DRAFT";
  let validation = null;
  if (hasDraft) {
    try { validation = await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/validation`); }
    catch (_) { validation = null; }
  }
  let runtime = null;
  try { runtime = await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/runtime`); }
  catch (_) { runtime = null; }

  state.validation = validation;
  const canModify = canEdit();
  const canControl = canRuntime();
  const runtimeMode = runtime?.mode || "EDIT";
  const runtimeCuesByID = new Map((runtime?.cues || []).map((cue) => [cue.cue_id, cue]));
  const runtimeBlackout = !!runtime?.managed_output_blackout;
  const hasPublishedSnapshot = !!runtime?.runtime_snapshot;
  // Validation can leave a revision VALIDATED without publishing it.
  // Show Publish for that revision instead of incorrectly implying that the
  // operator must fork a new Draft. A published VALIDATED revision still
  // requires a new Draft for edits.
  const validatedUnpublished = payload.revision?.status === "VALIDATED" &&
    runtime !== null &&
    runtime?.runtime_snapshot?.revision_id !== payload.revision?.revision_id;
  const cueMessageKind = ["success", "warn", "error"].includes(messageKind) ? messageKind : "success";
  const cueParents = cueParentMap();

  content.innerHTML = `
    <div class="page-head">
      <div><p class="eyebrow">CUE WORKSPACE</p><h1>Cues</h1><p>Revision r${esc(payload.revision.revision_number)} · ${esc(payload.revision.status)}</p></div>
      <div class="toolbar">
        ${hasDraft ? `<button id="validateButton" class="button" type="button">Validate</button>` : ""}
        ${canModify && hasDraft ? `<button id="createCueButton" class="button" type="button">+ Cue</button>` : ""}
        ${canModify && (hasDraft || validatedUnpublished) ? `<button id="publishButton" class="button primary" type="button">Publish Snapshot</button>` : ""}
        ${canModify && !hasDraft ? `<button id="createDraftButton" class="button" type="button">Create Draft</button>` : ""}
        ${canModify && hasPublishedSnapshot ? `<button id="syncDevicesButton" class="button" ${runtimeMode !== "EDIT" ? "disabled" : ""} type="button">Sync Devices</button>` : ""}
      </div>
    </div>
    ${message ? `<div class="message ${cueMessageKind}">${esc(message)}</div>` : ""}
    ${validatedUnpublished ? `<section class="card"><h3>Validated revision awaiting Publish</h3>
      <p class="muted">This validated revision is not the currently published Snapshot. Publish it to activate the configured StageLaser target. Use Create Draft only for further edits.</p></section>`
      : hasDraft ? renderValidation(validation) : renderNoDraftState(canModify)}
    <section class="card" style="margin-top:14px">
      <div class="section-title-row">
        <div>
          <h3>Run Published Cue in Rehearsal</h3>
          <p class="muted">Runs the complete published Cue only while an operator-started REHEARSAL is already active. This is NOT a signal-only test.</p>
        </div>
        ${pill(runtimeMode, runtimeMode === "SHOW" ? "bad" : runtimeMode === "REHEARSAL" ? "good" : "neutral")}
      </div>
      <div class="toolbar" style="margin-top:12px">
        <button id="cueCheckStopButton" class="button warn" ${!canControl || !runtime?.session ? "disabled" : ""} type="button">STOP CUE</button>
        <button id="cueCheckBlackoutButton" class="button danger" ${!canControl || !hasPublishedSnapshot ? "disabled" : ""} type="button">BLACKOUT</button>
        <button id="cueCheckClearButton" class="button ghost" ${!canControl || !hasPublishedSnapshot ? "disabled" : ""} type="button">CLEAR BLACKOUT</button>
        <button id="cueCheckOpenRuntime" class="button" type="button">Open Runtime</button>
      </div>
      <p class="muted" style="margin-top:10px">
        This action never starts a REHEARSAL or SHOW. Open Runtime and start REHEARSAL yourself before enabling this button. It runs the entire Cue (not just one output); use output-specific diagnostics for isolated checks. Draft-only changes must be published first.
      </p>
    </section>
    <div class="table-wrap" style="margin-top:14px">
      <table>
        <thead><tr><th>Order</th><th>Label</th><th>Name</th><th>Group</th><th>State</th><th>Actions</th><th>Controls</th></tr></thead>
        <tbody>
          ${state.cues.length ? state.cues.map((cue, index) => `
            <tr class="${cueRelationshipClass(cue, cueParents)}">
              <td>${esc(cue.order_index)}</td>
              <td>${esc(cue.display_label || "—")}</td>
              <td><strong>${esc(cue.name)}</strong><br><small class="muted">${esc(cue.criticality)}${Number(cuePolicyObject(cue).start_delay_ms || 0) > 0 ? " · Start Delay " + esc(Number(cuePolicyObject(cue).start_delay_ms) / 1000) + " s" : ""}</small></td>
              <td class="cue-link-cell">${renderCueRelationship(cue, state.cues, cueParents) || `<span class="cue-link-none">—</span>`}</td>
              <td>${pill(cue.enabled ? "ENABLED" : "DISABLED", cue.enabled ? "good" : "neutral")}</td>
              <td>${esc(cue.actions?.length || 0)}</td>
              <td><div class="row-actions">
                <button class="button cue-inspect" data-id="${esc(cue.cue_id)}" type="button">فحص الكيو</button>
                ${canControl ? `<button class="button primary cue-test" data-id="${esc(cue.cue_id)}" ${!runtimeCuesByID.has(cue.cue_id) || runtimeMode !== "REHEARSAL" || runtime?.session?.type !== "REHEARSAL" || runtimeBlackout ? "disabled" : ""} type="button">Run Cue in Rehearsal</button>` : ""}
                ${canModify && hasDraft ? `
                  <button class="button cue-up" data-id="${esc(cue.cue_id)}" ${index === 0 ? "disabled" : ""} type="button">↑</button>
                  <button class="button cue-down" data-id="${esc(cue.cue_id)}" ${index === state.cues.length - 1 ? "disabled" : ""} type="button">↓</button>
                  <button class="button cue-edit" data-id="${esc(cue.cue_id)}" type="button">Edit</button>
                  <button class="button cue-toggle" data-id="${esc(cue.cue_id)}" type="button">${cue.enabled ? "Disable" : "Enable"}</button>
                  <button class="button cue-duplicate" data-id="${esc(cue.cue_id)}" type="button">Duplicate</button>
                  <button class="button danger cue-delete" data-id="${esc(cue.cue_id)}" type="button">Delete</button>` : canModify ? `<span class="muted">Create Draft to edit</span>` : `<span class="muted">Read only</span>`}
              </div></td>
            </tr>`).join("") : `<tr><td colspan="7"><div class="empty">No Cues in this revision.</div></td></tr>`}
        </tbody>
      </table>
    </div>
    <section class="card hidden" id="cueInspection" role="status" aria-live="polite"></section>`;

  el("validateButton")?.addEventListener("click", validateDraft);
  el("createCueButton")?.addEventListener("click", () => openCueEditor(null));
  el("publishButton")?.addEventListener("click", publishDraft);
  el("createDraftButton")?.addEventListener("click", createCueDraft);
  el("syncDevicesButton")?.addEventListener("click", syncDevicesFromWorkspace);
  el("cueCheckStopButton")?.addEventListener("click", stopCueFromWorkspace);
  el("cueCheckBlackoutButton")?.addEventListener("click", () => setCueWorkspaceBlackout(true));
  el("cueCheckClearButton")?.addEventListener("click", () => setCueWorkspaceBlackout(false));
  el("cueCheckOpenRuntime")?.addEventListener("click", () => navigate("runtime"));
  content.querySelectorAll(".cue-inspect").forEach((button) => button.addEventListener("click", () => inspectCueFromWorkspace(button.dataset.id)));
  content.querySelectorAll(".cue-test").forEach((button) => button.addEventListener("click", () => testCueFromWorkspace(button.dataset.id)));
  content.querySelectorAll(".cue-edit").forEach((button) => button.addEventListener("click", () => openCueEditor(cueByID(button.dataset.id))));
  content.querySelectorAll(".cue-toggle").forEach((button) => button.addEventListener("click", () => toggleCue(button.dataset.id)));
  content.querySelectorAll(".cue-duplicate").forEach((button) => button.addEventListener("click", () => duplicateCue(button.dataset.id)));
  content.querySelectorAll(".cue-delete").forEach((button) => button.addEventListener("click", () => deleteCue(button.dataset.id)));
  content.querySelectorAll(".cue-up").forEach((button) => button.addEventListener("click", () => moveCue(button.dataset.id, -1)));
  content.querySelectorAll(".cue-down").forEach((button) => button.addEventListener("click", () => moveCue(button.dataset.id, 1)));
}

async function testCueFromWorkspace(cueID) {
  try {
    const runtime = await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/runtime`);
    if (runtime.mode === "SHOW" || runtime.session?.type === "SHOW") {
      setMessage(globalMessage, "Cue execution from Workspace is blocked in SHOW. Use Runtime controls for live operation.", "warn");
      return;
    }
    // Deliberate operator-start only. Never POST /runtime/start from a Cue
    // test: the localized label previously implied an isolated signal probe.
    if (runtime.mode !== "REHEARSAL" || runtime.session?.type !== "REHEARSAL") {
      setMessage(globalMessage, "Start REHEARSAL from Runtime before running an individual published Cue. This button cannot start a session.", "warn");
      return;
    }
    if (runtime.managed_output_blackout) {
      setMessage(globalMessage, "Clear managed blackout before running a Cue.", "warn");
      return;
    }
    const publishedCue = (runtime.cues || []).find((cue) => cue.cue_id === cueID);
    if (!publishedCue) {
      setMessage(globalMessage, "This Cue is not in the latest Published Runtime Snapshot. Publish before testing it.", "warn");
      return;
    }
    if (!confirm(`Execute ALL actions of published Cue “${publishedCue.display_label || ""} · ${publishedCue.name}” inside the CURRENT REHEARSAL? This is not a signal-only test.`)) return;

    // A late mode change is checked again by the Hub's runtime/jump
    // authority gate, never by implicitly starting a new session here.
    await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/runtime/jump`, {
      method: "POST",
      json: {
        request_id: requestID(),
        cue_id: cueID,
        expected_current_cue_id: runtime.current_cue?.cue_id || null,
        operator_note: "Cue Workspace explicit REHEARSAL Cue execution",
        confirm: true,
      },
    });
    await renderCues(`Ran published Cue in REHEARSAL: ${publishedCue.display_label || ""} · ${publishedCue.name}`);
  } catch (error) {
    setMessage(globalMessage, errorMessage(error), error.status === 409 ? "warn" : "error");
  }
}

async function stopCueFromWorkspace() {
  if (!confirm("Stop the currently running Cue? STOP CUE does not guarantee blackout.")) return;
  try {
    const runtime = await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/runtime`);
    if (!runtime.session) {
      setMessage(globalMessage, "No active Session or Cue to stop.", "warn");
      return;
    }
    await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/runtime/stop`, {
      method: "POST",
      json: { request_id: requestID() },
    });
    await renderCues("Current Cue stop requested.");
  } catch (error) {
    setMessage(globalMessage, errorMessage(error), error.status === 409 ? "warn" : "error");
  }
}

async function setCueWorkspaceBlackout(enabled) {
  try {
    const runtime = await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/runtime`);
    const activeSession = !!runtime.session;
    const warning = enabled
      ? `${activeSession ? "Emergency" : "EDIT"} BLACKOUT managed Lighting, StageLaser, Tablet and Native Visual outputs? Audio and external VDMX/OSC remain unchanged.`
      : "Clear managed Tablet / Native Visual blackout? Lighting remains dark and StageLaser remains DISARMED/OFF until explicit recovery actions.";
    if (!confirm(warning)) return;

    const endpoint = activeSession ? "emergency-blackout" : "project-blackout";
    const payload = await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/runtime/${endpoint}`, {
      method: "POST",
      json: {
        request_id: requestID(),
        enabled,
        confirm: enabled ? "BLACKOUT" : "CLEAR",
      },
    });
    const summary = emergencyDomainSummary(payload.result?.payload);
    await renderCues(`${enabled ? "Managed blackout applied." : "Managed blackout clear requested."}${summary ? " " + summary : ""}`);
  } catch (error) {
    setMessage(globalMessage, errorMessage(error), "error");
  }
}

function renderNoDraftState(canModify) {
  return `<section class="card">
    <div class="section-title-row">
      <div><h3>No unpublished Draft</h3><p class="muted">${canModify ? "Create a Draft to make Cue changes. The published Runtime Snapshot stays unchanged." : "This Project currently has no unpublished Cue changes."}</p></div>
      ${pill("PUBLISHED", "neutral")}
    </div>
  </section>`;
}

function renderValidation(report) {
  if (!report) return `<section class="card"><strong>Validation unavailable</strong></section>`;
  const kind = report.valid ? "good" : "bad";
  return `<section class="card">
    <div class="section-title-row"><div><h3>Draft validation</h3><p class="muted">Publish blockers are evaluated by the Hub.</p></div>${pill(report.valid ? "PASS" : "BLOCK", kind)}</div>
    ${report.findings?.length ? `<ul class="validation-list">${report.findings.map((finding) => `<li class="validation-item"><strong>${esc(finding.code || finding.severity || "Finding")}</strong>${esc(finding.message || "Validation finding")}</li>`).join("")}</ul>` : `<p class="muted">No blocking findings.</p>`}
  </section>`;
}

function cuePolicyObject(cue) {
  const value = cue?.execution_policy;
  return value && typeof value === "object" && !Array.isArray(value) ? { ...value } : {};
}

function cueLinkedIDs(cue) {
  const policy = cuePolicyObject(cue);
  return Array.isArray(policy.linked_cue_ids)
    ? [...new Set(policy.linked_cue_ids.map((id) => String(id || "").trim()).filter(Boolean))]
    : [];
}

function cueParentMapFor(cues) {
  const parents = new Map();
  (cues || []).forEach((cue) => {
    cueLinkedIDs(cue).forEach((childID) => {
      if (!parents.has(childID)) parents.set(childID, cue.cue_id);
    });
  });
  return parents;
}

function cueParentMap() {
  return cueParentMapFor(state.cues || []);
}

function cueInList(cues, cueID) {
  return (cues || []).find((cue) => cue.cue_id === cueID) || null;
}

function cueDisplayName(cue) {
  if (!cue) return "Unknown Cue";
  return `${cue.display_label || "—"} · ${cue.name || "Unnamed Cue"}`;
}

function cueRelationshipClass(cue, parents) {
  const isParent = cueLinkedIDs(cue).length > 0;
  const isChild = parents.has(cue.cue_id);
  if (isParent && isChild) return "cue-row-linked cue-row-parent cue-row-child";
  if (isParent) return "cue-row-linked cue-row-parent";
  if (isChild) return "cue-row-linked cue-row-child";
  return "";
}

function cueRelationshipLabel(cue, cues, parents) {
  if (!cue) return "";
  const childCount = cueLinkedIDs(cue).length;
  const parentID = parents.get(cue.cue_id);
  if (childCount && parentID) return `GROUP +${childCount} · LINKED CHILD`;
  if (childCount) return `GROUP +${childCount}`;
  if (parentID) return "LINKED CHILD";
  return "";
}

function renderCueRelationship(cue, cues, parents, compact = false) {
  if (!cue) return "";
  const children = cueLinkedIDs(cue)
    .map((cueID) => cueInList(cues, cueID))
    .filter(Boolean);
  const parentID = parents.get(cue.cue_id);
  const parent = parentID ? cueInList(cues, parentID) : null;
  const parts = [];

  if (children.length) {
    parts.push(`
      <div class="cue-link-status cue-link-parent">
        <span class="cue-link-badge cue-link-badge-parent">GROUP · ${esc(children.length)} LINKED</span>
        ${compact ? "" : `<div class="cue-link-chips">${children.map((child) => `<span class="cue-link-chip">↳ ${esc(cueDisplayName(child))}</span>`).join("")}</div>`}
      </div>`);
  }

  if (parent) {
    parts.push(`
      <div class="cue-link-status cue-link-child">
        <span class="cue-link-badge cue-link-badge-child">LINKED CHILD</span>
        <span class="cue-link-parent-name">↳ Runs with ${esc(cueDisplayName(parent))}</span>
      </div>`);
  }

  return parts.join("");
}

function updateLinkedCuesEditorState() {
  const checkboxes = [...document.querySelectorAll("#linkedCuesEditor .linked-cue-checkbox")];
  const selectedCount = checkboxes.filter((input) => input.checked).length;
  const count = el("linkedCuesCount");
  if (count) {
    count.textContent = selectedCount ? `${selectedCount} LINKED` : "NO LINKS";
    count.className = `pill ${selectedCount ? "good" : "neutral"}`;
  }
  checkboxes.forEach((input) => {
    input.closest(".linked-cue-option")?.classList.toggle("selected", input.checked);
  });
}

function renderLinkedCuesEditor(cue) {
  const host = el("linkedCuesEditor");
  if (!host) return;
  const currentID = cue?.cue_id || "";
  const selected = new Set(cueLinkedIDs(cue));
  const parents = cueParentMap();
  const ancestors = new Set();
  let cursor = currentID;
  while (cursor && parents.has(cursor)) {
    cursor = parents.get(cursor);
    if (!cursor || ancestors.has(cursor)) break;
    ancestors.add(cursor);
  }
  const candidates = (state.cues || []).filter((item) => item.cue_id !== currentID);
  if (!candidates.length) {
    host.innerHTML = '<p class="muted">Create another Cue first, then you can link it here.</p>';
    updateLinkedCuesEditorState();
    return;
  }
  host.innerHTML = candidates.map((item) => {
    const existingParent = parents.get(item.cue_id);
    const ownedElsewhere = existingParent && existingParent !== currentID;
    const wouldCycle = ancestors.has(item.cue_id);
    const disabled = ownedElsewhere || wouldCycle || !item.enabled;
    const parentCue = existingParent ? cueByID(existingParent) : null;
    const note = ownedElsewhere
      ? ` · inside ${parentCue?.display_label || parentCue?.name || "another Cue"}`
      : wouldCycle
        ? " · ancestor (would create a cycle)"
        : !item.enabled
          ? " · disabled"
          : "";
    return `<label class="linked-cue-option ${selected.has(item.cue_id) ? "selected" : ""} ${disabled ? "unavailable" : ""}">
      <input class="linked-cue-checkbox" type="checkbox" value="${esc(item.cue_id)}" ${selected.has(item.cue_id) ? "checked" : ""} ${disabled && !selected.has(item.cue_id) ? "disabled" : ""}>
      <span class="linked-cue-main">
        <strong>${esc(item.display_label || "—")} · ${esc(item.name)}</strong>
        ${note ? `<small class="muted">${esc(note)}</small>` : `<small class="muted">Available to run together</small>`}
      </span>
      <span class="linked-cue-checkmark">LINKED</span>
    </label>`;
  }).join("");
  host.querySelectorAll(".linked-cue-checkbox").forEach((input) => {
    input.addEventListener("change", updateLinkedCuesEditorState);
  });
  updateLinkedCuesEditorState();
}

function linkedCuePolicyFromEditor(basePolicy) {
  const policy = basePolicy && typeof basePolicy === "object" && !Array.isArray(basePolicy) ? { ...basePolicy } : {};
  const linked = [...document.querySelectorAll("#linkedCuesEditor .linked-cue-checkbox:checked")]
    .map((input) => String(input.value || "").trim())
    .filter(Boolean);
  if (linked.length) {
    policy.linked_cue_ids = [...new Set(linked)];
    policy.linked_cue_mode = "TOGETHER";
  } else {
    delete policy.linked_cue_ids;
    delete policy.linked_cue_mode;
  }
  return policy;
}

function cueByID(id) {
  return state.cues.find((cue) => cue.cue_id === id);
}

function openCueEditor(cue) {
  el("cueDialogTitle").textContent = cue ? "Edit Cue" : "Create Cue";
  el("cueId").value = cue?.cue_id || "";
  el("cueLabel").value = cue?.display_label || String((state.cues.length || 0) + 1);
  el("cueName").value = cue?.name || "";
  el("cueCriticality").value = cue?.criticality === "CRITICAL" ? "CRITICAL" : "NORMAL";
  el("cueEnabled").checked = cue?.enabled ?? true;
  el("cueExecutionPolicy").value = jsonText(cue?.execution_policy || {});
  el("cueStartDelaySeconds").value = String(Number(cuePolicyObject(cue).start_delay_ms || 0) / 1000);
  renderLinkedCuesEditor(cue);
  el("cueNotes").value = cue?.notes_summary || "";
  actionsEditor.innerHTML = "";
  (cue?.actions || []).forEach(addActionEditor);
  cueDialog.showModal();
}

function addActionEditor(action = null) {
  const fragment = el("actionTemplate").content.cloneNode(true);
  const card = fragment.querySelector(".action-editor");
  card.querySelector(".action-id").value = action?.action_id || "";
  card.querySelector(".action-target").value = action?.target_ref || "";
  card.querySelector(".action-capability").value = action?.capability_key || "osc.send";
  card.querySelector(".action-mode").value = action?.execution_mode || "SEQUENTIAL";
  card.querySelector(".action-priority").value = action?.priority_class || "P1";
  card.querySelector(".action-enabled").checked = action?.enabled ?? true;
  card.querySelector(".action-parameters").value = jsonText(action?.parameters || {});
  card.querySelector(".action-timeout").value = jsonText(action?.timeout_policy || {});
  card.querySelector(".action-error").value = jsonText(action?.error_policy || {});
  card.querySelector(".remove-action").addEventListener("click", () => card.remove());
  actionsEditor.appendChild(fragment);
  renumberActionEditors();
}

function renumberActionEditors() {
  [...actionsEditor.querySelectorAll(".action-editor")].forEach((card, index) => {
    card.querySelector(".action-title").textContent = `Action ${index + 1}`;
  });
}

el("addActionButton").addEventListener("click", () => addActionEditor());
el("closeCueDialog").addEventListener("click", () => cueDialog.close());
el("cancelCueButton").addEventListener("click", () => cueDialog.close());

cueForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  const cueID = el("cueId").value;
  try {
    const actions = [...actionsEditor.querySelectorAll(".action-editor")].map((card, index) => ({
      action_id: card.querySelector(".action-id").value || undefined,
      order_index: index,
      execution_mode: card.querySelector(".action-mode").value,
      target_ref: card.querySelector(".action-target").value.trim(),
      capability_key: card.querySelector(".action-capability").value.trim(),
      parameters: parseJSONField(card.querySelector(".action-parameters").value, `Action ${index + 1} parameters`),
      timeout_policy: parseJSONField(card.querySelector(".action-timeout").value, `Action ${index + 1} timeout policy`),
      error_policy: parseJSONField(card.querySelector(".action-error").value, `Action ${index + 1} error policy`),
      priority_class: card.querySelector(".action-priority").value,
      enabled: card.querySelector(".action-enabled").checked,
    }));
    const existing = cueID ? cueByID(cueID) : null;
    const body = {
      display_label: el("cueLabel").value.trim(),
      name: el("cueName").value.trim(),
      order_index: existing?.order_index || nextCueOrder(),
      cue_type: "STANDARD",
      criticality: el("cueCriticality").value,
      enabled: el("cueEnabled").checked,
      execution_policy: (() => {
        const policy = linkedCuePolicyFromEditor(parseJSONField(el("cueExecutionPolicy").value, "Cue execution policy"));
        const rawDelay = String(el("cueStartDelaySeconds").value || "0").trim();
        const delaySeconds = Number(rawDelay);
        const delayMS = Math.round(delaySeconds * 1000);
        if (!/^[0-9]+(?:\.[0-9]{1,3})?$/.test(rawDelay) ||
            !Number.isSafeInteger(delayMS) || delayMS > 600000) {
          throw new Error("Cue Start Delay must be between 0 and 600 seconds (up to 3 decimal places).");
        }
        if (delayMS) policy.start_delay_ms = delayMS;
        else delete policy.start_delay_ms;
        return policy;
      })(),
      notes_summary: el("cueNotes").value.trim(),
      actions,
    };
    const path = cueID
      ? `/api/v1/projects/${encodeURIComponent(state.project.project_id)}/cues/${encodeURIComponent(cueID)}`
      : `/api/v1/projects/${encodeURIComponent(state.project.project_id)}/cues`;
    await api(path, { method: cueID ? "PUT" : "POST", json: body });
    cueDialog.close();
    await renderCues(cueID ? "Cue updated in Draft. Publish a new Runtime Snapshot and start a new Rehearsal to apply the delay." : "Cue created in Draft. Publish a Runtime Snapshot and start a new Rehearsal to test it.");
  } catch (error) {
    setMessage(globalMessage, errorMessage(error), "error");
  }
});

function nextCueOrder() {
  return state.cues.reduce((max, cue) => Math.max(max, Number(cue.order_index) || 0), 0) + 1;
}

async function toggleCue(id) {
  const cue = cueByID(id);
  if (!cue) return;
  if (cue.enabled) {
    const parentID = cueParentMap().get(cue.cue_id);
    const children = cueLinkedIDs(cue);
    if (parentID || children.length) {
      const relation = parentID ? `it is a child of ${cueByID(parentID)?.name || parentID}` : `it contains ${children.length} child Cue(s)`;
      setMessage(globalMessage, `Unlink this Cue Group relationship before disabling “${cue.name}”; ${relation}.`, "warn");
      return;
    }
  }
  try {
    await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/cues/${encodeURIComponent(id)}`, {
      method: "PUT",
      json: cueWriteBody(cue, { enabled: !cue.enabled }),
    });
    await renderCues(cue.enabled ? "Cue disabled." : "Cue enabled.");
  } catch (error) { setMessage(globalMessage, errorMessage(error), "error"); }
}

function cueWriteBody(cue, overrides = {}) {
  return {
    display_label: cue.display_label,
    name: cue.name,
    order_index: cue.order_index,
    cue_type: cue.cue_type || "STANDARD",
    criticality: cue.criticality || "NORMAL",
    enabled: cue.enabled,
    execution_policy: cue.execution_policy || {},
    notes_summary: cue.notes_summary || "",
    actions: (cue.actions || []).map((action, index) => ({
      action_id: action.action_id,
      order_index: action.order_index ?? index,
      execution_mode: action.execution_mode,
      target_ref: action.target_ref,
      capability_key: action.capability_key,
      parameters: action.parameters || {},
      timeout_policy: action.timeout_policy || {},
      error_policy: action.error_policy || {},
      priority_class: action.priority_class || "P1",
      enabled: action.enabled,
    })),
    ...overrides,
  };
}

async function duplicateCue(id) {
  const cue = cueByID(id);
  if (!cue) return;
  try {
    await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/cues/${encodeURIComponent(id)}/duplicate`, {
      method: "POST",
      json: { display_label: `${cue.display_label} copy`, name: `${cue.name} Copy`, order_index: nextCueOrder() },
    });
    await renderCues("Cue duplicated.");
  } catch (error) { setMessage(globalMessage, errorMessage(error), "error"); }
}

async function deleteCue(id) {
  const cue = cueByID(id);
  if (!cue || !confirm(`Delete Draft Cue “${cue.name}”?`)) return;
  try {
    await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/cues/${encodeURIComponent(id)}?confirm=true`, { method: "DELETE" });
    await renderCues("Cue deleted from Draft.");
  } catch (error) { setMessage(globalMessage, errorMessage(error), "error"); }
}

async function moveCue(id, delta) {
  const currentIndex = state.cues.findIndex((cue) => cue.cue_id === id);
  const targetIndex = currentIndex + delta;
  if (currentIndex < 0 || targetIndex < 0 || targetIndex >= state.cues.length) return;
  const ordered = [...state.cues];
  [ordered[currentIndex], ordered[targetIndex]] = [ordered[targetIndex], ordered[currentIndex]];
  try {
    await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/cues/reorder`, {
      method: "POST",
      json: { cue_ids: ordered.map((cue) => cue.cue_id) },
    });
    await renderCues("Cue order updated.");
  } catch (error) { setMessage(globalMessage, errorMessage(error), "error"); }
}

async function createCueDraft() {
  try {
    await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/configuration/draft`, { method: "POST" });
    await renderCues("New Draft created. Published Runtime remains unchanged.");
  } catch (error) {
    setMessage(globalMessage, errorMessage(error), "error");
  }
}

async function validateDraft() {
  try {
    state.validation = await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/validation`);
    await renderCues(state.validation.valid ? "Draft validation passed." : "Draft contains blocking validation findings.");
  } catch (error) { setMessage(globalMessage, errorMessage(error), "error"); }
}

async function syncPublishedSnapshotDevices(runtimeSnapshotID) {
  const snapshotID = String(runtimeSnapshotID || "").trim();
  if (!snapshotID) throw new Error("No Published Runtime Snapshot is available for device sync.");
  return api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/stage-devices/sync-runtime-snapshot`, {
    method: "POST",
    json: { runtime_snapshot_id: snapshotID },
  });
}

function deviceSyncMessage(sync) {
  const results = Array.isArray(sync?.results) ? sync.results : [];
  const synced = results.filter((item) => item.status === "SYNCED" || item.status === "ALREADY_SYNCED").length;
  const failed = results.filter((item) => item.status === "FAILED");
  if (!results.length) return "No managed v2 Stage Devices required snapshot synchronization.";
  if (!failed.length && sync?.complete) return `Device sync complete: ${synced}/${results.length} ready on the Published Runtime Snapshot.`;
  const names = failed.slice(0, 3).map((item) => item.display_name || item.device_id).join(", ");
  const extra = failed.length > 3 ? ` +${failed.length - 3} more` : "";
  return `Device sync partial: ${synced}/${results.length} ready. Failed: ${names}${extra}.`;
}

async function syncDevicesFromWorkspace() {
  try {
    const runtime = await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/runtime`);
    const snapshotID = runtime.runtime_snapshot?.runtime_snapshot_id;
    if (!snapshotID) {
      setMessage(globalMessage, "Publish a Runtime Snapshot before syncing Stage Devices.", "warn");
      return;
    }
    if (runtime.session) {
      setMessage(globalMessage, "End the active Session before syncing Stage Devices to the Published Runtime Snapshot.", "warn");
      return;
    }
    if (!confirm("Synchronize all managed v2 Tablets and Lighting to the current Published Runtime Snapshot? Devices may briefly enter safe-media / blackout while authority changes.")) return;
    const sync = await syncPublishedSnapshotDevices(snapshotID);
    await renderCues(deviceSyncMessage(sync), sync.complete ? "success" : "warn");
  } catch (error) {
    setMessage(globalMessage, errorMessage(error), "error");
  }
}

async function publishDraft() {
  if (!confirm("Publish this validated Draft and automatically synchronize all managed v2 Tablets and Lighting to the new immutable Runtime Snapshot? Devices may briefly enter safe-media / blackout while authority changes.")) return;
  try {
    const payload = await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/publish`, { method: "POST" });
    const snapshot = payload.runtime_snapshot;
    try {
      const sync = await syncPublishedSnapshotDevices(snapshot.runtime_snapshot_id);
      await renderCues(
        `Published Runtime Snapshot v${snapshot.snapshot_version}. ${deviceSyncMessage(sync)}`,
        sync.complete ? "success" : "warn",
      );
    } catch (syncError) {
      await renderCues(
        `Published Runtime Snapshot v${snapshot.snapshot_version}, but automatic device sync did not complete: ${errorMessage(syncError)}`,
        "warn",
      );
    }
  } catch (error) {
    if (error.payload?.validation?.findings) {
      state.validation = error.payload.validation;
    }
    setMessage(globalMessage, errorMessage(error), "error");
    try { await renderCues(); } catch (_) {}
  }
}

function groupRuntimeReadinessIssues(checks) {
  const groups = new Map();
  (checks || []).filter((check) => check.status !== "PASS").forEach((check, index) => {
    const entityID = String(check.entity_id || "").trim();
    const key = entityID || `${check.category || "runtime"}:${check.key || index}`;
    if (!groups.has(key)) {
      groups.set(key, {
        key,
        entityID,
        status: check.status || "WARN",
        issues: [],
        categories: new Set(),
      });
    }
    const group = groups.get(key);
    group.issues.push(check);
    group.categories.add(check.category || "runtime");
    if (check.status === "BLOCK") group.status = "BLOCK";
  });
  return [...groups.values()];
}

function runtimeReadinessGroupTitle(group) {
  const preferred = group.issues.find((check) => check.category !== "network") || group.issues[0] || {};
  const summary = String(preferred.summary || preferred.key || "Runtime readiness issue").trim();
  const colon = summary.indexOf(":");
  if (group.entityID && colon >= 0 && colon < summary.length - 1) {
    const suffix = summary.slice(colon + 1).trim();
    if (suffix && suffix !== group.entityID) return suffix;
  }
  return summary;
}

async function renderRuntime(startPolling = false) {
  const projectID = encodeURIComponent(state.project.project_id);
  const [runtime, preflight] = await Promise.all([
    api(`/api/v1/projects/${projectID}/runtime`),
    api(`/api/v1/projects/${projectID}/preflight`).catch(() => null),
  ]);
  const active = runtime.session;
  if (!active) state.runtimeForceExitAvailable = false;
  const current = runtime.current_cue;
  const next = runtime.next_cue;
  const runtimeCues = runtime.cues || [];
  const runtimeCueParents = cueParentMapFor(runtimeCues);
  const currentCue = current ? (cueInList(runtimeCues, current.cue_id) || current) : null;
  const nextCue = next ? (cueInList(runtimeCues, next.cue_id) || next) : null;
  // Notes belong to the published Runtime Snapshot, never unpublished Draft cues.
  const runtimeCueNotes = (cue) => String(cue?.notes_summary || "").trim();
  const canControl = canRuntime();
  const snapshot = runtime.runtime_snapshot;
  const emergencyBlackout = !!runtime.managed_output_blackout;
  const blockers = (preflight?.checks || []).filter((check) => check.status === "BLOCK").length;
  const warnings = (preflight?.checks || []).filter((check) => check.status === "WARN").length;
  const showBlocked = preflight?.status === "BLOCK";
  const runtimeIssues = (preflight?.checks || []).filter((check) => check.status !== "PASS");
  const runtimeIssueGroups = groupRuntimeReadinessIssues(preflight?.checks || []);
  const runtimeIssueMarkup = runtimeIssueGroups.length
    ? `<section class="card runtime-readiness-issues">
        <div class="section-title-row">
          <div>
            <h3>Runtime readiness · degraded operation allowed</h3>
            <p class="muted">WARN means degraded live operation is allowed: healthy outputs continue and unavailable Actions are recorded without stopping later Actions unless FAIL_CUE is explicit. Repeated offline / stale Snapshot / stale network observations are grouped by affected resource below.</p>
          </div>
          ${pill(
            `${runtimeIssueGroups.length} RESOURCE${runtimeIssueGroups.length === 1 ? "" : "S"} · ${runtimeIssues.length} DETAIL${runtimeIssues.length === 1 ? "" : "S"}`,
            blockers ? "bad" : "warn",
          )}
        </div>
        <div class="validation-list">
          ${runtimeIssueGroups.map((group) => `<div class="validation-item">
            <div class="section-title-row">
              <strong>${esc(runtimeReadinessGroupTitle(group))}</strong>
              ${pill(group.status, group.status === "BLOCK" ? "bad" : "warn")}
            </div>
            <p class="muted">${esc([...group.categories].join(" · "))}${group.entityID ? ` · ${esc(group.entityID)}` : ""}</p>
            <ul>
              ${group.issues.map((check) => `<li>
                <strong>${esc(check.summary || check.key || "Runtime readiness issue")}</strong>
                ${check.detail ? `<br><span class="muted">${esc(check.detail)}</span>` : ""}
              </li>`).join("")}
            </ul>
          </div>`).join("")}
        </div>
      </section>`
    : `<section class="card runtime-readiness-issues">
        <div class="section-title-row"><div><h3>Runtime readiness</h3><p class="muted">No current Preflight issues.</p></div>${pill("READY", "good")}</div>
      </section>`;
  content.innerHTML = `
    <div class="page-head">
      <div><p class="eyebrow">RUNTIME</p><h1>${esc(runtime.project.name)}</h1><p>${snapshot ? `Snapshot v${esc(snapshot.snapshot_version)}` : "No published Runtime Snapshot"}</p></div>
      <div class="toolbar">${pill(runtime.mode, runtime.mode === "SHOW" ? "bad" : runtime.mode === "REHEARSAL" ? "good" : "neutral")}</div>
    </div>
    <div class="runtime-hero">
      <section class="cue-focus">
        <div>
          <p class="eyebrow">CURRENT CUE</p>
          <div class="current">${current ? `${esc(current.display_label)} · ${esc(current.name)}` : "—"}</div>
          ${currentCue ? `<div class="runtime-cue-links">${renderCueRelationship(currentCue, runtimeCues, runtimeCueParents, true)}</div>` : ""}
          ${runtimeCueNotes(currentCue) ? `<p class="runtime-cue-operator-notes" aria-label="Current Cue notes"><strong>Notes:</strong> ${esc(runtimeCueNotes(currentCue))}</p>` : ""}
        </div>
        <div class="next">
          <p class="eyebrow">NEXT CUE</p>
          <div class="runtime-next-cue-name">${next ? `${esc(next.display_label)} · ${esc(next.name)}` : "—"}</div>
          ${nextCue ? `<div class="runtime-cue-links compact">${renderCueRelationship(nextCue, runtimeCues, runtimeCueParents, true)}</div>` : ""}
          ${runtimeCueNotes(nextCue) ? `<p class="runtime-cue-operator-notes" aria-label="Next Cue notes"><strong>Notes:</strong> ${esc(runtimeCueNotes(nextCue))}</p>` : ""}
        </div>
      </section>
      <section class="runtime-controls">
        ${!active ? `
          <p class="muted">Runtime is in EDIT mode.</p>
          <div class="message ${preflight?.status === "BLOCK" ? "error" : preflight?.status === "WARN" ? "warn" : ""}">
            <strong>Preflight: ${esc(preflight?.status || "UNKNOWN")}</strong>
            <span> · ${esc(blockers)} critical issue(s) · ${esc(warnings)} warning(s) · advisory for live start</span>
            <button id="runtimeOpenPreflight" class="button ghost" type="button">Open Preflight</button>
          </div>
          <button id="startRehearsalButton" class="button primary big" ${!canControl || !snapshot ? "disabled" : ""} type="button">Start Rehearsal</button>
          <button id="startShowButton" class="button warn" ${!canControl || !snapshot || showBlocked ? "disabled" : ""} type="button">Enter SHOW</button>
          <button id="editBlackoutButton" class="button danger" ${!canControl || !snapshot ? "disabled" : ""} type="button">BLACKOUT MANAGED OUTPUTS</button>
          <button id="editBlackoutClearButton" class="button ghost" ${!canControl || !snapshot ? "disabled" : ""} type="button">Clear Tablet / Native Visual Blackout</button>
          <small class="muted">EDIT Blackout is sessionless. Lighting stays dark after Clear until an explicit Lighting action restores it.</small>
          <small class="muted">Operational readiness is advisory: missing Mac/Companion, Stage Devices, live sources or stale snapshots stay visible below and do not disable SHOW. Structural Snapshot/security/storage/timecode configuration BLOCK conditions still prevent SHOW entry.</small>` : `
          <button id="goButton" class="button primary big" ${!canControl || !next || emergencyBlackout ? "disabled" : ""} type="button">GO</button>
          <button id="stopCueButton" class="button danger big" ${!canControl ? "disabled" : ""} type="button">STOP LATEST CUE</button>
          <button id="emergencyBlackoutButton" class="button ${emergencyBlackout ? "warn" : "danger"} big" ${!canControl ? "disabled" : ""} type="button">${emergencyBlackout ? "CLEAR MANAGED BLACKOUT" : "EMERGENCY BLACKOUT"}</button>
          <label>Jump to Cue
            <select id="jumpCueSelect">
              <option value="">Select published Cue…</option>
              ${runtimeCues.map((cue) => {
                const relation = cueRelationshipLabel(cue, runtimeCues, runtimeCueParents);
                return `<option value="${esc(cue.cue_id)}">${relation ? `[${esc(relation)}] · ` : ""}${esc(cue.display_label)} · ${esc(cue.name)}</option>`;
              }).join("")}
            </select>
          </label>
          <button id="jumpButton" class="button warn" ${!canControl || emergencyBlackout ? "disabled" : ""} type="button">Confirmed Jump</button>
          <button id="stopSessionButton" class="button ghost" ${!canControl ? "disabled" : ""} type="button">Stop ${esc(active.type)} Session</button>\n          ${state.runtimeForceExitAvailable ? `<button id="forceStopSessionButton" class="button danger" ${!canControl ? "disabled" : ""} type="button">FORCE EXIT · bypass blackout confirmation</button>` : ""}\n          <div class="message ${emergencyBlackout ? "error" : "warn"}"><strong>${emergencyBlackout ? "MANAGED BLACKOUT ACTIVE — GO/JUMP are blocked." : "STOP CUE is not a blackout."}</strong> ${emergencyBlackout ? "Managed Lighting, Tablet and Native Visual outputs have been commanded to their blackout state. Audio and external VDMX/OSC are unchanged by design." : "STOP LATEST CUE interrupts only the newest running Cue. STOP SESSION and EMERGENCY BLACKOUT stop all active Cues. EMERGENCY BLACKOUT is a separate P0 operation for managed Lighting, Tablet and Native Visual outputs. Audio and external VDMX/OSC are never silently stopped."}</div>`}
        <div class="runtime-meta">
          <span>Session: ${esc(active?.session_id || "—")}</span>
          <span>Snapshot: ${esc(snapshot?.runtime_snapshot_id || "—")}</span>
        </div>
      </section>
    </div>
    <div class="stat-grid">
      <article class="stat"><span class="label">Latest Result</span><span class="value">${runtime.latest_execution ? pill(runtime.latest_execution.result, runtime.latest_execution.result === "COMPLETED" ? "good" : runtime.latest_execution.result === "RUNNING" ? "warn" : "bad") : "—"}</span><span class="sub">${esc(runtime.latest_execution?.cue_execution_id || "No Cue execution yet")}</span></article>
      <article class="stat"><span class="label">Session Started</span><span class="value">${esc(fmtDate(active?.started_at))}</span><span class="sub">${esc(active?.status || "No active Session")}</span></article>
    </div>
    <section class="card">
      <div class="section-title-row">
        <div><h2>Running Cue Executions</h2><p class="muted">Older Cue actions may continue after the next GO. STOP LATEST targets the newest active execution; STOP SESSION and Emergency Blackout stop all.</p></div>
      </div>
      ${Array.isArray(runtime.running_executions) && runtime.running_executions.length ? `
        <ul class="check-list">
          ${runtime.running_executions.map((run) => {
            const cue = runtimeCues.find((item) => item.cue_id === run.cue_id);
            return `<li>
              <strong>${esc(cue?.display_label || "")} · ${esc(cue?.name || run.cue_id)}</strong>
              <span class="pill warn">RUNNING</span>
              <small class="muted">${esc(run.cue_execution_id)} · ${esc(fmtDate(run.started_at))}</small>
            </li>`;
          }).join("")}
        </ul>` : '<p class="muted">No currently running Cue executions.</p>'}
    </section>
    ${runtimeIssueMarkup}`;

  el("runtimeOpenPreflight")?.addEventListener("click", () => navigate("preflight"));
  el("startRehearsalButton")?.addEventListener("click", () => startRuntime("REHEARSAL"));
  el("startShowButton")?.addEventListener("click", () => startRuntime("SHOW"));
  el("editBlackoutButton")?.addEventListener("click", () => setProjectBlackoutRuntime(true));
  el("editBlackoutClearButton")?.addEventListener("click", () => setProjectBlackoutRuntime(false));
  el("goButton")?.addEventListener("click", goRuntime);
  el("stopCueButton")?.addEventListener("click", stopCueRuntime);
  el("emergencyBlackoutButton")?.addEventListener("click", () => setEmergencyBlackoutRuntime(!emergencyBlackout));
  el("jumpButton")?.addEventListener("click", jumpRuntime);
  el("stopSessionButton")?.addEventListener("click", stopSessionRuntime);
  el("forceStopSessionButton")?.addEventListener("click", forceStopSessionRuntime);
  if (startPolling) startRuntimePolling();
}

async function startRuntime(mode) {
  if (mode === "SHOW" && !confirm("Enter SHOW mode? Operational readiness warnings are advisory: healthy outputs continue, unavailable outputs stay visible as degraded, and their Actions do not stop later Actions unless FAIL_CUE is explicit. Structural Preflight BLOCK conditions still prevent SHOW entry.")) return;
  try {
    state.runtimeForceExitAvailable = false;
    const started = await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/runtime/start`, {
      method: "POST",
      json: { mode, name: `${mode} ${new Date().toLocaleString()}`, request_id: requestID() },
    });
    const payload = started.result?.payload || {};
    const degradedReasons = Array.isArray(payload.degraded_reasons)
      ? payload.degraded_reasons.map((value) => String(value || "").trim()).filter(Boolean)
      : [payload.device_scope_warning, payload.preflight_warning].map((value) => String(value || "").trim()).filter(Boolean);
    const degraded = degradedReasons.length > 0;
    setMessage(
      globalMessage,
      degraded ? `${mode} Session started in DEGRADED mode. ${degradedReasons.join(" ")}` : `${mode} Session started.`,
      degraded ? "warn" : "success",
    );
    await renderRuntime(true);
  } catch (error) { setMessage(globalMessage, errorMessage(error), error.status === 409 ? "warn" : "error"); }
}

async function goRuntime() {
  try {
    const runtime = await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/runtime`);
    await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/runtime/go`, {
      method: "POST",
      json: { request_id: requestID(), expected_current_cue_id: runtime.current_cue?.cue_id || null, async: true },
    });
    await renderRuntime(true);
  } catch (error) { setMessage(globalMessage, errorMessage(error), "error"); }
}

async function stopCueRuntime() {
  if (!confirm("STOP LATEST CUE cancels the newest running Cue only; older Cues may continue. This is not a blackout. Continue?")) return;
  try {
    await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/runtime/stop`, {
      method: "POST", json: { request_id: requestID() },
    });
    await renderRuntime(true);
  } catch (error) { setMessage(globalMessage, errorMessage(error), error.status === 409 ? "warn" : "error"); }
}

function emergencyDomainSummary(payload) {
  if (!payload || typeof payload !== "object") return "";
  const domains = [
    ["Lighting", payload.lighting],
    ["StageLaser", payload.stagelaser],
    ["Tablets", payload.tablets],
    ["Native Visual", payload.native_visual],
    ["Audio", payload.audio],
    ["External", payload.external_adapters],
  ];
  return domains
    .filter(([, value]) => value && typeof value === "object")
    .map(([name, value]) => `${name}: ${value.status || "UNKNOWN"}${Number.isFinite(value.completed) && Number.isFinite(value.attempted) ? ` ${value.completed}/${value.attempted}` : ""}`)
    .join(" · ");
}

async function setEmergencyBlackoutRuntime(enabled) {
  const warning = enabled
    ? "Activate EMERGENCY BLACKOUT? StageCore will first latch blackout and block GO, then interrupt all active Cues and command managed Lighting, StageLaser, Tablet and Native Visual outputs to safe state. Audio and external VDMX/OSC will NOT be stopped."
    : "Clear managed blackout? Tablet and Native Visual blackout will be cleared, but Lighting stays dark and StageLaser stays DISARMED/OFF until explicit Lighting / ARM / Laser Cue actions. GO will only unlock after the managed clear succeeds.";
  if (!confirm(warning)) return;
  try {
    const payload = await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/runtime/emergency-blackout`, {
      method: "POST",
      json: {
        request_id: requestID(),
        enabled,
        confirm: enabled ? "BLACKOUT" : "CLEAR",
      },
    });
    const summary = emergencyDomainSummary(payload.result?.payload);
    setMessage(globalMessage, `${enabled ? "Emergency Blackout applied." : "Managed blackout cleared."}${summary ? " " + summary : ""}`, enabled ? "warn" : "success");
    await renderRuntime(true);
  } catch (error) {
    try { await renderRuntime(true); } catch (_) {}
    setMessage(globalMessage, errorMessage(error), "error");
  }
}

async function jumpRuntime() {
  const cueID = el("jumpCueSelect")?.value;
  if (!cueID) {
    setMessage(globalMessage, "Choose a published Cue before Jump.", "warn");
    return;
  }
  const runtime = await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/runtime`);
  const label = (runtime.cues || []).find((cue) => cue.cue_id === cueID);
  if (!confirm(`Jump runtime to “${label?.name || cueID}”? This is an explicit operator override.`)) return;
  try {
    await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/runtime/jump`, {
      method: "POST",
      json: {
        request_id: requestID(), cue_id: cueID,
        expected_current_cue_id: runtime.current_cue?.cue_id || null,
        confirm: true,
        async: true,
      },
    });
    await renderRuntime(true);
  } catch (error) { setMessage(globalMessage, errorMessage(error), "error"); }
}

async function stopSessionRuntime() {
  if (!confirm("End the active runtime Session? StageCore will first require its managed blackout safety to complete.")) return;
  try {
    await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/runtime/stop-session`, {
      method: "POST", json: { request_id: requestID() },
    });
    state.runtimeForceExitAvailable = false;
    await renderRuntime(true);
  } catch (error) {
    const code = error.payload?.result?.error?.error_code || "";
    state.runtimeForceExitAvailable = code === "SESSION_STOP_SAFETY_FAILED" || code === "SESSION_STOP_CUE_UNCONFIRMED";
    const suffix = state.runtimeForceExitAvailable
      ? " You can retry normal Stop, or use FORCE EXIT if you intentionally accept an unconfirmed safe state."
      : "";
    if (state.runtimeForceExitAvailable) {
      try { await renderRuntime(true); } catch (_) {}
    }
    setMessage(globalMessage, errorMessage(error) + suffix, "error");
  }
}

async function forceStopSessionRuntime() {
  const entered = prompt("FORCE EXIT will end the Session even if managed blackout was not confirmed. Outputs may remain active or unknown. Type FORCE to continue.");
  if (String(entered || "").trim().toUpperCase() !== "FORCE") return;
  try {
    await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/runtime/stop-session`, {
      method: "POST",
      json: { request_id: requestID(), force: true, confirm: "FORCE_EXIT_WITHOUT_BLACKOUT" },
    });
    state.runtimeForceExitAvailable = false;
    setMessage(globalMessage, "Session force-exited. Safe-state confirmation was intentionally bypassed and recorded.", "warn");
    await renderRuntime(true);
  } catch (error) { setMessage(globalMessage, errorMessage(error), "error"); }
}

async function setProjectBlackoutRuntime(enabled) {
  const warning = enabled
    ? "BLACKOUT managed Lighting, StageLaser, Tablet and Native Visual outputs while in EDIT? Audio and external VDMX/OSC remain unchanged."
    : "Clear sessionless Tablet / Native Visual blackout? Lighting stays dark and StageLaser stays DISARMED/OFF until explicit recovery actions.";
  if (!confirm(warning)) return;
  try {
    const payload = await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/runtime/project-blackout`, {
      method: "POST",
      json: { request_id: requestID(), enabled, confirm: enabled ? "BLACKOUT" : "CLEAR" },
    });
    const summary = emergencyDomainSummary(payload.result?.payload);
    setMessage(globalMessage, `${enabled ? "EDIT Blackout applied." : "EDIT managed blackout clear requested."}${summary ? " " + summary : ""}`, enabled ? "warn" : "success");
    await renderRuntime(true);
  } catch (error) { setMessage(globalMessage, errorMessage(error), "error"); }
}

function startRuntimePolling() {
  stopRuntimePolling();
  state.runtimeTimer = setInterval(async () => {
    if (state.page !== "runtime" || document.hidden) return;
    try { await renderRuntime(false); } catch (_) {}
  }, 1500);
}

function stopRuntimePolling() {
  if (state.runtimeTimer) clearInterval(state.runtimeTimer);
  state.runtimeTimer = null;
}

window.addEventListener("beforeunload", stopRuntimePolling);
boot();
