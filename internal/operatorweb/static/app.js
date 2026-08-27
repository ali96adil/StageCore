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
  return ["OWNER", "OPERATOR"].includes(state.user?.role);
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
          <button class="button primary open-project" data-project-id="${esc(project.project_id)}" type="button">Open Project</button>
        </article>`).join("") : `<div class="empty">No Projects yet.</div>`}
    </div>`;

  content.querySelectorAll(".open-project").forEach((button) => {
    button.addEventListener("click", () => openProject(button.dataset.projectId));
  });
  el("createProjectForm")?.addEventListener("submit", createProject);
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
  const dashboard = await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/dashboard`);
  state.project = dashboard.project;
  updateWorkspaceProject();
  const published = dashboard.published_snapshot;
  const draft = dashboard.draft_revision;
  const publicationKind = dashboard.unpublished_changes ? "warn" : (published ? "good" : "bad");
  const readinessKind = dashboard.readiness?.status === "PASS" ? "good" : "warn";
  content.innerHTML = `
    <div class="page-head">
      <div><p class="eyebrow">DASHBOARD</p><h1>${esc(dashboard.project.name)}</h1><p>${esc(dashboard.project.description || "No description")}</p></div>
      <div class="toolbar">${pill(dashboard.mode, dashboard.mode === "SHOW" ? "bad" : dashboard.mode === "REHEARSAL" ? "good" : "neutral")}${pill(dashboard.unpublished_changes ? "Unpublished Changes" : dashboard.publication_state, publicationKind)}</div>
    </div>
    <div class="stat-grid">
      <article class="stat"><span class="label">Draft Revision</span><span class="value">r${esc(draft.revision_number)} · ${esc(draft.status)}</span><span class="sub mono">${esc(draft.revision_id)}</span></article>
      <article class="stat"><span class="label">Published Runtime</span><span class="value">${published ? `Snapshot v${esc(published.snapshot_version)}` : "Not Published"}</span><span class="sub mono">${esc(published?.runtime_snapshot_id || "—")}</span></article>
      <article class="stat"><span class="label">Current Cue</span><span class="value">${dashboard.current_cue ? `${esc(dashboard.current_cue.display_label)} · ${esc(dashboard.current_cue.name)}` : "—"}</span><span class="sub">Mode ${esc(dashboard.mode)}</span></article>
      <article class="stat"><span class="label">Next Cue</span><span class="value">${dashboard.next_cue ? `${esc(dashboard.next_cue.display_label)} · ${esc(dashboard.next_cue.name)}` : "—"}</span><span class="sub">${dashboard.active_session ? `Session ${esc(dashboard.active_session.type)}` : "No active Session"}</span></article>
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

