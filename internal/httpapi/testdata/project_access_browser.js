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

    const securityButton = document.getElementById("securityNav");
    if (!securityButton || securityButton.classList.contains("hidden")) {
      throw new Error("Owner Security navigation is unavailable");
    }
    securityButton.click();
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
    load.click();
    await waitFor(() => !level.disabled && !save.disabled &&
      document.getElementById("projectAccessCurrent").textContent.includes("Current: NONE"),
    "loading the Operator's current Project access");

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
    level.value = "OPERATE";
    level.dispatchEvent(new Event("change", { bubbles: true }));
    if (save.disabled || !document.getElementById("projectAccessLevelDescription").textContent.includes("Emergency Blackout")) {
      throw new Error("Operate level did not enable the guarded save with its permission summary");
    }
    save.click();
    await waitFor(() => document.getElementById("projectAccessCurrent").textContent.includes(
      "Saved OPERATE. PREPARED_NOT_ENFORCED."), "saving the not-enforced Operate grant");

    const readbackResponse = await fetch(
      `/api/v1/projects/${encodeURIComponent(config.project)}/access/users/${encodeURIComponent(config.operator)}`,
      { cache: "no-store" },
    );
    if (!readbackResponse.ok) throw new Error(`grant readback returned HTTP ${readbackResponse.status}`);
    const readback = await readbackResponse.json();
    const unsafeCall = requestTrace.some((request) =>
      /\/publish(?:\?|\s|$)|\/runtime\/(?:go|jump|stop)(?:\?|\s|$)|emergency-blackout/i.test(request));
    if (readback.access_level !== "OPERATE" || readback.enforced !== false ||
      readback.phase !== "PREPARED_NOT_ENFORCED" || unsafeCall) {
      throw new Error(`grant readback or non-actuation boundary failed: ${JSON.stringify(readback)}`);
    }
    await report({
      status: "PASS", access_level: readback.access_level, phase: readback.phase,
      enforced: readback.enforced, warning_shown: document.getElementById("content").innerText.includes("not enforced by this Hub"),
      unsafe_call: unsafeCall, rtl: getComputedStyle(form).direction === "rtl", fields_stack: fieldsStack,
      page_overflow: pageOverflow, keyboard_focusable: keyboardFocusable, tab_traversal: tabTraversal,
      focus_visible: focusVisible,
      viewport_width: document.documentElement.clientWidth, requests: requestTrace,
    });
  } catch (error) {
    await report({ status: "FAIL", error: String(error.stack || error), requests: requestTrace });
  }
})();
