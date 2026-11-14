"use strict";

(async () => {
  const requestTrace = [];
  const browserFetch = window.fetch.bind(window);
  window.fetch = async (...args) => {
    const response = await browserFetch(...args);
    requestTrace.push(`${args[0]} → ${response.status}`);
    return response;
  };
  const report = async (value) => {
    await fetch("/project-access-browser-result", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(value),
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
  const pressKey = async (key, shift = false) => {
    const response = await fetch("/project-access-browser-key-ready", {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ key, shift }),
    });
    if (!response.ok) throw new Error(`native ${key} key returned HTTP ${response.status}`);
  };

  try {
    if (document.readyState === "loading") {
      await new Promise((resolve) => document.addEventListener("DOMContentLoaded", resolve, { once: true }));
    }
    document.documentElement.dir = "rtl";
    const configResponse = await fetch("/project-access-browser-config", { cache: "no-store" });
    if (!configResponse.ok) throw new Error(`test configuration returned HTTP ${configResponse.status}`);
    const config = await configResponse.json();
    state.user = { username: "owner", role: "OWNER" };
    state.csrf = config.csrf;
    sessionStorage.setItem("stagecore_csrf", state.csrf);
    window.confirm = () => true;
    showApp();
    let sawTouchPointer = false;
    document.addEventListener("pointerdown", (event) => {
      if (event.pointerType === "touch") sawTouchPointer = true;
    }, true);

    const securityButton = document.getElementById("securityNav");
    if (!securityButton || securityButton.classList.contains("hidden")) {
      throw new Error("Owner Security navigation is unavailable");
    }
    const securityRect = securityButton.getBoundingClientRect();
    const touchResponse = await fetch("/project-access-browser-touch-ready", {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ x: securityRect.left + securityRect.width / 2, y: securityRect.top + securityRect.height / 2 }),
    });
    if (!touchResponse.ok) throw new Error(`emulated touch request returned HTTP ${touchResponse.status}`);
    const touchActivated = sawTouchPointer;
    if (!touchActivated) throw new Error("Project Access navigation did not receive a touch pointer event");
    await waitFor(() => !!document.getElementById("projectAccessGrantForm"), "prepared Project grant editor");
    if (!document.getElementById("content").innerText.includes("not enforced by this Hub")) {
      throw new Error("grant editor did not explain that saved access is not enforced");
    }

    const project = document.getElementById("projectAccessProject");
    const user = document.getElementById("projectAccessUser");
    const level = document.getElementById("projectAccessLevel");
    const load = document.getElementById("loadProjectAccess");
    const save = document.getElementById("saveProjectAccess");
    project.focus();
    if (document.activeElement !== project) throw new Error("Project selector did not accept keyboard focus");
    const focusResponse = await fetch("/project-access-browser-focus-ready", { method: "POST" });
    if (!focusResponse.ok) throw new Error(`native Tab key request returned HTTP ${focusResponse.status}`);
    const tabTraversal = document.activeElement === user && user.matches(":focus-visible");
    const userFocus = getComputedStyle(user);
    const focusVisible = userFocus.outlineStyle !== "none" && userFocus.outlineWidth === "3px";
    if (!tabTraversal || !focusVisible) {
      throw new Error(`project grant keyboard traversal failed: ${JSON.stringify({
        active: document.activeElement?.id, tabTraversal, focusVisible,
        outlineStyle: userFocus.outlineStyle, outlineWidth: userFocus.outlineWidth,
      })}`);
    }
    project.value = config.project;
    project.dispatchEvent(new Event("change", { bubbles: true }));
    user.value = config.operator;
    user.dispatchEvent(new Event("change", { bubbles: true }));
    await pressKey("Tab");
    if (document.activeElement !== load) throw new Error("Tab did not move from the account to Load current access");
    await pressKey("Enter");
    await waitFor(() => !level.disabled && !save.disabled &&
      document.getElementById("projectAccessCurrent").textContent.includes("Current: NONE"),
    "loading the Operator's current Project access");
    if (document.activeElement !== level) throw new Error("loading access did not move keyboard focus to Access level");
    const observedLevelKeys = [];
    level.addEventListener("keydown", (event) => observedLevelKeys.push({ key: event.key, code: event.code, keyCode: event.keyCode }));
    level.addEventListener("input", () => observedLevelKeys.push({ input: level.value }));
    level.addEventListener("change", () => observedLevelKeys.push({ change: level.value }));

    const form = document.getElementById("projectAccessGrantForm");
    const fieldRects = [project, user, level].map((field) => field.getBoundingClientRect());
    const fieldsStack = fieldRects[1].top > fieldRects[0].top && fieldRects[2].top > fieldRects[1].top;
    const pageOverflow = document.documentElement.scrollWidth > document.documentElement.clientWidth + 1;
    const keyboardFocusable = [project, user, level, load, save].every((control) => control.tabIndex >= 0);
    if (getComputedStyle(form).direction !== "rtl" || !fieldsStack || pageOverflow || !keyboardFocusable ||
      document.documentElement.clientWidth > 560) {
      throw new Error(`prepared access form failed RTL/mobile checks: ${JSON.stringify({
        direction: getComputedStyle(form).direction, fieldsStack, pageOverflow, keyboardFocusable,
        viewport: document.documentElement.clientWidth,
        documentWidth: document.documentElement.scrollWidth,
      })}`);
    }

    const offeredLevels = [...level.options].map((option) => option.value).filter(Boolean);
    if (!["VIEW", "EDIT", "OPERATE", "MANAGE", "NONE"].every((value) => offeredLevels.includes(value))) {
      throw new Error(`Operator access levels are incomplete: ${offeredLevels.join(",")}`);
    }
    await pressKey("o");
    const nativeKeyboardSelection = level.value === "OPERATE" && !save.disabled &&
      document.getElementById("projectAccessLevelDescription").textContent.includes("Emergency Blackout");
    if (!nativeKeyboardSelection) {
      throw new Error(`native type-to-select did not choose OPERATE and enable its guarded save: ${JSON.stringify({
        value: level.value, saveDisabled: save.disabled,
        description: document.getElementById("projectAccessLevelDescription").textContent,
        active: document.activeElement?.id, keys: observedLevelKeys,
      })}`);
    }
    await pressKey("Tab");
    if (document.activeElement !== load) throw new Error("Tab did not move from Access level to Load current access");
    await pressKey("Tab");
    if (document.activeElement !== save) throw new Error("Tab did not move to Save prepared access");
    await pressKey("Enter");
    await waitFor(() => document.getElementById("projectAccessCurrent").textContent.includes(
      "Saved OPERATE. PREPARED_NOT_ENFORCED."), "saving the not-enforced Operate grant");

    const readbackResponse = await fetch(
      `/api/v1/projects/${encodeURIComponent(config.project)}/access/users/${encodeURIComponent(config.operator)}`,
      { cache: "no-store" },
    );
    if (!readbackResponse.ok) throw new Error(`grant readback returned HTTP ${readbackResponse.status}`);
    const readback = await readbackResponse.json();
    const requiredOperatePermissions = ["project.view", "rehearsal.control", "show.go", "show.stop", "show.blackout.engage"];
    const grantedPermissions = readback.permissions || [];
    if (readback.access_level !== "OPERATE" || readback.enforced !== false ||
      readback.phase !== "PREPARED_NOT_ENFORCED" ||
      !requiredOperatePermissions.every((permission) => grantedPermissions.includes(permission))) {
      throw new Error(`grant readback or non-actuation boundary failed: ${JSON.stringify(readback)}`);
    }
    await pressKey("Tab", true);
    await pressKey("Tab", true);
    if (document.activeElement !== level) throw new Error("Shift+Tab did not return keyboard focus to Access level");
    await pressKey("n");
    if (level.value !== "NONE" || save.disabled || document.activeElement !== level) {
      throw new Error(`native type-to-select did not choose NONE: ${JSON.stringify({
        value: level.value, saveDisabled: save.disabled, active: document.activeElement?.id,
      })}`);
    }
    await pressKey("Tab");
    await pressKey("Tab");
    if (save.disabled || document.activeElement !== save) {
      throw new Error("saved keyboard focus was not available to remove Project access");
    }
    await pressKey("Enter");
    await waitFor(() => document.getElementById("projectAccessCurrent").textContent.includes(
      "Saved NONE. PREPARED_NOT_ENFORCED."), "saving the prepared Project access revocation");
    const revokeResponse = await fetch(
      `/api/v1/projects/${encodeURIComponent(config.project)}/access/users/${encodeURIComponent(config.operator)}`,
      { cache: "no-store" },
    );
    if (!revokeResponse.ok) throw new Error(`revoked grant readback returned HTTP ${revokeResponse.status}`);
    const revoked = await revokeResponse.json();
    const unsafeCall = requestTrace.some((request) =>
      /\/publish(?:\?|\s|$)|\/runtime\/(?:go|jump|stop)(?:\?|\s|$)|emergency-blackout/i.test(request));
    if (revoked.access_level !== "NONE" || (revoked.permissions || []).length !== 0 ||
      revoked.enforced !== false || revoked.phase !== "PREPARED_NOT_ENFORCED" || unsafeCall) {
      throw new Error(`revoked grant or non-actuation boundary failed: ${JSON.stringify(revoked)}`);
    }
    await report({
      status: "PASS", access_level: revoked.access_level, granted_access_level: readback.access_level,
      granted_permissions: grantedPermissions, phase: revoked.phase,
      enforced: revoked.enforced, warning_shown: document.getElementById("content").innerText.includes("not enforced by this Hub"),
      unsafe_call: unsafeCall, rtl: getComputedStyle(form).direction === "rtl", fields_stack: fieldsStack,
      page_overflow: pageOverflow, keyboard_focusable: keyboardFocusable, tab_traversal: tabTraversal,
      focus_visible: focusVisible, native_keyboard_selection: nativeKeyboardSelection, touch_activated: touchActivated,
      viewport_width: document.documentElement.clientWidth, requests: requestTrace,
    });
  } catch (error) {
    await report({ status: "FAIL", error: String(error.stack || error), requests: requestTrace });
  }
})();
