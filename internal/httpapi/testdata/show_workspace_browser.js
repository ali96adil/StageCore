"use strict";

(async () => {
  const report = async (value) => {
    await fetch("/show-workspace-result", {
      method: "POST",
      headers: { "Content-Type": "text/plain" },
      body: value,
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

    f017RememberPage(config.project, "cues");
    if (f017PreferredPage(config.project) !== "cues") throw new Error("fixture did not remember the authoring page");
    await openProject(config.project);

    if (state.page !== "runtime") throw new Error(`active SHOW opened ${state.page}, expected runtime`);
    if (!state.showSessionActive) throw new Error("active SHOW state was not reflected in the operator UI");
    if (document.getElementById("appView").dataset.showSession !== "active") throw new Error("SHOW presentation marker is missing");
    assertNoHorizontalOverflow("active SHOW workspace");
    for (const id of ["goButton", "stopCueButton", "emergencyBlackoutButton"]) {
      if (!document.getElementById(id)) throw new Error(`runtime safety control ${id} is missing`);
    }

    const sidebar = document.querySelector(".sidebar");
    const profile = document.getElementById("workspaceProfileControl");
    if (getComputedStyle(sidebar).display !== "none") throw new Error("workspace navigation remained visible during SHOW");
    if (getComputedStyle(profile).display !== "none") throw new Error("workspace profile control remained visible during SHOW");

    const originalFetch = window.fetch.bind(window);
    const postRequests = [];
    const fetchTrace = [];
    window.fetch = async (input, init = {}) => {
      const method = String(init.method || "GET").toUpperCase();
      const url = String(input);
      if (method === "POST" && url.startsWith("/api/v1/")) postRequests.push(url);
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

    const cuesNav = document.querySelector('[data-page="cues"]');
    cuesNav.focus();
    await fetch("/show-workspace-key-ready", { method: "POST" });
    const cuesDeadline = Date.now() + 5000;
    while (Date.now() < cuesDeadline && !document.querySelector("#content h1")?.textContent.includes("Cues")) {
      await new Promise((resolve) => setTimeout(resolve, 25));
    }
    if (state.page !== "cues" || !document.querySelector("#content h1")?.textContent.includes("Cues")) {
      throw new Error(`operator could not re-enter Cue authoring after ending SHOW: page=${state.page}, heading=${document.querySelector("#content h1")?.textContent || "missing"}, message=${document.getElementById("globalMessage")?.textContent || ""}, requests=${fetchTrace.join(" | ")}`);
    }
    if (!cuesNav.matches(":focus-visible")) throw new Error("native keyboard navigation did not preserve visible focus");
    if (!document.getElementById("createDraftButton") && !document.getElementById("createCueButton")) {
      throw new Error("Cue authoring controls did not return after SHOW ended");
    }
    assertNoHorizontalOverflow("Cue authoring workspace");
    for (const [width, height] of [[768, 1024], [1280, 800]]) {
      await setViewport(width, height);
      assertNoHorizontalOverflow(`Cue authoring workspace at ${width}px`);
    }
    await setViewport(390, 844);

    const runtimeNav = document.querySelector('[data-page="runtime"]');
    const runtimeBounds = runtimeNav.getBoundingClientRect();
    await fetch("/show-workspace-touch-ready", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ x: runtimeBounds.left + runtimeBounds.width / 2, y: runtimeBounds.top + runtimeBounds.height / 2 }),
    });
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
    await report("PASS");
  } catch (error) {
    await report(`FAIL: ${error?.stack || error}`);
  }
})();
