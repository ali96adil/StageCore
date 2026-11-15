"use strict";

(async () => {
  let phase = "startup";
  let hubRequests = 0;
  const originalFetch = window.fetch.bind(window);
  window.fetch = (...args) => {
    if (String(args[0]).startsWith("/api/v1/")) hubRequests++;
    return originalFetch(...args);
  };
  const report = async (value) => {
    clearTimeout(watchdog);
    await fetch("/workspace-profile-result", {
      method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ ...value, phase }),
    });
    window.close();
  };
  const watchdog = setTimeout(() => report({ status: "FAIL", error: "workspace-profile browser watchdog expired" }), 15000);
  const waitFor = async (predicate, label) => {
    const deadline = Date.now() + 5000;
    while (Date.now() < deadline) {
      if (predicate()) return;
      await new Promise((resolve) => setTimeout(resolve, 20));
    }
    throw new Error(`Timed out waiting for ${label}`);
  };

  try {
    phase = "wait for initialized workspace controls";
    await waitFor(() => document.getElementById("workspaceProfileSelect")?.options.length > 0 &&
      document.getElementById("workspaceProfileManage"), "initialized workspace profile controls");
    phase = "open manager";
    document.getElementById("workspaceProfileManage").click();
    phase = "wait for manager dialog";
    await waitFor(() => document.getElementById("workspaceProfileDialog")?.open, "workspace profile manager dialog");
    const dialog = document.getElementById("workspaceProfileDialog");
    phase = "wait for built-in profile manager";
    await waitFor(() => dialog.querySelector(".f017-copy"), "built-in profile manager");
    dialog.querySelector(".f017-copy").click();
    phase = "wait for editable copy";
    await waitFor(() => dialog.open && dialog.querySelector("#f017ProfileName"), "editable custom profile copy");
    const name = dialog.querySelector("#f017ProfileName");
    phase = "qualify Workspace Profile at actual 200 percent zoom";
    const saveButton = dialog.querySelector(".f017-save");
    saveButton.scrollIntoView({ block: "nearest", inline: "nearest" });
    await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
    const viewportWidth = window.innerWidth;
    const documentWidth = Math.max(document.documentElement.scrollWidth, document.body.scrollWidth);
    const saveRect = saveButton.getBoundingClientRect();
    const dialogRect = dialog.getBoundingClientRect();
    const saveVisibleAtZoom = saveRect.width > 0 && saveRect.height > 0 && saveRect.top >= 0 &&
      saveRect.bottom <= window.innerHeight && saveRect.top >= dialogRect.top + 1 &&
      saveRect.bottom <= dialogRect.bottom - 1 && dialogRect.left >= 0 && dialogRect.right <= viewportWidth;
    const zoomPercent = Math.round(window.devicePixelRatio * 100);
    if (viewportWidth !== 720 || documentWidth > viewportWidth || zoomPercent !== 200 || !saveVisibleAtZoom) {
      throw new Error(`Workspace Profile does not fit at actual 200% zoom: ${JSON.stringify({ viewportWidth, documentWidth, zoomPercent, saveVisibleAtZoom, saveRect: saveRect.toJSON(), dialogRect: dialogRect.toJSON(), devicePixelRatio: window.devicePixelRatio })}`);
    }
    await fetch("/workspace-profile-screenshot-ready", { method: "POST" });

    const initial = JSON.parse(localStorage.getItem("stagecore_workspace_profiles_v1"));
    const profileID = initial.active_profile_id;
    const storedBeforeEnter = initial.custom_profiles.find((item) => item.profile_id === profileID)?.name || "";
    name.value = "Keyboard-tested rehearsal profile";
    name.dispatchEvent(new Event("input", { bubbles: true }));
    name.focus();
    phase = "dispatch Enter to profile name";
    await fetch("/workspace-profile-key-ready", { method: "POST" });
    await new Promise((resolve) => setTimeout(resolve, 100));

    phase = "verify Enter preserves editor draft";
    const afterEnter = JSON.parse(localStorage.getItem("stagecore_workspace_profiles_v1"));
    const storedAfterEnter = afterEnter.custom_profiles.find((item) => item.profile_id === profileID)?.name || "";
    const dialogOpenAfterEnter = dialog.open;
    if (!dialogOpenAfterEnter || storedAfterEnter !== storedBeforeEnter) {
      throw new Error(`Enter closed the editor or persisted an unsaved profile: ${JSON.stringify({ dialogOpenAfterEnter, storedBeforeEnter, storedAfterEnter })}`);
    }
    phase = "save profile explicitly";
    dialog.querySelector(".f017-save").click();
    await waitFor(() => JSON.parse(localStorage.getItem("stagecore_workspace_profiles_v1"))
      .custom_profiles.find((item) => item.profile_id === profileID)?.name === "Keyboard-tested rehearsal profile", "explicit Save to persist the profile");
    const saved = JSON.parse(localStorage.getItem("stagecore_workspace_profiles_v1"));
    phase = "report saved profile";
    await report({
      status: "PASS", dialog_open_after_enter: dialogOpenAfterEnter,
      stored_name_before_enter: storedBeforeEnter, stored_name_after_enter: storedAfterEnter,
      stored_name_after_save: saved.custom_profiles.find((item) => item.profile_id === profileID)?.name || "",
      no_hub_mutation: hubRequests === 0,
      viewport_width: viewportWidth, document_width: documentWidth, zoom_percent: zoomPercent,
      device_pixel_ratio: window.devicePixelRatio, save_visible_at_zoom: saveVisibleAtZoom,
    });
  } catch (error) {
    await report({ status: "FAIL", error: String(error.stack || error) });
  }
})();