async function renderCues(message = "") {
  const payload = await loadCues();
  let validation = null;
  try { validation = await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/validation`); }
  catch (_) { validation = null; }
  state.validation = validation;
  const canModify = canEdit();
  content.innerHTML = `
    <div class="page-head">
      <div><p class="eyebrow">CUE WORKSPACE</p><h1>Cues</h1><p>Revision r${esc(payload.revision.revision_number)} · ${esc(payload.revision.status)}</p></div>
      <div class="toolbar">
        <button id="validateButton" class="button" type="button">Validate</button>
        ${canModify ? `<button id="createCueButton" class="button" type="button">+ Cue</button><button id="publishButton" class="button primary" type="button">Publish Snapshot</button>` : ""}
      </div>
    </div>
    ${message ? `<div class="message success">${esc(message)}</div>` : ""}
    ${renderValidation(validation)}
    <div class="table-wrap" style="margin-top:14px">
      <table>
        <thead><tr><th>Order</th><th>Label</th><th>Name</th><th>State</th><th>Actions</th><th>Controls</th></tr></thead>
        <tbody>
          ${state.cues.length ? state.cues.map((cue, index) => `
            <tr>
              <td>${esc(cue.order_index)}</td>
              <td>${esc(cue.display_label || "—")}</td>
              <td><strong>${esc(cue.name)}</strong><br><small class="muted">${esc(cue.criticality)}</small></td>
              <td>${pill(cue.enabled ? "ENABLED" : "DISABLED", cue.enabled ? "good" : "neutral")}</td>
              <td>${esc(cue.actions?.length || 0)}</td>
              <td><div class="row-actions">
                ${canModify ? `
                  <button class="button cue-up" data-id="${esc(cue.cue_id)}" ${index === 0 ? "disabled" : ""} type="button">↑</button>
                  <button class="button cue-down" data-id="${esc(cue.cue_id)}" ${index === state.cues.length - 1 ? "disabled" : ""} type="button">↓</button>
                  <button class="button cue-edit" data-id="${esc(cue.cue_id)}" type="button">Edit</button>
                  <button class="button cue-toggle" data-id="${esc(cue.cue_id)}" type="button">${cue.enabled ? "Disable" : "Enable"}</button>
                  <button class="button cue-duplicate" data-id="${esc(cue.cue_id)}" type="button">Duplicate</button>
                  <button class="button danger cue-delete" data-id="${esc(cue.cue_id)}" type="button">Delete</button>` : `<span class="muted">Read only</span>`}
              </div></td>
            </tr>`).join("") : `<tr><td colspan="6"><div class="empty">No Cues in this revision.</div></td></tr>`}
        </tbody>
      </table>
    </div>`;

  el("validateButton").addEventListener("click", validateDraft);
  el("createCueButton")?.addEventListener("click", () => openCueEditor(null));
  el("publishButton")?.addEventListener("click", publishDraft);
  content.querySelectorAll(".cue-edit").forEach((button) => button.addEventListener("click", () => openCueEditor(cueByID(button.dataset.id))));
  content.querySelectorAll(".cue-toggle").forEach((button) => button.addEventListener("click", () => toggleCue(button.dataset.id)));
  content.querySelectorAll(".cue-duplicate").forEach((button) => button.addEventListener("click", () => duplicateCue(button.dataset.id)));
  content.querySelectorAll(".cue-delete").forEach((button) => button.addEventListener("click", () => deleteCue(button.dataset.id)));
  content.querySelectorAll(".cue-up").forEach((button) => button.addEventListener("click", () => moveCue(button.dataset.id, -1)));
  content.querySelectorAll(".cue-down").forEach((button) => button.addEventListener("click", () => moveCue(button.dataset.id, 1)));
}

function renderValidation(report) {
  if (!report) return `<section class="card"><strong>Validation unavailable</strong></section>`;
  const kind = report.valid ? "good" : "bad";
  return `<section class="card">
    <div class="section-title-row"><div><h3>Draft validation</h3><p class="muted">Publish blockers are evaluated by the Hub.</p></div>${pill(report.valid ? "PASS" : "BLOCK", kind)}</div>
    ${report.findings?.length ? `<ul class="validation-list">${report.findings.map((finding) => `<li class="validation-item"><strong>${esc(finding.code || finding.severity || "Finding")}</strong>${esc(finding.message || "Validation finding")}</li>`).join("")}</ul>` : `<p class="muted">No blocking findings.</p>`}
  </section>`;
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
      execution_policy: parseJSONField(el("cueExecutionPolicy").value, "Cue execution policy"),
      notes_summary: el("cueNotes").value.trim(),
      actions,
    };
    const path = cueID
      ? `/api/v1/projects/${encodeURIComponent(state.project.project_id)}/cues/${encodeURIComponent(cueID)}`
      : `/api/v1/projects/${encodeURIComponent(state.project.project_id)}/cues`;
    await api(path, { method: cueID ? "PUT" : "POST", json: body });
    cueDialog.close();
    await renderCues(cueID ? "Cue updated in Draft." : "Cue created in Draft.");
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

async function validateDraft() {
  try {
    state.validation = await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/validation`);
    await renderCues(state.validation.valid ? "Draft validation passed." : "Draft contains blocking validation findings.");
  } catch (error) { setMessage(globalMessage, errorMessage(error), "error"); }
}

