"use strict";

(async () => {
  const trace = [];
  const nativeFetch = window.fetch.bind(window);
  window.fetch = async (...args) => {
    const response = await nativeFetch(...args);
    const method = String(args[1]?.method || "GET").toUpperCase();
    const url = String(args[0]);
    if (url.startsWith("/api/v1/")) trace.push({method, url, status: response.status});
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
    await nativeFetch("/route-draft-history-result", {
      method: "POST", headers: {"Content-Type": "application/json"}, body: JSON.stringify(payload),
    });
    window.close();
  };

  try {
    if (document.readyState === "loading") {
      await new Promise((resolve) => document.addEventListener("DOMContentLoaded", resolve, {once: true}));
    }
    const response = await nativeFetch("/route-draft-history-config", {cache: "no-store"});
    if (!response.ok) throw new Error(`browser config returned HTTP ${response.status}`);
    const config = await response.json();
    state.user = {username: "owner", role: "OWNER"};
    state.csrf = config.csrf;
    state.project = {project_id: config.project, name: config.project_name};
    sessionStorage.setItem("stagecore_csrf", state.csrf);
    document.documentElement.lang = "ar";
    document.documentElement.dir = "rtl";
    showApp();
    await navigate("configuration");

    await waitFor(() => document.getElementById("routeHistoryUndo") || document.querySelector("#globalMessage:not(.hidden)"),
      "Route history controls or a visible page error");
    if (!document.getElementById("routeHistoryUndo")) {
      throw new Error(`Route configuration did not render controls: page=${state.page}; project=${JSON.stringify(state.project)}; message=${document.getElementById("globalMessage")?.textContent}; content=${document.getElementById("content")?.innerHTML.slice(0, 1600)}`);
    }
    const undo = document.getElementById("routeHistoryUndo");
    if (undo.disabled) throw new Error("Route Undo is disabled despite a persisted Draft change");
    if (document.documentElement.scrollWidth > window.innerWidth) throw new Error("Route workspace overflows the mobile RTL viewport");
    const initialUndoEnabled = !undo.disabled;
    undo.focus();
    if (document.activeElement !== undo) throw new Error("Route Undo did not receive keyboard focus");
    if (!undo.matches(":focus-visible")) throw new Error("Route Undo keyboard focus ring is not visible");
    const keyboardReady = await nativeFetch("/route-draft-history-key-ready", {method: "POST"});
    if (!keyboardReady.ok) throw new Error(`native keyboard gate returned HTTP ${keyboardReady.status}`);
    await waitFor(() => !document.querySelector(`.route-remove[data-route-id="${config.route}"]`),
      "Route removal after keyboard Undo");
    await waitFor(() => document.getElementById("routeHistoryRedo") && !document.getElementById("routeHistoryRedo").disabled,
      "Redo becoming available after Undo");

    const redo = document.getElementById("routeHistoryRedo");
    redo.scrollIntoView({block: "center"});
    const rect = redo.getBoundingClientRect();
    const touchReady = await nativeFetch("/route-draft-history-touch-ready", {
      method: "POST", headers: {"Content-Type": "application/json"},
      body: JSON.stringify({x: rect.left + rect.width / 2, y: rect.top + rect.height / 2}),
    });
    if (!touchReady.ok) throw new Error(`native touch gate returned HTTP ${touchReady.status}`);
    await waitFor(() => !!document.querySelector(`.route-remove[data-route-id="${config.route}"]`),
      "Route restoration after touch Redo");
    await waitFor(() => !document.getElementById("routeHistoryUndo")?.disabled,
      "Undo becoming available after Redo");

    const routeHistoryRequests = trace.filter(({url}) => url.includes("/route-draft-history")).map(({method, status}) => `${method}:${status}`);
    const unexpectedMutations = trace.filter(({method, url}) => method !== "GET" && !url.includes("/route-draft-history"));
    const overflow = document.documentElement.scrollWidth > window.innerWidth;
    await report({
      status: "PASS", undo_enabled: initialUndoEnabled, keyboard_undo: true, touch_redo: true,
      restored_same_route: !!document.querySelector(`.route-remove[data-route-id="${config.route}"]`),
      overflow, route_history_requests: routeHistoryRequests, unexpected_mutations: unexpectedMutations,
    });
  } catch (error) {
    await report({status: "FAIL", error: String(error?.stack || error), trace});
  }
})();
