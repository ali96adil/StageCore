(() => {
  "use strict";

  const text = {
    en: {
      nav: "Tablet Scenes", title: "Tablet Scenes / Playlist", sub: "Build ordered tablet cues visually. StageCore stores them as normal Draft Cues and runs them through the Cue Engine.",
      refresh: "Refresh", newScene: "New scene", allTablets: "All tablets", playlistFor: "Playlist for", noScenes: "No Tablet Scenes yet.",
      scene: "Scene", actions: "actions", edit: "Edit", duplicate: "Duplicate", remove: "Remove", moveUp: "Move up", moveDown: "Move down",
      back: "Back", save: "Save scene", name: "Scene name", label: "Label", addAction: "Add tablet action", tablet: "Tablet", operation: "Layer / action",
      mainPrepare: "Main · Prepare", mainPlay: "Main · Play", mainPause: "Main · Pause", mainStop: "Main · Stop",
      overlayPlay: "Overlay · Play", overlayClear: "Overlay · Clear", liveShow: "Live · Show", liveHide: "Live · Hide",
      blackout: "Screen · Blackout", blackoutClear: "Screen · Clear blackout", mediaNumber: "Media number", tabletCue: "Tablet Cue ID", contentMode: "Content", media: "Media number", cue: "Tablet Cue ID", liveMode: "Live source type", liveByKey: "Media key", liveByURL: "Direct URL", liveKey: "Live media key", liveURL: "Live URL", dissolve: "Dissolve (ms)", clear: "Clear action", deleteAction: "Remove action", emptyAction: "Add at least one tablet action.", chooseTablet: "Choose a tablet for every action.", needCueID: "Enter a Tablet Cue ID.", needLiveKey: "Enter a live media key.", needLiveURL: "Enter an absolute HTTP(S) live URL.", saved: "Tablet Scene saved.", duplicated: "Tablet Scene duplicated.", deleted: "Tablet Scene removed.", reordered: "Tablet Scene order updated.", confirmDelete: "Remove this Tablet Scene?", readonly: "You do not have permission to edit project cues.",
    },
    ar: {
      nav: "مشاهد التابلت", title: "مشاهد التابلت / Playlist", sub: "ابنِ كيوهات التابلت المرتبة بصورة رسومية. تبقى مخزنة كـ Draft Cues عادية وتتنفذ من Cue Engine.",
      refresh: "تحديث", newScene: "مشهد جديد", allTablets: "كل التابلتات", playlistFor: "Playlist لـ", noScenes: "ماكو Tablet Scenes حالياً.",
      scene: "مشهد", actions: "أوامر", edit: "تعديل", duplicate: "نسخ", remove: "حذف", moveUp: "للأعلى", moveDown: "للأسفل",
      back: "رجوع", save: "حفظ المشهد", name: "اسم المشهد", label: "الرمز", addAction: "إضافة أمر تابلت", tablet: "التابلت", operation: "الطبقة / الأمر",
      mainPrepare: "Main · تهيئة", mainPlay: "Main · تشغيل", mainPause: "Main · إيقاف مؤقت", mainStop: "Main · إيقاف",
      overlayPlay: "Overlay · تشغيل", overlayClear: "Overlay · مسح", liveShow: "Live · إظهار", liveHide: "Live · إخفاء",
      blackout: "الشاشة · Blackout", blackoutClear: "الشاشة · إلغاء Blackout", mediaNumber: "رقم الميديا", tabletCue: "Tablet Cue ID", contentMode: "المحتوى", media: "رقم الميديا", cue: "Tablet Cue ID", liveMode: "نوع مصدر البث", liveByKey: "Media key", liveByURL: "رابط مباشر", liveKey: "Live media key", liveURL: "رابط البث", dissolve: "Dissolve (ms)", clear: "Clear", deleteAction: "حذف الأمر", emptyAction: "أضف أمر تابلت واحد على الأقل.", chooseTablet: "اختار تابلت لكل أمر.", needCueID: "دخل Tablet Cue ID.", needLiveKey: "دخل Live media key.", needLiveURL: "دخل رابط HTTP(S) كامل للبث.", saved: "تم حفظ Tablet Scene.", duplicated: "تم نسخ Tablet Scene.", deleted: "تم حذف Tablet Scene.", reordered: "تم تحديث ترتيب Tablet Scenes.", confirmDelete: "تحذف هذا الـ Tablet Scene؟", readonly: "ما عندك صلاحية تعديل كيوهات المشروع.",
    },
  };

  const commands = [
    ["TABLET_PREPARE", "mainPrepare"], ["TABLET_PLAY", "mainPlay"],
    ["TABLET_PAUSE", "mainPause"], ["TABLET_STOP", "mainStop"], ["TABLET_OVERLAY_PLAY", "overlayPlay"],
    ["TABLET_OVERLAY_CLEAR", "overlayClear"], ["TABLET_LIVE_SHOW", "liveShow"], ["TABLET_LIVE_HIDE", "liveHide"],
    ["TABLET_BLACKOUT", "blackout"], ["TABLET_BLACKOUT_CLEAR", "blackoutClear"],
  ];

  let sceneModel = { scenes: [] };
  let tabletModel = { devices: [] };
  let filterDeviceID = "";

  function lang() { return document.documentElement.lang?.toLowerCase().startsWith("ar") ? "ar" : "en"; }
  function tx(key) { return text[lang()][key] || text.en[key] || key; }
  function pid() { return state.project?.project_id || state.project?.id || ""; }
  function devices() { return (tabletModel.devices || []).filter((device) => device.enabled !== false); }
  function deviceName(id) { return devices().find((device) => device.device_id === id)?.display_name || id || "—"; }
  function commandLabel(command) { return tx(commands.find(([value]) => value === command)?.[1] || command); }
  function commandOptions(selected = "TABLET_PLAY") { return commands.map(([value, key]) => `<option value="${value}" ${value === selected ? "selected" : ""}>${esc(tx(key))}</option>`).join(""); }
  function deviceOptions(selected = "") { return `<option value="">—</option>${devices().map((device) => `<option value="${esc(device.device_id)}" ${device.device_id === selected ? "selected" : ""}>${esc(device.display_name || device.device_id)}</option>`).join("")}`; }

  function parameterSummary(action) {
    const p = action.parameters || {};
    if (p.media_number != null) return `${tx("mediaNumber")}: ${p.media_number}`;
    if (p.tablet_cue_id) return `${tx("tabletCue")}: ${p.tablet_cue_id}`;
    if (p.media_key) return `${tx("liveKey")}: ${p.media_key}`;
    if (p.url) return `${tx("liveURL")}: ${p.url}`;
    if (p.dissolve_ms != null) return `${tx("dissolve")}: ${p.dissolve_ms}`;
    return tx("clear");
  }

  function visibleScenes() {
    const scenes = sceneModel.scenes || [];
    if (!filterDeviceID) return scenes;
    return scenes.filter((scene) => (scene.actions || []).some((action) => action.device_id === filterDeviceID));
  }

  function sceneCard(scene, index, total) {
    const actions = scene.actions || [];
    return `<article class="card tablet-scene-card" data-scene-id="${esc(scene.cue_id)}">
      <div class="tablet-scene-head">
        <div><p class="eyebrow">${esc(scene.display_label || `${tx("scene")} ${index + 1}`)}</p><h2>${esc(scene.name)}</h2><p class="muted">${actions.length} ${esc(tx("actions"))}</p></div>
        ${canEdit() ? `<div class="tablet-scene-order"><button class="button ghost tablet-scene-move" data-direction="up" type="button" ${index === 0 ? "disabled" : ""}>↑ ${esc(tx("moveUp"))}</button><button class="button ghost tablet-scene-move" data-direction="down" type="button" ${index === total - 1 ? "disabled" : ""}>↓ ${esc(tx("moveDown"))}</button></div>` : ""}
      </div>
      <div class="tablet-scene-action-list">${actions.map((action) => `<div class="tablet-scene-action-summary"><strong>${esc(action.display_name || deviceName(action.device_id))}</strong><span>${esc(commandLabel(action.command_type))}</span><small>${esc(parameterSummary(action))}</small></div>`).join("")}</div>
      ${canEdit() ? `<div class="tablet-scene-card-actions"><button class="button tablet-scene-edit" type="button">${esc(tx("edit"))}</button><button class="button ghost tablet-scene-duplicate" type="button">${esc(tx("duplicate"))}</button><button class="button danger tablet-scene-delete" type="button">${esc(tx("remove"))}</button></div>` : ""}
    </article>`;
  }

  async function loadModels() {
    [tabletModel, sceneModel] = await Promise.all([
      api(`/api/v1/projects/${encodeURIComponent(pid())}/tablet-controller`),
      api(`/api/v1/projects/${encodeURIComponent(pid())}/tablet-controller/scenes`),
    ]);
    if (filterDeviceID && !devices().some((device) => device.device_id === filterDeviceID)) filterDeviceID = "";
  }

  async function renderTabletScenes(message = "", kind = "") {
    if (!state.project) return;
    setPage("tablet-scenes");
    await loadModels();
    const scenes = visibleScenes();
    content.innerHTML = `<div class="page-head tablet-scenes-head">
      <div><p class="eyebrow">TABLET AUTHORING</p><h1>${esc(tx("title"))}</h1><p>${esc(tx("sub"))}</p></div>
      <div class="toolbar"><select id="tabletPlaylistFilter"><option value="">${esc(tx("allTablets"))}</option>${devices().map((device) => `<option value="${esc(device.device_id)}" ${filterDeviceID === device.device_id ? "selected" : ""}>${esc(`${tx("playlistFor")} ${device.display_name || device.device_id}`)}</option>`).join("")}</select><button id="tabletScenesRefresh" class="button ghost" type="button">${esc(tx("refresh"))}</button>${canEdit() ? `<button id="tabletNewScene" class="button primary" type="button">${esc(tx("newScene"))}</button>` : ""}</div>
    </div>
    <div id="tabletScenesMessage" class="message ${message ? kind : "hidden"}">${message ? esc(message) : ""}</div>
    ${canEdit() ? "" : `<div class="message warn">${esc(tx("readonly"))}</div>`}
    <section class="tablet-scene-list">${scenes.length ? scenes.map((scene, index) => sceneCard(scene, index, scenes.length)).join("") : `<div class="empty">${esc(tx("noScenes"))}</div>`}</section>`;
    bindSceneList();
  }

  function bindSceneList() {
    document.getElementById("tabletPlaylistFilter")?.addEventListener("change", (event) => { filterDeviceID = event.target.value; renderTabletScenes().catch(showGlobalError); });
    document.getElementById("tabletScenesRefresh")?.addEventListener("click", () => renderTabletScenes().catch(showGlobalError));
    document.getElementById("tabletNewScene")?.addEventListener("click", () => renderSceneEditor(null));
    document.querySelectorAll(".tablet-scene-card").forEach((card) => {
      const scene = (sceneModel.scenes || []).find((item) => item.cue_id === card.dataset.sceneId);
      card.querySelector(".tablet-scene-edit")?.addEventListener("click", () => renderSceneEditor(scene));
      card.querySelector(".tablet-scene-duplicate")?.addEventListener("click", () => duplicateScene(scene));
      card.querySelector(".tablet-scene-delete")?.addEventListener("click", () => deleteScene(scene));
      card.querySelectorAll(".tablet-scene-move").forEach((button) => button.addEventListener("click", () => moveScene(scene, button.dataset.direction)));
    });
  }

  function defaultAction() {
    return { action_id: "", device_id: filterDeviceID || devices()[0]?.device_id || "", command_type: "TABLET_PLAY", parameters: { media_number: 1 }, execution_mode: "PARALLEL_BARRIER", priority: "P1", enabled: true };
  }

  function renderSceneEditor(scene) {
    setPage("tablet-scenes");
    const editing = !!scene;
    const actions = editing && scene.actions?.length ? scene.actions : [defaultAction()];
    content.innerHTML = `<div class="page-head"><div><p class="eyebrow">TABLET SCENE</p><h1>${esc(editing ? scene.name : tx("newScene"))}</h1></div><div class="toolbar"><button id="tabletSceneBack" class="button ghost" type="button">${esc(tx("back"))}</button><button id="tabletSceneSave" class="button primary" type="button">${esc(tx("save"))}</button></div></div>
      <div id="tabletSceneEditorMessage" class="message hidden"></div>
      <section class="card tablet-scene-editor" data-cue-id="${esc(scene?.cue_id || "")}" data-order-index="${Number(scene?.order_index ?? nextOrderIndex())}">
        <div class="tablet-scene-fields"><label>${esc(tx("name"))}<input id="tabletSceneName" value="${esc(scene?.name || "")}" maxlength="120"></label><label>${esc(tx("label"))}<input id="tabletSceneLabel" value="${esc(scene?.display_label || "")}" maxlength="40"></label></div>
        <div class="section-title-row"><h2>${esc(tx("actions"))}</h2><button id="tabletSceneAddAction" class="button" type="button">+ ${esc(tx("addAction"))}</button></div>
        <div id="tabletSceneActions" class="tablet-scene-editor-actions"></div>
      </section>`;
    actions.forEach((action) => addActionRow(action));
    document.getElementById("tabletSceneBack")?.addEventListener("click", () => renderTabletScenes().catch(showGlobalError));
    document.getElementById("tabletSceneAddAction")?.addEventListener("click", () => addActionRow(defaultAction()));
    document.getElementById("tabletSceneSave")?.addEventListener("click", saveScene);
  }

  function nextOrderIndex() {
    return (sceneModel.scenes || []).reduce((max, scene) => Math.max(max, Number(scene.order_index) || 0), -1) + 1;
  }

  function addActionRow(action) {
    const host = document.getElementById("tabletSceneActions");
    if (!host) return;
    const row = document.createElement("div");
    row.className = "tablet-scene-action-editor";
    row.dataset.actionId = action.action_id || "";
    row.innerHTML = `<label>${esc(tx("tablet"))}<select class="tablet-scene-device">${deviceOptions(action.device_id)}</select></label><label>${esc(tx("operation"))}<select class="tablet-scene-command">${commandOptions(action.command_type)}</select></label><div class="tablet-scene-params"></div><button class="button danger tablet-scene-remove-action" type="button">${esc(tx("deleteAction"))}</button>`;
    host.appendChild(row);
    renderActionParameters(row, action.command_type, action.parameters || {});
    row.querySelector(".tablet-scene-command")?.addEventListener("change", (event) => renderActionParameters(row, event.target.value, {}));
    row.querySelector(".tablet-scene-remove-action")?.addEventListener("click", () => row.remove());
  }

  function renderActionParameters(row, command, params) {
    const host = row.querySelector(".tablet-scene-params");
    if (!host) return;
    if (["TABLET_PREPARE", "TABLET_PLAY"].includes(command)) {
      const cueMode = !!params.tablet_cue_id;
      host.innerHTML = `<label>${esc(tx("contentMode"))}<select class="tablet-param-mode"><option value="media" ${cueMode ? "" : "selected"}>${esc(tx("media"))}</option><option value="cue" ${cueMode ? "selected" : ""}>${esc(tx("cue"))}</option></select></label><label class="tablet-param-media ${cueMode ? "hidden" : ""}">${esc(tx("mediaNumber"))}<input type="number" min="1" value="${Number(params.media_number || 1)}"></label><label class="tablet-param-cue ${cueMode ? "" : "hidden"}">${esc(tx("tabletCue"))}<input value="${esc(params.tablet_cue_id || "")}" dir="ltr"></label>`;
      host.querySelector(".tablet-param-mode")?.addEventListener("change", (event) => {
        const cue = event.target.value === "cue";
        host.querySelector(".tablet-param-media")?.classList.toggle("hidden", cue);
        host.querySelector(".tablet-param-cue")?.classList.toggle("hidden", !cue);
      });
      return;
    }
    if (command === "TABLET_OVERLAY_PLAY") {
      host.innerHTML = `<label>${esc(tx("mediaNumber"))}<input class="tablet-param-single" type="number" min="1" value="${Number(params.media_number || 1)}"></label>`;
      return;
    }
    if (command === "TABLET_OVERLAY_CLEAR") {
      host.innerHTML = `<label>${esc(tx("dissolve"))}<input class="tablet-param-single" type="number" min="0" max="10000" value="${Number(params.dissolve_ms || 0)}"></label>`;
      return;
    }
    if (command === "TABLET_LIVE_SHOW") {
      const direct = !!params.url;
      host.innerHTML = `<label>${esc(tx("liveMode"))}<select class="tablet-live-mode"><option value="key" ${direct ? "" : "selected"}>${esc(tx("liveByKey"))}</option><option value="url" ${direct ? "selected" : ""}>${esc(tx("liveByURL"))}</option></select></label><label class="tablet-live-key ${direct ? "hidden" : ""}">${esc(tx("liveKey"))}<input value="${esc(params.media_key || "")}" dir="ltr"></label><label class="tablet-live-url ${direct ? "" : "hidden"}">${esc(tx("liveURL"))}<input value="${esc(params.url || "")}" placeholder="http://stagecore-pi:9081/api/v0/stream" dir="ltr"></label>`;
      host.querySelector(".tablet-live-mode")?.addEventListener("change", (event) => {
        const urlMode = event.target.value === "url";
        host.querySelector(".tablet-live-key")?.classList.toggle("hidden", urlMode);
        host.querySelector(".tablet-live-url")?.classList.toggle("hidden", !urlMode);
      });
      return;
    }
    host.innerHTML = `<span class="pill neutral">${esc(tx("clear"))}</span>`;
  }

  function actionPayload(row, command) {
    const params = row.querySelector(".tablet-scene-params");
    if (["TABLET_PREPARE", "TABLET_PLAY"].includes(command)) {
      if (params.querySelector(".tablet-param-mode")?.value === "cue") {
        const cueID = params.querySelector(".tablet-param-cue input")?.value.trim() || "";
        if (!cueID) throw new Error(tx("needCueID"));
        return { tablet_cue_id: cueID };
      }
      return { media_number: Math.max(1, Number(params.querySelector(".tablet-param-media input")?.value || 1)) };
    }
    if (command === "TABLET_OVERLAY_PLAY") return { media_number: Math.max(1, Number(params.querySelector(".tablet-param-single")?.value || 1)) };
    if (command === "TABLET_OVERLAY_CLEAR") return { dissolve_ms: Math.max(0, Number(params.querySelector(".tablet-param-single")?.value || 0)) };
    if (command === "TABLET_LIVE_SHOW") {
      if (params.querySelector(".tablet-live-mode")?.value === "url") {
        const value = params.querySelector(".tablet-live-url input")?.value.trim() || "";
        let parsed = null;
        try { parsed = new URL(value); } catch (_) {}
        if (!parsed || !["http:", "https:"].includes(parsed.protocol) || parsed.username || parsed.password) {
          throw new Error(tx("needLiveURL"));
        }
        return { url: value };
      }
      const mediaKey = params.querySelector(".tablet-live-key input")?.value.trim() || "";
      if (!mediaKey) throw new Error(tx("needLiveKey"));
      return { media_key: mediaKey };
    }
    return {};
  }

  async function saveScene() {
    const editor = document.querySelector(".tablet-scene-editor");
    const rows = [...document.querySelectorAll(".tablet-scene-action-editor")];
    const message = document.getElementById("tabletSceneEditorMessage");
    const name = document.getElementById("tabletSceneName")?.value.trim() || "";
    if (!name) { setMessage(message, tx("name"), "warn"); return; }
    if (!rows.length) { setMessage(message, tx("emptyAction"), "warn"); return; }
    if (rows.some((row) => !row.querySelector(".tablet-scene-device")?.value)) { setMessage(message, tx("chooseTablet"), "warn"); return; }
    const button = document.getElementById("tabletSceneSave");
    if (button) button.disabled = true;
    try {
      const actions = [];
      for (let index = 0; index < rows.length; index += 1) {
        const row = rows[index];
        const deviceID = row.querySelector(".tablet-scene-device").value;
        const command = row.querySelector(".tablet-scene-command").value;
        const translated = await api(`/api/v1/projects/${encodeURIComponent(pid())}/tablet-controller/cue-actions`, { method: "POST", json: { device_ids: [deviceID], command_type: command, execution_mode: "PARALLEL_BARRIER", priority: command.includes("BLACKOUT") ? "P0" : "P1", payload: actionPayload(row, command) } });
        const action = translated.actions?.[0];
        if (!action) throw new Error("TABLET_ACTION_TRANSLATION_FAILED");
        actions.push({ action_id: row.dataset.actionId || "", order_index: index, execution_mode: action.execution_mode, target_ref: action.target_ref, capability_key: action.capability_key, parameters: action.parameters || {}, timeout_policy: {}, error_policy: {}, priority_class: action.priority, enabled: true });
      }
      const cueID = editor.dataset.cueId || "";
      const body = { display_label: document.getElementById("tabletSceneLabel")?.value.trim() || "", name, order_index: Number(editor.dataset.orderIndex || 0), cue_type: "TABLET_SCENE", criticality: "NORMAL", enabled: true, execution_policy: {}, notes_summary: "Tablet Scene", actions };
      await api(`/api/v1/projects/${encodeURIComponent(pid())}/cues${cueID ? `/${encodeURIComponent(cueID)}` : ""}`, { method: cueID ? "PUT" : "POST", json: body });
      await renderTabletScenes(tx("saved"), "success");
    } catch (error) { setMessage(message, errorMessage(error), "error"); }
    finally { if (button) button.disabled = false; }
  }

  async function duplicateScene(scene) {
    try {
      await api(`/api/v1/projects/${encodeURIComponent(pid())}/cues/${encodeURIComponent(scene.cue_id)}/duplicate`, { method: "POST", json: { display_label: scene.display_label ? `${scene.display_label} copy` : "", name: `${scene.name} Copy`, order_index: nextOrderIndex() } });
      await renderTabletScenes(tx("duplicated"), "success");
    } catch (error) { showGlobalError(error); }
  }

  async function deleteScene(scene) {
    if (!window.confirm(tx("confirmDelete"))) return;
    try {
      await api(`/api/v1/projects/${encodeURIComponent(pid())}/cues/${encodeURIComponent(scene.cue_id)}?confirm=true`, { method: "DELETE" });
      await renderTabletScenes(tx("deleted"), "success");
    } catch (error) { showGlobalError(error); }
  }

  async function moveScene(scene, direction) {
    try {
      const payload = await api(`/api/v1/projects/${encodeURIComponent(pid())}/cues`);
      const all = payload.cues || [];
      const orderedScenes = visibleScenes().slice().sort((a, b) => a.order_index - b.order_index);
      const sceneIndex = orderedScenes.findIndex((item) => item.cue_id === scene.cue_id);
      const other = orderedScenes[sceneIndex + (direction === "up" ? -1 : 1)];
      if (!other) return;
      const ids = all.map((cue) => cue.cue_id);
      const a = ids.indexOf(scene.cue_id); const b = ids.indexOf(other.cue_id);
      if (a < 0 || b < 0) throw new Error("TABLET_SCENE_REORDER_MAPPING_FAILED");
      [ids[a], ids[b]] = [ids[b], ids[a]];
      await api(`/api/v1/projects/${encodeURIComponent(pid())}/cues/reorder`, { method: "POST", json: { cue_ids: ids } });
      await renderTabletScenes(tx("reordered"), "success");
    } catch (error) { showGlobalError(error); }
  }

  function showGlobalError(error) { setMessage(globalMessage, errorMessage(error), "error"); }

  function installNavigation() {
    const nav = document.getElementById("workspaceNav");
    if (!nav || nav.querySelector('[data-tablet-scenes-nav="true"]')) return;
    const button = document.createElement("button");
    button.type = "button"; button.className = "nav-button"; button.dataset.page = "tablet-scenes"; button.dataset.phase4Nav = "true"; button.dataset.tabletScenesNav = "true"; button.textContent = tx("nav");
    button.addEventListener("click", () => renderTabletScenes().catch(showGlobalError));
    const controller = nav.querySelector('[data-tablet-controller-nav="true"]');
    if (controller?.nextSibling) nav.insertBefore(button, controller.nextSibling); else nav.appendChild(button);
  }

  window.renderTabletScenes = renderTabletScenes;
  document.addEventListener("DOMContentLoaded", () => {
    installNavigation();
    document.getElementById("languageSelect")?.addEventListener("change", () => {
      document.querySelector('[data-tablet-scenes-nav="true"]')?.remove(); installNavigation();
      if (state.page === "tablet-scenes") renderTabletScenes().catch(() => {});
    });
  });
})();
