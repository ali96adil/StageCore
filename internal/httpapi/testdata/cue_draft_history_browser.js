"use strict";

(async () => {
  const trace = [];
  const nativeFetch = window.fetch.bind(window);
  window.fetch = async (...args) => {
    const response = await nativeFetch(...args);
    trace.push({method: args[1]?.method || "GET", url: String(args[0]), status: response.status});
    return response;
  };
  const waitFor = async (predicate, label) => {
    const deadline = Date.now() + 10000;
    while (Date.now() < deadline) {
      if (predicate()) return;
      await new Promise((resolve) => setTimeout(resolve, 20));
    }
    throw new Error(`Timed out waiting for ${label}`);
  };
  const report = async (payload) => {
    await nativeFetch("/cue-draft-history-result", {
      method: "POST", headers: {"Content-Type": "application/json"}, body: JSON.stringify(payload),
    });
    window.close();
  };

  try {
    if (document.readyState === "loading") {
      await new Promise((resolve) => document.addEventListener("DOMContentLoaded", resolve, {once: true}));
    }
    const response = await nativeFetch("/cue-draft-history-config", {cache: "no-store"});
    if (!response.ok) throw new Error(`browser config returned HTTP ${response.status}`);
    const config = await response.json();
    state.user = {username: "owner", role: "OWNER"};
    state.csrf = config.csrf;
    state.project = {project_id: config.project, name: config.project_name};
    sessionStorage.setItem("stagecore_csrf", state.csrf);
    showApp();
    await navigate("cues");

    const toolbar = document.getElementById("sc009ProjectDraftHistory");
    await waitFor(() => toolbar && toolbar.querySelectorAll("button").length === 2,
      "persistent Cue Draft history toolbar");
    const buttons = toolbar.querySelectorAll("button");
    const undo = buttons[0];
    const historyStatus = toolbar.querySelector('[role="status"]');
    await waitFor(() => historyStatus?.textContent.trim(), "persisted history status");
    const initialUndoEnabled = !undo.disabled;
    if (!initialUndoEnabled) throw new Error("Undo was disabled despite a persisted Draft change");
    undo.focus();
    if (document.activeElement !== undo) throw new Error("Undo button did not receive keyboard focus");
    const keyboardReady = await nativeFetch("/cue-draft-history-key-ready", {method: "POST"});
    if (!keyboardReady.ok) throw new Error(`native keyboard gate returned HTTP ${keyboardReady.status}`);
    await waitFor(() => !document.querySelector(`#content tr[data-cue-id="${config.created_cue}"]`),
      "Cue removal after keyboard Undo");

    await waitFor(() => {
      const toolbar = document.getElementById("sc009ProjectDraftHistory");
      const redo = toolbar?.querySelectorAll("button")[1];
      return redo && !redo.disabled;
    }, "Redo becoming available after Undo");
    const afterUndoToolbar = document.getElementById("sc009ProjectDraftHistory");
    const redo = afterUndoToolbar.querySelectorAll("button")[1];
    const rect = redo.getBoundingClientRect();
    const touchReady = await nativeFetch("/cue-draft-history-touch-ready", {
      method: "POST", headers: {"Content-Type": "application/json"},
      body: JSON.stringify({x: rect.left + rect.width / 2, y: rect.top + rect.height / 2}),
    });
    if (!touchReady.ok) throw new Error(`native touch gate returned HTTP ${touchReady.status}`);
    await waitFor(() => !!document.querySelector(`#content tr[data-cue-id="${config.created_cue}"]`),
      "Cue restoration after touch Redo");
    await waitFor(() => !document.getElementById("sc009ProjectDraftHistory")?.querySelectorAll("button")[0]?.disabled,
      "Undo becoming available after Redo");

    const executionRequests = trace.filter(({url, method}) =>
      method !== "GET" && /\/api\/v1\/projects\/[^/]+\/(?:runtime\/(?:go|jump|start)|publish)(?:\?|$)/.test(url));
    const historyRequests = trace.filter(({url}) => url.includes("/cue-draft-history")).map(({method, status}) => `${method}:${status}`);
    const overflow = document.documentElement.scrollWidth > window.innerWidth;
    await report({
      status: "PASS", undo_enabled: initialUndoEnabled, redo_after_undo: true,
      keyboard_undo: true, touch_redo: true,
      restored_same_cue: !!document.querySelector(`#content tr[data-cue-id="${config.created_cue}"]`),
      overflow, history_requests: historyRequests, execution_requests: executionRequests,
    });
  } catch (error) {
    await report({status: "FAIL", error: String(error?.stack || error), trace});
  }
})();
