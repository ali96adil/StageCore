"use strict";

const stagecoreMemoryNavigateBase = navigate;

function memoryKind(status) {
  if (["COMPLETED", "OPEN"].includes(status)) return "good";
  if (["ACTIVE", "RUNNING"].includes(status)) return "warn";
  if (["ABORTED", "FAILED", "TIMED_OUT", "CANCELLED"].includes(status)) return "bad";
  if (status === "RESOLVED") return "neutral";
  return "neutral";
}

function canWriteNotes() {
  return ["OWNER", "TECHNICIAN", "OPERATOR"].includes(state.user?.role);
}

async function loadSessions() {
  const payload = await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/sessions?limit=200`);
  return payload.sessions || [];
}

async function renderSessions() {
  const current = beginSessionMemoryRead();
  const sessions = await loadSessions();
  if (!current()) return;
  content.innerHTML = `
    <div class="page-head">
      <div><p class="eyebrow">SESSION MEMORY</p><h1>Rehearsal / Show history</h1><p>Structured runtime history stored on this Hub. Raw logs are not required for normal inspection.</p></div>
      <button id="refreshSessions" class="button" type="button">Refresh</button>
    </div>
    <div class="grid cards">
      ${sessions.length ? sessions.map((session) => `
        <article class="card">
          <div class="section-title-row">
            <div><p class="eyebrow">${esc(session.session_type)}</p><h3>${esc(session.name || "Unnamed Session")}</h3></div>
            ${pill(session.status, memoryKind(session.status))}
          </div>
          <div class="meta" style="margin-top:10px">
            <span>Started ${esc(fmtDate(session.started_at))}</span>
            <span>${session.ended_at ? `Ended ${esc(fmtDate(session.ended_at))}` : "Still active"}</span>
          </div>
          <p class="mono muted" style="margin-top:8px">${esc(session.session_id)}</p>
          <div class="toolbar" style="margin-top:12px"><button class="button open-session-trace" data-session-id="${esc(session.session_id)}" type="button">Open execution trace</button></div>
        </article>`).join("") : `<div class="empty">No rehearsal or show Sessions yet.</div>`}
    </div>`;
  el("refreshSessions").addEventListener("click", () => renderSessions().catch((error) => setMessage(globalMessage, errorMessage(error), "error")));
  content.querySelectorAll(".open-session-trace").forEach((button) => {
    button.addEventListener("click", () => renderSessionDetail(button.dataset.sessionId).catch((error) => setMessage(globalMessage, errorMessage(error), "error")));
  });
}

function actionTraceCard(action) {
  const interrupted = action.interrupted ? pill("INTERRUPTED", "bad") : "";
  const deliveryEvidence = {
    UNKNOWN: "External execution not observed",
    EXTERNAL_EXECUTION_UNKNOWN: "Transport sent; external execution unknown",
    OUTPUT_ACCEPTED: "Output reported acceptance",
    DEVICE_ACKNOWLEDGED: "Device acknowledged execution",
    APPLICATION_ACKNOWLEDGED: "Application acknowledged execution",
    STATE_VERIFIED: "Output state verified",
  }[action.delivery_evidence] || "External execution not observed";
  const error = action.error_code ? `<p class="muted" style="margin-top:8px"><strong>${esc(action.error_code)}</strong> · ${esc(action.response_summary || "No response summary")}</p>` : (action.response_summary ? `<p class="muted" style="margin-top:8px">${esc(action.response_summary)}</p>` : "");
  return `
    <article class="action-editor">
      <div class="section-title-row">
        <div><p class="eyebrow">ACTION · ${esc(action.capability_key || "UNKNOWN")}</p><h3>${esc(action.target_ref || "No target")}</h3></div>
        <div class="toolbar">${pill(action.result, memoryKind(action.result))}${interrupted}</div>
      </div>
      <div class="meta" style="margin-top:10px">
        <span>${esc(deliveryEvidence)}</span>
        <span>Ack ${esc(action.ack_level || "NONE")}</span>
        ${action.dispatch_attempts === null || action.dispatch_attempts === undefined ? "" : `<span>Hub dispatch attempts ${esc(action.dispatch_attempts)}</span>`}
        <span>${action.latency_ms === null || action.latency_ms === undefined ? "Latency —" : `${esc(action.latency_ms)} ms`}</span>
        <span>${esc(fmtDate(action.started_at))}</span>
      </div>
      ${error}
      <p class="mono muted" style="margin-top:8px">${esc(action.action_execution_id)}</p>
    </article>`;
}

function cueTraceCard(cue) {
  const started = Date.parse(cue.started_at);
  const completed = cue.completed_at ? Date.parse(cue.completed_at) : NaN;
  const durationMS = Number.isFinite(started) && Number.isFinite(completed) ? Math.max(0, completed - started) : null;
  return `
    <article class="card">
      <div class="section-title-row">
        <div><p class="eyebrow">CUE ${esc(cue.display_label || "")}</p><h2>${esc(cue.name || cue.cue_id)}</h2></div>
        <div class="toolbar">${pill(cue.result, memoryKind(cue.result))}${cue.interrupted ? pill("INTERRUPTED", "bad") : ""}</div>
      </div>
      <div class="meta" style="margin-top:10px"><span>${esc(fmtDate(cue.started_at))}</span><span>Trigger ${esc(cue.trigger_source || "—")}</span>${durationMS === null ? "" : `<span>Duration ${durationMS} ms</span>`}</div>
      <p class="mono muted" style="margin-top:8px">Cue execution ${esc(cue.cue_execution_id)} · Correlation ${esc(cue.correlation_id || "—")}</p>
      <div class="actions-editor" style="margin-top:14px">${(cue.actions || []).length ? cue.actions.map(actionTraceCard).join("") : `<div class="empty">Cue had no Action executions.</div>`}</div>
    </article>`;
}

const sessionEventLabels = {
  "rehearsal.started": "Rehearsal started",
  "rehearsal.stopped": "Rehearsal stopped",
  "rehearsal.force_stopped": "Rehearsal force-stopped",
  "show.entered": "SHOW entered",
  "show.exited": "SHOW exited",
  "show.force_exited": "SHOW force-exited",
  "runtime.emergency_blackout.applied": "Emergency Blackout applied",
  "runtime.emergency_blackout.cleared": "Emergency Blackout cleared",
  "runtime.emergency_blackout.clear_failed": "Emergency Blackout clear failed",
  "runtime.edit_blackout.applied": "Edit Blackout applied",
  "runtime.edit_blackout.cleared": "Edit Blackout cleared",
  "runtime.edit_blackout.clear_failed": "Edit Blackout clear failed",
  "stage_device.command.accepted": "Device command accepted",
  "stage_device.command.completed": "Device command completed",
  "stage_device.command.timed_out": "Device command timed out",
  "stage_device.command.cancelled": "Device command cancelled",
  "stage_device.command.rejected": "Device command rejected",
  "stage_device.command.failed": "Device command failed",
  "simulation.stopped": "Simulation stopped",
  "simulation.execution.completed": "Simulation action completed",
  "simulation.execution.failed": "Simulation action failed",
  "simulation.execution.timed_out": "Simulation action timed out",
  "simulation.execution.cancelled": "Simulation action cancelled",
};

const sessionWarningLabels = {
  degraded_start: "Degraded start",
  device_scope: "Device scope warning",
  preflight: "Preflight warning",
};

function sessionEventTimelineMarkup(events, category = "all") {
  const filtered = category === "all" ? events : events.filter((event) => event.category === category);
  if (!filtered.length) return `<div class="empty">No runtime events match this filter.</div>`;
  return filtered.map((event) => `
    <article class="action-editor">
      <div class="section-title-row">
        <div><p class="eyebrow">${esc(event.category || "event")} · ${esc(event.priority || "")}</p><h3>${esc(sessionEventLabels[event.event_type] || event.event_type)}</h3></div>
        ${event.warning_count ? `<div class="toolbar">${pill(`${event.warning_count} warning${event.warning_count === 1 ? "" : "s"}`, "warn")}${(event.warning_kinds || []).map((kind) => pill(sessionWarningLabels[kind] || "Warning", "warn")).join("")}</div>` : ""}
      </div>
      <div class="meta" style="margin-top:8px"><span>${esc(fmtDate(event.occurred_at))}</span>${event.correlation_id ? `<span>Correlation ${esc(event.correlation_id)}</span>` : ""}</div>
      <p class="mono muted" style="margin-top:6px">${esc(event.event_id)}</p>
    </article>`).join("");
}

function sessionTraceExportProjection(session, cues, events) {
  return {
    export_version: 1,
    session: {
      session_id: session.session_id,
      project_id: session.project_id,
      session_type: session.session_type,
      name: session.name,
      runtime_snapshot_id: session.runtime_snapshot_id,
      started_at: session.started_at,
      ended_at: session.ended_at || null,
      status: session.status,
    },
    cue_executions: cues.map((cue) => ({
      cue_execution_id: cue.cue_execution_id,
      cue_id: cue.cue_id,
      display_label: cue.display_label,
      name: cue.name,
      correlation_id: cue.correlation_id,
      trigger_source: cue.trigger_source,
      started_at: cue.started_at,
      completed_at: cue.completed_at || null,
      result: cue.result,
      interrupted: Boolean(cue.interrupted),
      actions: (cue.actions || []).map((action) => ({
        action_execution_id: action.action_execution_id,
        action_id: action.action_id,
        target_ref: action.target_ref,
        capability_key: action.capability_key,
        started_at: action.started_at,
        completed_at: action.completed_at || null,
        result: action.result,
        latency_ms: action.latency_ms ?? null,
        ack_level: action.ack_level,
        delivery_evidence: action.delivery_evidence,
        dispatch_attempts: action.dispatch_attempts ?? null,
        error_code: action.error_code || null,
        interrupted: Boolean(action.interrupted),
      })),
    })),
    events,
  };
}

function sessionComparisonMetrics(session, cues, events) {
  const started = Date.parse(session.started_at || "");
  const ended = Date.parse(session.ended_at || "");
  const actions = cues.flatMap((cue) => cue.actions || []);
  return {
    duration_ms: Number.isFinite(started) && Number.isFinite(ended) ? Math.max(0, ended - started) : null,
    cue_count: cues.length,
    action_count: actions.length,
    failed_cues: cues.filter((cue) => cue.result !== "COMPLETED").length,
    failed_actions: actions.filter((action) => action.result !== "COMPLETED").length,
    interrupted_cues: cues.filter((cue) => cue.interrupted).length,
    safety_events: events.filter((event) => event.category === "safety").length,
    device_events: events.filter((event) => event.category === "device").length,
    snapshot_id: session.runtime_snapshot_id || "",
  };
}

function sessionComparisonMarkup(primarySession, primaryCues, primaryEvents, otherSession, otherCues, otherEvents) {
  const current = sessionComparisonMetrics(primarySession, primaryCues, primaryEvents);
  const compared = sessionComparisonMetrics(otherSession, otherCues, otherEvents);
  const rows = [
    ["Duration", current.duration_ms, compared.duration_ms, true],
    ["Cue executions", current.cue_count, compared.cue_count],
    ["Action executions", current.action_count, compared.action_count],
    ["Non-completed Cues", current.failed_cues, compared.failed_cues],
    ["Non-completed Actions", current.failed_actions, compared.failed_actions],
    ["Interrupted Cues", current.interrupted_cues, compared.interrupted_cues],
    ["Safety events", current.safety_events, compared.safety_events],
    ["Device events", current.device_events, compared.device_events],
  ];
  const value = (metric, duration = false) => metric === null ? "—" : duration ? `${(metric / 1000).toFixed(1)} s` : String(metric);
  const delta = (left, right, duration = false) => {
    if (left === null || right === null) return "—";
    const difference = left - right;
    const label = difference > 0 ? `+${difference}` : String(difference);
    return duration ? `${difference > 0 ? "+" : ""}${(difference / 1000).toFixed(1)} s` : label;
  };
  return `
    <div class="section-title-row"><div><h3>${esc(primarySession.name || "Current Session")} · ${esc(otherSession.name || "Compared Session")}</h3><p class="muted">Snapshot ${current.snapshot_id === compared.snapshot_id ? "matches" : "differs"}</p></div></div>
    <div class="table-wrap"><table><thead><tr><th>Measure</th><th>Current</th><th>Compared</th><th>Difference</th></tr></thead><tbody>
      ${rows.map(([label, left, right, duration]) => `<tr><th scope="row">${esc(label)}</th><td>${esc(value(left, duration))}</td><td>${esc(value(right, duration))}</td><td>${esc(delta(left, right, duration))}</td></tr>`).join("")}
    </tbody></table></div>`;
}

function downloadSessionTrace(session, cues, events) {
  const json = JSON.stringify(sessionTraceExportProjection(session, cues, events), null, 2);
  const blobURL = URL.createObjectURL(new Blob([json], { type: "application/json" }));
  const link = document.createElement("a");
  const safeName = String(session.name || session.session_id || "session").replace(/[^a-z0-9_-]+/gi, "-").replace(/^-+|-+$/g, "").slice(0, 64) || "session";
  link.href = blobURL;
  link.download = `stagecore-session-${safeName}.json`;
  link.click();
  URL.revokeObjectURL(blobURL);
}

async function renderRehearsalApproval(container, projectID, session) {
  const user = state.user;
  // The same approval card may refresh twice before its first read returns.
  // Only the newest request may render or submit a selection.
  const requestToken = {};
  container.stagecoreApprovalRequest = requestToken;
  const current = () => container.stagecoreApprovalRequest === requestToken &&
    container.isConnected && state.user === user &&
    state.project?.project_id === projectID && state.page === "sessions";
  const base = `/api/v1/projects/${encodeURIComponent(projectID)}/timing-intelligence`;
  try {
    const { report } = await api(`${base}?runtime_snapshot_id=${encodeURIComponent(session.runtime_snapshot_id)}&session_id=${encodeURIComponent(session.session_id)}`);
    if (!current()) return;
    const candidate = (report.sessions || []).find((item) => item.session_id === session.session_id && item.runtime_snapshot_id === session.runtime_snapshot_id);
    if (!candidate) {
      container.textContent = "No timing approval record is available for this rehearsal.";
      return;
    }
    container.innerHTML = `
      <p>${esc(candidate.observation_count)} timing observations · ${candidate.effective ? "Used for timing" : "Excluded from timing"}</p>
      <p class="muted">${esc(candidate.reason || "")}</p>
      ${candidate.eligible && f028CanEditSelection() ? `<label>Timing approval<select>
        <option value="AUTO">Not approved</option><option value="INCLUDE">Approved</option><option value="EXCLUDE">Excluded</option>
      </select></label>` : `<p>Selection: ${esc(candidate.selection_mode)}</p>`}`;
    const select = container.querySelector("select");
    if (!select) return;
    select.value = candidate.selection_mode;
    select.addEventListener("change", async () => {
      if (!current()) return;
      if (candidate.selection_mode === "INCLUDE" && !window.confirm(f028T("timing.confirm_revoke"))) {
        select.value = candidate.selection_mode;
        return;
      }
      select.disabled = true;
      try {
        await api(`${base}/sessions/${encodeURIComponent(session.session_id)}`, { method: "PUT", json: { mode: select.value, review: { runtime_snapshot_id: session.runtime_snapshot_id, mode: candidate.selection_mode } } });
        if (current()) await renderRehearsalApproval(container, projectID, session);
      } catch (error) {
        if (!current()) return;
        setMessage(globalMessage, errorMessage(error), "error");
        if (error.status === 409 || error.status === 403) {
          await renderRehearsalApproval(container, projectID, session);
          return;
        }
        select.value = candidate.selection_mode;
        select.disabled = false;
      }
    });
  } catch (error) {
    if (!current()) return;
    container.textContent = errorMessage(error);
  }
}

function beginSessionMemoryRead() {
  const request = { user: state.user, projectID: state.project.project_id };
  state.sessionMemoryRequest = request;
  return () => state.sessionMemoryRequest === request && state.user === request.user &&
    state.project?.project_id === request.projectID && state.page === "sessions";
}

async function renderSessionDetail(sessionID, executionID = "", eventCategory = "all") {
  const current = beginSessionMemoryRead();
  const projectID = state.project.project_id;
  const detail = await api(`/api/v1/projects/${encodeURIComponent(projectID)}/sessions/${encodeURIComponent(sessionID)}`);
  if (!current()) return;
  const session = detail.session;
  const traces = detail.cue_executions || [];
  const events = detail.events || [];
  const rehearsalSummary = !executionID && session.session_type === "REHEARSAL" && session.ended_at;
  const interrupted = traces.filter((cue) => cue.interrupted).length;
  const cues = executionID ? traces.filter((cue) => cue.cue_execution_id === executionID) : traces;
  content.innerHTML = `
    <div class="page-head">
      <div><p class="eyebrow">SESSION TRACE · ${esc(session.session_type)}</p><h1>${esc(session.name || "Unnamed Session")}</h1><p>${esc(fmtDate(session.started_at))} → ${session.ended_at ? esc(fmtDate(session.ended_at)) : "active"}</p></div>
      <div class="toolbar">${pill(session.status, memoryKind(session.status))}<button id="backSessions" class="button" type="button">Back to Sessions</button></div>
    </div>
    <article class="card" style="margin-bottom:16px">
      <div class="meta"><span>Snapshot</span><span class="mono">${esc(session.runtime_snapshot_id)}</span></div>
      <p class="mono muted" style="margin-top:8px">Session ${esc(session.session_id)}</p>
    </article>
    <section class="card" style="margin-bottom:16px">
      <div class="section-title-row"><div><p class="eyebrow">SESSION MEMORY</p><h2>Runtime timeline</h2><p class="muted">Recorded session, safety, device and simulation events. Connection state is shown only when an explicit event exists.</p></div><div class="toolbar"><label>Filter<select id="sessionEventCategory"><option value="all">All events</option><option value="session" ${eventCategory === "session" ? "selected" : ""}>Session</option><option value="safety" ${eventCategory === "safety" ? "selected" : ""}>Safety</option><option value="device" ${eventCategory === "device" ? "selected" : ""}>Device</option><option value="simulation" ${eventCategory === "simulation" ? "selected" : ""}>Simulation</option></select></label><button id="exportSessionTrace" class="button ghost" type="button">Export JSON</button></div></div>
      <div class="actions-editor" style="margin-top:14px">${sessionEventTimelineMarkup(events, eventCategory)}</div>
    </section>
    ${!executionID ? `<section class="card" style="margin-bottom:16px"><div class="section-title-row"><div><p class="eyebrow">SESSION MEMORY</p><h2>Session comparison</h2><p class="muted">Compare recorded rehearsal metrics from this Project. No execution or device state is changed.</p></div><button id="prepareSessionCompare" class="button ghost" type="button">Compare another session</button></div><div id="sessionComparisonPanel" style="margin-top:12px"></div></section>` : ""}
    ${rehearsalSummary ? `<article class="card" style="margin-bottom:16px">
      <h2>Rehearsal summary</h2>
      <p>${traces.length} recorded Cue executions · ${interrupted} interrupted</p>
      <div id="rehearsalApproval"><button type="button" class="button">Review timing approval</button></div>
    </article>` : ""}
    <div class="grid cards">
      ${cues.length ? cues.map(cueTraceCard).join("") : `<div class="empty">${executionID ? "This execution was not found in the session trace." : "No Cue executions were recorded for this Session."}</div>`}
    </div>`;
  const approval = el("rehearsalApproval");
  approval?.querySelector("button").addEventListener("click", () => {
    approval.textContent = "Loading timing approval…";
    renderRehearsalApproval(approval, projectID, session);
  });
  el("sessionEventCategory")?.addEventListener("change", (event) => renderSessionDetail(sessionID, executionID, event.target.value).catch((error) => setMessage(globalMessage, errorMessage(error), "error")));
  el("exportSessionTrace")?.addEventListener("click", () => downloadSessionTrace(session, traces, events));
  el("prepareSessionCompare")?.addEventListener("click", () => prepareSessionComparison(session, traces, events, projectID, current).catch((error) => setMessage(globalMessage, errorMessage(error), "error")));
  el("backSessions").addEventListener("click", () => renderSessions().catch((error) => setMessage(globalMessage, errorMessage(error), "error")));
}

async function prepareSessionComparison(primarySession, primaryCues, primaryEvents, projectID, current) {
  const panel = el("sessionComparisonPanel");
  if (!panel) return;
  panel.textContent = "Loading completed Sessions…";
  const sessions = await loadSessions();
  if (!current()) return;
  const candidates = sessions.filter((item) => item.session_id !== primarySession.session_id && item.session_type === primarySession.session_type && item.ended_at && ["COMPLETED", "ABORTED"].includes(item.status));
  if (!candidates.length) {
    panel.textContent = "No other completed Session of this type is available in this Project.";
    return;
  }
  panel.innerHTML = `<label>Compare with<select id="sessionComparisonChoice"><option value="">Choose a Session</option>${candidates.map((item) => `<option value="${esc(item.session_id)}">${esc(item.name || item.session_id)} · ${esc(fmtDate(item.started_at))}</option>`).join("")}</select></label><div id="sessionComparisonResult" style="margin-top:12px"></div>`;
  const choice = el("sessionComparisonChoice");
  choice?.addEventListener("change", async () => {
    const result = el("sessionComparisonResult");
    if (!result || !choice.value) return;
    const request = {};
    panel.stagecoreComparisonRequest = request;
    result.textContent = "Loading comparison…";
    try {
      const detail = await api(`/api/v1/projects/${encodeURIComponent(projectID)}/sessions/${encodeURIComponent(choice.value)}`);
      if (!current() || panel.stagecoreComparisonRequest !== request) return;
      result.innerHTML = sessionComparisonMarkup(primarySession, primaryCues, primaryEvents, detail.session, detail.cue_executions || [], detail.events || []);
    } catch (error) {
      if (current() && panel.stagecoreComparisonRequest === request) result.textContent = errorMessage(error);
    }
  });
}

async function loadNoteChoices() {
  const [sessions, cuePayload] = await Promise.all([
    loadSessions(),
    api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/cues`),
  ]);
  return { sessions, cues: cuePayload.cues || [] };
}

