(() => {
  "use strict";

  const text = {
    en: {
      nav: "Lighting Cues", title: "Lighting Cues", sub: "Build DMX lighting cues visually. Cues store logical aliases; Runtime Snapshots resolve them to the configured ESP32 channels.",
      refresh: "Refresh", newCue: "New lighting cue", noCues: "No Lighting Cues yet.", edit: "Edit", duplicate: "Duplicate", remove: "Remove",
      moveUp: "Move up", moveDown: "Move down", back: "Back", save: "Save lighting cue", name: "Cue name", label: "Label",
      operation: "Operation", set: "Set levels", fade: "Fade levels", blackout: "Blackout", fadeMS: "Fade duration (ms)", addChannel: "Add channel",
      dmxEstimate: "Estimated DMX value (offline)",
      preview: "Manual DMX preview · simulation only", previewRun: "Run preview", previewStop: "Simulate blackout",
      previewHold: "Continuous", previewTimed: "Timed", previewDuration: "Duration (ms)",
      previewIdle: "Physical output disabled", previewActive: "Simulated levels", previewBlackout: "Simulated blackout",
      previewDurationInvalid: "Enter an integer duration from 1 to 600000 ms.",
      previewRaw: "Raw channels (optional, one channel=value per line)", previewRawInvalid: "Use unique channels 1–512 and values 0–255.",
      previewCopy: "Copy raw values to draft", previewCopied: "Mapped values copied to the draft.",
      previewUnmapped: "Each raw channel needs one unique logical alias and a value within its configured range.",
      channel: "Logical channel", level: "Level %", removeChannel: "Remove", nodes: "Lighting nodes", noChannels: "Configure at least one logical channel alias first.",
      emptyLevels: "Add at least one lighting channel.", duplicateAlias: "Each logical channel can appear only once.", chooseNode: "Choose at least one lighting node.",
      saved: "Lighting Cue saved.", duplicated: "Lighting Cue duplicated.", deleted: "Lighting Cue removed.", reordered: "Lighting Cue order updated.",
      confirmDelete: "Remove this Lighting Cue?", readonly: "You do not have permission to edit project cues.", immediate: "0 = immediate",
      channels: "channels", nodesCount: "nodes", cue: "Cue", alias: "Alias",
    },
    ar: {
      nav: "كيوهات الإضاءة", title: "كيوهات الإضاءة", sub: "ابنِ كيوهات DMX بصورة رسومية. الكيو يخزن الأسماء المنطقية، والـ Runtime Snapshot يحولها لقنوات ESP32 المضبوطة.",
      refresh: "تحديث", newCue: "كيو إضاءة جديد", noCues: "ماكو كيوهات إضاءة حالياً.", edit: "تعديل", duplicate: "نسخ", remove: "حذف",
      moveUp: "للأعلى", moveDown: "للأسفل", back: "رجوع", save: "حفظ كيو الإضاءة", name: "اسم الكيو", label: "الرمز",
      operation: "العملية", set: "تعيين المستويات", fade: "Fade للمستويات", blackout: "Blackout", fadeMS: "مدة الـ Fade (ms)", addChannel: "إضافة قناة",
      dmxEstimate: "قيمة DMX المتوقعة بدون تشغيل",
      preview: "معاينة DMX يدوية · محاكاة فقط", previewRun: "ابدأ المعاينة", previewStop: "حاكي الـ Blackout",
      previewHold: "مستمر", previewTimed: "مؤقت", previewDuration: "المدة (ms)",
      previewIdle: "الخرج الفعلي معطل", previewActive: "المستويات المحاكاة", previewBlackout: "Blackout محاكى",
      previewDurationInvalid: "أدخل مدة صحيحة من 1 إلى 600000 ms.",
      previewRaw: "القنوات المباشرة (اختياري، قناة=قيمة في كل سطر)", previewRawInvalid: "استخدم قنوات فريدة من 1 إلى 512 وقيم من 0 إلى 255.",
      previewCopy: "انسخ القيم المباشرة للمسودة", previewCopied: "تم نسخ القيم المطابقة للمسودة.",
      previewUnmapped: "كل قناة تحتاج Alias منطقي واحد وقيمة ضمن نطاقه المضبوط.",
      channel: "القناة المنطقية", level: "المستوى %", removeChannel: "حذف", nodes: "عقد الإضاءة", noChannels: "اضبط Alias منطقي لقناة واحدة على الأقل أولاً.",
      emptyLevels: "أضف قناة إضاءة واحدة على الأقل.", duplicateAlias: "كل قناة منطقية تظهر مرة وحدة فقط.", chooseNode: "اختار عقدة إضاءة واحدة على الأقل.",
      saved: "تم حفظ كيو الإضاءة.", duplicated: "تم نسخ كيو الإضاءة.", deleted: "تم حذف كيو الإضاءة.", reordered: "تم تحديث ترتيب كيوهات الإضاءة.",
      confirmDelete: "تحذف كيو الإضاءة هذا؟", readonly: "ما عندك صلاحية تعديل كيوهات المشروع.", immediate: "0 = فوري",
      channels: "قنوات", nodesCount: "عقد", cue: "كيو", alias: "Alias",
    },
  };

  const commands = [
    ["LIGHTING_CHANNELS_SET", "set"],
    ["LIGHTING_CHANNELS_FADE", "fade"],
    ["LIGHTING_BLACKOUT", "blackout"],
  ];

  let controllerModel = { nodes: [] };
  let cueModel = { cues: [] };
  let allCueModel = { cues: [] };

  function lang() { return document.documentElement.lang?.toLowerCase().startsWith("ar") ? "ar" : "en"; }
  function tx(key) { return text[lang()][key] || text.en[key] || key; }
  function pid() { return state.project?.project_id || state.project?.id || ""; }
  function nodes() { return (controllerModel.nodes || []).filter((node) => node.enabled !== false); }
  function channels() {
    return nodes().flatMap((node) => (node.channels || [])
      .filter((channel) => channel.alias)
      .map((channel) => ({ ...channel, node_name: node.display_name || node.device_id })));
  }
  function operationLabel(command) { return tx(commands.find(([value]) => value === command)?.[1] || command); }
  function operationOptions(selected = "LIGHTING_CHANNELS_FADE") {
    return commands.map(([value, key]) => `<option value="${value}" ${value === selected ? "selected" : ""}>${esc(tx(key))}</option>`).join("");
  }
  function channelOptions(selected = "") {
    return `<option value="">—</option>${channels().map((channel) => {
      const label = `${channel.alias} · ${channel.display_name || channel.channel_key} · ${channel.node_name}${channel.channel_number ? ` · CH ${channel.channel_number}` : ""}`;
      return `<option value="${esc(channel.alias)}" ${channel.alias === selected ? "selected" : ""}>${esc(label)}</option>`;
    }).join("")}`;
  }
  function nodeName(id) { return nodes().find((node) => node.device_id === id)?.display_name || id || "—"; }

  async function loadModels() {
    [controllerModel, cueModel, allCueModel] = await Promise.all([
      api(`/api/v1/projects/${encodeURIComponent(pid())}/lighting-controller`),
      api(`/api/v1/projects/${encodeURIComponent(pid())}/lighting-controller/cues`),
      // The lighting-only view excludes OSC and other Cue types, but order_index
      // is unique across the entire project revision. Never allocate from only
      // the filtered lighting list: it can collide with an existing OSC Cue.
      api(`/api/v1/projects/${encodeURIComponent(pid())}/cues`),
    ]);
  }

  function cueSummary(cue) {
    if (cue.command_type === "LIGHTING_BLACKOUT") {
      return `${(cue.device_ids || []).length} ${tx("nodesCount")}${cue.fade_ms ? ` · ${cue.fade_ms} ms` : ""}`;
    }
    const count = Object.keys(cue.levels || {}).length;
    return `${count} ${tx("channels")}${cue.command_type === "LIGHTING_CHANNELS_FADE" ? ` · ${cue.fade_ms || 0} ms` : ""}`;
  }

  function cueDetails(cue) {
    if (cue.command_type === "LIGHTING_BLACKOUT") {
      return (cue.device_ids || []).map((id) => `<span class="pill neutral">${esc(nodeName(id))}</span>`).join("");
    }
    return Object.entries(cue.levels || {}).sort(([a], [b]) => a.localeCompare(b)).map(([alias, level]) =>
      `<span class="pill neutral"><span dir="ltr">${esc(alias)}</span> · ${Number(level)}%</span>`
    ).join("");
  }

  function cueCard(cue, index, total) {
    return `<article class="card lighting-cue-card" data-cue-id="${esc(cue.cue_id)}">
      <div class="lighting-cue-head">
        <div><p class="eyebrow">${esc(cue.display_label || `${tx("cue")} ${index + 1}`)}</p><h2>${esc(cue.name)}</h2><p class="muted">${esc(operationLabel(cue.command_type))} · ${esc(cueSummary(cue))}</p></div>
        ${canEdit() ? `<div class="lighting-cue-order"><button class="button ghost lighting-cue-move" data-direction="up" type="button" ${index === 0 ? "disabled" : ""}>↑ ${esc(tx("moveUp"))}</button><button class="button ghost lighting-cue-move" data-direction="down" type="button" ${index === total - 1 ? "disabled" : ""}>↓ ${esc(tx("moveDown"))}</button></div>` : ""}
      </div>
      <div class="lighting-cue-summary">${cueDetails(cue)}</div>
      ${canEdit() ? `<div class="lighting-cue-card-actions"><button class="button lighting-cue-edit" type="button">${esc(tx("edit"))}</button><button class="button ghost lighting-cue-duplicate" type="button">${esc(tx("duplicate"))}</button><button class="button danger lighting-cue-delete" type="button">${esc(tx("remove"))}</button></div>` : ""}
    </article>`;
  }

  async function renderLightingCues(message = "", kind = "") {
    if (!state.project) return;
    setPage("lighting-cues");
    await loadModels();
    const cues = cueModel.cues || [];
    content.innerHTML = `<div class="page-head lighting-cues-head">
      <div><p class="eyebrow">LIGHTING AUTHORING</p><h1>${esc(tx("title"))}</h1><p>${esc(tx("sub"))}</p></div>
      <div class="toolbar"><button id="lightingCuesRefresh" class="button ghost" type="button">${esc(tx("refresh"))}</button>${canEdit() ? `<button id="lightingNewCue" class="button primary" type="button">${esc(tx("newCue"))}</button>` : ""}</div>
    </div>
    <div id="lightingCuesMessage" class="message ${message ? kind : "hidden"}">${message ? esc(message) : ""}</div>
    ${canEdit() ? "" : `<div class="message warn">${esc(tx("readonly"))}</div>`}
    ${channels().length ? "" : `<div class="message warn">${esc(tx("noChannels"))}</div>`}
    <section class="lighting-cue-list">${cues.length ? cues.map((cue, index) => cueCard(cue, index, cues.length)).join("") : `<div class="empty">${esc(tx("noCues"))}</div>`}</section>`;
    bindCueList();
  }

  function bindCueList() {
    document.getElementById("lightingCuesRefresh")?.addEventListener("click", () => renderLightingCues().catch(showGlobalError));
    document.getElementById("lightingNewCue")?.addEventListener("click", () => renderLightingEditor(null));
    document.querySelectorAll(".lighting-cue-card").forEach((card) => {
      const cue = (cueModel.cues || []).find((item) => item.cue_id === card.dataset.cueId);
      card.querySelector(".lighting-cue-edit")?.addEventListener("click", () => renderLightingEditor(cue));
      card.querySelector(".lighting-cue-duplicate")?.addEventListener("click", () => duplicateCue(cue));
      card.querySelector(".lighting-cue-delete")?.addEventListener("click", () => deleteCue(cue));
      card.querySelectorAll(".lighting-cue-move").forEach((button) => button.addEventListener("click", () => moveCue(cue, button.dataset.direction)));
    });
  }

  function defaultCue() {
    const first = channels()[0]?.alias || "";
    return { command_type: "LIGHTING_CHANNELS_FADE", fade_ms: 1200, levels: first ? { [first]: 0 } : {}, device_ids: [] };
  }

  function nextOrderIndex() {
    return (allCueModel.cues || []).reduce((max, cue) => Math.max(max, Number(cue.order_index) || 0), -1) + 1;
  }

  function renderLightingEditor(cue) {
    setPage("lighting-cues");
    const editing = !!cue;
    const model = cue || defaultCue();
    content.innerHTML = `<div class="page-head"><div><p class="eyebrow">LIGHTING CUE</p><h1>${esc(editing ? cue.name : tx("newCue"))}</h1></div><div class="toolbar"><button id="lightingCueBack" class="button ghost" type="button">${esc(tx("back"))}</button><button id="lightingCueSave" class="button primary" type="button">${esc(tx("save"))}</button></div></div>
      <div id="lightingCueEditorMessage" class="message hidden"></div>
      <section class="card lighting-cue-editor" data-cue-id="${esc(cue?.cue_id || "")}" data-order-index="${Number(cue?.order_index ?? nextOrderIndex())}">
        <div class="lighting-cue-fields"><label>${esc(tx("name"))}<input id="lightingCueName" value="${esc(cue?.name || "")}" maxlength="120"></label><label>${esc(tx("label"))}<input id="lightingCueLabel" value="${esc(cue?.display_label || "")}" maxlength="40"></label><label>${esc(tx("operation"))}<select id="lightingCueOperation">${operationOptions(model.command_type)}</select></label></div>
        <div id="lightingCueControls"></div>
      </section>`;
    renderOperationControls(model);
    document.getElementById("lightingCueBack")?.addEventListener("click", () => renderLightingCues().catch(showGlobalError));
    document.getElementById("lightingCueOperation")?.addEventListener("change", (event) => renderOperationControls({ command_type: event.target.value, fade_ms: event.target.value === "LIGHTING_CHANNELS_FADE" ? 1200 : 0, levels: {}, device_ids: [] }));
    document.getElementById("lightingCueSave")?.addEventListener("click", saveCue);
  }

  function renderOperationControls(model) {
    const host = document.getElementById("lightingCueControls");
    if (!host) return;
    const command = document.getElementById("lightingCueOperation")?.value || model.command_type;
    if (command === "LIGHTING_BLACKOUT") {
      const selected = new Set(model.device_ids || []);
      host.innerHTML = `<div class="lighting-fade-field"><label>${esc(tx("fadeMS"))}<input id="lightingFadeMS" type="number" min="0" max="600000" value="${Number(model.fade_ms || 0)}"><small>${esc(tx("immediate"))}</small></label></div>
        <div class="section-title-row"><h2>${esc(tx("nodes"))}</h2></div>
        <div class="lighting-node-select">${nodes().map((node) => `<label class="check-row"><input class="lighting-blackout-node" type="checkbox" value="${esc(node.device_id)}" ${selected.has(node.device_id) ? "checked" : ""}> ${esc(node.display_name || node.device_id)}</label>`).join("")}</div>`;
      return;
    }
    const levels = Object.entries(model.levels || {});
    host.innerHTML = `${command === "LIGHTING_CHANNELS_FADE" ? `<div class="lighting-fade-field"><label>${esc(tx("fadeMS"))}<input id="lightingFadeMS" type="number" min="1" max="600000" value="${Number(model.fade_ms || 1200)}"></label></div>` : ""}
      <div class="section-title-row"><h2>${esc(tx("channels"))}</h2><button id="lightingAddChannel" class="button" type="button">+ ${esc(tx("addChannel"))}</button></div>
      <div id="lightingLevelRows" class="lighting-level-rows"></div>
      <details class="card">
        <summary>${esc(tx("preview"))}</summary>
        <label>${esc(tx("operation"))}
          <select id="lightingPreviewMode"><option value="continuous">${esc(tx("previewHold"))}</option><option value="timed">${esc(tx("previewTimed"))}</option></select>
        </label>
        <label>${esc(tx("previewRaw"))}<textarea id="lightingPreviewRaw" class="mono" dir="ltr" maxlength="8192" rows="3" placeholder="1=128"></textarea></label>
        <label>${esc(tx("previewDuration"))}<input id="lightingPreviewDuration" type="number" min="1" max="600000" step="1" value="1000" disabled></label>
        <div class="toolbar">
          <button id="lightingPreviewRun" class="button" type="button">${esc(tx("previewRun"))}</button>
          <button id="lightingPreviewStop" class="button ghost" type="button" disabled>${esc(tx("previewStop"))}</button>
          <button id="lightingPreviewCopy" class="button ghost" type="button">${esc(tx("previewCopy"))}</button>
        </div>
        <output id="lightingPreviewResult" aria-live="polite">${esc(tx("previewIdle"))}</output>
      </details>`;
    (levels.length ? levels : [["", 0]]).forEach(([alias, level]) => addLevelRow(alias, level));
    document.getElementById("lightingAddChannel")?.addEventListener("click", () => addLevelRow("", 0));
    bindManualPreview(host);
  }

  function previewDMXByte(channel, requested) {
    const level = Math.max(channel.minimum_level, Math.min(channel.maximum_level, requested));
    const value = Math.round((level / 100) * 255);
    return channel.inverted ? 255 - value : value;
  }

  function parseManualDMXRows(text) {
    const channels = new Map();
    if (text.length > 8192) throw new Error(tx("previewRawInvalid"));
    for (const line of text.split(/\r?\n/)) {
      if (!line.trim()) continue;
      const match = /^\s*(\d{1,3})\s*=\s*(\d{1,3})\s*$/.exec(line);
      const channel = Number(match?.[1]);
      const value = Number(match?.[2]);
      if (!match || channel < 1 || channel > 512 || value > 255 || channels.has(channel)) {
        throw new Error(tx("previewRawInvalid"));
      }
      channels.set(channel, value);
    }
    return channels;
  }

  function mapRawDMXToDraft(direct, configured) {
    if (!direct.size) throw new Error(tx("previewRawInvalid"));
    const mapped = new Map();
    for (const [number, value] of direct) {
      const candidates = configured.filter((channel) => channel.channel_number === number);
      if (candidates.length !== 1) throw new Error(tx("previewUnmapped"));
      const channel = candidates[0];
      if (configured.filter((candidate) => candidate.alias === channel.alias).length !== 1) {
        throw new Error(tx("previewUnmapped"));
      }
      const requested = ((channel.inverted ? 255 - value : value) / 255) * 100;
      const level = Math.max(channel.minimum_level, Math.min(channel.maximum_level, requested));
      if (!Number.isFinite(level) || previewDMXByte(channel, level) !== value) {
        throw new Error(tx("previewUnmapped"));
      }
      mapped.set(channel.alias, level);
    }
    return mapped;
  }

  function bindManualPreview(host) {
    const mode = host.querySelector("#lightingPreviewMode");
    const duration = host.querySelector("#lightingPreviewDuration");
    const run = host.querySelector("#lightingPreviewRun");
    const stop = host.querySelector("#lightingPreviewStop");
    const output = host.querySelector("#lightingPreviewResult");
    const raw = host.querySelector("#lightingPreviewRaw");
    const copy = host.querySelector("#lightingPreviewCopy");
    let timer;
    let frame = [];
    const paint = (blackout) => {
      if (!output.isConnected) return;
      const levels = frame.map(({channel, level, rawChannel}) => rawChannel
        ? `CH ${rawChannel} = ${blackout ? 0 : level}`
        : `${channel.alias} · CH ${channel.channel_number} = ${previewDMXByte(channel, blackout ? 0 : level)}`);
      output.textContent = `${tx(blackout ? "previewBlackout" : "previewActive")}: ${levels.join(" | ")}`;
      stop.disabled = blackout;
    };
    const blackout = () => {
      clearTimeout(timer);
      paint(true);
    };
    mode.addEventListener("change", () => { duration.disabled = mode.value !== "timed"; });
    stop.addEventListener("click", blackout);
    copy?.addEventListener("click", () => {
      try {
        const mapped = mapRawDMXToDraft(parseManualDMXRows(raw.value), channels());
        const rows = [...host.querySelectorAll(".lighting-level-row")];
        for (const [alias, level] of mapped) {
          const existing = rows.find((row) => row.querySelector(".lighting-level-alias")?.value === alias);
          if (existing) {
            const input = existing.querySelector(".lighting-level-value");
            input.value = String(level);
            input.dispatchEvent(new Event("input"));
          } else {
            addLevelRow(alias, level);
          }
        }
        for (const row of rows) {
          if (!row.querySelector(".lighting-level-alias")?.value) row.remove();
        }
        output.textContent = tx("previewCopied");
      } catch (error) {
        output.textContent = error.message;
      }
    });
    run.addEventListener("click", () => {
      try {
        const timed = mode.value === "timed";
        const milliseconds = duration.valueAsNumber;
        if (timed && (!Number.isInteger(milliseconds) || milliseconds < 1 || milliseconds > 600000)) {
          throw new Error(tx("previewDurationInvalid"));
        }
        const available = new Map(channels().map((channel) => [channel.alias, channel]));
        const direct = parseManualDMXRows(raw?.value || "");
        const next = direct.size
          ? Array.from(direct, ([rawChannel, level]) => ({rawChannel, level}))
          : Object.entries(readOperation().levels || {}).map(([alias, level]) => {
          const channel = available.get(alias);
          if (!channel?.channel_number || !Number.isFinite(level)) throw new Error(tx("emptyLevels"));
          return {channel, level};
        });
        clearTimeout(timer);
        frame = next;
        paint(false);
        if (timed) timer = setTimeout(blackout, milliseconds);
      } catch (error) {
        output.textContent = error.message;
      }
    });
  }

  function addLevelRow(alias, level) {
    const host = document.getElementById("lightingLevelRows");
    if (!host) return;
    const row = document.createElement("div");
    row.className = "lighting-level-row";
    row.innerHTML = `<label>${esc(tx("channel"))}<select class="lighting-level-alias">${channelOptions(alias)}</select></label><label>${esc(tx("level"))}<input class="lighting-level-value" type="number" min="0" max="100" step="any" value="${Number(level ?? 0)}"></label><button class="button danger lighting-level-remove" type="button">${esc(tx("removeChannel"))}</button>`;
    host.appendChild(row);
    const aliasInput = row.querySelector(".lighting-level-alias");
    const levelInput = row.querySelector(".lighting-level-value");
    const estimate = document.createElement("small");
    estimate.className = "muted";
    levelInput.after(estimate);
    const updateEstimate = () => {
      const channel = channels().find((candidate) => candidate.alias === aliasInput.value);
      const requested = levelInput.valueAsNumber;
      if (!channel?.channel_number || !Number.isFinite(requested) || requested < 0 || requested > 100) {
        estimate.textContent = "";
        return;
      }
      estimate.textContent = `${tx("dmxEstimate")}: ${previewDMXByte(channel, requested)} / 255`;
    };
    aliasInput.addEventListener("change", updateEstimate);
    levelInput.addEventListener("input", updateEstimate);
    updateEstimate();
    row.querySelector(".lighting-level-remove")?.addEventListener("click", () => row.remove());
  }

  function readOperation() {
    const command = document.getElementById("lightingCueOperation")?.value || "LIGHTING_CHANNELS_FADE";
    const fadeMS = Math.max(0, Number(document.getElementById("lightingFadeMS")?.value || 0));
    if (command === "LIGHTING_BLACKOUT") {
      const deviceIDs = [...document.querySelectorAll(".lighting-blackout-node:checked")].map((node) => node.value);
      if (!deviceIDs.length) throw new Error(tx("chooseNode"));
      return { command_type: command, device_ids: deviceIDs, fade_ms: fadeMS };
    }
    const rows = [...document.querySelectorAll(".lighting-level-row")];
    if (!rows.length) throw new Error(tx("emptyLevels"));
    const levels = {};
    for (const row of rows) {
      const alias = row.querySelector(".lighting-level-alias")?.value || "";
      if (!alias) throw new Error(tx("emptyLevels"));
      if (Object.prototype.hasOwnProperty.call(levels, alias)) throw new Error(tx("duplicateAlias"));
      levels[alias] = Math.min(100, Math.max(0, Number(row.querySelector(".lighting-level-value")?.value || 0)));
    }
    if (command === "LIGHTING_CHANNELS_FADE" && fadeMS < 1) throw new Error(tx("fadeMS"));
    return { command_type: command, levels, fade_ms: command === "LIGHTING_CHANNELS_FADE" ? fadeMS : 0 };
  }

  async function saveCue() {
    const editor = document.querySelector(".lighting-cue-editor");
    const message = document.getElementById("lightingCueEditorMessage");
    const name = document.getElementById("lightingCueName")?.value.trim() || "";
    if (!name) { setMessage(message, tx("name"), "warn"); return; }
    const button = document.getElementById("lightingCueSave");
    if (button) button.disabled = true;
    try {
      const request = readOperation();
      const translated = await api(`/api/v1/projects/${encodeURIComponent(pid())}/lighting-controller/cue-actions`, {
        method: "POST", json: { ...request, execution_mode: "PARALLEL_BARRIER" },
      });
      const descriptors = translated.actions || [];
      if (!descriptors.length) throw new Error("LIGHTING_ACTION_TRANSLATION_FAILED");
      const cueID = editor.dataset.cueId || "";
      const existing = (allCueModel.cues || []).find((cue) => cue.cue_id === cueID);
      if (cueID && !existing) throw new Error("LIGHTING_CUE_RELOAD_REQUIRED");
      const previousActions = existing?.actions || [];
      const actions = descriptors.map((action, index) => {
        const previous = previousActions.find((item) =>
          item.target_ref === action.target_ref && item.capability_key === action.capability_key);
        return {
          action_id: previous?.action_id || "",
          order_index: previous?.order_index ?? previousActions.length + index,
          execution_mode: previous?.execution_mode || action.execution_mode,
          target_ref: action.target_ref, capability_key: action.capability_key,
          parameters: { ...previous?.parameters, ...action.parameters },
          timeout_policy: { ...previous?.timeout_policy, ...action.timeout_policy },
          error_policy: previous?.error_policy || {},
          priority_class: previous?.priority_class || action.priority, enabled: previous?.enabled ?? true,
        };
      }).sort((a, b) => a.order_index - b.order_index);
      actions.forEach((action, index) => { action.order_index = index; });
      const body = {
        display_label: document.getElementById("lightingCueLabel")?.value.trim() || "",
        name, order_index: Number(editor.dataset.orderIndex || 0), cue_type: existing?.cue_type || "LIGHTING_SCENE",
        criticality: request.command_type === "LIGHTING_BLACKOUT" ? "CRITICAL" : (existing?.criticality || "NORMAL"),
        enabled: existing?.enabled ?? true, execution_policy: existing?.execution_policy || {},
        notes_summary: existing?.notes_summary ?? "Lighting Cue", actions,
      };
      await api(`/api/v1/projects/${encodeURIComponent(pid())}/cues${cueID ? `/${encodeURIComponent(cueID)}` : ""}`, { method: cueID ? "PUT" : "POST", json: body });
      await renderLightingCues(tx("saved"), "success");
    } catch (error) { setMessage(message, errorMessage(error), "error"); }
    finally { if (button) button.disabled = false; }
  }

  async function duplicateCue(cue) {
    try {
      await api(`/api/v1/projects/${encodeURIComponent(pid())}/cues/${encodeURIComponent(cue.cue_id)}/duplicate`, {
        method: "POST", json: { display_label: cue.display_label ? `${cue.display_label} copy` : "", name: `${cue.name} Copy`, order_index: nextOrderIndex() },
      });
      await renderLightingCues(tx("duplicated"), "success");
    } catch (error) { showGlobalError(error); }
  }

  async function deleteCue(cue) {
    if (!window.confirm(tx("confirmDelete"))) return;
    try {
      await api(`/api/v1/projects/${encodeURIComponent(pid())}/cues/${encodeURIComponent(cue.cue_id)}?confirm=true`, { method: "DELETE" });
      await renderLightingCues(tx("deleted"), "success");
    } catch (error) { showGlobalError(error); }
  }

  async function moveCue(cue, direction) {
    try {
      const payload = await api(`/api/v1/projects/${encodeURIComponent(pid())}/cues`);
      const all = payload.cues || [];
      const lighting = (cueModel.cues || []).slice().sort((a, b) => a.order_index - b.order_index);
      const index = lighting.findIndex((item) => item.cue_id === cue.cue_id);
      const other = lighting[index + (direction === "up" ? -1 : 1)];
      if (!other) return;
      const ids = all.map((item) => item.cue_id);
      const a = ids.indexOf(cue.cue_id); const b = ids.indexOf(other.cue_id);
      if (a < 0 || b < 0) throw new Error("LIGHTING_CUE_REORDER_MAPPING_FAILED");
      [ids[a], ids[b]] = [ids[b], ids[a]];
      await api(`/api/v1/projects/${encodeURIComponent(pid())}/cues/reorder`, { method: "POST", json: { cue_ids: ids } });
      await renderLightingCues(tx("reordered"), "success");
    } catch (error) { showGlobalError(error); }
  }

  function showGlobalError(error) { setMessage(globalMessage, errorMessage(error), "error"); }

  function installNavigation() {
    const nav = document.getElementById("workspaceNav");
    if (!nav || nav.querySelector('[data-lighting-cues-nav="true"]')) return;
    const button = document.createElement("button");
    button.type = "button"; button.className = "nav-button"; button.dataset.page = "lighting-cues"; button.dataset.phase4Nav = "true"; button.dataset.lightingCuesNav = "true"; button.textContent = tx("nav");
    button.addEventListener("click", () => renderLightingCues().catch(showGlobalError));
    const tabletScenes = nav.querySelector('[data-tablet-scenes-nav="true"]');
    if (tabletScenes?.nextSibling) nav.insertBefore(button, tabletScenes.nextSibling); else nav.appendChild(button);
    if (typeof f017FeatureNavigationChanged === "function") f017FeatureNavigationChanged();
  }

  window.renderLightingCues = renderLightingCues;
  document.addEventListener("DOMContentLoaded", () => {
    installNavigation();
    document.getElementById("languageSelect")?.addEventListener("change", () => {
      document.querySelector('[data-lighting-cues-nav="true"]')?.remove();
      installNavigation();
      if (state.page === "lighting-cues") renderLightingCues().catch(() => {});
    });
  });
})();
