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
    const configResponse = await fetch("/show-workspace-config", { cache: "no-store" });
    if (!configResponse.ok) throw new Error(`test configuration returned HTTP ${configResponse.status}`);
    const config = await configResponse.json();
    state.user = { username: "owner", role: "OWNER" };
    state.csrf = config.csrf;
    sessionStorage.setItem("stagecore_csrf", state.csrf);

    f017RememberPage(config.project, "cues");
    if (f017PreferredPage(config.project) !== "cues") throw new Error("fixture did not remember the authoring page");
    await openProject(config.project);

    if (state.page !== "runtime") throw new Error(`active SHOW opened ${state.page}, expected runtime`);
    if (!state.showSessionActive) throw new Error("active SHOW state was not reflected in the operator UI");
    if (document.getElementById("appView").dataset.showSession !== "active") throw new Error("SHOW presentation marker is missing");
    for (const id of ["goButton", "stopCueButton", "emergencyBlackoutButton"]) {
      if (!document.getElementById(id)) throw new Error(`runtime safety control ${id} is missing`);
    }

    const sidebar = document.querySelector(".sidebar");
    const profile = document.getElementById("workspaceProfileControl");
    if (getComputedStyle(sidebar).display !== "none") throw new Error("workspace navigation remained visible during SHOW");
    if (getComputedStyle(profile).display !== "none") throw new Error("workspace profile control remained visible during SHOW");

    setShowSessionPresentation(false);
    if (getComputedStyle(sidebar).display === "none") throw new Error("workspace navigation did not return after SHOW presentation cleared");
    if (getComputedStyle(profile).display === "none") throw new Error("workspace profile control did not return after SHOW presentation cleared");
    await report("PASS");
  } catch (error) {
    await report(`FAIL: ${error?.stack || error}`);
  }
})();
