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
  const sessions = await loadSessions();
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
  const error = action.error_code ? `<p class="muted" style="margin-top:8px"><strong>${esc(action.error_code)}</strong> · ${esc(action.response_summary || "No response summary")}</p>` : (action.response_summary ? `<p class="muted" style="margin-top:8px">${esc(action.response_summary)}</p>` : "");
  return `
    <article class="action-editor">
      <div class="section-title-row">
        <div><p class="eyebrow">ACTION · ${esc(action.capability_key || "UNKNOWN")}</p><h3>${esc(action.target_ref || "No target")}</h3></div>
        <div class="toolbar">${pill(action.result, memoryKind(action.result))}${interrupted}</div>
      </div>
      <div class="meta" style="margin-top:10px">
        <span>Ack ${esc(action.ack_level || "NONE")}</span>
        <span>${action.latency_ms === null || action.latency_ms === undefined ? "Latency —" : `${esc(action.latency_ms)} ms`}</span>
        <span>${esc(fmtDate(action.started_at))}</span>
      </div>
      ${error}
      <p class="mono muted" style="margin-top:8px">${esc(action.action_execution_id)}</p>
    </article>`;
}

async function renderSessionDetail(sessionID) {
  const detail = await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/sessions/${encodeURIComponent(sessionID)}`);
  const session = detail.session;
  const cues = detail.cue_executions || [];
  content.innerHTML = `
    <div class="page-head">
      <div><p class="eyebrow">SESSION TRACE · ${esc(session.session_type)}</p><h1>${esc(session.name || "Unnamed Session")}</h1><p>${esc(fmtDate(session.started_at))} → ${session.ended_at ? esc(fmtDate(session.ended_at)) : "active"}</p></div>
      <div class="toolbar">${pill(session.status, memoryKind(session.status))}<button id="backSessions" class="button" type="button">Back to Sessions</button></div>
    </div>
    <article class="card" style="margin-bottom:16px">
      <div class="meta"><span>Snapshot</span><span class="mono">${esc(session.runtime_snapshot_id)}</span></div>
      <p class="mono muted" style="margin-top:8px">Session ${esc(session.session_id)}</p>
    </article>
    <div class="grid cards">
      ${cues.length ? cues.map((cue) => `
        <article class="card">
          <div class="section-title-row">
            <div><p class="eyebrow">CUE ${esc(cue.display_label || "")}</p><h2>${esc(cue.name || cue.cue_id)}</h2></div>
            <div class="toolbar">${pill(cue.result, memoryKind(cue.result))}${cue.interrupted ? pill("INTERRUPTED", "bad") : ""}</div>
          </div>
          <div class="meta" style="margin-top:10px"><span>${esc(fmtDate(cue.started_at))}</span><span>Trigger ${esc(cue.trigger_source || "—")}</span></div>
          <p class="mono muted" style="margin-top:8px">${esc(cue.cue_execution_id)}</p>
          <div class="actions-editor" style="margin-top:14px">${(cue.actions || []).length ? cue.actions.map(actionTraceCard).join("") : `<div class="empty">Cue had no Action executions.</div>`}</div>
        </article>`).join("") : `<div class="empty">No Cue executions were recorded for this Session.</div>`}
    </div>`;
  el("backSessions").addEventListener("click", () => renderSessions().catch((error) => setMessage(globalMessage, errorMessage(error), "error")));
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
