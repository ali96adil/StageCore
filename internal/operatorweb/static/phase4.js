(() => {
  "use strict";

  const copy = {
    en: {
      devices: "Stage Devices",
      callboard: "Callboard",
      video: "Live Video",
      network: "Network Cockpit",
      devicesTitle: "Tablets and Stage Devices",
      devicesSub: "Paired devices appear here automatically. Normal operation never requires manual IP addresses or raw OSC.",
      callboardTitle: "Stage Display / Callboard",
      callboardSub: "Send messages, countdowns and alerts to one display, a group, or every display.",
      videoTitle: "Live Video Sources",
      videoSub: "Define camera/capture/stream sources and assign execution to a capable Render Node when needed.",
      networkTitle: "Stage Network Cockpit",
      networkSub: "Latest bounded observations for Hub, Companion, Stage Devices, live sources and endpoints.",
      refresh: "Refresh",
      noDevices: "No paired Stage Devices are registered for this project yet.",
      noDisplays: "No Stage Displays are registered for this project yet.",
      noSources: "No live video sources are configured yet.",
      noNetwork: "No network observations have been recorded yet.",
      media: "Media file / logical media name",
      prepare: "Prepare",
      play: "Play",
      pause: "Pause",
      stop: "Stop",
      blackout: "Blackout",
      select: "Select media",
      name: "Name",
      kind: "Kind",
      group: "Group",
      location: "Location",
      connection: "Connection",
      readiness: "Readiness",
      lastSeen: "Last seen",
      version: "Client",
      capabilities: "Capabilities",
      target: "Target",
      allDisplays: "All displays",
      message: "Message",
      countdownSeconds: "Countdown seconds",
      sendMessage: "Show message",
      startCountdown: "Start countdown",
      alert: "Alert",
      clear: "Clear",
      chime: "Chime",
      sourceId: "Source ID",
      sourceName: "Source name",
      sourceClass: "Source class",
      endpoint: "Endpoint / logical source reference",
      renderNode: "Execution / Render Node",
      required: "Required for show",
      enabled: "Enabled",
      saveSource: "Save source",
      localCamera: "Local camera",
      usbCapture: "USB capture",
      networkStream: "Network stream",
      targetKind: "Target kind",
      transport: "Transport",
      reachability: "Reachability",
      latency: "Latency",
      jitter: "Jitter",
      reason: "Reason",
      observed: "Observed",
      commandAccepted: "Command accepted.",
      commandFailed: "Command failed.",
      sourceSaved: "Live video source saved.",
      draftDangerTitle: "Discard current Draft",
      draftDangerBody: "Restore the validated parent revision and keep this Draft as SUPERSEDED for audit history.",
      discardDraft: "Discard Draft",
      discardConfirm: "Discard Draft {draft} and restore {parent}? The abandoned Draft will remain in history as SUPERSEDED.",
      discardReason: "Optional reason for discarding this Draft",
      draftDiscarded: "Draft discarded and validated revision restored.",
      unknown: "Unknown",
      none: "None",
    },
    ar: {
      devices: "أجهزة المسرح",
      callboard: "شاشة الكواليس",
      video: "الفيديو الحي",
      network: "شبكة المسرح",
      devicesTitle: "التابلت وأجهزة المسرح",
      devicesSub: "الأجهزة المقترنة تظهر هنا تلقائياً. التشغيل الطبيعي لا يحتاج IP يدوي ولا أوامر OSC خام.",
      callboardTitle: "شاشة المسرح / Callboard",
      callboardSub: "أرسل رسالة أو عدّاً تنازلياً أو تنبيهاً لشاشة واحدة أو مجموعة أو لكل الشاشات.",
      videoTitle: "مصادر الفيديو الحي",
      videoSub: "عرّف الكاميرا أو كرت الالتقاط أو البث الشبكي وحدد Render Node للتنفيذ عند الحاجة.",
      networkTitle: "مراقبة شبكة المسرح",
      networkSub: "آخر حالة مسجلة للـHub والـCompanion وأجهزة المسرح ومصادر الفيديو ونقاط الاتصال.",
      refresh: "تحديث",
      noDevices: "ماكو أجهزة Stage Device مقترنة بهذا المشروع حالياً.",
      noDisplays: "ماكو شاشات Stage Display مسجلة بهذا المشروع حالياً.",
      noSources: "ماكو مصادر فيديو حي معرفة حالياً.",
      noNetwork: "ماكو قراءات شبكة مسجلة حالياً.",
      media: "ملف الفيديو / اسم الميديا المنطقي",
      prepare: "تهيئة",
      play: "تشغيل",
      pause: "إيقاف مؤقت",
      stop: "إيقاف",
      blackout: "إظلام",
      select: "اختيار الميديا",
      name: "الاسم",
      kind: "النوع",
      group: "المجموعة",
      location: "الموقع",
      connection: "الاتصال",
      readiness: "الجاهزية",
      lastSeen: "آخر ظهور",
      version: "نسخة العميل",
      capabilities: "القدرات",
      target: "الهدف",
      allDisplays: "كل الشاشات",
      message: "الرسالة",
      countdownSeconds: "ثواني العد التنازلي",
      sendMessage: "عرض الرسالة",
      startCountdown: "بدء العد",
      alert: "تنبيه",
      clear: "مسح",
      chime: "جرس",
      sourceId: "معرف المصدر",
      sourceName: "اسم المصدر",
      sourceClass: "نوع المصدر",
      endpoint: "مرجع المصدر / Endpoint",
      renderNode: "جهاز التنفيذ / Render Node",
      required: "مطلوب للعرض",
      enabled: "مفعّل",
      saveSource: "حفظ المصدر",
      localCamera: "كاميرا محلية",
      usbCapture: "كرت التقاط USB",
      networkStream: "بث شبكي",
      targetKind: "نوع الهدف",
      transport: "النقل",
      reachability: "الوصول",
      latency: "التأخير",
      jitter: "التذبذب",
      reason: "السبب",
      observed: "وقت القراءة",
      commandAccepted: "تم قبول الأمر.",
      commandFailed: "فشل الأمر.",
      sourceSaved: "تم حفظ مصدر الفيديو الحي.",
      draftDangerTitle: "إلغاء الـDraft الحالي",
      draftDangerBody: "يرجع آخر نسخة VALIDATED ويحتفظ بالـDraft الملغى كـSUPERSEDED ضمن سجل التدقيق.",
      discardDraft: "إلغاء الـDraft",
      discardConfirm: "تلغي Draft {draft} وترجع {parent}؟ الـDraft الملغى يبقى محفوظاً بالتاريخ كـSUPERSEDED.",
      discardReason: "سبب الإلغاء (اختياري)",
      draftDiscarded: "تم إلغاء الـDraft وإرجاع النسخة المعتمدة.",
      unknown: "غير معروف",
      none: "لا يوجد",
    },
  };

  function lang() {
    return document.documentElement.lang?.toLowerCase().startsWith("ar") ? "ar" : "en";
  }

  function t(key, vars = {}) {
    let value = copy[lang()][key] || copy.en[key] || key;
    Object.entries(vars).forEach(([name, replacement]) => {
      value = value.replaceAll(`{${name}}`, String(replacement));
    });
    return value;
  }

  function statusClass(value) {
    return String(value || "UNKNOWN").toLowerCase().replaceAll("_", "-");
  }

  function pulse(value) {
    const display = value || "UNKNOWN";
    return `<span class="phase4-pulse ${statusClass(display)}">${esc(display)}</span>`;
  }

  function when(value) {
    if (!value) return "—";
    const parsed = new Date(value);
    return Number.isNaN(parsed.getTime()) ? esc(String(value)) : esc(parsed.toLocaleString());
  }

  function currentProjectID() {
    return state.project?.project_id || state.project?.id || "";
  }

  function pageHeader(title, subtitle, refreshPage) {
    content.innerHTML = `
      <div class="page-head">
        <div>
          <p class="eyebrow">PHASE 4 · DEVICE EXPERIENCE</p>
          <h1>${esc(title)}</h1>
          <p class="muted">${esc(subtitle)}</p>
        </div>
        <button id="phase4Refresh" class="button ghost" type="button">${esc(t("refresh"))}</button>
      </div>
      <div id="phase4Message" class="message hidden" role="status"></div>
      <div id="phase4Body"></div>`;
    document.getElementById("phase4Refresh")?.addEventListener("click", refreshPage);
  }

  function phase4Message(message, kind = "") {
    const target = document.getElementById("phase4Message");
    if (target) setMessage(target, message, kind);
  }

  async function issueCommand(deviceID, commandType, payload = {}) {
    const projectID = currentProjectID();
    if (!projectID) throw new Error("No active project");
    const result = await api(`/api/v1/stage-devices/${encodeURIComponent(deviceID)}/commands`, {
      method: "POST",
      body: JSON.stringify({
        command_type: commandType,
        idempotency_key: `${commandType}:${deviceID}:${requestID()}`,
        correlation_id: requestID(),
        priority: commandType.includes("BLACKOUT") || commandType === "DISPLAY_ALERT" ? "P0" : "P1",
        deadline_at: new Date(Date.now() + 10000).toISOString(),
        payload,
      }),
    });
    if (result.status === "FAILED" || result.status === "REJECTED" || result.status === "TIMED_OUT") {
      phase4Message(t("commandFailed"), "error");
    } else {
      phase4Message(t("commandAccepted"), "success");
    }
    return result;
  }

  function tabletControls(device) {
    if (device.device_kind !== "TABLET_PLAYER" || !canRuntime()) return "";
    return `
      <label>${esc(t("media"))}
        <input class="phase4-media" data-device="${esc(device.device_id)}" placeholder="01.mp4" dir="ltr">
      </label>
      <div class="phase4-actions" data-controls="${esc(device.device_id)}">
        <button class="button ghost" data-command="TABLET_PREPARE" type="button">${esc(t("prepare"))}</button>
        <button class="button primary" data-command="TABLET_PLAY" type="button">${esc(t("play"))}</button>
        <button class="button ghost" data-command="TABLET_PAUSE" type="button">${esc(t("pause"))}</button>
        <button class="button ghost" data-command="TABLET_STOP" type="button">${esc(t("stop"))}</button>
        <button class="button warn" data-command="TABLET_BLACKOUT" type="button">${esc(t("blackout"))}</button>
      </div>`;
  }

  function deviceCard(device) {
    const runtime = device.runtime || {};
    const caps = Array.isArray(device.capabilities) ? device.capabilities : [];
    return `
      <article class="phase4-card">
        <div class="phase4-card-head">
          <div>
            <p class="eyebrow">${esc(device.device_kind || "STAGE_DEVICE")}</p>
            <h3>${esc(device.display_name || device.device_id)}</h3>
          </div>
          <div class="phase4-status-row">${pulse(runtime.connection_state || "OFFLINE")} ${pulse(runtime.readiness || "UNKNOWN")}</div>
        </div>
        <dl class="phase4-kv">
          <div><dt>${esc(t("group"))}</dt><dd>${esc(device.group_name || "—")}</dd></div>
          <div><dt>${esc(t("location"))}</dt><dd>${esc(device.location_name || "—")}</dd></div>
          <div><dt>${esc(t("version"))}</dt><dd>${esc(device.client_version || "—")}</dd></div>
          <div><dt>${esc(t("lastSeen"))}</dt><dd>${when(runtime.last_seen_at)}</dd></div>
          <div><dt>ID</dt><dd class="mono">${esc(device.device_id)}</dd></div>
          <div><dt>Protocol</dt><dd class="mono">${esc(device.protocol_version || "—")}</dd></div>
        </dl>
        <div>
          <p class="muted">${esc(t("capabilities"))}</p>
          <div class="phase4-capabilities">${caps.length ? caps.map((cap) => `<span>${esc(cap)}</span>`).join("") : `<span>${esc(t("none"))}</span>`}</div>
        </div>
        ${tabletControls(device)}
      </article>`;
  }

  async function renderStageDevices() {
    pageHeader(t("devicesTitle"), t("devicesSub"), renderStageDevices);
    const projectID = currentProjectID();
    const payload = await api(`/api/v1/projects/${encodeURIComponent(projectID)}/stage-devices`);
    const devices = payload.devices || [];
    const body = document.getElementById("phase4Body");
    body.innerHTML = devices.length
      ? `<div class="phase4-grid">${devices.map(deviceCard).join("")}</div>`
      : `<div class="phase4-empty">${esc(t("noDevices"))}</div>`;

    body.querySelectorAll("[data-command]").forEach((button) => {
      button.addEventListener("click", async () => {
        const controls = button.closest("[data-controls]");
        const deviceID = controls?.dataset.controls;
        const media = body.querySelector(`.phase4-media[data-device="${CSS.escape(deviceID)}"]`)?.value.trim() || "";
        button.disabled = true;
        try {
          const payload = media ? { media_ref: media } : {};
          await issueCommand(deviceID, button.dataset.command, payload);
          await renderStageDevices();
        } catch (error) {
          phase4Message(errorMessage(error), "error");
        } finally {
          button.disabled = false;
        }
      });
    });
  }

  function displayTargets(displays) {
    const groups = [...new Set(displays.map((device) => device.group_name).filter(Boolean))].sort();
    return [
      `<option value="all">${esc(t("allDisplays"))}</option>`,
      ...groups.map((group) => `<option value="group:${esc(group)}">${esc(t("group"))}: ${esc(group)}</option>`),
      ...displays.map((device) => `<option value="device:${esc(device.device_id)}">${esc(device.display_name || device.device_id)}</option>`),
    ].join("");
  }

  function resolveDisplays(displays, target) {
    if (target === "all") return displays;
    if (target.startsWith("group:")) return displays.filter((device) => device.group_name === target.slice(6));
    if (target.startsWith("device:")) return displays.filter((device) => device.device_id === target.slice(7));
    return [];
  }

  async function renderCallboard() {
    pageHeader(t("callboardTitle"), t("callboardSub"), renderCallboard);
    const projectID = currentProjectID();
    const payload = await api(`/api/v1/projects/${encodeURIComponent(projectID)}/stage-devices`);
    const displays = (payload.devices || []).filter((device) => device.device_kind === "STAGE_DISPLAY");
    const body = document.getElementById("phase4Body");
    if (!displays.length) {
      body.innerHTML = `<div class="phase4-empty">${esc(t("noDisplays"))}</div>`;
      return;
    }
    body.innerHTML = `
      <section class="phase4-form">
        <div class="phase4-form-grid">
          <label>${esc(t("target"))}<select id="callboardTarget">${displayTargets(displays)}</select></label>
          <label>${esc(t("message"))}<input id="callboardMessage" maxlength="240"></label>
          <label>${esc(t("countdownSeconds"))}<input id="callboardCountdown" type="number" min="1" max="86400" value="60"></label>
        </div>
        <div class="phase4-actions">
          <button class="button primary" data-display-command="DISPLAY_MESSAGE" type="button">${esc(t("sendMessage"))}</button>
          <button class="button primary" data-display-command="DISPLAY_COUNTDOWN" type="button">${esc(t("startCountdown"))}</button>
          <button class="button warn" data-display-command="DISPLAY_ALERT" type="button">${esc(t("alert"))}</button>
          <button class="button ghost" data-display-command="DISPLAY_CHIME" type="button">${esc(t("chime"))}</button>
          <button class="button ghost" data-display-command="DISPLAY_CLEAR" type="button">${esc(t("clear"))}</button>
          <button class="button danger" data-display-command="DISPLAY_BLACKOUT" type="button">${esc(t("blackout"))}</button>
        </div>
      </section>
      <div class="phase4-grid">${displays.map(deviceCard).join("")}</div>`;

    body.querySelectorAll("[data-display-command]").forEach((button) => {
      button.addEventListener("click", async () => {
        const command = button.dataset.displayCommand;
        const selected = resolveDisplays(displays, document.getElementById("callboardTarget").value);
        const message = document.getElementById("callboardMessage").value.trim();
        const countdown = Number(document.getElementById("callboardCountdown").value || 0);
        const commandPayload = command === "DISPLAY_COUNTDOWN"
          ? { duration_seconds: countdown, message }
          : message ? { message } : {};
        button.disabled = true;
        try {
          for (const device of selected) await issueCommand(device.device_id, command, commandPayload);
        } catch (error) {
          phase4Message(errorMessage(error), "error");
        } finally {
          button.disabled = false;
        }
      });
    });
  }

  function sourceCard(source) {
    return `
      <article class="phase4-card">
        <div class="phase4-card-head">
          <div><p class="eyebrow">${esc(source.source_class)}</p><h3>${esc(source.name)}</h3></div>
          ${pulse(source.readiness || "UNKNOWN")}
        </div>
        <dl class="phase4-kv">
          <div><dt>${esc(t("endpoint"))}</dt><dd class="mono">${esc(source.endpoint_ref || "—")}</dd></div>
          <div><dt>${esc(t("renderNode"))}</dt><dd class="mono">${esc(source.execution_device_id || "—")}</dd></div>
          <div><dt>${esc(t("required"))}</dt><dd>${source.required ? "✓" : "—"}</dd></div>
          <div><dt>${esc(t("enabled"))}</dt><dd>${source.desired_enabled ? "✓" : "—"}</dd></div>
          <div><dt>ID</dt><dd class="mono">${esc(source.source_id)}</dd></div>
          <div><dt>${esc(t("observed"))}</dt><dd>${when(source.last_observed_at)}</dd></div>
        </dl>
      </article>`;
  }

  async function renderLiveVideo() {
    pageHeader(t("videoTitle"), t("videoSub"), renderLiveVideo);
    const projectID = currentProjectID();
    const [sourcesPayload, devicesPayload] = await Promise.all([
      api(`/api/v1/projects/${encodeURIComponent(projectID)}/live-video-sources`),
      api(`/api/v1/projects/${encodeURIComponent(projectID)}/stage-devices`),
    ]);
    const sources = sourcesPayload.sources || [];
    const renderNodes = (devicesPayload.devices || []).filter((device) => device.device_kind === "RENDER_NODE");
    const body = document.getElementById("phase4Body");
    const editable = canEdit();
    body.innerHTML = `
      ${editable ? `<form id="liveSourceForm" class="phase4-form">
        <div class="phase4-form-grid">
          <label>${esc(t("sourceName"))}<input id="liveSourceName" required></label>
          <label>${esc(t("sourceClass"))}
            <select id="liveSourceClass">
              <option value="LOCAL_CAMERA">${esc(t("localCamera"))}</option>
              <option value="USB_CAPTURE">${esc(t("usbCapture"))}</option>
              <option value="NETWORK_STREAM">${esc(t("networkStream"))}</option>
            </select>
          </label>
          <label>${esc(t("endpoint"))}<input id="liveSourceEndpoint" dir="ltr" placeholder="camera://main or rtsp://…"></label>
          <label>${esc(t("renderNode"))}
            <select id="liveSourceNode"><option value="">${esc(t("none"))}</option>${renderNodes.map((device) => `<option value="${esc(device.device_id)}">${esc(device.display_name || device.device_id)}</option>`).join("")}</select>
          </label>
          <label class="check-row"><input id="liveSourceRequired" type="checkbox"> ${esc(t("required"))}</label>
          <label class="check-row"><input id="liveSourceEnabled" type="checkbox" checked> ${esc(t("enabled"))}</label>
        </div>
        <div><button class="button primary" type="submit">${esc(t("saveSource"))}</button></div>
      </form>` : ""}
      ${sources.length ? `<div class="phase4-grid">${sources.map(sourceCard).join("")}</div>` : `<div class="phase4-empty">${esc(t("noSources"))}</div>`}`;

    document.getElementById("liveSourceForm")?.addEventListener("submit", async (event) => {
      event.preventDefault();
      const sourceID = globalThis.crypto?.randomUUID ? crypto.randomUUID() : `source-${Date.now()}`;
      const source = {
        source_id: sourceID,
        name: document.getElementById("liveSourceName").value.trim(),
        source_class: document.getElementById("liveSourceClass").value,
        endpoint_ref: document.getElementById("liveSourceEndpoint").value.trim(),
        execution_device_id: document.getElementById("liveSourceNode").value,
        capabilities: [],
        config: {},
        required: document.getElementById("liveSourceRequired").checked,
        desired_enabled: document.getElementById("liveSourceEnabled").checked,
        readiness: "UNKNOWN",
      };
      try {
        await api(`/api/v1/projects/${encodeURIComponent(projectID)}/live-video-sources/${encodeURIComponent(sourceID)}`, {
          method: "PUT",
          body: JSON.stringify(source),
        });
        await renderLiveVideo();
        phase4Message(t("sourceSaved"), "success");
      } catch (error) {
        phase4Message(errorMessage(error), "error");
      }
    });
  }

  function cockpitCard(target) {
    const observation = target.observation || {};
    return `
      <article class="phase4-card">
        <div class="phase4-card-head">
          <div><p class="eyebrow">${esc(target.target_kind || "TARGET")}</p><h3 class="mono">${esc(target.target_id || "—")}</h3></div>
          ${pulse(target.readiness || "UNKNOWN")}
        </div>
        <dl class="phase4-kv">
          <div><dt>${esc(t("reachability"))}</dt><dd>${pulse(observation.reachability || "UNKNOWN")}</dd></div>
          <div><dt>${esc(t("transport"))}</dt><dd>${esc(observation.transport_state || "—")}</dd></div>
          <div><dt>${esc(t("latency"))}</dt><dd>${observation.latency_ms == null ? "—" : `${esc(observation.latency_ms)} ms`}</dd></div>
          <div><dt>${esc(t("jitter"))}</dt><dd>${observation.jitter_ms == null ? "—" : `${esc(observation.jitter_ms)} ms`}</dd></div>
          <div><dt>${esc(t("reason"))}</dt><dd class="mono">${esc(target.reason_code || observation.error_code || "—")}</dd></div>
          <div><dt>${esc(t("observed"))}</dt><dd>${when(observation.observed_at)}</dd></div>
        </dl>
      </article>`;
  }

  async function renderNetworkCockpit() {
    pageHeader(t("networkTitle"), t("networkSub"), renderNetworkCockpit);
    const payload = await api("/api/v1/network/cockpit");
    const targets = payload.targets || [];
    document.getElementById("phase4Body").innerHTML = targets.length
      ? `<div class="phase4-grid">${targets.map(cockpitCard).join("")}</div>`
      : `<div class="phase4-empty">${esc(t("noNetwork"))}</div>`;
  }

  async function renderPhase4Page(page) {
    if (!state.project) return;
    setPage(page);
    setMessage(globalMessage, "");
    try {
      if (page === "devices") await renderStageDevices();
      if (page === "callboard") await renderCallboard();
      if (page === "video") await renderLiveVideo();
      if (page === "network") await renderNetworkCockpit();
    } catch (error) {
      setMessage(globalMessage, errorMessage(error), "error");
    }
  }

  function installNavigation() {
    const nav = document.getElementById("workspaceNav");
    if (!nav || nav.querySelector('[data-phase4-nav="true"]')) return;
    const before = nav.querySelector('[data-page="cues"]');
    const items = [
      ["devices", t("devices")],
      ["callboard", t("callboard")],
      ["video", t("video")],
      ["network", t("network")],
    ];
    items.forEach(([page, label]) => {
      const button = document.createElement("button");
      button.type = "button";
      button.className = "nav-button";
      button.dataset.page = page;
      button.dataset.phase4Nav = "true";
      button.textContent = label;
      button.addEventListener("click", () => renderPhase4Page(page));
      nav.insertBefore(button, before);
    });
  }

  async function injectDraftDiscard() {
    if (state.user?.role !== "OWNER" || !state.project || state.page !== "configuration") return;
    let model;
    try {
      model = await loadConfiguration();
    } catch (_) {
      return;
    }
    const revision = model?.revision;
    if (!revision || revision.status !== "DRAFT" || !revision.parent_revision_id) return;
    const target = document.getElementById("content");
    if (!target || target.querySelector("#discardDraftZone")) return;
    const zone = document.createElement("section");
    zone.id = "discardDraftZone";
    zone.className = "phase4-danger-zone";
    zone.innerHTML = `
      <strong>${esc(t("draftDangerTitle"))}</strong>
      <p>${esc(t("draftDangerBody"))}</p>
      <p class="mono">Draft: ${esc(revision.revision_id)} · Parent: ${esc(revision.parent_revision_id)}</p>
      <button id="discardDraftButton" class="button danger" type="button">${esc(t("discardDraft"))}</button>`;
    target.appendChild(zone);
    document.getElementById("discardDraftButton")?.addEventListener("click", async () => {
      const confirmation = t("discardConfirm", { draft: revision.revision_id, parent: revision.parent_revision_id });
      if (!globalThis.confirm(confirmation)) return;
      const reason = globalThis.prompt(t("discardReason"), "") ?? "";
      const button = document.getElementById("discardDraftButton");
      button.disabled = true;
      try {
        await api(`/api/v1/projects/${encodeURIComponent(currentProjectID())}/configuration/draft`, {
          method: "DELETE",
          body: JSON.stringify({ reason }),
        });
        await loadProjects();
        state.project = state.projects.find((project) => (project.project_id || project.id) === currentProjectID()) || state.project;
        await window.renderConfiguration();
        setMessage(globalMessage, t("draftDiscarded"), "success");
      } catch (error) {
        setMessage(globalMessage, errorMessage(error), "error");
        button.disabled = false;
      }
    });
  }

  function wrapConfiguration() {
    const base = window.renderConfiguration;
    if (typeof base !== "function" || base.__phase4Wrapped) return;
    const wrapped = async function (...args) {
      const result = await base.apply(this, args);
      await injectDraftDiscard();
      return result;
    };
    wrapped.__phase4Wrapped = true;
    window.renderConfiguration = wrapped;
  }

  window.renderStageDevices = renderStageDevices;
  window.renderStageCallboard = renderCallboard;
  window.renderLiveVideo = renderLiveVideo;
  window.renderNetworkCockpit = renderNetworkCockpit;

  document.addEventListener("DOMContentLoaded", () => {
    installNavigation();
    wrapConfiguration();
    document.getElementById("languageSelect")?.addEventListener("change", () => {
      document.querySelectorAll('[data-phase4-nav="true"]').forEach((button) => button.remove());
      installNavigation();
      if (["devices", "callboard", "video", "network"].includes(state.page)) renderPhase4Page(state.page);
    });
  });
})();
