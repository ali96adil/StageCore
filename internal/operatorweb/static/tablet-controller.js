(() => {
  "use strict";

  const copy = {
    en: {
      nav: "Tablet Controller",
      title: "Tablet Controller",
      sub: "Control paired Android stage tablets without IP addresses, raw OSC, target refs or JSON.",
      refresh: "Refresh",
      selectAll: "Select all",
      clear: "Clear selection",
      selected: "selected",
      noTablets: "No paired Tablet Player devices are registered for this project yet.",
      main: "Main video",
      mainSub: "Prepare and play by media number or Tablet Cue ID.",
      byMedia: "Media number",
      byCue: "Tablet Cue ID",
      prepare: "PREPARE",
      play: "GO / PLAY",
      pause: "Pause",
      stop: "Stop",
      overlay: "Overlay",
      overlaySub: "Play or clear the overlay layer without disturbing the main video.",
      overlayPlay: "Play overlay",
      overlayClear: "Clear overlay",
      dissolve: "Dissolve ms",
      live: "Live source",
      liveSub: "Show or hide a media key already configured on the tablet.",
      liveKey: "Media key",
      liveShow: "Show live",
      liveHide: "Hide live",
      blackout: "Screen safety",
      blackoutSub: "Blackout is P0. Clear blackout restores the player surface.",
      blackoutOn: "BLACKOUT",
      blackoutOff: "Clear blackout",
      cueBuilder: "Cue Builder",
      cueBuilderSub: "Convert the current graphical selection into canonical StageCore Cue Actions.",
      action: "Action",
      addCue: "Add to new Cue",
      runtimeScope: "Runtime scope",
      manifest: "Manifest",
      snapshot: "Snapshot",
      offline: "Offline",
      commandOK: "Tablet command dispatched.",
      commandPartial: "Command completed with one or more tablet errors.",
      cueReady: "Tablet actions added to a new Draft Cue. Name it and save the Cue.",
      chooseTablet: "Choose at least one tablet.",
      needLiveKey: "Enter a live media key first.",
      needCueID: "Enter a Tablet Cue ID first.",
      group: "Group",
      all: "All",
    },
    ar: {
      nav: "تحكم التابلت",
      title: "تحكم التابلت",
      sub: "تحكم بتابلتات العرض المقترنة بدون IP يدوي أو OSC خام أو Target Ref أو JSON.",
      refresh: "تحديث",
      selectAll: "اختيار الكل",
      clear: "إلغاء الاختيار",
      selected: "محدد",
      noTablets: "ماكو Tablet Player مقترن بهذا المشروع حالياً.",
      main: "الفيديو الرئيسي",
      mainSub: "تهيئة وتشغيل برقم الميديا أو Tablet Cue ID.",
      byMedia: "رقم الميديا",
      byCue: "Tablet Cue ID",
      prepare: "تهيئة PREPARE",
      play: "GO / تشغيل",
      pause: "إيقاف مؤقت",
      stop: "إيقاف",
      overlay: "Overlay",
      overlaySub: "شغّل أو امسح طبقة الـOverlay بدون ما توقف الفيديو الرئيسي.",
      overlayPlay: "تشغيل Overlay",
      overlayClear: "مسح Overlay",
      dissolve: "Dissolve ms",
      live: "المصدر الحي",
      liveSub: "إظهار أو إخفاء media key معرف مسبقاً على التابلت.",
      liveKey: "Media key",
      liveShow: "إظهار Live",
      liveHide: "إخفاء Live",
      blackout: "أمان الشاشة",
      blackoutSub: "الـBlackout أولوية P0. الإلغاء يرجع سطح المشغل.",
      blackoutOn: "BLACKOUT",
      blackoutOff: "إلغاء Blackout",
      cueBuilder: "بناء Cue",
      cueBuilderSub: "حوّل اختيارك الرسومي الحالي إلى Cue Actions رسمية داخل StageCore.",
      action: "الأمر",
      addCue: "إضافة إلى Cue جديد",
      runtimeScope: "Runtime scope",
      manifest: "Manifest",
      snapshot: "Snapshot",
      offline: "غير متصل",
      commandOK: "تم إرسال أمر التابلت.",
      commandPartial: "تم التنفيذ لكن أكو خطأ بواحد أو أكثر من التابلتات.",
      cueReady: "انضافت أوامر التابلت إلى Draft Cue جديد. سمّه واحفظه.",
      chooseTablet: "اختار تابلت واحد على الأقل.",
      needLiveKey: "دخل Live media key أولاً.",
      needCueID: "دخل Tablet Cue ID أولاً.",
      group: "مجموعة",
      all: "الكل",
    },
  };

  let model = null;
  const selected = new Set();

  function lang() {
    return document.documentElement.lang?.toLowerCase().startsWith("ar") ? "ar" : "en";
  }

  function t(key) {
    return copy[lang()][key] || copy.en[key] || key;
  }

  function projectID() {
    return state.project?.project_id || state.project?.id || "";
  }

  function shortID(value) {
    const text = String(value || "");
    if (!text) return "—";
    return text.length > 16 ? `${text.slice(0, 8)}…${text.slice(-6)}` : text;
  }

  function observed(device) {
    return device?.runtime?.observed_state && typeof device.runtime.observed_state === "object"
      ? device.runtime.observed_state
      : {};
  }

  function selectedDevices() {
    const devices = model?.devices || [];
    return devices.filter((device) => selected.has(device.device_id));
  }

  function selectionText() {
    return `${selected.size} ${t("selected")}`;
  }

  function syncSelection(devices) {
    const valid = new Set(devices.map((device) => device.device_id));
    [...selected].forEach((id) => { if (!valid.has(id)) selected.delete(id); });
    if (!selected.size) devices.filter((device) => device.enabled !== false).forEach((device) => selected.add(device.device_id));
  }

  function deviceCard(device) {
    const runtime = device.runtime || {};
    const scope = observed(device);
    const online = runtime.connection_state === "ONLINE";
    const checked = selected.has(device.device_id) ? "checked" : "";
    return `
      <label class="tablet-device-card ${online ? "online" : "offline"}">
        <div class="tablet-device-head">
          <input class="tablet-device-check" type="checkbox" data-device-id="${esc(device.device_id)}" ${checked}>
          <div><strong>${esc(device.display_name || device.device_id)}</strong><span>${esc(device.group_name || "—")}</span></div>
          <span class="pill ${online ? "good" : "bad"}">${esc(runtime.connection_state || t("offline"))}</span>
        </div>
        <div class="tablet-device-meta">
          <span>${esc(runtime.readiness || "UNKNOWN")}</span>
          <span>${esc(t("snapshot"))}: <span class="mono">${esc(shortID(scope.runtime_snapshot_id))}</span></span>
          <span>${esc(t("manifest"))}: <span class="mono">${esc(shortID(scope.tablet_manifest_id))}</span></span>
        </div>
      </label>`;
  }

  function groupButtons(groups) {
    return (groups || []).map((group) => `<button class="button ghost tablet-group-select" data-group="${esc(group)}" type="button">${esc(t("group"))}: ${esc(group)}</button>`).join("");
  }

  function commandOptions() {
    const options = [
      ["TABLET_PREPARE", t("prepare")], ["TABLET_PLAY", t("play")], ["TABLET_PAUSE", t("pause")], ["TABLET_STOP", t("stop")],
      ["TABLET_OVERLAY_PLAY", t("overlayPlay")], ["TABLET_OVERLAY_CLEAR", t("overlayClear")],
      ["TABLET_LIVE_SHOW", t("liveShow")], ["TABLET_LIVE_HIDE", t("liveHide")],
      ["TABLET_BLACKOUT", t("blackoutOn")], ["TABLET_BLACKOUT_CLEAR", t("blackoutOff")],
    ];
    return options.map(([value, label]) => `<option value="${value}">${esc(label)}</option>`).join("");
  }

  async function renderTabletController(message = "", kind = "") {
    if (!state.project) return;
    setPage("tablet-controller");
    const pid = projectID();
    model = await api(`/api/v1/projects/${encodeURIComponent(pid)}/tablet-controller`);
    const devices = model.devices || [];
    syncSelection(devices);
    content.innerHTML = `
      <div class="page-head tablet-controller-head">
        <div><p class="eyebrow">TABLET CONTROLLER</p><h1>${esc(t("title"))}</h1><p>${esc(t("sub"))}</p></div>
        <div class="toolbar"><span id="tabletSelectionCount" class="pill neutral">${esc(selectionText())}</span><button id="tabletRefresh" class="button ghost" type="button">${esc(t("refresh"))}</button></div>
      </div>
      <div id="tabletControllerMessage" class="message ${message ? kind : "hidden"}">${message ? esc(message) : ""}</div>
      ${devices.length ? `
        <section class="tablet-targets card">
          <div class="toolbar"><button id="tabletSelectAll" class="button" type="button">${esc(t("selectAll"))}</button><button id="tabletClearSelection" class="button ghost" type="button">${esc(t("clear"))}</button>${groupButtons(model.groups)}</div>
          <div class="tablet-device-grid">${devices.map(deviceCard).join("")}</div>
        </section>
        <div class="tablet-control-grid">
          <section class="card tablet-control-panel">
            <div><p class="eyebrow">MAIN</p><h2>${esc(t("main"))}</h2><p class="muted">${esc(t("mainSub"))}</p></div>
            <div class="tablet-inline-fields">
              <label><select id="tabletMainMode"><option value="media">${esc(t("byMedia"))}</option><option value="cue">${esc(t("byCue"))}</option></select></label>
              <label><input id="tabletMediaNumber" type="number" min="1" value="1" inputmode="numeric"></label>
              <label id="tabletCueIDWrap" class="hidden"><input id="tabletCueID" placeholder="cue-id" dir="ltr"></label>
            </div>
            <div class="tablet-command-row">
              <button class="button" data-tablet-command="TABLET_PREPARE" type="button">${esc(t("prepare"))}</button>
              <button class="button primary" data-tablet-command="TABLET_PLAY" type="button">${esc(t("play"))}</button>
              <button class="button ghost" data-tablet-command="TABLET_PAUSE" type="button">${esc(t("pause"))}</button>
              <button class="button ghost" data-tablet-command="TABLET_STOP" type="button">${esc(t("stop"))}</button>
            </div>
          </section>
          <section class="card tablet-control-panel">
            <div><p class="eyebrow">OVERLAY</p><h2>${esc(t("overlay"))}</h2><p class="muted">${esc(t("overlaySub"))}</p></div>
            <div class="tablet-inline-fields"><label>${esc(t("byMedia"))}<input id="tabletOverlayNumber" type="number" min="1" value="1"></label><label>${esc(t("dissolve"))}<input id="tabletOverlayDissolve" type="number" min="0" max="10000" value="0"></label></div>
            <div class="tablet-command-row"><button class="button primary" data-tablet-command="TABLET_OVERLAY_PLAY" type="button">${esc(t("overlayPlay"))}</button><button class="button ghost" data-tablet-command="TABLET_OVERLAY_CLEAR" type="button">${esc(t("overlayClear"))}</button></div>
          </section>
          <section class="card tablet-control-panel">
            <div><p class="eyebrow">LIVE</p><h2>${esc(t("live"))}</h2><p class="muted">${esc(t("liveSub"))}</p></div>
            <label>${esc(t("liveKey"))}<input id="tabletLiveKey" placeholder="camera-main" dir="ltr"></label>
            <div class="tablet-command-row"><button class="button primary" data-tablet-command="TABLET_LIVE_SHOW" type="button">${esc(t("liveShow"))}</button><button class="button ghost" data-tablet-command="TABLET_LIVE_HIDE" type="button">${esc(t("liveHide"))}</button></div>
          </section>
          <section class="card tablet-control-panel tablet-safety-panel">
            <div><p class="eyebrow">P0</p><h2>${esc(t("blackout"))}</h2><p class="muted">${esc(t("blackoutSub"))}</p></div>
            <div class="tablet-command-row"><button class="button danger big" data-tablet-command="TABLET_BLACKOUT" type="button">${esc(t("blackoutOn"))}</button><button class="button ghost" data-tablet-command="TABLET_BLACKOUT_CLEAR" type="button">${esc(t("blackoutOff"))}</button></div>
          </section>
        </div>
        ${canEdit() ? `<section class="card tablet-cue-builder"><div><p class="eyebrow">CUE ENGINE</p><h2>${esc(t("cueBuilder"))}</h2><p class="muted">${esc(t("cueBuilderSub"))}</p></div><div class="tablet-cue-row"><label>${esc(t("action"))}<select id="tabletCueCommand">${commandOptions()}</select></label><button id="tabletAddCueAction" class="button primary" type="button">${esc(t("addCue"))}</button></div></section>` : ""}
      ` : `<div class="empty">${esc(t("noTablets"))}</div>`}`;

    bindTabletController();
  }

  function setControllerMessage(message, kind = "") {
    const target = document.getElementById("tabletControllerMessage");
    if (target) setMessage(target, message, kind);
  }

  function refreshSelectionUI() {
    document.getElementById("tabletSelectionCount")?.replaceChildren(document.createTextNode(selectionText()));
    document.querySelectorAll(".tablet-device-check").forEach((checkbox) => { checkbox.checked = selected.has(checkbox.dataset.deviceId); });
  }

  function bindTabletController() {
    document.getElementById("tabletRefresh")?.addEventListener("click", () => renderTabletController());
    document.getElementById("tabletSelectAll")?.addEventListener("click", () => { (model.devices || []).forEach((device) => selected.add(device.device_id)); refreshSelectionUI(); });
    document.getElementById("tabletClearSelection")?.addEventListener("click", () => { selected.clear(); refreshSelectionUI(); });
    document.querySelectorAll(".tablet-group-select").forEach((button) => button.addEventListener("click", () => {
      selected.clear();
      (model.devices || []).filter((device) => device.group_name === button.dataset.group).forEach((device) => selected.add(device.device_id));
      refreshSelectionUI();
    }));
    document.querySelectorAll(".tablet-device-check").forEach((checkbox) => checkbox.addEventListener("change", () => {
      if (checkbox.checked) selected.add(checkbox.dataset.deviceId); else selected.delete(checkbox.dataset.deviceId);
      refreshSelectionUI();
    }));
    document.getElementById("tabletMainMode")?.addEventListener("change", (event) => {
      const cueMode = event.target.value === "cue";
      document.getElementById("tabletCueIDWrap")?.classList.toggle("hidden", !cueMode);
      document.getElementById("tabletMediaNumber")?.closest("label")?.classList.toggle("hidden", cueMode);
    });
    document.querySelectorAll("[data-tablet-command]").forEach((button) => button.addEventListener("click", async () => {
      button.disabled = true;
      try { await dispatchTabletCommand(button.dataset.tabletCommand); }
      finally { button.disabled = false; }
    }));
    document.getElementById("tabletAddCueAction")?.addEventListener("click", addTabletActionToCue);
  }

  function payloadFor(command) {
    if (command === "TABLET_PREPARE" || command === "TABLET_PLAY") {
      if (document.getElementById("tabletMainMode")?.value === "cue") {
        const cueID = document.getElementById("tabletCueID")?.value.trim() || "";
        if (!cueID) throw new Error(t("needCueID"));
        return { tablet_cue_id: cueID };
      }
      return { media_number: Math.max(1, Number(document.getElementById("tabletMediaNumber")?.value || 1)) };
    }
    if (command === "TABLET_OVERLAY_PLAY") return { media_number: Math.max(1, Number(document.getElementById("tabletOverlayNumber")?.value || 1)) };
    if (command === "TABLET_OVERLAY_CLEAR") return { dissolve_ms: Math.max(0, Number(document.getElementById("tabletOverlayDissolve")?.value || 0)) };
    if (command === "TABLET_LIVE_SHOW") {
      const key = document.getElementById("tabletLiveKey")?.value.trim() || "";
      if (!key) throw new Error(t("needLiveKey"));
      return { media_key: key };
    }
    return {};
  }

  async function dispatchTabletCommand(command, deviceIDs = null, payloadOverride = null) {
    const ids = deviceIDs || selectedDevices().map((device) => device.device_id);
    if (!ids.length) { setControllerMessage(t("chooseTablet"), "warn"); return; }
    let payload;
    try { payload = payloadOverride || payloadFor(command); }
    catch (error) { setControllerMessage(error.message, "warn"); return; }
    let runtime = null;
    try { runtime = await api(`/api/v1/projects/${encodeURIComponent(projectID())}/runtime`); } catch (_) {}
    const response = await api(`/api/v1/projects/${encodeURIComponent(projectID())}/tablet-controller/commands`, {
      method: "POST",
      json: {
        device_ids: ids,
        command_type: command,
        session_id: runtime?.session?.session_id || "",
        correlation_id: requestID(),
        priority: command.includes("BLACKOUT") ? "P0" : "P1",
        payload,
      },
    });
    const failures = (response.results || []).filter((item) => item.error || ["FAILED", "REJECTED", "TIMED_OUT", "CANCELLED"].includes(item.command?.status));
    setControllerMessage(failures.length ? `${t("commandPartial")} ${failures.map((item) => item.display_name || item.device_id).join(", ")}` : t("commandOK"), failures.length ? "warn" : "success");
  }

  async function addTabletActionToCue() {
    const ids = selectedDevices().map((device) => device.device_id);
    if (!ids.length) { setControllerMessage(t("chooseTablet"), "warn"); return; }
    const command = document.getElementById("tabletCueCommand")?.value || "TABLET_PLAY";
    let payload;
    try { payload = payloadFor(command); }
    catch (error) { setControllerMessage(error.message, "warn"); return; }
    try {
      const response = await api(`/api/v1/projects/${encodeURIComponent(projectID())}/tablet-controller/cue-actions`, {
        method: "POST",
        json: { device_ids: ids, command_type: command, execution_mode: "PARALLEL_BARRIER", priority: command.includes("BLACKOUT") ? "P0" : "P1", payload },
      });
      openCueEditor(null);
      const label = document.getElementById("cueName");
      if (label) label.value = `Tablet ${command.replace("TABLET_", "").replaceAll("_", " ")}`;
      (response.actions || []).forEach((action) => addActionEditor({
        target_ref: action.target_ref,
        capability_key: action.capability_key,
        execution_mode: action.execution_mode,
        priority_class: action.priority,
        parameters: action.parameters || {},
        timeout_policy: {},
        error_policy: {},
        enabled: true,
      }));
      setMessage(globalMessage, t("cueReady"), "success");
    } catch (error) {
      setControllerMessage(errorMessage(error), "error");
    }
  }

  function installNavigation() {
    const nav = document.getElementById("workspaceNav");
    if (!nav || nav.querySelector('[data-tablet-controller-nav="true"]')) return;
    const button = document.createElement("button");
    button.type = "button";
    button.className = "nav-button";
    button.dataset.page = "tablet-controller";
    button.dataset.phase4Nav = "true";
    button.dataset.tabletControllerNav = "true";
    button.textContent = t("nav");
    button.addEventListener("click", () => renderTabletController().catch((error) => setMessage(globalMessage, errorMessage(error), "error")));
    const before = nav.querySelector('[data-page="devices"]') || nav.querySelector('[data-page="cues"]');
    nav.insertBefore(button, before);
  }

  // Compatibility shim for the older Stage Devices quick controls. It prevents
  // their legacy media_ref payload from bypassing the canonical Tablet facade.
  document.addEventListener("click", async (event) => {
    const button = event.target.closest?.('[data-command^="TABLET_"]');
    if (!button || state.page !== "devices") return;
    event.preventDefault();
    event.stopImmediatePropagation();
    const controls = button.closest("[data-controls]");
    const deviceID = controls?.dataset.controls;
    if (!deviceID) return;
    const raw = controls.parentElement?.querySelector(".phase4-media")?.value.trim() || "";
    const match = raw.match(/\d+/);
    const payload = ["TABLET_PREPARE", "TABLET_PLAY"].includes(button.dataset.command) && match ? { media_number: Math.max(1, Number(match[0])) } : {};
    try {
      const response = await api(`/api/v1/projects/${encodeURIComponent(projectID())}/tablet-controller/commands`, {
        method: "POST",
        json: { device_ids: [deviceID], command_type: button.dataset.command, correlation_id: requestID(), priority: button.dataset.command.includes("BLACKOUT") ? "P0" : "P1", payload },
      });
      const failed = (response.results || []).some((item) => item.error);
      setMessage(globalMessage, failed ? t("commandPartial") : t("commandOK"), failed ? "warn" : "success");
    } catch (error) {
      setMessage(globalMessage, errorMessage(error), "error");
    }
  }, true);

  window.renderTabletController = renderTabletController;

  document.addEventListener("DOMContentLoaded", () => {
    installNavigation();
    document.getElementById("languageSelect")?.addEventListener("change", () => {
      document.querySelector('[data-tablet-controller-nav="true"]')?.remove();
      installNavigation();
      if (state.page === "tablet-controller") renderTabletController().catch(() => {});
    });
  });
})();