function choiceOptions(items, idKey, labeler, selected = "") {
  return `<option value="">All / none</option>` + items.map((item) => `<option value="${esc(item[idKey])}" ${item[idKey] === selected ? "selected" : ""}>${esc(labeler(item))}</option>`).join("");
}

async function renderNotes(filters = {}) {
  const choices = await loadNoteChoices();
  const params = new URLSearchParams();
  if (filters.status) params.set("status", filters.status);
  if (filters.category) params.set("category", filters.category);
  if (filters.sessionID) params.set("session_id", filters.sessionID);
  if (filters.cueID) params.set("cue_id", filters.cueID);
  const query = params.toString() ? `?${params}` : "";
  const payload = await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/notes${query}`);
  const notes = payload.notes || [];
  const writable = canWriteNotes();
  content.innerHTML = `
    <div class="page-head">
      <div><p class="eyebrow">NOTES</p><h1>Rehearsal / Show notes</h1><p>Lightweight operator notes with OPEN / RESOLVED lifecycle.</p></div>
      ${writable ? `<button id="newNoteButton" class="button primary" type="button">New Note</button>` : pill("READ ONLY", "neutral")}
    </div>
    <section class="card" style="margin-bottom:16px">
      <div class="form-grid four">
        <label>Status<select id="noteFilterStatus"><option value="">All</option><option value="OPEN" ${filters.status === "OPEN" ? "selected" : ""}>OPEN</option><option value="RESOLVED" ${filters.status === "RESOLVED" ? "selected" : ""}>RESOLVED</option></select></label>
        <label>Category<input id="noteFilterCategory" value="${esc(filters.category || "")}" placeholder="video"></label>
        <label>Session<select id="noteFilterSession">${choiceOptions(choices.sessions, "session_id", (item) => `${item.session_type} · ${item.name || fmtDate(item.started_at)}`, filters.sessionID || "")}</select></label>
        <label>Cue<select id="noteFilterCue">${choiceOptions(choices.cues, "cue_id", (item) => `${item.display_label || ""} · ${item.name}`, filters.cueID || "")}</select></label>
      </div>
      <div class="toolbar" style="margin-top:12px"><button id="applyNoteFilters" class="button" type="button">Apply filters</button><button id="clearNoteFilters" class="button ghost" type="button">Clear</button></div>
    </section>
    ${writable ? `
      <section id="noteEditor" class="card hidden" style="margin-bottom:16px">
        <div class="section-title-row"><div><p class="eyebrow">NOTE EDITOR</p><h2 id="noteEditorTitle">New Note</h2></div><button id="cancelNoteEdit" class="button ghost" type="button">Cancel</button></div>
        <form id="noteForm" style="margin-top:14px">
          <input id="editNoteID" type="hidden">
          <div class="form-grid three">
            <label>Category<input id="noteCategory" maxlength="80" placeholder="video / timing / blocking"></label>
            <label>Session<select id="noteSession"><option value="">No Session</option>${choices.sessions.map((item) => `<option value="${esc(item.session_id)}">${esc(`${item.session_type} · ${item.name || fmtDate(item.started_at)}`)}</option>`).join("")}</select></label>
            <label>Cue<select id="noteCue"><option value="">No Cue</option>${choices.cues.map((item) => `<option value="${esc(item.cue_id)}">${esc(`${item.display_label || ""} · ${item.name}`)}</option>`).join("")}</select></label>
          </div>
          <label>Note<textarea id="noteBody" rows="4" required maxlength="4000"></textarea></label>
          <div class="toolbar" style="margin-top:12px"><button class="button primary" type="submit">Save Note</button></div>
        </form>
      </section>` : ""}
    <div class="grid cards">
      ${notes.length ? notes.map((note) => `
        <article class="card note-card" data-note-id="${esc(note.note_id)}">
          <div class="section-title-row">
            <div><p class="eyebrow">${esc(note.category || "GENERAL")}</p><h3>${esc(note.body)}</h3></div>
            ${pill(note.status, memoryKind(note.status))}
          </div>
          <div class="meta" style="margin-top:10px"><span>By ${esc(note.created_by || "—")}</span><span>Updated ${esc(fmtDate(note.updated_at))}</span></div>
          ${note.session_id ? `<p class="mono muted" style="margin-top:8px">Session ${esc(note.session_id)}</p>` : ""}
          ${note.cue_id ? `<p class="mono muted">Cue ${esc(note.cue_id)}</p>` : ""}
          ${writable ? `<div class="toolbar" style="margin-top:12px"><button class="button edit-note" type="button">Edit</button>${note.status === "OPEN" ? `<button class="button resolve-note" type="button">Resolve</button>` : `<button class="button reopen-note" type="button">Reopen</button>`}</div>` : ""}
        </article>`).join("") : `<div class="empty">No Notes match these filters.</div>`}
    </div>`;

  const currentFilters = () => ({
    status: el("noteFilterStatus").value,
    category: el("noteFilterCategory").value.trim(),
    sessionID: el("noteFilterSession").value,
    cueID: el("noteFilterCue").value,
  });
  el("applyNoteFilters").addEventListener("click", () => renderNotes(currentFilters()).catch((error) => setMessage(globalMessage, errorMessage(error), "error")));
  el("clearNoteFilters").addEventListener("click", () => renderNotes({}).catch((error) => setMessage(globalMessage, errorMessage(error), "error")));

  if (!writable) return;
  const editor = el("noteEditor");
  const resetEditor = () => {
    el("editNoteID").value = "";
    el("noteCategory").value = "";
    el("noteSession").value = "";
    el("noteCue").value = "";
    el("noteSession").disabled = false;
    el("noteCue").disabled = false;
    el("noteBody").value = "";
    el("noteEditorTitle").textContent = "New Note";
  };
  el("newNoteButton").addEventListener("click", () => { resetEditor(); editor.classList.remove("hidden"); el("noteBody").focus(); });
  el("cancelNoteEdit").addEventListener("click", () => { resetEditor(); editor.classList.add("hidden"); });
  el("noteForm").addEventListener("submit", async (event) => {
    event.preventDefault();
    try {
      const noteID = el("editNoteID").value;
      if (noteID) {
        await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/notes/${encodeURIComponent(noteID)}`, {
          method: "PATCH", json: { category: el("noteCategory").value.trim(), body: el("noteBody").value.trim() },
        });
      } else {
        await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/notes`, {
          method: "POST", json: {
            category: el("noteCategory").value.trim(), body: el("noteBody").value.trim(),
            session_id: el("noteSession").value || null, cue_id: el("noteCue").value || null,
          },
        });
      }
      setMessage(globalMessage, noteID ? "Note updated." : "Note created.", "success");
      await renderNotes(currentFilters());
    } catch (error) { setMessage(globalMessage, errorMessage(error), "error"); }
  });
  content.querySelectorAll(".edit-note").forEach((button) => {
    button.addEventListener("click", () => {
      const card = button.closest(".note-card");
      const note = notes.find((item) => item.note_id === card.dataset.noteId);
      if (!note) return;
      el("editNoteID").value = note.note_id;
      el("noteCategory").value = note.category || "";
      el("noteSession").value = note.session_id || "";
      el("noteCue").value = note.cue_id || "";
      el("noteSession").disabled = true;
      el("noteCue").disabled = true;
      el("noteBody").value = note.body;
      el("noteEditorTitle").textContent = "Edit Note";
      editor.classList.remove("hidden");
      el("noteBody").focus();
    });
  });
  content.querySelectorAll(".resolve-note, .reopen-note").forEach((button) => {
    button.addEventListener("click", async () => {
      const noteID = button.closest(".note-card").dataset.noteId;
      const action = button.classList.contains("resolve-note") ? "resolve" : "reopen";
      try {
        await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/notes/${encodeURIComponent(noteID)}/${action}`, { method: "POST" });
        await renderNotes(currentFilters());
      } catch (error) { setMessage(globalMessage, errorMessage(error), "error"); }
    });
  });
}

navigate = async function stagecoreMemoryNavigate(page) {
  if (page !== "sessions" && page !== "notes") return stagecoreMemoryNavigateBase(page);
  if (!state.project) return;
  setPage(page);
  setMessage(globalMessage, "");
  try {
    if (page === "sessions") await renderSessions();
    else await renderNotes();
  } catch (error) {
    setMessage(globalMessage, errorMessage(error), "error");
  }
};
