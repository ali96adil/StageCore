"use strict";

(async () => {
  const apiMutations = [];
  const originalFetch = window.fetch.bind(window);
  window.fetch = async (...args) => {
    const url = String(args[0]);
    const method = String(args[1]?.method || "GET").toUpperCase();
    if (url.startsWith("/api/v1/") && !["GET", "HEAD"].includes(method)) apiMutations.push(`${method} ${url}`);
    return originalFetch(...args);
  };
  const waitFor = async (predicate, label) => {
    const deadline = Date.now() + 15000;
    while (Date.now() < deadline) {
      if (predicate()) return;
      await new Promise((resolve) => setTimeout(resolve, 25));
    }
    throw new Error(`Timed out waiting for ${label}`);
  };
  const report = async (result) => originalFetch("/project-access-browser-zoom-result", {
    method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(result),
  });
  try {
    if (document.readyState === "loading") {
      await new Promise((resolve) => document.addEventListener("DOMContentLoaded", resolve, { once: true }));
    }
    await waitFor(() => typeof state !== "undefined" && typeof navigate === "function", "Operator scripts");
    const configResponse = await originalFetch("/project-access-browser-config", { cache: "no-store" });
    if (!configResponse.ok) throw new Error(`Project Access zoom config returned HTTP ${configResponse.status}`);
    const config = await configResponse.json();
    state.user = { username: "owner", role: "OWNER" };
    state.csrf = config.csrf;
    sessionStorage.setItem("stagecore_csrf", state.csrf);
    showApp();
    const securityButton = document.getElementById("securityNav");
    if (!securityButton || securityButton.classList.contains("hidden")) throw new Error("Owner Security navigation is unavailable");
    securityButton.click();
    await waitFor(() => !!document.getElementById("projectAccessGrantForm"), "prepared Project grant editor");
    const project = document.getElementById("projectAccessProject");
    const user = document.getElementById("projectAccessUser");
    const level = document.getElementById("projectAccessLevel");
    const load = document.getElementById("loadProjectAccess");
    const save = document.getElementById("saveProjectAccess");
    project.value = config.project;
    project.dispatchEvent(new Event("change", { bubbles: true }));
    user.value = config.operator;
    user.dispatchEvent(new Event("change", { bubbles: true }));
    load.click();
    await waitFor(() => !level.disabled && document.getElementById("projectAccessCurrent")?.textContent.includes("Current: NONE"),
      "reading the stored NONE access level");
    level.value = "OPERATE";
    level.dispatchEvent(new Event("change", { bubbles: true }));
    const warningVisible = document.getElementById("content").innerText.includes("not enforced by this Hub");
    const form = document.getElementById("projectAccessGrantForm");
    save.scrollIntoView({ block: "nearest" });
    await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
    const bounds = save.getBoundingClientRect();
    const pageWidth = Math.max(document.documentElement.scrollWidth, document.body.scrollWidth);
    const formOverflow = form.scrollWidth > form.clientWidth + 1;
    const saveVisible = !save.disabled && bounds.width > 0 && bounds.left >= -1 && bounds.right <= window.innerWidth + 1 &&
      bounds.top >= -1 && bounds.bottom <= window.innerHeight + 1;
    const result = {
      status: "PASS", direction: getComputedStyle(form).direction,
      zoom_percent: Math.round(window.devicePixelRatio * 100), device_pixel_ratio: window.devicePixelRatio,
      viewport_width: window.innerWidth, page_width: pageWidth, page_overflow: pageWidth > window.innerWidth,
      form_overflow: formOverflow, save_visible: saveVisible, warning_visible: warningVisible,
      selected_level: level.value, api_mutations: apiMutations.length,
    };
    if (result.zoom_percent !== 200 || result.direction !== "rtl" || result.viewport_width !== 720 ||
        result.page_overflow || result.form_overflow || !result.save_visible || !result.warning_visible ||
        result.selected_level !== "OPERATE" || apiMutations.length) {
      throw new Error(`Project Access 200% layout failed: ${JSON.stringify(result)}; mutations=${apiMutations.join(",")}`);
    }
    const ready = await originalFetch("/project-access-browser-zoom-ready", { method: "POST" });
    if (!ready.ok) throw new Error(`Project Access screenshot handshake returned HTTP ${ready.status}`);
    await report(result);
  } catch (error) {
    await report({ status: "FAIL", failure: error?.stack || String(error) });
  }
})();
