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
  const measureDialogAction = async (dialog, action, direction) => {
    document.documentElement.dir = direction;
    action.scrollIntoView({ block: "nearest", inline: "nearest" });
    await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
    const pageWidth = Math.max(document.documentElement.scrollWidth, document.body.scrollWidth);
    const actionRect = action.getBoundingClientRect();
    const dialogRect = dialog.getBoundingClientRect();
    return {
      pageWidth,
      actionVisible: actionRect.width > 0 && actionRect.height > 0 && actionRect.top >= 0 &&
        actionRect.bottom <= window.innerHeight && actionRect.top >= dialogRect.top + 1 &&
        actionRect.bottom <= dialogRect.bottom - 1 && dialogRect.left >= 0 && dialogRect.right <= window.innerWidth,
      actionRect: actionRect.toJSON(),
      dialogRect: dialogRect.toJSON(),
    };
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
    const viewportWidth = window.innerWidth;
    const rtl = await measureDialogAction(dialog, saveButton, "rtl");
    const zoomPercent = Math.round(window.devicePixelRatio * 100);
    if (viewportWidth !== 720 || rtl.pageWidth > viewportWidth || zoomPercent !== 200 || !rtl.actionVisible) {
      throw new Error(`Workspace Profile does not fit at actual 200% RTL zoom: ${JSON.stringify({ viewportWidth, pageWidth: rtl.pageWidth, zoomPercent, actionVisible: rtl.actionVisible, actionRect: rtl.actionRect, dialogRect: rtl.dialogRect, devicePixelRatio: window.devicePixelRatio })}`);
    }
    await fetch("/workspace-profile-screenshot-ready?surface=workspace-profile&direction=rtl", { method: "POST" });
    const ltr = await measureDialogAction(dialog, saveButton, "ltr");
    if (ltr.pageWidth > viewportWidth || !ltr.actionVisible) {
      throw new Error(`Workspace Profile does not fit at actual 200% LTR zoom: ${JSON.stringify({ viewportWidth, pageWidth: ltr.pageWidth, actionVisible: ltr.actionVisible, actionRect: ltr.actionRect, dialogRect: ltr.dialogRect })}`);
    }
    await fetch("/workspace-profile-screenshot-ready?surface=workspace-profile&direction=ltr", { method: "POST" });
    document.documentElement.dir = "rtl";
    await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));

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
    dialog.querySelector(".f017-close").click();
    await waitFor(() => !dialog.open, "Workspace Profile dialog to close");

    phase = "qualify shared Appearance dialog at actual 200 percent zoom";
    document.getElementById("appearanceTrigger").click();
    const appearanceDialog = document.getElementById("appearanceDialog");
    await waitFor(() => appearanceDialog?.open, "Appearance dialog");
    const appearanceClose = appearanceDialog.querySelector(".dialog-actions button[type='submit']");
    const appearanceRTL = await measureDialogAction(appearanceDialog, appearanceClose, "rtl");
    if (appearanceRTL.pageWidth > viewportWidth || !appearanceRTL.actionVisible) {
      throw new Error(`Appearance close action does not fit at actual 200% RTL zoom: ${JSON.stringify({ viewportWidth, ...appearanceRTL })}`);
    }
    await fetch("/workspace-profile-screenshot-ready?surface=appearance&direction=rtl", { method: "POST" });
    const appearanceLTR = await measureDialogAction(appearanceDialog, appearanceClose, "ltr");
    if (appearanceLTR.pageWidth > viewportWidth || !appearanceLTR.actionVisible) {
      throw new Error(`Appearance close action does not fit at actual 200% LTR zoom: ${JSON.stringify({ viewportWidth, ...appearanceLTR })}`);
    }
    await fetch("/workspace-profile-screenshot-ready?surface=appearance&direction=ltr", { method: "POST" });

    phase = "report saved profile";
    await report({
      status: "PASS", dialog_open_after_enter: dialogOpenAfterEnter,
      stored_name_before_enter: storedBeforeEnter, stored_name_after_enter: storedAfterEnter,
      stored_name_after_save: saved.custom_profiles.find((item) => item.profile_id === profileID)?.name || "",
      no_hub_mutation: hubRequests === 0,
      viewport_width: viewportWidth, document_width: rtl.pageWidth, zoom_percent: zoomPercent,
      device_pixel_ratio: window.devicePixelRatio, save_visible_at_zoom: rtl.actionVisible,
      ltr_document_width: ltr.pageWidth, ltr_save_visible_at_zoom: ltr.actionVisible,
      appearance_rtl_document_width: appearanceRTL.pageWidth,
      appearance_ltr_document_width: appearanceLTR.pageWidth,
      appearance_rtl_close_visible: appearanceRTL.actionVisible,
      appearance_ltr_close_visible: appearanceLTR.actionVisible,
    });
  } catch (error) {
    await report({ status: "FAIL", error: String(error.stack || error) });
  }
})();
