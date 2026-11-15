"use strict";

(async () => {
  const reportURL = "/mixed-cue-zoom-result";
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
  const reportFailure = async (error) => {
    await originalFetch(reportURL, {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ status: "FAIL", failure: error?.stack || String(error) }),
    });
  };
  try {
    if (document.readyState === "loading") {
      await new Promise((resolve) => document.addEventListener("DOMContentLoaded", resolve, { once: true }));
    }
    await waitFor(() => typeof state !== "undefined" && typeof navigate === "function", "Operator app scripts");
    const configResponse = await originalFetch("/mixed-cue-config", { cache: "no-store" });
    if (!configResponse.ok) throw new Error(`zoom audit config returned HTTP ${configResponse.status}`);
    const config = await configResponse.json();
    state.user = { user_id: config.user, username: "owner", role: "OWNER" };
    state.csrf = config.csrf;
    state.project = { project_id: config.project, name: config.name };
    sessionStorage.setItem("stagecore_csrf", state.csrf);
    showApp();
    await navigate("cues");
    await waitFor(() => document.querySelectorAll("#content tr[data-cue-id]").length === 123, "123 Draft Cue rows");

    const locale = localStorage.getItem("stagecore_sc018_zoom_locale") || "ar";
    const direction = locale === "en" ? "ltr" : "rtl";
    await waitFor(() => document.documentElement.lang.startsWith(locale) && document.documentElement.dir === direction,
      `${locale} ${direction} localization`);
    const firstRow = document.querySelector("#content tr[data-cue-id]");
    const details = firstRow?.querySelector("details");
    const summary = details?.querySelector("summary");
    if (!summary) throw new Error("first Cue has no Inspector control");
    summary.click();
    await waitFor(() => details.querySelector(".cue-inspection-navigation"), "Cue Inspector navigation");
    const navigation = details.querySelector(".cue-inspection-navigation");
    navigation.scrollIntoView({ block: "nearest" });
    await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
    const bounds = navigation.getBoundingClientRect();
    const pageWidth = Math.max(document.documentElement.scrollWidth, document.body.scrollWidth);
    const table = navigation.closest(".table-wrap");
    const measurement = {
      locale, direction,
      zoom_percent: Math.round(window.devicePixelRatio * 100),
      device_pixel_ratio: window.devicePixelRatio,
      viewport_width: window.innerWidth,
      page_width: pageWidth,
      page_overflow: pageWidth > window.innerWidth,
      inspector_visible: bounds.width > 0,
      inspector_in_viewport: bounds.left >= -1 && bounds.right <= window.innerWidth + 1 &&
        bounds.top >= -1 && bounds.bottom <= window.innerHeight + 1,
      table_client_width: table?.clientWidth || 0,
      table_scroll_width: table?.scrollWidth || 0,
      api_mutations: apiMutations.length,
    };
    if (measurement.zoom_percent !== 200 || measurement.viewport_width !== 720 || measurement.page_overflow ||
        !measurement.inspector_visible || !measurement.inspector_in_viewport || apiMutations.length) {
      throw new Error(`200% Cue Inspector layout failed: ${JSON.stringify(measurement)}; mutations=${apiMutations.join(",")}`);
    }
    const ready = await originalFetch("/mixed-cue-zoom-ready", {
      method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ locale }),
    });
    if (!ready.ok) throw new Error(`200% screenshot capture handshake returned HTTP ${ready.status}`);

    if (locale === "ar") {
      localStorage.setItem("stagecore_sc018_zoom_rtl", JSON.stringify(measurement));
      localStorage.setItem("stagecore_sc018_zoom_locale", "en");
      localStorage.setItem("stagecore_locale", "en");
      window.location.reload();
      return;
    }
    const rtl = JSON.parse(localStorage.getItem("stagecore_sc018_zoom_rtl") || "null");
    if (!rtl || rtl.locale !== "ar" || rtl.direction !== "rtl") throw new Error("RTL zoom evidence was not retained across locale reload");
    await originalFetch(reportURL, {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ status: "PASS", measurements: [rtl, measurement] }),
    });
  } catch (error) {
    await reportFailure(error);
  }
})();
