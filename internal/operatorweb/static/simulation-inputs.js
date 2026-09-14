"use strict";

(() => {
  let observer = null;
  let rendering = false;

  function ar() {
    return (document.documentElement.lang || "ar").toLowerCase().startsWith("ar");
  }

  function t(arText, enText) {
    return ar() ? arText : enText;
  }

  function projectID() {
    return state.project?.project_id || "";
  }

  function inputOptions(items) {
    return (items || []).map((item) => `<option value="${esc(item.input_id)}">${esc(item.name || item.input_id)} · ${esc(item.event_type || "event")}</option>`).join("");
  }

  async function loadPanel() {
    if (rendering || state.page !== "simulation" || !projectID()) return;
    rendering = true;
    try {
      const old = el("simulationInputPanel");
      let payload;
      try {
        payload = await api(`/api/v1/projects/${encodeURIComponent(projectID())}/simulation/inputs`);
      } catch (_) {
        if (old) old.remove();
        return;
      }
      const items = payload.inputs || [];
      const html = `<section id="simulationInputPanel" class="phase4-form">
        <div class="phase4-card-head">
          <div><p class="eyebrow">SIMULATED INPUT EVENTS</p><h2>${t("إدخال حدث افتراضي", "Inject simulated input")}</h2><p class="muted">${t("يمر الحدث عبر RoutingEngine الحقيقي لكن كل output في SIMULATION يبقى داخل Digital Twin. المصدر يسجل TEST.", "The event runs through the canonical RoutingEngine, while every SIMULATION output remains inside the Digital Twin. Source is recorded as TEST.")}</p></div>
          <span class="phase4-pulse warning">SIMULATION_ONLY · TEST</span>
        </div>
        ${items.length ? `<form id="simInputForm">
          <div class="phase4-form-grid">
            <label>${t("Input", "Input")}<select id="simInputID">${inputOptions(items)}</select></label>
            <label>${t("قيمة JSON", "JSON value")}<textarea id="simInputValue" class="mono" rows="3">true</textarea></label>
            <label class="check-row"><input id="simInputConfirmCritical" type="checkbox"> ${t("أؤكد تنفيذ route حرج داخل Simulation فقط", "Confirm critical route inside Simulation only")}</label>
          </div>
          <div class="phase4-actions"><button class="button" type="submit">${t("Inject TEST event", "Inject TEST event")}</button></div>
        </form>` : `<div class="phase4-empty">${t("لا توجد Inputs مفعلة في Runtime Snapshot الحالي.", "No enabled Inputs in the current Runtime Snapshot.")}</div>`}
      </section>`;
      if (old) old.outerHTML = html;
      else {
        const report = el("simulationReportPanel");
        if (report) report.insertAdjacentHTML("beforebegin", html);
        else content.insertAdjacentHTML("beforeend", html);
      }
      bind();
    } finally {
      rendering = false;
    }
  }

  function bind() {
    el("simInputForm")?.addEventListener("submit", async (event) => {
      event.preventDefault();
      let value;
      try {
        value = JSON.parse(el("simInputValue").value);
      } catch (_) {
        setMessage(globalMessage, t("قيمة Input لازم تكون JSON صحيحة.", "Input value must be valid JSON."), "error");
        return;
      }
      try {
        await api(`/api/v1/projects/${encodeURIComponent(projectID())}/simulation/inputs/inject`, {
          method: "POST",
          json: {
            request_id: requestID(),
            input_id: el("simInputID").value,
            value,
            confirm_critical: !!el("simInputConfirmCritical").checked,
          },
        });
        setMessage(globalMessage, t("تم حقن TEST input داخل Simulation فقط.", "TEST input injected into Simulation only."), "success");
      } catch (error) {
        setMessage(globalMessage, errorMessage(error), "error");
      }
    });
  }

  function installObserver() {
    if (observer || !content) return;
    observer = new MutationObserver(() => {
      if (state.page === "simulation" && !el("simulationInputPanel")) loadPanel().catch(() => {});
    });
    observer.observe(content, { childList: true, subtree: false });
  }

  installObserver();
  setInterval(() => {
    if (state.page === "simulation" && !el("simulationInputPanel")) loadPanel().catch(() => {});
  }, 3000);
})();
