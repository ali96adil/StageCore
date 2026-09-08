(() => {
  "use strict";

  const copy = {
    en: {
      broadSendConfirm: "Send this command to {count} displays?",
      openSource: "Open source",
      closeSource: "Close source",
      testSource: "Test source",
      sourceCommandAccepted: "Live-video command accepted.",
      sourceCommandFailed: "Live-video command failed.",
      noTargets: "No matching Stage Displays are available for this target.",
    },
    ar: {
      broadSendConfirm: "إرسال هذا الأمر إلى {count} شاشة؟",
      openSource: "فتح المصدر",
      closeSource: "إغلاق المصدر",
      testSource: "فحص المصدر",
      sourceCommandAccepted: "تم قبول أمر الفيديو الحي.",
      sourceCommandFailed: "فشل أمر الفيديو الحي.",
      noTargets: "ماكو شاشات Stage Display مطابقة لهذا الهدف.",
    },
  };

  function lang() {
    return document.documentElement.lang?.toLowerCase().startsWith("ar") ? "ar" : "en";
  }

  function text(key, vars = {}) {
    let value = copy[lang()][key] || copy.en[key] || key;
    Object.entries(vars).forEach(([name, replacement]) => {
      value = value.replaceAll(`{${name}}`, String(replacement));
    });
    return value;
  }

  function projectID() {
    return state.project?.project_id || state.project?.id || "";
  }

  function show(message, kind = "") {
    const target = document.getElementById("phase4Message") || globalMessage;
    if (target) setMessage(target, message, kind);
  }

  function displaySelection(displays, target) {
    if (target === "all") return displays;
    if (target.startsWith("group:")) return displays.filter((device) => device.group_name === target.slice(6));
    if (target.startsWith("device:")) return displays.filter((device) => device.device_id === target.slice(7));
    return [];
  }

  function targetBody(target, selected) {
    if (target === "all") return { all: true };
    if (target.startsWith("group:")) return { group_name: target.slice(6) };
    return { device_ids: selected.map((device) => device.device_id) };
  }

  async function sendCallboardBatch(button) {
    const currentProject = projectID();
    if (!currentProject) return;
    const target = document.getElementById("callboardTarget")?.value || "";
    const devicesPayload = await api(`/api/v1/projects/${encodeURIComponent(currentProject)}/stage-devices`);
    const displays = (devicesPayload.devices || []).filter((device) => device.device_kind === "STAGE_DISPLAY");
    const selected = displaySelection(displays, target);
    if (!selected.length) {
      show(text("noTargets"), "error");
      return;
    }
    if (selected.length > 1 && !globalThis.confirm(text("broadSendConfirm", { count: selected.length }))) return;

    const command = button.dataset.displayCommand;
    const message = document.getElementById("callboardMessage")?.value.trim() || "";
    const countdown = Number(document.getElementById("callboardCountdown")?.value || 0);
    const payload = command === "DISPLAY_COUNTDOWN"
      ? { duration_seconds: countdown, message }
      : message ? { message } : {};
    const correlationID = requestID();
    const body = {
      ...targetBody(target, selected),
      command_type: command,
      correlation_id: correlationID,
      idempotency_key: `callboard:${command}:${correlationID}`,
      priority: command.includes("BLACKOUT") || command === "DISPLAY_ALERT" ? "P0" : "P1",
      deadline_at: new Date(Date.now() + 10000).toISOString(),
      payload,
    };

    button.disabled = true;
    try {
      const response = await api(`/api/v1/projects/${encodeURIComponent(currentProject)}/stage-device-commands`, {
        method: "POST",
        body: JSON.stringify(body),
      });
      const failed = (response.results || []).some((item) => {
        const status = item.command?.status;
        return item.error || ["FAILED", "REJECTED", "TIMED_OUT", "CANCELLED"].includes(status);
      });
      show(failed ? text("sourceCommandFailed") : `${selected.length} · ${response.correlation_id}`, failed ? "error" : "success");
    } catch (error) {
      show(errorMessage(error), "error");
    } finally {
      button.disabled = false;
    }
  }

  async function sendSourceCommand(source, commandType, button) {
    if (!source.execution_device_id) return;
    button.disabled = true;
    try {
      const correlationID = requestID();
      const result = await api(`/api/v1/stage-devices/${encodeURIComponent(source.execution_device_id)}/commands`, {
        method: "POST",
        body: JSON.stringify({
          command_type: commandType,
          correlation_id: correlationID,
          idempotency_key: `live-video:${source.source_id}:${commandType}:${correlationID}`,
          priority: "P1",
          deadline_at: new Date(Date.now() + 10000).toISOString(),
          payload: {
            source_id: source.source_id,
            source_class: source.source_class,
            endpoint_ref: source.endpoint_ref || "",
            config: source.config || {},
          },
        }),
      });
      const failed = ["FAILED", "REJECTED", "TIMED_OUT", "CANCELLED"].includes(result.status);
      show(failed ? text("sourceCommandFailed") : text("sourceCommandAccepted"), failed ? "error" : "success");
    } catch (error) {
      show(errorMessage(error), "error");
    } finally {
      button.disabled = false;
    }
  }

  let enhanceScheduled = false;
  async function enhanceLiveVideo() {
    if (state.page !== "video" || !projectID()) return;
    const body = document.getElementById("phase4Body");
    if (!body || body.dataset.phase4PolishBusy === "true") return;
    body.dataset.phase4PolishBusy = "true";
    try {
      const payload = await api(`/api/v1/projects/${encodeURIComponent(projectID())}/live-video-sources`);
      const sources = payload.sources || [];
      const cards = [...body.querySelectorAll(".phase4-grid .phase4-card")];
      const runtimeAllowed = typeof canRuntime === "function" ? canRuntime() : true;
      sources.forEach((source, index) => {
        const card = cards[index];
        if (!card || card.querySelector(".phase4-source-actions") || !runtimeAllowed || !source.execution_device_id) return;
        const actions = document.createElement("div");
        actions.className = "phase4-actions phase4-source-actions";
        actions.innerHTML = `
          <button class="button primary" data-source-command="VIDEO_SOURCE_OPEN" type="button">${text("openSource")}</button>
          <button class="button ghost" data-source-command="VIDEO_SOURCE_CLOSE" type="button">${text("closeSource")}</button>
          <button class="button ghost" data-source-command="VIDEO_SOURCE_INSPECT" type="button">${text("testSource")}</button>`;
        actions.querySelectorAll("[data-source-command]").forEach((button) => {
          button.addEventListener("click", () => sendSourceCommand(source, button.dataset.sourceCommand, button));
        });
        card.appendChild(actions);
      });
    } catch (_) {
      // The base Phase 4 page owns primary error presentation. Enhancement
      // failures must not replace or hide the canonical page state.
    } finally {
      body.dataset.phase4PolishBusy = "false";
    }
  }

  function scheduleEnhance() {
    if (enhanceScheduled) return;
    enhanceScheduled = true;
    queueMicrotask(async () => {
      enhanceScheduled = false;
      await enhanceLiveVideo();
    });
  }

  document.addEventListener("click", (event) => {
    const button = event.target.closest?.("[data-display-command]");
    if (!button) return;
    event.preventDefault();
    event.stopImmediatePropagation();
    void sendCallboardBatch(button);
  }, true);

  document.addEventListener("DOMContentLoaded", () => {
    const content = document.getElementById("content");
    if (!content) return;
    new MutationObserver(scheduleEnhance).observe(content, { childList: true, subtree: true });
    scheduleEnhance();
  });
})();