async function publishDraft() {
  if (!confirm("Publish this validated Draft as a new immutable Runtime Snapshot?")) return;
  try {
    const payload = await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/publish`, { method: "POST" });
    await renderCues(`Published Runtime Snapshot v${payload.runtime_snapshot.snapshot_version}.`);
  } catch (error) {
    if (error.payload?.validation?.findings) {
      state.validation = error.payload.validation;
    }
    setMessage(globalMessage, errorMessage(error), "error");
    try { await renderCues(); } catch (_) {}
  }
}

async function renderRuntime(startPolling = false) {
  const runtime = await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/runtime`);
  const active = runtime.session;
  const current = runtime.current_cue;
  const next = runtime.next_cue;
  const canControl = canRuntime();
  const snapshot = runtime.runtime_snapshot;
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
        </div>
        <div class="next"><strong>Next:</strong> ${next ? `${esc(next.display_label)} · ${esc(next.name)}` : "—"}</div>
      </section>
      <section class="runtime-controls">
        ${!active ? `
          <p class="muted">Runtime is in EDIT mode.</p>
          <button id="startRehearsalButton" class="button primary big" ${!canControl || !snapshot ? "disabled" : ""} type="button">Start Rehearsal</button>
          <button id="startShowButton" class="button warn" ${!canControl || !snapshot ? "disabled" : ""} type="button">Enter SHOW</button>
          <small class="muted">SHOW remains blocked until the S3 Preflight gate passes.</small>` : `
          <button id="goButton" class="button primary big" ${!canControl || !next ? "disabled" : ""} type="button">GO</button>
          <button id="stopCueButton" class="button danger big" ${!canControl ? "disabled" : ""} type="button">STOP</button>
          <label>Jump to Cue
            <select id="jumpCueSelect">
              <option value="">Select published Cue…</option>
              ${(runtime.cues || []).map((cue) => `<option value="${esc(cue.cue_id)}">${esc(cue.display_label)} · ${esc(cue.name)}</option>`).join("")}
            </select>
          </label>
          <button id="jumpButton" class="button warn" ${!canControl ? "disabled" : ""} type="button">Confirmed Jump</button>
          <button id="stopSessionButton" class="button ghost" ${!canControl ? "disabled" : ""} type="button">Stop ${esc(active.type)} Session</button>`}
        <div class="runtime-meta">
          <span>Session: ${esc(active?.session_id || "—")}</span>
          <span>Snapshot: ${esc(snapshot?.runtime_snapshot_id || "—")}</span>
        </div>
      </section>
    </div>
    <div class="stat-grid">
      <article class="stat"><span class="label">Latest Result</span><span class="value">${runtime.latest_execution ? pill(runtime.latest_execution.result, runtime.latest_execution.result === "COMPLETED" ? "good" : runtime.latest_execution.result === "RUNNING" ? "warn" : "bad") : "—"}</span><span class="sub">${esc(runtime.latest_execution?.cue_execution_id || "No Cue execution yet")}</span></article>
      <article class="stat"><span class="label">Session Started</span><span class="value">${esc(fmtDate(active?.started_at))}</span><span class="sub">${esc(active?.status || "No active Session")}</span></article>
    </div>`;

  el("startRehearsalButton")?.addEventListener("click", () => startRuntime("REHEARSAL"));
  el("startShowButton")?.addEventListener("click", () => startRuntime("SHOW"));
  el("goButton")?.addEventListener("click", goRuntime);
  el("stopCueButton")?.addEventListener("click", stopCueRuntime);
  el("jumpButton")?.addEventListener("click", jumpRuntime);
  el("stopSessionButton")?.addEventListener("click", stopSessionRuntime);
  if (startPolling) startRuntimePolling();
}

async function startRuntime(mode) {
  if (mode === "SHOW" && !confirm("Enter SHOW mode? StageCore will enforce the SHOW Preflight gate.")) return;
  try {
    await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/runtime/start`, {
      method: "POST",
      json: { mode, name: `${mode} ${new Date().toLocaleString()}`, request_id: requestID() },
    });
    setMessage(globalMessage, `${mode} Session started.`, "success");
    await renderRuntime(true);
  } catch (error) { setMessage(globalMessage, errorMessage(error), error.status === 409 ? "warn" : "error"); }
}

async function goRuntime() {
  try {
    const runtime = await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/runtime`);
    await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/runtime/go`, {
      method: "POST",
      json: { request_id: requestID(), expected_current_cue_id: runtime.current_cue?.cue_id || null },
    });
    await renderRuntime(true);
  } catch (error) { setMessage(globalMessage, errorMessage(error), "error"); }
}

async function stopCueRuntime() {
  if (!confirm("Request STOP for the currently running Cue/interruptible Actions?")) return;
  try {
    await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/runtime/stop`, {
      method: "POST", json: { request_id: requestID() },
    });
    await renderRuntime(true);
  } catch (error) { setMessage(globalMessage, errorMessage(error), error.status === 409 ? "warn" : "error"); }
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
      },
    });
    await renderRuntime(true);
  } catch (error) { setMessage(globalMessage, errorMessage(error), "error"); }
}

async function stopSessionRuntime() {
  if (!confirm("End the active runtime Session explicitly?")) return;
  try {
    await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/runtime/stop-session`, {
      method: "POST", json: { request_id: requestID() },
    });
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
