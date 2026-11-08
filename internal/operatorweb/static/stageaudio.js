"use strict";

// SC-021: no browser WebAudio; the Raspberry Pi 4 is the sound source.
// The UI never connects to mpv / BlueZ directly or plays music on the phone.
const scAudioUI = {
  busy: false, refreshing: false, status: null,
  outputs: [], media: [], bluetooth: [], lastError: "",
};
const scAudioL10n = {
  title: {en: "StageAudio · Rehearsal player", "ar-IQ": "StageAudio · مشغل البروفات"},
  description: {en: "Two independent audio layers play on the StageCore Pi 4, not your phone. Rehearsal prototype — not SHOW-qualified.", "ar-IQ": "طبقتان صوت تعملان على Pi 4 نفسه، مو على الهاتف. نسخة تجريبية للبروفات وليست معتمدة للعرض."},
  unavailable: {en: "StageAudio service is not connected. The normal StageCore show system still works.", "ar-IQ": "خدمة StageAudio غير متصلة. تشغيل StageCore الاعتيادي يبقى شغّال."},
  showLock: {en: "SHOW mode is active: StageAudio controls are locked.", "ar-IQ": "وضع العرض SHOW نشط: التحكم بـStageAudio مقفول."},
  viewer: {en: "Read only — operator permission is required to change audio.", "ar-IQ": "عرض فقط — تحتاج صلاحية المشغل للتحكم بالصوت."},
  note: {en: "Local command acknowledgement is not proof the speaker is audible. Bluetooth may have delay.", "ar-IQ": "قبول الأمر من المحرك مو دليل أن السماعة أصدرت صوتاً. البلوتوث ممكن يتأخر."},
  refresh: {en: "Refresh", "ar-IQ": "تحديث"},
  decks: {en: "Audio layers", "ar-IQ": "طبقات الصوت"},
  output: {en: "Audio outputs", "ar-IQ": "مخارج الصوت"},
  select: {en: "Use selected output", "ar-IQ": "اعتماد المخرج المختار"},
  stopFirst: {en: "Stop both decks before changing output or connecting Bluetooth.", "ar-IQ": "أوقف الطبقتين قبل تبديل المخرج أو الاتصال بالبلوتوث."},
  none: {en: "No compatible output found", "ar-IQ": "ماكو مخرج صوت متاح"},
  bluetooth: {en: "Paired Bluetooth speakers", "ar-IQ": "سماعات Bluetooth المقترنة"},
  connected: {en: "Connected", "ar-IQ": "متصل"},
  paired: {en: "Paired, not connected", "ar-IQ": "مقترن، غير متصل"},
  connect: {en: "Connect", "ar-IQ": "اتصال"},
  scan: {en: "Refresh paired speakers", "ar-IQ": "تحديث قائمة السماعات المقترنة"},
  noBT: {en: "No paired A2DP speakers. First-time pairing is not available from this prototype.", "ar-IQ": "ماكو سماعة A2DP مقترنة. الاقتران لأول مرة بعده غير متاح من هذه النسخة."},
  files: {en: "Rehearsal media", "ar-IQ": "ملفات صوت البروفة"},
  noFiles: {en: "No WAV, MP3, or FLAC files in the configured Pi folder.", "ar-IQ": "ماكو ملفات WAV أو MP3 أو FLAC في مجلد الصوت المحدد بالـPi."},
  load: {en: "Load", "ar-IQ": "تحميل"},
  play: {en: "Play", "ar-IQ": "تشغيل"},
  pause: {en: "Pause", "ar-IQ": "إيقاف مؤقت"},
  stop: {en: "Stop", "ar-IQ": "إيقاف"},
  loop: {en: "Loop", "ar-IQ": "تكرار"},
  volume: {en: "Volume", "ar-IQ": "الصوت"},
  master: {en: "Master volume", "ar-IQ": "الصوت العام"},
  state: {en: "State", "ar-IQ": "الحالة"},
  stopped: {en: "Stopped", "ar-IQ": "متوقف"},
  playing: {en: "Playing (commanded)", "ar-IQ": "تشغيل (حسب الأمر)"},
  paused: {en: "Paused", "ar-IQ": "متوقف مؤقتاً"},
  notReady: {en: "No confirmed local output", "ar-IQ": "المخرج المحلي غير مؤكد"},
};
function scAudioText(key) {
  const locale = localStorage.getItem("stagecore_locale") === "en" ? "en" : "ar-IQ";
  return scAudioL10n[key]?.[locale] || key;
}
function scAudioControlsAllowed() {
  return canRuntime() && scAudioUI.status?.available &&
    scAudioUI.status?.active_session !== "SHOW";
}
function scAudioDeckStopped() {
  const decks = scAudioUI.status?.status?.decks || {};
  return ["A", "B"].every((id) => decks[id]?.commanded_state === "STOPPED");
}
function scAudioOptions(items, field, selected) {
  return items.map((item) => `<option value="${esc(item[field])}" ${item[field] === selected ? "selected" : ""}>${esc(item.label || item)}</option>`).join("");
}
function scAudioDeckHtml(id) {
  const d = scAudioUI.status?.status?.decks?.[id] || {};
  const files = scAudioUI.media;
  const allowed = scAudioControlsAllowed();
  const canAct = allowed && !scAudioUI.busy;
  const disabled = canAct ? "" : "disabled";
  const hasFile = !!d.file;
  return `<article class="card sc-audio-deck" data-deck="${id}">
    <div class="section-title-row">
      <h2>Layer ${id}</h2><span class="pill neutral">${esc(d.commanded_state || "UNKNOWN")}</span>
    </div>
    <p class="muted">${esc(d.file || scAudioText("stopped"))}</p>
    <label>${esc(scAudioText("files"))}
      <select class="sc-audio-file" ${disabled}>
        ${files.length ? files.map(name => `<option value="${esc(name)}" ${name === d.file ? "selected" : ""}>${esc(name)}</option>`).join("") :
          `<option value="">${esc(scAudioText("noFiles"))}</option>`}
      </select>
    </label>
    <div class="toolbar sc-audio-actions">
      ${["LOAD", "PLAY", "PAUSE", "STOP"].map(op =>
        `<button type="button" class="button ghost sc-audio-command" data-operation="${op}" data-deck="${id}" ${disabled}
          ${op === "LOAD" && !files.length || op === "PLAY" && !hasFile ? "disabled" : ""}>${esc(scAudioText(op.toLowerCase()))}</button>`).join("")}
    </div>
    <label class="sc-audio-range">${esc(scAudioText("volume"))}: <strong>${Number(d.volume ?? 70)}%</strong>
      <input class="sc-audio-deck-volume" data-deck="${id}" type="range" min="0" max="100"
        step="1" value="${Number(d.volume ?? 70)}" ${disabled}>
    </label>
    <label class="check-row"><input type="checkbox" class="sc-audio-loop" data-deck="${id}" ${d.loop ? "checked" : ""} ${disabled}>
      ${esc(scAudioText("loop"))}</label>
    ${d.last_error ? `<p class="muted" role="alert">${esc(d.last_error)}</p>` : ""}
  </article>`;
}
function scAudioRender() {
  if (state.page !== "audio") return;
  const s = scAudioUI.status;
  const alive = !!s?.available;
  const allowed = scAudioControlsAllowed() && !scAudioUI.busy;
  const disabled = allowed ? "" : "disabled";
  const stopped = scAudioDeckStopped();
  const selected = s?.status?.selected_output_id || "";
  content.innerHTML = `
    <div class="page-head">
      <div><p class="eyebrow">STAGECORE / AUDIO</p><h1>${esc(scAudioText("title"))}</h1>
        <p class="muted">${esc(scAudioText("description"))}</p></div>
      <div class="toolbar"><button id="scAudioRefresh" type="button" class="button">${esc(scAudioText("refresh"))}</button></div>
    </div>
    <div id="scAudioError" class="message ${scAudioUI.lastError ? "error" : "hidden"}" role="alert">${esc(scAudioUI.lastError)}</div>
    ${!alive ? `<article class="card"><p>${esc(scAudioText("unavailable"))}</p></article>` : `
      ${s.active_session === "SHOW" ? `<p class="message warn">${esc(scAudioText("showLock"))}</p>` : ""}
      ${!canRuntime() ? `<p class="message warn">${esc(scAudioText("viewer"))}</p>` : ""}
      <div class="grid cards sc-audio-output-grid">
        <article class="card">
          <h2>${esc(scAudioText("output"))}</h2>
          <p class="muted">${esc(selected || scAudioText("notReady"))}</p>
          ${s.status?.output_error ? `<p class="message warn">${esc(s.status.output_error)}</p>` : ""}
          <select id="scAudioOutput" ${disabled}>
            ${scAudioUI.outputs.length ? scAudioUI.outputs.map(output =>
              `<option value="${esc(output.id)}" ${output.id === selected ? "selected" : ""}>
                ${esc(output.label)} · ${esc(output.kind)}</option>`).join("") :
              `<option value="">${esc(scAudioText("none"))}</option>`}
          </select>
          <button id="scAudioSelectOutput" type="button" class="button" ${disabled} ${!stopped ? "disabled" : ""}>
            ${esc(scAudioText("select"))}</button>
          <p class="muted">${esc(scAudioText("stopFirst"))}</p>
        </article>
        <article class="card">
          <h2>${esc(scAudioText("bluetooth"))}</h2>
          <button id="scAudioBTRefresh" type="button" class="button ghost">${esc(scAudioText("scan"))}</button>
          <div class="sc-audio-bt-list">
            ${scAudioUI.bluetooth.length ? scAudioUI.bluetooth.map(d =>
              `<div class="sc-audio-bt-item"><span><strong>${esc(d.name || d.address)}</strong>
              <small>${esc(d.connected ? scAudioText("connected") : scAudioText("paired"))}</small></span>
              <button class="button ghost sc-audio-connect" type="button" data-address="${esc(d.address)}"
                ${disabled} ${!stopped || d.connected ? "disabled" : ""}>${esc(scAudioText("connect"))}</button></div>`).join("") :
              `<p class="muted">${esc(scAudioText("noBT"))}</p>`}
          </div>
        </article>
      </div>
      <section class="card sc-audio-master">
        <label class="sc-audio-range">${esc(scAudioText("master"))}: <strong>${Number(s.status?.master_volume ?? 70)}%</strong>
          <input id="scAudioMaster" type="range" min="0" max="100" step="1"
            value="${Number(s.status?.master_volume ?? 70)}" ${disabled}>
        </label>
      </section>
      <h2>${esc(scAudioText("decks"))}</h2>
      <div class="grid cards sc-audio-decks">${scAudioDeckHtml("A")}${scAudioDeckHtml("B")}</div>
      <p class="muted">${esc(scAudioText("note"))}</p>`}
  `;
  el("scAudioRefresh")?.addEventListener("click", () => { void scAudioRefresh(true); });
  el("scAudioBTRefresh")?.addEventListener("click", () => { void scAudioLoadBT(); });
  el("scAudioSelectOutput")?.addEventListener("click", () => {
    const output_id = el("scAudioOutput")?.value || "";
    if (output_id && stopped) void scAudioSend({operation:"SELECT_OUTPUT",output_id});
  });
  el("scAudioMaster")?.addEventListener("change", (e) =>
    { void scAudioSend({operation:"MASTER_VOLUME",volume:Number(e.target.value)}); });
  content.querySelectorAll(".sc-audio-command").forEach(btn => btn.addEventListener("click", () => {
    const operation = btn.dataset.operation;
    const deck = btn.dataset.deck;
    const row = btn.closest(".sc-audio-deck");
    const file = operation === "LOAD" ? row?.querySelector(".sc-audio-file")?.value : undefined;
    void scAudioSend({operation,deck,...(file ? {file} : {})});
  }));
  content.querySelectorAll(".sc-audio-deck-volume").forEach(input => input.addEventListener("change", () =>
    { void scAudioSend({operation:"VOLUME",deck:input.dataset.deck,volume:Number(input.value)}); }));
  content.querySelectorAll(".sc-audio-loop").forEach(input => input.addEventListener("change", () =>
    { void scAudioSend({operation:"LOOP",deck:input.dataset.deck,loop:input.checked}); }));
  content.querySelectorAll(".sc-audio-connect").forEach(btn => btn.addEventListener("click", () => {
    if (!stopped) return;
    void scAudioSend({operation:"BT_CONNECT",address:btn.dataset.address});
  }));
}
async function scAudioRefresh(full = false) {
  if (scAudioUI.refreshing || scAudioUI.busy || state.page !== "audio") return;
  scAudioUI.refreshing = true;
  try {
    const model = await api("/api/v1/system/audio/status");
    if (state.page !== "audio") return;
    scAudioUI.status = model;
    scAudioUI.lastError = "";
    if (full && model.available) {
      const [outputs, media] = await Promise.all([
        api("/api/v1/system/audio/outputs"),
        api("/api/v1/system/audio/media"),
      ]);
      if (state.page !== "audio") return;
      scAudioUI.outputs = outputs.outputs || [];
      scAudioUI.media = media.media || [];
    }
  } catch (e) {
    if (state.page === "audio") {
      scAudioUI.status = {available:false};
      scAudioUI.lastError = errorMessage(e);
    }
  } finally {
    scAudioUI.refreshing = false;
    scAudioRender();
  }
}
async function scAudioLoadBT() {
  if (scAudioUI.busy || state.page !== "audio" || !scAudioUI.status?.available) return;
  scAudioUI.busy = true;
  try {
    const data = await api("/api/v1/system/audio/bluetooth");
    scAudioUI.bluetooth = data.bluetooth_devices || [];
    scAudioUI.lastError = "";
  } catch (error) {
    scAudioUI.lastError = errorMessage(error);
  } finally {
    scAudioUI.busy = false;
    scAudioRender();
  }
}
async function scAudioSend(command) {
  if (scAudioUI.busy || !scAudioControlsAllowed() || state.page !== "audio") return;
  scAudioUI.busy = true;
  try {
    const result = await api("/api/v1/system/audio/command",{method:"POST",json:command});
    scAudioUI.status = {...(scAudioUI.status || {}),status:result.status};
    if (result.bluetooth_devices?.length) {
      scAudioUI.bluetooth = result.bluetooth_devices;
    }
    scAudioUI.lastError = "";
  } catch (error) {
    scAudioUI.lastError = errorMessage(error);
  } finally {
    scAudioUI.busy = false;
    scAudioRender();
    void scAudioRefresh(true);
  }
}
el("stageAudioNav")?.addEventListener("click", () => {
  setPage("audio");
  el("stageAudioNav").classList.add("active");
  scAudioUI.lastError = "";
  scAudioRender();
  void scAudioRefresh(true);
});
setInterval(() => {
  if (state.page === "audio" && state.user && !scAudioUI.busy) void scAudioRefresh(false);
}, 6000);
