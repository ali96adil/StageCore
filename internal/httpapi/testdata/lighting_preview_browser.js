"use strict";

(async () => {
  const trace = [];
  const nativeFetch = window.fetch.bind(window);
  window.fetch = async (...args) => {
    const response = await nativeFetch(...args);
    trace.push({method: args[1]?.method || "GET", url: String(args[0]), status: response.status});
    return response;
  };
  let reported = false;
  const report = async (payload) => {
    if (reported) return;
    reported = true;
    await nativeFetch("/lighting-preview-result", {
      method: "POST", headers: {"Content-Type": "application/json"}, body: JSON.stringify(payload),
    });
    window.close();
  };
  const waitFor = async (predicate, label) => {
    const deadline = Date.now() + 10000;
    while (Date.now() < deadline) {
      if (predicate()) return;
      await new Promise((resolve) => setTimeout(resolve, 20));
    }
    throw new Error(`Timed out waiting for ${label}`);
  };
  window.addEventListener("error", (event) => report({status: "FAIL", error: event.message, trace}));
  window.addEventListener("unhandledrejection", (event) => report({status: "FAIL", error: String(event.reason), trace}));

  try {
    if (document.readyState === "loading") {
      await new Promise((resolve) => document.addEventListener("DOMContentLoaded", resolve, {once: true}));
    }
    const configResponse = await nativeFetch("/lighting-preview-config", {cache: "no-store"});
    if (!configResponse.ok) throw new Error(`browser configuration returned HTTP ${configResponse.status}`);
    const config = await configResponse.json();
    state.user = {username: "owner", role: "OWNER"};
    state.csrf = config.csrf;
    state.project = {project_id: config.project};
    sessionStorage.setItem("stagecore_csrf", state.csrf);
    showApp();
    await renderLightingCues();

    const newCue = document.getElementById("lightingNewCue");
    if (!newCue) throw new Error("Lighting Cue create control was not rendered");
    newCue.click();
    await waitFor(() => document.querySelector(".lighting-cue-editor"), "Lighting Cue editor");
    document.getElementById("lightingCueName").value = "Lighting Preview Recovery";
    const operation = document.getElementById("lightingCueOperation");
    operation.value = "LIGHTING_CHANNELS_SET";
    operation.dispatchEvent(new Event("change", {bubbles: true}));
    await waitFor(() => document.getElementById("lightingPreviewRun"), "manual preview controls");
    const raw = document.getElementById("lightingPreviewRaw");
    const output = document.getElementById("lightingPreviewResult");
    const setRaw = (value) => {
      raw.value = value;
      raw.dispatchEvent(new Event("input", {bubbles: true}));
    };
    const previewRequestCount = trace.length;

    setRaw("2=128");
    document.getElementById("lightingPreviewRun").click();
    const simulatedRun = output.textContent.includes("CH 2 = 128");
    document.getElementById("lightingPreviewStop").click();
    const simulatedStop = output.textContent.includes("CH 2 = 0");
    document.getElementById("lightingPreviewCopy").click();
    const unmappedError = output.textContent.includes("تحتاج") || output.textContent.includes("unmapped");

    setRaw("1=128");
    document.getElementById("lightingPreviewRun").click();
    const correctedRun = output.textContent.includes("CH 1 = 128");
    document.getElementById("lightingPreviewCopy").click();
    const correctedCopy = output.textContent.includes("تم نسخ القيم المطابقة للمسودة") || output.textContent.includes("Mapped values copied to the draft");
    const rows = [...document.querySelectorAll("#lightingLevelRows .lighting-level-row")];
    const alias = rows[0]?.querySelector(".lighting-level-alias")?.value || "";
    const level = Number(rows[0]?.querySelector(".lighting-level-value")?.value);
    await new Promise((resolve) => setTimeout(resolve, 40));
    const previewRequestURLs = trace.slice(previewRequestCount).map(({url}) => url);
    const previewNoRequests = previewRequestURLs.length === 0;
    if (!simulatedRun || !simulatedStop || !unmappedError || !correctedRun || !correctedCopy ||
      !previewNoRequests || alias !== "front_warm" || !Number.isFinite(level) || level < 49.9 || level > 50.3) {
      throw new Error(`preview recovery failed: run=${simulatedRun}, stop=${simulatedStop}, unmapped=${unmappedError}, correctedRun=${correctedRun}, copy=${correctedCopy}, noRequests=${previewNoRequests}, alias=${alias}, level=${level}`);
    }

    document.getElementById("lightingCueSave").click();
    await waitFor(() => document.querySelector(".lighting-cue-card h2")?.textContent === "Lighting Preview Recovery",
      "saved Lighting Cue in the Draft list");
    const savedCue = document.querySelector(".lighting-cue-card h2")?.textContent || "";
    await window.renderLightingCues();
    document.getElementById("lightingNewCue").click();
    await waitFor(() => document.querySelector(".lighting-cue-editor"), "second Lighting Cue editor");
    document.getElementById("lightingCueName").value = "Must Stay in Original Project";
    const armSaveGate = await nativeFetch("/lighting-preview-arm-save-gate", {method: "POST"});
    if (!armSaveGate.ok) throw new Error(`pending-save gate arm returned HTTP ${armSaveGate.status}`);
    document.getElementById("lightingCueSave").click();
    const saveReady = await nativeFetch("/lighting-preview-save-ready", {method: "POST"});
    if (!saveReady.ok) throw new Error(`pending-save gate returned HTTP ${saveReady.status}`);
    await window.openProject(config.other_project);
    await window.renderLightingCues();
    const saveRelease = await nativeFetch("/lighting-preview-save-release", {method: "POST"});
    if (!saveRelease.ok) throw new Error(`pending-save release returned HTTP ${saveRelease.status}`);
    await waitFor(() => trace.some(({method, url}) => method === "POST" &&
      url === `/api/v1/projects/${config.project}/lighting-controller/cue-actions`),
    "completed translation response for the original Project");
    await new Promise((resolve) => setTimeout(resolve, 250));
    const crossProjectCueWrites = trace.filter(({method, url}) => method !== "GET" &&
      url === `/api/v1/projects/${config.other_project}/cues`).map(({method, url}) => `${method} ${url}`);
    const projectSwitchBlockedStaleSave = crossProjectCueWrites.length === 0;
    await window.openProject(config.project);
    await window.renderLightingCues();
    const armReadGate = await nativeFetch("/lighting-preview-arm-read-gate", {method: "POST"});
    if (!armReadGate.ok) throw new Error(`lighting-read gate arm returned HTTP ${armReadGate.status}`);
    const staleLightingRender = window.renderLightingCues();
    const readReady = await nativeFetch("/lighting-preview-read-ready", {method: "POST"});
    if (!readReady.ok) throw new Error(`pending-lighting-read gate returned HTTP ${readReady.status}`);
    await window.openProject(config.other_project);
    await window.renderLightingCues();
    const readRelease = await nativeFetch("/lighting-preview-read-release", {method: "POST"});
    if (!readRelease.ok) throw new Error(`pending-lighting-read release returned HTTP ${readRelease.status}`);
    await staleLightingRender;
    const projectSwitchBlockedStaleLightingRender = state.project?.project_id === config.other_project &&
      !document.querySelector(".lighting-cue-card h2")?.textContent.includes(savedCue);
    const executionRequests = trace.filter(({method, url}) => method !== "GET" &&
      /\/api\/v1\/projects\/[^/]+\/(?:runtime\/(?:go|jump|start)|publish|lighting-controller\/(?:nodes|cue-actions)\/.*(?:apply|identify))(?:\?|$)/.test(url));
    await report({
      status: "PASS", simulated_run: simulatedRun, simulated_stop: simulatedStop,
      unmapped_error: unmappedError, corrected_copy: correctedCopy, preview_no_requests: previewNoRequests,
      saved_cue: savedCue,
      preview_request_urls: previewRequestURLs, execution_requests: executionRequests,
      project_switch_blocked_stale_save: projectSwitchBlockedStaleSave,
      project_switch_blocked_stale_lighting_render: projectSwitchBlockedStaleLightingRender,
      cross_project_cue_writes: crossProjectCueWrites,
    });
  } catch (error) {
    await report({status: "FAIL", error: String(error?.stack || error), trace});
  }
})();
