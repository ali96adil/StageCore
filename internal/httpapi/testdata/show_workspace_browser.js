"use strict";

(async () => {
  const seededSessions = [];
  const report = async (value) => {
    await fetch("/show-workspace-result", {
      method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(value),
    });
    window.close();
  };

  try {
    if (document.readyState === "loading") {
      await new Promise((resolve) => document.addEventListener("DOMContentLoaded", resolve, { once: true }));
    }
    await fetch("/show-workspace-viewport-ready", { method: "POST" });
    const configResponse = await fetch("/show-workspace-config", { cache: "no-store" });
    if (!configResponse.ok) throw new Error(`test configuration returned HTTP ${configResponse.status}`);
    const config = await configResponse.json();
    state.user = { username: "owner", role: "OWNER" };
    state.csrf = config.csrf;
    sessionStorage.setItem("stagecore_csrf", state.csrf);
    document.documentElement.lang = "ar";
    document.documentElement.dir = "rtl";
    if (window.innerWidth > 390) throw new Error(`mobile viewport was not applied: ${window.innerWidth}px`);
    const assertNoHorizontalOverflow = (stage) => {
      const width = Math.max(document.documentElement.scrollWidth, document.body.scrollWidth);
      if (width > window.innerWidth + 1) throw new Error(`${stage} overflows the ${window.innerWidth}px RTL viewport: ${width}px`);
    };
    const setViewport = async (width, height) => {
      await fetch("/show-workspace-resize-ready", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ width, height }),
      });
      await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
      if (window.innerWidth !== width) throw new Error(`viewport resize applied ${window.innerWidth}px, expected ${width}px`);
    };
    const assertResponsiveAtAllWidths = async (stage) => {
      assertNoHorizontalOverflow(`${stage} at 390px`);
      for (const [width, height] of [[768, 1024], [1280, 800]]) {
        await setViewport(width, height);
        assertNoHorizontalOverflow(`${stage} at ${width}px`);
      }
      await setViewport(390, 844);
    };
    const keyboardNavigate = async (page) => {
      const nav = document.querySelector(`[data-page="${page}"]`);
      if (!nav) throw new Error(`${page} navigation is unavailable for ${state.user.role}`);
      nav.focus();
      await fetch("/show-workspace-key-ready", { method: "POST" });
      const deadline = Date.now() + 5000;
      const pageReady = () => page === "cues"
        ? document.querySelector("#content h1")?.textContent.includes("Cues")
        : !!document.getElementById("startShowButton");
      while (Date.now() < deadline && !pageReady()) await new Promise((resolve) => setTimeout(resolve, 25));
      if (state.page !== page || !pageReady()) throw new Error(`native keyboard did not open ${page} for ${state.user.role}`);
      if (!nav.matches(":focus-visible")) throw new Error(`native keyboard focus is not visible for ${state.user.role}`);
    };
    const touchNavigate = async (page) => {
      const nav = document.querySelector(`[data-page="${page}"]`);
      if (!nav) throw new Error(`${page} navigation is unavailable for ${state.user.role}`);
      const bounds = nav.getBoundingClientRect();
      await fetch("/show-workspace-touch-ready", {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ x: bounds.left + bounds.width / 2, y: bounds.top + bounds.height / 2 }),
      });
      const deadline = Date.now() + 5000;
      const pageReady = () => page === "cues"
        ? document.querySelector("#content h1")?.textContent.includes("Cues")
        : !!document.getElementById("startShowButton");
      while (Date.now() < deadline && !pageReady()) await new Promise((resolve) => setTimeout(resolve, 25));
      if (state.page !== page || !pageReady()) throw new Error(`emulated touch did not open ${page} for ${state.user.role}`);
    };

    f017RememberPage(config.project, "cues");
    if (f017PreferredPage(config.project) !== "cues") throw new Error("fixture did not remember the authoring page");
    await openProject(config.project);

    if (state.page !== "runtime") throw new Error(`active SHOW opened ${state.page}, expected runtime`);
    if (!state.showSessionActive) throw new Error("active SHOW state was not reflected in the operator UI");
    if (document.getElementById("appView").dataset.showSession !== "active") throw new Error("SHOW presentation marker is missing");
    await assertResponsiveAtAllWidths("Owner active SHOW workspace");
    for (const id of ["goButton", "stopCueButton", "emergencyBlackoutButton"]) {
      if (!document.getElementById(id)) throw new Error(`runtime safety control ${id} is missing`);
    }

    const sidebar = document.querySelector(".sidebar");
    const profile = document.getElementById("workspaceProfileControl");
    if (getComputedStyle(sidebar).display !== "none") throw new Error("workspace navigation remained visible during SHOW");
    if (getComputedStyle(profile).display !== "none") throw new Error("workspace profile control remained visible during SHOW");

    const originalFetch = window.fetch.bind(window);
    const postRequests = [];
    const apiMutations = [];
    const fetchTrace = [];
    window.fetch = async (input, init = {}) => {
      const method = String(init.method || "GET").toUpperCase();
      const url = String(input);
      if (method === "POST" && url.startsWith("/api/v1/")) postRequests.push(url);
      if (method !== "GET" && url.startsWith("/api/v1/")) apiMutations.push(`${method} ${url}`);
      const response = await originalFetch(input, init);
      fetchTrace.push(`${method} ${url} -> ${response.status}`);
      return response;
    };
    window.confirm = () => true;
    document.getElementById("stopSessionButton").click();
    const stopDeadline = Date.now() + 5000;
    while (Date.now() < stopDeadline && document.getElementById("appView").dataset.showSession !== "inactive") {
      await new Promise((resolve) => setTimeout(resolve, 25));
    }
    if (document.getElementById("appView").dataset.showSession !== "inactive") throw new Error("normal SHOW stop did not clear the active presentation");
    if (document.getElementById("stopSessionButton")) throw new Error("runtime still shows an active Session after SHOW stop");
    if (postRequests.length !== 1 || !postRequests[0].includes("/runtime/stop-session")) {
      throw new Error(`SHOW exit made unexpected mutation requests: ${postRequests.join(", ")}`);
    }
    if (getComputedStyle(sidebar).display === "none") throw new Error("workspace navigation did not return after SHOW presentation cleared");
    if (getComputedStyle(profile).display === "none") throw new Error("workspace profile control did not return after SHOW presentation cleared");
    assertNoHorizontalOverflow("stopped SHOW workspace");

    const previewResponse = await fetch("/show-workspace-seed-preview", { method: "POST" });
    if (!previewResponse.ok) throw new Error(`Preview rehearsal fixture returned HTTP ${previewResponse.status}`);
    const previewSession = await previewResponse.json();
    await navigate("runtime");
    const previewReview = document.getElementById("runtimeRehearsalApproval");
    if (!previewReview || state.showSessionActive) {
      throw new Error("active Preview rehearsal does not expose reviewed timing approval navigation");
    }
    previewReview.click();
    if (state.page !== "timing") throw new Error("Preview timing review control did not open the Timing workspace");
    const mutationsBeforeReview = apiMutations.length;
    const captureToggle = () => document.querySelector(`tr[data-timing-session="${previewSession.session_id}"] .timing-capture-toggle`);
    const touchActivate = async (button) => {
      button.scrollIntoView({ block: "center", inline: "nearest" });
      const bounds = button.getBoundingClientRect();
      if (bounds.left < 0 || bounds.top < 0 || bounds.right > 390 || bounds.bottom > 844) {
        throw new Error(`timing capture control is outside the emulated touch viewport: ${bounds.toJSON()}`);
      }
      await fetch("/show-workspace-touch-ready", {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ x: bounds.left + bounds.width / 2, y: bounds.top + bounds.height / 2 }),
      });
    };
    const keyboardActivate = async (button) => {
      button.focus();
      if (!button.matches(":focus")) throw new Error("timing capture control did not receive keyboard focus");
      await fetch("/show-workspace-key-ready", { method: "POST" });
    };
    const reviewDeadline = Date.now() + 5000;
    while (Date.now() < reviewDeadline && !captureToggle()) await new Promise((resolve) => setTimeout(resolve, 25));
    if (!captureToggle() || captureToggle().dataset.paused !== "false") {
      throw new Error(`active Preview timing controls did not load: ${document.getElementById("globalMessage")?.textContent || document.querySelector("#content .empty")?.textContent || "missing session row"}`);
    }
    if (apiMutations.length !== mutationsBeforeReview) throw new Error(`opening Preview timing review changed session approval: ${apiMutations.slice(mutationsBeforeReview).join(", ")}`);
    const mutationsBeforeCapture = apiMutations.length;
    await touchActivate(captureToggle());
    const pauseDeadline = Date.now() + 5000;
    while (Date.now() < pauseDeadline && captureToggle()?.dataset.paused !== "true") await new Promise((resolve) => setTimeout(resolve, 25));
    if (captureToggle()?.dataset.paused !== "true") throw new Error("Preview timing capture did not enter the paused state");
    await keyboardActivate(captureToggle());
    const resumeDeadline = Date.now() + 5000;
    while (Date.now() < resumeDeadline && captureToggle()?.dataset.paused !== "false") await new Promise((resolve) => setTimeout(resolve, 25));
    if (captureToggle()?.dataset.paused !== "false") throw new Error("Preview timing capture did not resume");
    const captureWrites = apiMutations.slice(mutationsBeforeCapture);
    if (captureWrites.length !== 2 || captureWrites.some((request) => !request.startsWith(`PUT /api/v1/projects/${config.project}/timing-intelligence/sessions/${previewSession.session_id}`))) {
      throw new Error(`Pause/Resume wrote unexpected API mutations: ${captureWrites.join(", ")}`);
    }
    await authenticateRole("TECHNICIAN");
    await navigate("runtime");
    if (!document.getElementById("runtimeRehearsalApproval")) {
      throw new Error("active Preview rehearsal does not expose timing review navigation to the Technician");
    }
    document.getElementById("runtimeRehearsalApproval").click();
    if (state.page !== "timing") throw new Error("Technician Preview timing review control did not open the Timing workspace");
    await navigate("runtime");
    document.getElementById("stopSessionButton")?.click();
    const previewStopDeadline = Date.now() + 5000;
    while (Date.now() < previewStopDeadline && document.getElementById("appView").dataset.showSession !== "inactive") {
      await new Promise((resolve) => setTimeout(resolve, 25));
    }
    if (document.getElementById("appView").dataset.showSession !== "inactive" ||
        !document.getElementById("runtimeRehearsalApproval")) {
      throw new Error("ended Preview rehearsal did not retain access to timing review");
    }
    await keyboardNavigate("cues");
    if (state.page !== "cues" || !document.querySelector("#content h1")?.textContent.includes("Cues")) {
      throw new Error(`operator could not re-enter Cue authoring after ending SHOW: page=${state.page}, heading=${document.querySelector("#content h1")?.textContent || "missing"}, message=${document.getElementById("globalMessage")?.textContent || ""}, requests=${fetchTrace.join(" | ")}`);
    }
    if (!document.getElementById("createDraftButton") && !document.getElementById("createCueButton")) {
      throw new Error("Cue authoring controls did not return after SHOW ended");
    }
    assertNoHorizontalOverflow("Cue authoring workspace");
    for (const [width, height] of [[768, 1024], [1280, 800]]) {
      await setViewport(width, height);
      assertNoHorizontalOverflow(`Cue authoring workspace at ${width}px`);
    }
    await setViewport(390, 844);

    await touchNavigate("runtime");
    const runtimeDeadline = Date.now() + 5000;
    while (Date.now() < runtimeDeadline && !document.getElementById("startShowButton")) {
      await new Promise((resolve) => setTimeout(resolve, 25));
    }
    if (state.page !== "runtime" || !document.getElementById("startShowButton") || document.getElementById("stopSessionButton")) {
      throw new Error("emulated touch did not return to the inactive Runtime workspace");
    }
    assertNoHorizontalOverflow("inactive Runtime workspace");
    for (const [width, height] of [[768, 1024], [1280, 800]]) {
      await setViewport(width, height);
      assertNoHorizontalOverflow(`inactive Runtime workspace at ${width}px`);
    }
    await setViewport(390, 844);

    const stopCurrentShow = async () => {
      document.getElementById("stopSessionButton").click();
      const deadline = Date.now() + 5000;
      while (Date.now() < deadline && document.getElementById("appView").dataset.showSession !== "inactive") {
        await new Promise((resolve) => setTimeout(resolve, 25));
      }
      if (document.getElementById("appView").dataset.showSession !== "inactive" || document.getElementById("stopSessionButton")) {
        throw new Error(`SHOW did not stop for ${state.user.role}`);
      }
    };
    async function authenticateRole(role) {
      const response = await fetch("/show-workspace-switch-role", {
        method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ role }),
      });
      if (!response.ok) throw new Error(`test role switch to ${role} returned HTTP ${response.status}`);
      const session = await response.json();
      state.user = session.user;
      state.csrf = session.csrf;
      sessionStorage.setItem("stagecore_csrf", state.csrf);
    }
    const switchRole = async (role) => {
      await authenticateRole(role);
      await keyboardNavigate("cues");
      const mayEdit = ["OWNER", "TECHNICIAN"].includes(role);
      const canAuthor = !!document.getElementById("createDraftButton") || !!document.getElementById("createCueButton");
      if (canAuthor !== mayEdit) throw new Error(`${role} Cue authoring controls=${canAuthor}, expected ${mayEdit}`);
      await assertResponsiveAtAllWidths(`${role} Cue workspace`);
      await touchNavigate("runtime");
      if (!document.getElementById("startShowButton")) throw new Error(`${role} Runtime workspace is unavailable`);
      if (document.getElementById("startShowButton").disabled !== (role === "VIEWER")) {
        throw new Error(`${role} SHOW entry enabled state is incorrect`);
      }
      await assertResponsiveAtAllWidths(`${role} inactive Runtime workspace`);
      if (role === "VIEWER") return;

      const seededResponse = await fetch("/show-workspace-seed-show", { method: "POST" });
      if (!seededResponse.ok) throw new Error(`${role} SHOW fixture returned HTTP ${seededResponse.status}`);
      const seeded = await seededResponse.json();
      seededSessions.push(seeded.session_id);
      await navigate("runtime");
      if (!state.showSessionActive || !document.getElementById("stopSessionButton") || document.getElementById("stopSessionButton").disabled) {
        throw new Error(`${role} cannot operate the seeded SHOW exit control`);
      }
      await assertResponsiveAtAllWidths(`${role} active SHOW workspace`);
      await stopCurrentShow();
    };
    await switchRole("TECHNICIAN");
    await switchRole("OPERATOR");
    await switchRole("VIEWER");
    const viewerShowResponse = await fetch("/show-workspace-seed-show", { method: "POST" });
    if (!viewerShowResponse.ok) throw new Error(`Viewer SHOW fixture returned HTTP ${viewerShowResponse.status}`);
    const viewerShow = await viewerShowResponse.json();
    seededSessions.push(viewerShow.session_id);
    await navigate("runtime");
    if (!state.showSessionActive) throw new Error("active SHOW did not pin Viewer to Runtime");
    for (const id of ["goButton", "stopCueButton", "emergencyBlackoutButton", "stopSessionButton"]) {
      if (!document.getElementById(id)?.disabled) throw new Error(`Viewer can operate ${id} during SHOW`);
    }
    await assertResponsiveAtAllWidths("Viewer active SHOW workspace");
    await authenticateRole("OWNER");
    await openProject(config.project);
    if (!state.showSessionActive || !document.getElementById("stopSessionButton") || document.getElementById("stopSessionButton").disabled) {
      throw new Error("Owner could not safely exit the Viewer-seeded SHOW");
    }
    await assertResponsiveAtAllWidths("Owner Viewer-session exit workspace");
    await stopCurrentShow();
    const stopMutations = apiMutations.filter((request) => request.startsWith("POST ") && request.includes("/runtime/stop-session"));
    const timingMutations = apiMutations.filter((request) => request.startsWith("PUT ") && request.includes("/timing-intelligence/sessions/"));
    if (postRequests.length !== 5 || postRequests.some((url) => !url.includes("/runtime/stop-session")) ||
        stopMutations.length !== 5 || timingMutations.length !== 2 || apiMutations.length !== 7) {
      throw new Error(`unexpected role workflow mutations: POST=${postRequests.join(", ")}; API=${apiMutations.join(", ")}`);
    }
    await report({ status: "PASS", seeded_sessions: seededSessions });
  } catch (error) {
    await report({ status: "FAIL", failure: error?.stack || String(error) });
  }
})();
