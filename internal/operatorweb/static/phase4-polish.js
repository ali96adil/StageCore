(() => {
  "use strict";

  const copy = {
    en: {
      broadSendConfirm: "Send this command to {count} displays?",
      broadTabletConfirm: "Send this command to {count} tablets?",
      openSource: "Open source",
      closeSource: "Close source",
      testSource: "Test source",
      sourceCommandAccepted: "Live-video command accepted.",
      sourceCommandFailed: "Live-video command failed.",
      noTargets: "No matching Stage Displays are available for this target.",
      noTabletTargets: "No matching Stage Tablets are available for this target.",
      allDisplays: "All displays",
      allTablets: "All tablets",
      group: "Group",
      location: "Location",
      device: "Device",
      tabletTarget: "Tablet target",
      tabletMedia: "Media file / logical media name",
      tabletBatchTitle: "Group tablet control",
      tabletBatchHint: "Target all tablets, a group, a location, or one device without managing raw IP/OSC.",
      prepare: "Prepare",
      play: "Play",
      pause: "Pause",
      stop: "Stop",
      blackout: "Blackout",
      selectMedia: "Select media",
      mediaRequired: "Enter a media reference for Prepare or Select media.",
      commandFailed: "One or more device commands failed.",
    },
    ar: {
      broadSendConfirm: "إرسال هذا الأمر إلى {count} شاشة؟",
      broadTabletConfirm: "إرسال هذا الأمر إلى {count} جهاز تابلت؟",
      openSource: "فتح المصدر",
      closeSource: "إغلاق المصدر",
      testSource: "فحص المصدر",
      sourceCommandAccepted: "تم قبول أمر الفيديو الحي.",
      sourceCommandFailed: "فشل أمر الفيديو الحي.",
      noTargets: "ماكو شاشات Stage Display مطابقة لهذا الهدف.",
      noTabletTargets: "ماكو أجهزة تابلت مسرح مطابقة لهذا الهدف.",
      allDisplays: "كل الشاشات",
      allTablets: "كل أجهزة التابلت",
      group: "المجموعة",
      location: "الموقع",
      device: "الجهاز",
      tabletTarget: "هدف التابلت",
      tabletMedia: "ملف الفيديو / اسم الميديا المنطقي",
      tabletBatchTitle: "تحكم جماعي بالتابلت",
      tabletBatchHint: "استهدف كل أجهزة التابلت أو مجموعة أو موقع أو جهاز واحد بدون إدارة IP أو OSC خام.",
      prepare: "تهيئة",
      play: "تشغيل",
      pause: "إيقاف مؤقت",
      stop: "إيقاف",
      blackout: "إظلام",
      selectMedia: "اختيار الميديا",
      mediaRequired: "أدخل مرجع الميديا لأمر التهيئة أو اختيار الميديا.",
      commandFailed: "فشل أمر واحد أو أكثر من أوامر الأجهزة.",
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

  function unique(values) {
    return [...new Set(values.filter(Boolean))].sort((a, b) => a.localeCompare(b));
  }

  function targetSelection(devices, target) {
    if (target === "all") return devices;
    if (target.startsWith("group:")) return devices.filter((device) => device.group_name === target.slice(6));
    if (target.startsWith("location:")) return devices.filter((device) => device.location_name === target.slice(9));
    if (target.startsWith("device:")) return devices.filter((device) => device.device_id === target.slice(7));
    return [];
  }

  function targetBody(target, selected) {
    if (target === "all") return { all: true };
    if (target.startsWith("group:")) return { group_name: target.slice(6) };
    if (target.startsWith("location:")) return { location_name: target.slice(9) };
    return { device_ids: selected.map((device) => device.device_id) };
  }

  function targetOptions(devices, allLabel) {
    const groups = unique(devices.map((device) => device.group_name));
    const locations = unique(devices.map((device) => device.location_name));
    return [
      `<option value="all">${allLabel}</option>`,
      ...groups.map((group) => `<option value="group:${esc(group)}">${text("group")}: ${esc(group)}</option>`),
      ...locations.map((location) => `<option value="location:${esc(location)}">${text("location")}: ${esc(location)}</option>`),
      ...devices.map((device) => `<option value="device:${esc(device.device_id)}">${text("device")}: ${esc(device.display_name || device.device_id)}</option>`),
    ].join("");
  }

  function responseFailed(response) {
    return (response.results || []).some((item) => {
      const status = item.command?.status;
      return item.error || ["FAILED", "REJECTED", "TIMED_OUT", "CANCELLED"].includes(status);
    });
  }

  async function sendBatch(currentProject, devices, target, command, payload, idempotencyPrefix, confirmKey, button) {
    const selected = targetSelection(devices, target);
    if (!selected.length) return { selected, empty: true };
    if (selected.length > 1 && !globalThis.confirm(text(confirmKey, { count: selected.length }))) {
      return { selected, cancelled: true };
    }
    const correlationID = requestID();
    const body = {
      ...targetBody(target, selected),
      command_type: command,
      correlation_id: correlationID,
      idempotency_key: `${idempotencyPrefix}:${command}:${correlationID}`,
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
      return { selected, response, failed: responseFailed(response) };
    } finally {
      button.disabled = false;
    }
  }

  async function sendCallboardBatch(button) {
    const currentProject = projectID();
    if (!currentProject) return;
    const target = document.getElementById("callboardTarget")?.value || "";
    const devicesPayload = await api(`/api/v1/projects/${encodeURIComponent(currentProject)}/stage-devices`);
    const displays = (devicesPayload.devices || []).filter((device) => device.device_kind === "STAGE_DISPLAY");
    const command = button.dataset.displayCommand;
    const message = document.getElementById("callboardMessage")?.value.trim() || "";
    const countdown = Number(document.getElementById("callboardCountdown")?.value || 0);
    const payload = command === "DISPLAY_COUNTDOWN"
      ? { duration_seconds: countdown, message }
      : message ? { message } : {};
    try {
      const result = await sendBatch(currentProject, displays, target, command, payload, "callboard", "broadSendConfirm", button);
      if (result.empty) {
        show(text("noTargets"), "error");
        return;
      }
      if (result.cancelled) return;
      show(result.failed ? text("commandFailed") : `${result.selected.length} · ${result.response.correlation_id}`, result.failed ? "error" : "success");
    } catch (error) {
      show(errorMessage(error), "error");
    }
  }

  async function sendTabletBatch(button) {
    const currentProject = projectID();
    if (!currentProject) return;
    const target = document.getElementById("tabletBatchTarget")?.value || "";
    const media = document.getElementById("tabletBatchMedia")?.value.trim() || "";
    const command = button.dataset.tabletBatchCommand;
    if (["TABLET_PREPARE", "TABLET_SELECT_MEDIA"].includes(command) && !media) {
      show(text("mediaRequired"), "error");
      return;
    }
    const devicesPayload = await api(`/api/v1/projects/${encodeURIComponent(currentProject)}/stage-devices`);
    const tablets = (devicesPayload.devices || []).filter((device) => device.device_kind === "TABLET_PLAYER");
    const payload = media ? { media_ref: media } : {};
    try {
      const result = await sendBatch(currentProject, tablets, target, command, payload, "tablet", "broadTabletConfirm", button);
      if (result.empty) {
        show(text("noTabletTargets"), "error");
        return;
      }
      if (result.cancelled) return;
      show(result.failed ? text("commandFailed") : `${result.selected.length} · ${result.response.correlation_id}`, result.failed ? "error" : "success");
    } catch (error) {
      show(errorMessage(error), "error");
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

  async function enhanceCallboardTargets() {
    if (state.page !== "callboard" || !projectID()) return;
    const select = document.getElementById("callboardTarget");
    if (!select || select.dataset.phase4PolishTargets === "true") return;
    try {
      const payload = await api(`/api/v1/projects/${encodeURIComponent(projectID())}/stage-devices`);
      const displays = (payload.devices || []).filter((device) => device.device_kind === "STAGE_DISPLAY");
      const previous = select.value;
      select.innerHTML = targetOptions(displays, text("allDisplays"));
      if ([...select.options].some((option) => option.value === previous)) select.value = previous;
      select.dataset.phase4PolishTargets = "true";
    } catch (_) {
      // Base Callboard remains usable if the enhancement cannot refresh targets.
    }
  }

  async function enhanceTabletBatch() {
    if (state.page !== "devices" || !projectID()) return;
    const body = document.getElementById("phase4Body");
    if (!body || document.getElementById("tabletBatchControls")) return;
    const runtimeAllowed = typeof canRuntime === "function" ? canRuntime() : true;
    if (!runtimeAllowed) return;
    try {
      const payload = await api(`/api/v1/projects/${encodeURIComponent(projectID())}/stage-devices`);
      const tablets = (payload.devices || []).filter((device) => device.device_kind === "TABLET_PLAYER");
      if (!tablets.length) return;
      const controls = document.createElement("section");
      controls.id = "tabletBatchControls";
      controls.className = "phase4-form";
      controls.innerHTML = `
        <div class="phase4-card-head">
          <div>
            <p class="eyebrow">TABLET PLAYER</p>
            <h3>${text("tabletBatchTitle")}</h3>
            <p class="muted">${text("tabletBatchHint")}</p>
          </div>
        </div>
        <div class="phase4-form-grid">
          <label>${text("tabletTarget")}<select id="tabletBatchTarget">${targetOptions(tablets, text("allTablets"))}</select></label>
          <label>${text("tabletMedia")}<input id="tabletBatchMedia" dir="ltr" placeholder="01.mp4"></label>
        </div>
        <div class="phase4-actions">
          <button class="button ghost" data-tablet-batch-command="TABLET_SELECT_MEDIA" type="button">${text("selectMedia")}</button>
          <button class="button ghost" data-tablet-batch-command="TABLET_PREPARE" type="button">${text("prepare")}</button>
          <button class="button primary" data-tablet-batch-command="TABLET_PLAY" type="button">${text("play")}</button>
          <button class="button ghost" data-tablet-batch-command="TABLET_PAUSE" type="button">${text("pause")}</button>
          <button class="button ghost" data-tablet-batch-command="TABLET_STOP" type="button">${text("stop")}</button>
          <button class="button warn" data-tablet-batch-command="TABLET_BLACKOUT" type="button">${text("blackout")}</button>
        </div>`;
      controls.querySelectorAll("[data-tablet-batch-command]").forEach((button) => {
        button.addEventListener("click", () => sendTabletBatch(button));
      });
      body.insertBefore(controls, body.firstChild);
    } catch (_) {
      // Per-device controls remain the canonical fallback.
    }
  }

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

  let enhanceScheduled = false;
  function scheduleEnhance() {
    if (enhanceScheduled) return;
    enhanceScheduled = true;
    queueMicrotask(async () => {
      enhanceScheduled = false;
      await enhanceCallboardTargets();
      await enhanceTabletBatch();
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
