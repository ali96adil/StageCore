(() => {
  "use strict";

  const copy = {
    en: {
      broadSendConfirm: "Send {command} to {count} displays ({target})?",
      broadTabletConfirm: "Send {command} to {count} tablets ({target})?",
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
      preset: "Message preset",
      custom: "Custom",
      audienceEntry: "Audience entering",
      standby: "Standby",
      places: "Places",
      showStart: "Show starts soon",
      countdownAt: "Countdown target time (optional)",
      alertRole: "Alert role",
      alertIntensity: "Alert intensity %",
      alertMotion: "Alert motion",
      alertDuration: "Alert duration seconds",
      chimeId: "Chime identifier",
      steady: "Steady",
      pulse: "Pulse",
      flash: "Flash",
      info: "Info",
      warning: "Warning",
      critical: "Critical",
      results: "Per-device results",
      unsupported: "Selected target includes a device that does not support this action.",
    },
    ar: {
      broadSendConfirm: "إرسال {command} إلى {count} شاشة ({target})؟",
      broadTabletConfirm: "إرسال {command} إلى {count} جهاز تابلت ({target})؟",
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
      preset: "رسالة جاهزة",
      custom: "مخصص",
      audienceEntry: "دخول الجمهور",
      standby: "استعداد",
      places: "إلى الأماكن",
      showStart: "العرض يبدأ قريباً",
      countdownAt: "وقت نهاية العد التنازلي (اختياري)",
      alertRole: "دور التنبيه",
      alertIntensity: "شدة التنبيه %",
      alertMotion: "حركة التنبيه",
      alertDuration: "مدة التنبيه بالثواني",
      chimeId: "معرف الجرس",
      steady: "ثابت",
      pulse: "نبض",
      flash: "وميض",
      info: "معلومة",
      warning: "تحذير",
      critical: "حرج",
      results: "نتائج كل جهاز",
      unsupported: "الهدف المحدد يتضمن جهازاً لا يدعم هذا الإجراء.",
    },
  };

  const displayCapabilities = {
    DISPLAY_MESSAGE: "display.message.show",
    DISPLAY_COUNTDOWN: "display.countdown.show",
    DISPLAY_ALERT: "display.alert.show",
    DISPLAY_CLEAR: "display.clear",
    DISPLAY_BLACKOUT: "display.blackout",
    DISPLAY_CHIME: "display.chime.play",
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

  function targetLabel(target) {
    if (target === "all") return text("allDisplays");
    if (target.startsWith("group:")) return `${text("group")}: ${target.slice(6)}`;
    if (target.startsWith("location:")) return `${text("location")}: ${target.slice(9)}`;
    if (target.startsWith("device:")) return `${text("device")}: ${target.slice(7)}`;
    return target;
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

  function renderBatchResults(response, devices) {
    const body = document.getElementById("phase4Body");
    if (!body || !response?.results) return;
    let panel = document.getElementById("phase4BatchResults");
    if (!panel) {
      panel = document.createElement("section");
      panel.id = "phase4BatchResults";
      panel.className = "phase4-card";
      body.appendChild(panel);
    }
    const names = new Map(devices.map((device) => [device.device_id, device.display_name || device.device_id]));
    panel.innerHTML = `
      <div class="phase4-card-head"><div><p class="eyebrow">${text("results")}</p><h3 class="mono">${esc(response.correlation_id || "—")}</h3></div></div>
      <dl class="phase4-kv">${response.results.map((item) => {
        const status = item.error ? "ERROR" : item.command?.status || "UNKNOWN";
        return `<div><dt>${esc(names.get(item.device_id) || item.device_id)}</dt><dd>${esc(status)}</dd></div>`;
      }).join("")}</dl>`;
  }

  async function sendBatch(currentProject, devices, target, command, payload, idempotencyPrefix, confirmKey, button) {
    const selected = targetSelection(devices, target);
    if (!selected.length) return { selected, empty: true };
    if (selected.length > 1 && !globalThis.confirm(text(confirmKey, { count: selected.length, command, target: targetLabel(target) }))) {
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
      renderBatchResults(response, selected);
      return { selected, response, failed: responseFailed(response) };
    } finally {
      button.disabled = false;
    }
  }

  function presetDefinition(value) {
    const arabic = lang() === "ar";
    const presets = {
      audience: { category: "INFO", message: arabic ? "دخول الجمهور" : "Audience entering" },
      standby: { category: "STANDBY", message: arabic ? "استعداد" : "Standby" },
      places: { category: "PLACES", message: arabic ? "إلى الأماكن" : "Places" },
      show_start: { category: "SHOW_START", message: arabic ? "العرض يبدأ قريباً" : "Show starts soon" },
    };
    return presets[value] || null;
  }

  function callboardPayload(command) {
    const message = document.getElementById("callboardMessage")?.value.trim() || "";
    const preset = presetDefinition(document.getElementById("callboardPreset")?.value || "");
    if (command === "DISPLAY_MESSAGE") {
      return { message, ...(preset ? { category: preset.category } : {}) };
    }
    if (command === "DISPLAY_COUNTDOWN") {
      const targetValue = document.getElementById("callboardTargetAt")?.value || "";
      if (targetValue) {
        const target = new Date(targetValue);
        if (!Number.isNaN(target.getTime())) return { target_at: target.toISOString(), message };
      }
      return { duration_seconds: Number(document.getElementById("callboardCountdown")?.value || 0), message };
    }
    if (command === "DISPLAY_ALERT") {
      const payload = {
        message,
        role: document.getElementById("callboardAlertRole")?.value || "WARNING",
        intensity_percent: Number(document.getElementById("callboardAlertIntensity")?.value || 100),
        motion: document.getElementById("callboardAlertMotion")?.value || "PULSE",
        duration_seconds: Number(document.getElementById("callboardAlertDuration")?.value || 10),
      };
      const chime = document.getElementById("callboardChime")?.value.trim() || "";
      if (chime) payload.chime_id = chime;
      return payload;
    }
    if (command === "DISPLAY_CHIME") {
      return { chime_id: document.getElementById("callboardChime")?.value.trim() || "default" };
    }
    return {};
  }

  function updateCallboardCapabilities(displays) {
    const target = document.getElementById("callboardTarget")?.value || "";
    const selected = targetSelection(displays, target);
    document.querySelectorAll("[data-display-command]").forEach((button) => {
      const required = displayCapabilities[button.dataset.displayCommand];
      if (!required) return;
      const unsupported = selected.length === 0 || selected.some((device) => !(device.capabilities || []).includes(required));
      button.disabled = unsupported;
      button.title = unsupported ? text("unsupported") : "";
    });
  }

  async function sendCallboardBatch(button) {
    const currentProject = projectID();
    if (!currentProject) return;
    const target = document.getElementById("callboardTarget")?.value || "";
    const devicesPayload = await api(`/api/v1/projects/${encodeURIComponent(currentProject)}/stage-devices`);
    const displays = (devicesPayload.devices || []).filter((device) => device.device_kind === "STAGE_DISPLAY");
    const command = button.dataset.displayCommand;
    try {
      const result = await sendBatch(currentProject, displays, target, command, callboardPayload(command), "callboard", "broadSendConfirm", button);
      if (result.empty) {
        show(text("noTargets"), "error");
        return;
      }
      if (result.cancelled) return;
      show(result.failed ? text("commandFailed") : `${result.selected.length} · ${result.response.correlation_id}`, result.failed ? "error" : "success");
    } catch (error) {
      show(errorMessage(error), "error");
    } finally {
      updateCallboardCapabilities(displays);
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

  async function enhanceCallboard() {
    if (state.page !== "callboard" || !projectID()) return;
    const select = document.getElementById("callboardTarget");
    const body = document.getElementById("phase4Body");
    if (!select || !body) return;
    try {
      const payload = await api(`/api/v1/projects/${encodeURIComponent(projectID())}/stage-devices`);
      const displays = (payload.devices || []).filter((device) => device.device_kind === "STAGE_DISPLAY");
      if (select.dataset.phase4PolishTargets !== "true") {
        const previous = select.value;
        select.innerHTML = targetOptions(displays, text("allDisplays"));
        if ([...select.options].some((option) => option.value === previous)) select.value = previous;
        select.dataset.phase4PolishTargets = "true";
        select.addEventListener("change", () => updateCallboardCapabilities(displays));
      }
      const grid = body.querySelector(".phase4-form-grid");
      if (grid && !document.getElementById("callboardPreset")) {
        grid.insertAdjacentHTML("beforeend", `
          <label>${text("preset")}
            <select id="callboardPreset">
              <option value="">${text("custom")}</option>
              <option value="audience">${text("audienceEntry")}</option>
              <option value="standby">${text("standby")}</option>
              <option value="places">${text("places")}</option>
              <option value="show_start">${text("showStart")}</option>
            </select>
          </label>
          <label>${text("countdownAt")}<input id="callboardTargetAt" type="datetime-local"></label>
          <label>${text("alertRole")}
            <select id="callboardAlertRole">
              <option value="INFO">${text("info")}</option>
              <option value="STANDBY">${text("standby")}</option>
              <option value="PLACES">${text("places")}</option>
              <option value="SHOW_START">${text("showStart")}</option>
              <option value="WARNING" selected>${text("warning")}</option>
              <option value="CRITICAL">${text("critical")}</option>
            </select>
          </label>
          <label>${text("alertIntensity")}<input id="callboardAlertIntensity" type="number" min="0" max="100" value="100"></label>
          <label>${text("alertMotion")}
            <select id="callboardAlertMotion">
              <option value="STEADY">${text("steady")}</option>
              <option value="PULSE" selected>${text("pulse")}</option>
              <option value="FLASH">${text("flash")}</option>
            </select>
          </label>
          <label>${text("alertDuration")}<input id="callboardAlertDuration" type="number" min="1" max="3600" value="10"></label>
          <label>${text("chimeId")}<input id="callboardChime" maxlength="64" dir="ltr" placeholder="default"></label>`);
        document.getElementById("callboardPreset")?.addEventListener("change", (event) => {
          const preset = presetDefinition(event.target.value);
          if (!preset) return;
          const message = document.getElementById("callboardMessage");
          if (message) message.value = preset.message;
          const role = document.getElementById("callboardAlertRole");
          if (role && [...role.options].some((option) => option.value === preset.category)) role.value = preset.category;
        });
      }
      updateCallboardCapabilities(displays);
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
      await enhanceCallboard();
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
