(() => {
  "use strict";

  const text = {
    en: {
      nav: "Lighting Setup", title: "Lighting Setup", sub: "Configure authenticated ESP32 DMX nodes, channel aliases and readiness without editing JSON.",
      refresh: "Refresh", noNodes: "No paired Lighting Nodes for this project.", configure: "Configure", apply: "Apply published config",
      remove: "Remove config", confirmRemove: "Remove this node configuration from the Draft?", back: "Back", save: "Save configuration",
      node: "Node", status: "Status", connection: "Connection", readiness: "Readiness", dmx: "DMX", authority: "Authority",
      expectedHash: "Draft config", observedHash: "Node config", matches: "Config match", yes: "Yes", no: "No", unknown: "Unknown",
      reasons: "Reasons", lastSeen: "Last seen", brownout: "Brownout", configured: "Configured", notConfigured: "Needs configuration",
      channels: "Channels", number: "DMX", enabled: "Enabled", alias: "Logical alias", displayName: "Display name", kind: "Type",
      zone: "Zone", min: "Min %", max: "Max %", inverted: "Invert", identify: "Identify", identifyUnavailable: "Apply matching config before Identify.",
      saved: "Lighting configuration saved to the Draft.", removed: "Lighting configuration removed from the Draft.",
      applied: "Published configuration command sent.", identified: "Identify command sent.", readonly: "You do not have permission to edit Project configuration.",
      runtimeOnly: "Runtime commissioning requires OWNER or OPERATOR permission.", channel: "Channel", hashUnknown: "—",
      dmxHealthy: "Healthy", dmxUnhealthy: "Unhealthy", enabledNode: "Enabled", disabledNode: "Disabled",
      publishHint: "Publish the Draft before applying it to the physical node.",
    },
    ar: {
      nav: "إعداد الإضاءة", title: "إعداد الإضاءة", sub: "اضبط عقد ESP32 DMX الموثقة والقنوات والـAliases والجاهزية بدون JSON.",
      refresh: "تحديث", noNodes: "ماكو Lighting Nodes مقترنة بهذا المشروع.", configure: "إعداد", apply: "تطبيق الإعداد المنشور",
      remove: "حذف الإعداد", confirmRemove: "تحذف إعداد هذه العقدة من الـDraft؟", back: "رجوع", save: "حفظ الإعداد",
      node: "العقدة", status: "الحالة", connection: "الاتصال", readiness: "الجاهزية", dmx: "DMX", authority: "السلطة",
      expectedHash: "إعداد الـDraft", observedHash: "إعداد العقدة", matches: "تطابق الإعداد", yes: "نعم", no: "لا", unknown: "غير معروف",
      reasons: "الأسباب", lastSeen: "آخر مشاهدة", brownout: "هبوط فولت", configured: "مضبوطة", notConfigured: "تحتاج إعداد",
      channels: "القنوات", number: "DMX", enabled: "مفعلة", alias: "الاسم المنطقي", displayName: "اسم العرض", kind: "النوع",
      zone: "المنطقة", min: "أدنى %", max: "أعلى %", inverted: "عكس", identify: "تمييز", identifyUnavailable: "طبّق إعداداً مطابقاً قبل التمييز.",
      saved: "تم حفظ إعداد الإضاءة بالـDraft.", removed: "تم حذف إعداد الإضاءة من الـDraft.",
      applied: "تم إرسال أمر تطبيق الإعداد المنشور.", identified: "تم إرسال أمر التمييز.", readonly: "ما عندك صلاحية تعديل إعدادات المشروع.",
      runtimeOnly: "أوامر commissioning تحتاج صلاحية OWNER أو OPERATOR.", channel: "قناة", hashUnknown: "—",
      dmxHealthy: "سليم", dmxUnhealthy: "غير سليم", enabledNode: "مفعلة", disabledNode: "معطلة",
      publishHint: "انشر الـDraft قبل تطبيقه على العقدة الفعلية.",
    },
  };

  const kinds = ["DIMMER", "WARM_WHITE", "COLD_WHITE", "RED", "GREEN", "BLUE", "UNUSED"];
  let configModel = { nodes: [], revision: null };

  function lang() { return document.documentElement.lang?.toLowerCase().startsWith("ar") ? "ar" : "en"; }
  function tx(key) { return text[lang()][key] || text.en[key] || key; }
  function pid() { return state.project?.project_id || state.project?.id || ""; }
  function hashShort(value) { return value ? `${String(value).slice(0, 10)}…` : tx("hashUnknown"); }
  function dateText(value) { return value ? new Date(value).toLocaleString() : "—"; }
  function statusClass(status) {
    const value = String(status || "UNKNOWN").toLowerCase();
    if (value === "ready" || value === "online") return "ready";
    if (value === "blocker" || value === "offline" || value === "revoked" || value === "stale") return "blocker";
    return "warning";
  }
  function badge(value) { return `<span class="lighting-health-badge ${statusClass(value)}">${esc(value || "UNKNOWN")}</span>`; }
  function configMatchText(value) { return value === true ? tx("yes") : value === false ? tx("no") : tx("unknown"); }

  async function loadConfiguration() {
    configModel = await api(`/api/v1/projects/${encodeURIComponent(pid())}/lighting-controller/configuration`);
  }

  function nodeCard(node) {
    const h = node.health || {};
    const observed = node.observed || {};
    const canIdentify = canRuntime() && node.configured && h.connection === "ONLINE" && h.configuration_matches === true;
    const aliases = node.aliases || {};
    const reverse = {};
    Object.entries(aliases).forEach(([alias, key]) => { reverse[key] = alias; });
    const identifyButtons = node.configured && node.configuration?.channels
      ? node.configuration.channels.filter((channel) => channel.enabled && channel.kind !== "UNUSED" && reverse[channel.channel_key]).map((channel) =>
          `<button class="button ghost lighting-identify" type="button" data-device-id="${esc(node.device_id)}" data-alias="${esc(reverse[channel.channel_key])}" ${canIdentify ? "" : "disabled"}>${esc(tx("identify"))} · ${esc(reverse[channel.channel_key])}</button>`
        ).join("")
      : "";
    return `<article class="card lighting-config-card" data-device-id="${esc(node.device_id)}">
      <div class="lighting-config-head">
        <div><p class="eyebrow">${esc(tx("node"))}</p><h2>${esc(node.display_name || node.device_id)}</h2><p class="mono muted">${esc(node.device_id)}</p></div>
        <div class="lighting-config-status">${badge(h.status)} ${badge(h.connection)} ${badge(h.readiness)}</div>
      </div>
      <div class="lighting-health-grid">
        <div><span>${esc(tx("configured"))}</span><strong>${node.configured ? esc(tx("configured")) : esc(tx("notConfigured"))}</strong></div>
        <div><span>${esc(tx("dmx"))}</span><strong>${h.dmx_healthy == null ? "—" : esc(h.dmx_healthy ? tx("dmxHealthy") : tx("dmxUnhealthy"))}</strong></div>
        <div><span>${esc(tx("authority"))}</span><strong>${esc(h.authority || "—")}</strong></div>
        <div><span>${esc(tx("matches"))}</span><strong>${esc(configMatchText(h.configuration_matches))}</strong></div>
        <div><span>${esc(tx("expectedHash"))}</span><strong class="mono" title="${esc(h.expected_configuration_hash || "")}">${esc(hashShort(h.expected_configuration_hash))}</strong></div>
        <div><span>${esc(tx("observedHash"))}</span><strong class="mono" title="${esc(h.observed_configuration_hash || "")}">${esc(hashShort(h.observed_configuration_hash))}</strong></div>
        <div><span>${esc(tx("lastSeen"))}</span><strong>${esc(dateText(h.last_seen_at))}</strong></div>
        <div><span>${esc(tx("brownout"))}</span><strong>${h.brownout_warning ? "⚠" : "—"}</strong></div>
      </div>
      ${(h.reasons || []).length ? `<div class="lighting-health-reasons">${(h.reasons || []).map((reason) => `<span class="pill neutral mono">${esc(reason)}</span>`).join("")}</div>` : ""}
      ${canEdit() || canRuntime() ? `<div class="lighting-config-actions">
        ${canEdit() ? `<button class="button lighting-open-config" type="button">${esc(tx("configure"))}</button>` : ""}
        ${canRuntime() && node.configured ? `<button class="button primary lighting-apply-config" type="button">${esc(tx("apply"))}</button>` : ""}
        ${canEdit() && node.configured ? `<button class="button danger lighting-remove-config" type="button">${esc(tx("remove"))}</button>` : ""}
      </div>` : ""}
      ${identifyButtons ? `<div class="lighting-identify-row">${identifyButtons}</div>` : ""}
      ${node.configured && !canIdentify ? `<p class="muted">${esc(tx("identifyUnavailable"))}</p>` : ""}
      ${observed.current_levels ? `<div class="lighting-current-levels">${Object.entries(observed.current_levels).sort(([a],[b]) => a.localeCompare(b)).map(([key, value]) => `<span class="pill neutral"><span class="mono">${esc(key)}</span> · ${Number(value).toFixed(0)}%</span>`).join("")}</div>` : ""}
    </article>`;
  }

  async function renderLightingSetup(message = "", kind = "") {
    if (!state.project) return;
    setPage("lighting-setup");
    await loadConfiguration();
    const nodes = configModel.nodes || [];
    content.innerHTML = `<div class="page-head lighting-setup-head">
      <div><p class="eyebrow">LIGHTING CONTROLLER</p><h1>${esc(tx("title"))}</h1><p>${esc(tx("sub"))}</p></div>
      <div class="toolbar"><button id="lightingSetupRefresh" class="button ghost" type="button">${esc(tx("refresh"))}</button></div>
    </div>
    <div id="lightingSetupMessage" class="message ${message ? kind : "hidden"}">${message ? esc(message) : ""}</div>
    ${canEdit() ? "" : `<div class="message warn">${esc(tx("readonly"))}</div>`}
    ${canRuntime() ? "" : `<div class="message warn">${esc(tx("runtimeOnly"))}</div>`}
    <section class="lighting-config-list">${nodes.length ? nodes.map(nodeCard).join("") : `<div class="empty">${esc(tx("noNodes"))}</div>`}</section>`;
    bindSetup();
  }

  function bindSetup() {
    document.getElementById("lightingSetupRefresh")?.addEventListener("click", () => renderLightingSetup().catch(showGlobalError));
    document.querySelectorAll(".lighting-config-card").forEach((card) => {
      const node = (configModel.nodes || []).find((item) => item.device_id === card.dataset.deviceId);
      card.querySelector(".lighting-open-config")?.addEventListener("click", () => renderConfigEditor(node));
      card.querySelector(".lighting-apply-config")?.addEventListener("click", () => applyPublished(node));
      card.querySelector(".lighting-remove-config")?.addEventListener("click", () => removeConfiguration(node));
      card.querySelectorAll(".lighting-identify").forEach((button) => button.addEventListener("click", () => identify(node, button.dataset.alias)));
    });
  }

  function blankChannel(number) {
    return {
      channel_key: `ch${String(number).padStart(2, "0")}`,
      channel_number: number,
      display_name: `${tx("channel")} ${number}`,
      kind: "UNUSED", physical_zone: "", minimum_level: 0, maximum_level: 100,
      inverted: false, enabled: false,
    };
  }

  function editorRows(node) {
    const existing = new Map((node.configuration?.channels || []).map((channel) => [Number(channel.channel_number), { ...channel }]));
    const reverse = {};
    Object.entries(node.aliases || {}).forEach(([alias, key]) => { reverse[key] = alias; });
    const rows = [];
    for (let number = 1; number <= 12; number += 1) {
      const channel = existing.get(number) || blankChannel(number);
      rows.push({ ...channel, alias: reverse[channel.channel_key] || "" });
    }
    return rows;
  }

  function kindOptions(selected) {
    return kinds.map((kind) => `<option value="${kind}" ${kind === selected ? "selected" : ""}>${kind}</option>`).join("");
  }

  function channelRow(row) {
    return `<div class="lighting-channel-row" data-channel-key="${esc(row.channel_key)}" data-channel-number="${Number(row.channel_number)}">
      <div class="lighting-channel-number">${Number(row.channel_number)}</div>
      <label class="lighting-check"><input class="lighting-channel-enabled" type="checkbox" ${row.enabled ? "checked" : ""}> ${esc(tx("enabled"))}</label>
      <label>${esc(tx("alias"))}<input class="lighting-channel-alias mono" value="${esc(row.alias || "")}" placeholder="front_warm" dir="ltr"></label>
      <label>${esc(tx("displayName"))}<input class="lighting-channel-name" value="${esc(row.display_name || "")}"></label>
      <label>${esc(tx("kind"))}<select class="lighting-channel-kind">${kindOptions(row.kind || "UNUSED")}</select></label>
      <label>${esc(tx("zone"))}<input class="lighting-channel-zone" value="${esc(row.physical_zone || "")}"></label>
      <label>${esc(tx("min"))}<input class="lighting-channel-min" type="number" min="0" max="100" step="1" value="${Number(row.minimum_level ?? 0)}"></label>
      <label>${esc(tx("max"))}<input class="lighting-channel-max" type="number" min="0" max="100" step="1" value="${Number(row.maximum_level ?? 100)}"></label>
      <label class="lighting-check"><input class="lighting-channel-inverted" type="checkbox" ${row.inverted ? "checked" : ""}> ${esc(tx("inverted"))}</label>
    </div>`;
  }

  function renderConfigEditor(node) {
    setPage("lighting-setup");
    const rows = editorRows(node);
    content.innerHTML = `<div class="page-head"><div><p class="eyebrow">LIGHTING CONFIGURATION</p><h1>${esc(node.display_name || node.device_id)}</h1><p class="mono muted">${esc(node.device_id)}</p></div><div class="toolbar"><button id="lightingConfigBack" class="button ghost" type="button">${esc(tx("back"))}</button><button id="lightingConfigSave" class="button primary" type="button">${esc(tx("save"))}</button></div></div>
      <div id="lightingConfigMessage" class="message hidden"></div>
      <section class="lighting-channel-editor" data-device-id="${esc(node.device_id)}">
        <div class="lighting-channel-header"><span>${esc(tx("number"))}</span><span>${esc(tx("enabled"))}</span><span>${esc(tx("alias"))}</span><span>${esc(tx("displayName"))}</span><span>${esc(tx("kind"))}</span><span>${esc(tx("zone"))}</span><span>${esc(tx("min"))}</span><span>${esc(tx("max"))}</span><span>${esc(tx("inverted"))}</span></div>
        <div class="lighting-channel-rows">${rows.map(channelRow).join("")}</div>
      </section>`;
    document.getElementById("lightingConfigBack")?.addEventListener("click", () => renderLightingSetup().catch(showGlobalError));
    document.getElementById("lightingConfigSave")?.addEventListener("click", () => saveConfiguration(node));
  }

  function readConfiguration() {
    const channels = [];
    const aliases = {};
    const seenAliases = new Set();
    for (const row of document.querySelectorAll(".lighting-channel-row")) {
      const number = Number(row.dataset.channelNumber);
      const key = row.dataset.channelKey || `ch${String(number).padStart(2, "0")}`;
      const enabled = !!row.querySelector(".lighting-channel-enabled")?.checked;
      const alias = row.querySelector(".lighting-channel-alias")?.value.trim() || "";
      const kind = row.querySelector(".lighting-channel-kind")?.value || "UNUSED";
      const displayName = row.querySelector(".lighting-channel-name")?.value.trim() || `${tx("channel")} ${number}`;
      const minimum = Math.min(100, Math.max(0, Number(row.querySelector(".lighting-channel-min")?.value || 0)));
      const maximum = Math.min(100, Math.max(0, Number(row.querySelector(".lighting-channel-max")?.value || 100)));
      channels.push({
        channel_key: key, channel_number: number, display_name: displayName,
        kind: enabled ? (kind === "UNUSED" ? "DIMMER" : kind) : kind,
        physical_zone: row.querySelector(".lighting-channel-zone")?.value.trim() || "",
        minimum_level: minimum, maximum_level: maximum,
        inverted: !!row.querySelector(".lighting-channel-inverted")?.checked, enabled,
      });
      if (enabled && alias) {
        if (seenAliases.has(alias)) throw new Error(tx("alias"));
        seenAliases.add(alias);
        aliases[alias] = key;
      }
    }
    return { configuration: { schema_version: 1, channels }, aliases };
  }

  async function saveConfiguration(node) {
    const message = document.getElementById("lightingConfigMessage");
    const button = document.getElementById("lightingConfigSave");
    if (button) button.disabled = true;
    try {
      const payload = readConfiguration();
      await api(`/api/v1/projects/${encodeURIComponent(pid())}/lighting-controller/configuration/${encodeURIComponent(node.device_id)}`, {
        method: "PUT", json: payload,
      });
      await renderLightingSetup(tx("saved"), "success");
    } catch (error) {
      setMessage(message, errorMessage(error), "error");
      if (button) button.disabled = false;
    }
  }

  async function removeConfiguration(node) {
    if (!window.confirm(tx("confirmRemove"))) return;
    try {
      await api(`/api/v1/projects/${encodeURIComponent(pid())}/lighting-controller/configuration/${encodeURIComponent(node.device_id)}?confirm=true`, { method: "DELETE" });
      await renderLightingSetup(tx("removed"), "success");
    } catch (error) { showGlobalError(error); }
  }

  async function applyPublished(node) {
    try {
      await api(`/api/v1/projects/${encodeURIComponent(pid())}/lighting-controller/nodes/${encodeURIComponent(node.device_id)}/apply-published-config`, { method: "POST", json: {} });
      await renderLightingSetup(tx("applied"), "success");
    } catch (error) { showGlobalError(error); }
  }

  async function identify(node, alias) {
    try {
      await api(`/api/v1/projects/${encodeURIComponent(pid())}/lighting-controller/nodes/${encodeURIComponent(node.device_id)}/identify`, {
        method: "POST", json: { alias, level: 50, duration_ms: 1200 },
      });
      setMessage(globalMessage, `${tx("identified")} ${alias}`, "success");
    } catch (error) { showGlobalError(error); }
  }

  function showGlobalError(error) { setMessage(globalMessage, errorMessage(error), "error"); }

  function installNavigation() {
    const nav = document.getElementById("workspaceNav");
    if (!nav || nav.querySelector('[data-lighting-setup-nav="true"]')) return;
    const button = document.createElement("button");
    button.type = "button"; button.className = "nav-button"; button.dataset.page = "lighting-setup"; button.dataset.phase4Nav = "true"; button.dataset.lightingSetupNav = "true"; button.textContent = tx("nav");
    button.addEventListener("click", () => renderLightingSetup().catch(showGlobalError));
    const cues = nav.querySelector('[data-lighting-cues-nav="true"]');
    if (cues) nav.insertBefore(button, cues); else nav.appendChild(button);
  }

  window.renderLightingSetup = renderLightingSetup;
  document.addEventListener("DOMContentLoaded", () => {
    installNavigation();
    document.getElementById("languageSelect")?.addEventListener("change", () => {
      document.querySelector('[data-lighting-setup-nav="true"]')?.remove();
      installNavigation();
      if (state.page === "lighting-setup") renderLightingSetup().catch(() => {});
    });
  });
})();
