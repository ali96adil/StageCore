"use strict";

(() => {
  const SIM_PAGE = "simulation";
  let timer = null;
  let latest = null;
  let cues = [];

  function ar() {
    return (document.documentElement.lang || "ar").toLowerCase().startsWith("ar");
  }

  function t(arText, enText) {
    return ar() ? arText : enText;
  }

  function projectID() {
    return state.project?.project_id || "";
  }

  function simulationPath(suffix = "") {
    return `/api/v1/projects/${encodeURIComponent(projectID())}/simulation${suffix}`;
  }

  function stopPolling() {
    if (timer) clearInterval(timer);
    timer = null;
  }

  function startPolling() {
    stopPolling();
    timer = setInterval(() => {
      if (state.page !== SIM_PAGE || !projectID()) {
        stopPolling();
        return;
      }
      refresh(false).catch(() => {});
    }, 2500);
  }

  async function loadCuesForSimulation() {
    if (!projectID()) return [];
    try {
      const runtime = await api(`/api/v1/projects/${encodeURIComponent(projectID())}/runtime`);
      return runtime.cues || [];
    } catch (_) {
      return [];
    }
  }

  function cueOptions(selected = "") {
    return cues.map((cue) => `<option value="${esc(cue.cue_id)}" ${cue.cue_id === selected ? "selected" : ""}>${esc(cue.display_label || "")} · ${esc(cue.name || cue.cue_id)}</option>`).join("");
  }

  function checkpointOptions(checkpoints, selected = "") {
    return (checkpoints || []).map((item) => `<option value="${esc(item.checkpoint_id)}" ${item.checkpoint_id === selected ? "selected" : ""}>${esc(fmtDate(item.captured_at))} · ${esc((item.content_hash || "").slice(0, 10))}</option>`).join("");
  }

  function truthLabel(truth) {
    if (!truth) return t("غير معروف", "UNKNOWN");
    if (truth.manual_confirmation_required) return t("يحتاج تأكيد المشغّل", "OPERATOR CONFIRMATION REQUIRED");
    return truth.restoration_status || t("غير معروف", "UNKNOWN");
  }

  function renderTarget(target) {
    const truth = target.state_truth || {};
    const online = target.online ? t("متصل افتراضياً", "VIRTUAL ONLINE") : t("غير متصل افتراضياً", "VIRTUAL OFFLINE");
    return `<article class="phase4-card">
      <div class="phase4-card-head">
        <div><strong class="mono">${esc(target.target_ref)}</strong><p class="muted">${esc(target.logical_type || "virtual target")}</p></div>
        <span class="phase4-pulse ${target.online ? "ready" : "offline"}">${esc(online)}</span>
      </div>
      <dl class="phase4-kv">
        <div><dt>${t("عدد التنفيذات", "Executions")}</dt><dd>${esc(target.execution_count || 0)}</dd></div>
        <div><dt>${t("آخر capability", "Last capability")}</dt><dd class="mono">${esc(target.last_capability || "—")}</dd></div>
        <div><dt>${t("آخر نتيجة", "Last result")}</dt><dd>${esc(target.last_result || "—")}</dd></div>
        <div><dt>${t("الحقيقة", "Truth")}</dt><dd>${esc(truth.scope || "SIMULATION_ONLY")}</dd></div>
      </dl>
      <div class="phase4-actions">
        <button class="button sim-target-online" data-target="${esc(target.target_ref)}" data-online="true" type="button">${t("Virtual Online", "Virtual Online")}</button>
        <button class="button sim-target-online" data-target="${esc(target.target_ref)}" data-online="false" type="button">${t("Virtual Offline", "Virtual Offline")}</button>
      </div>
    </article>`;
  }

  function renderFault(fault) {
    const selector = fault.target_ref || fault.capability || "—";
    return `<article class="phase4-card">
      <div class="phase4-card-head"><strong>${esc(fault.behavior)}</strong><span class="phase4-pulse warning">SIMULATION_ONLY</span></div>
      <p class="mono">${esc(selector)}</p>
      <div class="phase4-status-row"><span>${t("تأخير", "Delay")}: ${esc(fault.delay_ms || 0)} ms</span><span>${t("استخدامات", "Uses")}: ${esc(fault.uses || 0)}</span></div>
      <button class="button sim-clear-fault" data-target="${esc(fault.target_ref || "")}" data-capability="${esc(fault.capability || "")}" type="button">${t("مسح العطل", "Clear fault")}</button>
    </article>`;
  }

  function renderCheckpoint(item) {
    return `<article class="phase4-card">
      <div class="phase4-card-head"><strong>${esc(fmtDate(item.captured_at))}</strong><span class="phase4-pulse ready">CHECKPOINT</span></div>
      <p class="mono">${esc(item.checkpoint_id)}</p>
      <dl class="phase4-kv">
        <div><dt>${t("Current Cue", "Current Cue")}</dt><dd class="mono">${esc(item.current_cue_id || "—")}</dd></div>
        <div><dt>${t("Next Cue", "Next Cue")}</dt><dd class="mono">${esc(item.next_cue_id || "—")}</dd></div>
        <div><dt>SHA-256</dt><dd class="mono">${esc(item.content_hash || "—")}</dd></div>
        <div><dt>${t("إصدار الحقيقة", "State version")}</dt><dd>${esc(item.state_contract_version)}</dd></div>
      </dl>
      ${latest?.session ? `<button class="button sim-restore-checkpoint" data-checkpoint="${esc(item.checkpoint_id)}" type="button">${t("استعادة افتراضية بدون Replay", "Restore virtual state without replay")}</button>` : ""}
    </article>`;
  }

  function renderEvent(event) {
    return `<tr><td>${esc(fmtDate(event.occurred_at))}</td><td class="mono">${esc(event.event_type)}</td><td class="mono">${esc(event.source || event.source_ref || "—")}</td><td>${esc(event.priority || "—")}</td></tr>`;
  }

  function renderWorkspace() {
    if (!latest) return;
    const session = latest.session;
    const twin = latest.digital_twin || { targets: [], faults: [] };
    const checkpoints = latest.checkpoints || [];
    const active = !!session;
    const needsConfirm = !!session?.state_truth?.manual_confirmation_required;
    const mode = session?.start_position?.kind || "BEGINNING";

    content.innerHTML = `
      <div class="page-head">
        <div>
          <p class="eyebrow">FULL SHOW DIGITAL TWIN · F-024</p>
          <h1>${t("المحاكاة", "Simulation")}</h1>
          <p>${t("تشغيل العرض على Digital Twin فقط. لا يتم إرسال OSC/HTTP/Script/Companion/Device output حقيقي من جلسة SIMULATION.", "Run the show against the Digital Twin only. A SIMULATION Session cannot dispatch real OSC/HTTP/script/Companion/device output.")}</p>
        </div>
        <span class="phase4-pulse warning">SIMULATION · NO PHYSICAL OUTPUT</span>
      </div>

      <section class="phase4-danger-zone">
        <strong>${t("حد أمان دائم", "Persistent safety boundary")}</strong>
        ${t("هذه الصفحة تتحكم بالحالة الافتراضية فقط. REHEARSAL وSHOW يبقيان مسارين منفصلين.", "This workspace controls virtual state only. REHEARSAL and SHOW remain separate runtime modes.")}
      </section>

      ${!active ? `
        <form id="simulationStartForm" class="phase4-form">
          <div class="phase4-card-head"><div><h2>${t("بدء Simulation", "Start Simulation")}</h2><p class="muted">${t("يستخدم آخر Runtime Snapshot منشور.", "Uses the latest published Runtime Snapshot.")}</p></div></div>
          <div class="phase4-form-grid">
            <label>${t("الاسم", "Name")}<input id="simName" value="${t("محاكاة العرض", "Full show simulation")}"></label>
            <label>${t("نقطة البداية", "Start mode")}
              <select id="simStartKind">
                <option value="BEGINNING">BEGINNING</option>
                <option value="RANGE">RANGE</option>
                <option value="CHECKPOINT">CHECKPOINT</option>
              </select>
            </label>
            <label id="simStartCueWrap" class="hidden">${t("Cue البداية", "Start Cue")}<select id="simStartCue">${cueOptions()}</select></label>
            <label id="simEndCueWrap" class="hidden">${t("Cue النهاية", "End Cue")}<select id="simEndCue">${cueOptions()}</select></label>
            <label id="simCheckpointWrap" class="hidden">Checkpoint<select id="simCheckpoint">${checkpointOptions(checkpoints)}</select></label>
          </div>
          <div class="phase4-actions"><button class="button primary" type="submit">${t("ابدأ المحاكاة", "Start simulation")}</button></div>
        </form>` : `
        <section class="phase4-form">
          <div class="phase4-card-head">
            <div><h2>${t("جلسة Simulation فعالة", "Active Simulation Session")}</h2><p class="mono">${esc(session.session_id || session.id)}</p></div>
            <span class="phase4-pulse warning">${esc(mode)}</span>
          </div>
          <dl class="phase4-kv">
            <div><dt>Runtime Snapshot</dt><dd class="mono">${esc(session.runtime_snapshot_id)}</dd></div>
            <div><dt>${t("حقيقة الاستعادة", "Restoration truth")}</dt><dd>${esc(truthLabel(session.state_truth))}</dd></div>
            <div><dt>Current Cue</dt><dd class="mono">${esc(session.current_cue_id || "—")}</dd></div>
            <div><dt>Next Cue</dt><dd class="mono">${esc(session.next_cue_id || "—")}</dd></div>
          </dl>
          <div class="phase4-actions">
            ${needsConfirm ? `<button id="simConfirmStart" class="button primary" type="button">${t("أؤكد حالة البداية الافتراضية", "Confirm virtual start state")}</button>` : `<button id="simGo" class="button primary" type="button">GO · SIMULATION</button>`}
            <button id="simCheckpointCapture" class="button" type="button">${t("حفظ Checkpoint", "Capture checkpoint")}</button>
            <button id="simResetTwin" class="button" type="button">${t("Reset Digital Twin", "Reset Digital Twin")}</button>
            <button id="simStop" class="button danger" type="button">${t("إيقاف Simulation", "Stop Simulation")}</button>
          </div>
        </section>`}

      <div class="section-title-row"><div><p class="eyebrow">DIGITAL TWIN</p><h2>${t("الأهداف الافتراضية", "Virtual targets")}</h2></div></div>
      <div class="phase4-grid">${(twin.targets || []).length ? twin.targets.map(renderTarget).join("") : `<div class="phase4-empty">${t("لا توجد أهداف منفذة بعد. أول Cue سيُنشئ virtual truth للأهداف المستخدمة.", "No targets executed yet. The first Cue creates virtual truth for used targets.")}</div>`}</div>

      ${active ? `
        <form id="simulationFaultForm" class="phase4-form">
          <div><p class="eyebrow">FAULT INJECTION</p><h2>${t("عطل افتراضي محدد", "Deterministic virtual fault")}</h2></div>
          <div class="phase4-form-grid">
            <label>Target ref<input id="simFaultTarget" placeholder="VIDEO-MAIN"></label>
            <label>Capability<input id="simFaultCapability" placeholder="osc.send"></label>
            <label>Behavior<select id="simFaultBehavior"><option>FAIL</option><option>TIMEOUT</option><option>DELAY</option><option>OFFLINE</option><option>RECONNECT</option><option>REJECT</option><option>UNAVAILABLE</option><option>COMPLETE</option></select></label>
            <label>Delay ms<input id="simFaultDelay" type="number" min="0" value="0"></label>
            <label>Uses (0 = persistent)<input id="simFaultUses" type="number" min="0" value="1"></label>
            <label>Error code<input id="simFaultCode" placeholder="SIMULATED_FAILURE"></label>
          </div>
          <div class="phase4-actions"><button class="button" type="submit">${t("إضافة Fault", "Configure fault")}</button></div>
        </form>` : ""}
      <div class="phase4-grid">${(twin.faults || []).length ? twin.faults.map(renderFault).join("") : `<div class="phase4-empty">${t("لا توجد Faults فعالة.", "No active faults.")}</div>`}</div>

      <div class="section-title-row"><div><p class="eyebrow">CHECKPOINTS</p><h2>${t("نقاط الاستعادة الافتراضية", "Virtual restore points")}</h2></div></div>
      <div class="phase4-grid">${checkpoints.length ? checkpoints.map(renderCheckpoint).join("") : `<div class="phase4-empty">${t("لا توجد Checkpoints بعد.", "No checkpoints yet.")}</div>`}</div>

      <div class="section-title-row"><div><p class="eyebrow">FLIGHT RECORDER</p><h2>${t("أحداث المحاكاة", "Simulation events")}</h2></div></div>
      <div class="table-wrap"><table><thead><tr><th>${t("الوقت", "Time")}</th><th>Event</th><th>Source</th><th>Priority</th></tr></thead><tbody>${(latest.events || []).slice(-80).reverse().map(renderEvent).join("") || `<tr><td colspan="4">${t("لا توجد أحداث بعد.", "No events yet.")}</td></tr>`}</tbody></table></div>`;

    bindControls();
  }

  function toggleStartFields() {
    const kind = el("simStartKind")?.value || "BEGINNING";
    el("simStartCueWrap")?.classList.toggle("hidden", kind !== "RANGE");
    el("simEndCueWrap")?.classList.toggle("hidden", kind !== "RANGE");
    el("simCheckpointWrap")?.classList.toggle("hidden", kind !== "CHECKPOINT");
  }

  async function refresh(showMessage = true) {
    if (!projectID()) return;
    const [status, loadedCues] = await Promise.all([
      api(simulationPath()),
      loadCuesForSimulation(),
    ]);
    latest = status;
    cues = loadedCues;
    if (state.page === SIM_PAGE) renderWorkspace();
    if (showMessage) setMessage(globalMessage, "");
  }

  async function mutate(path, options, success) {
    try {
      await api(simulationPath(path), options);
      await refresh(false);
      setMessage(globalMessage, success, "success");
    } catch (error) {
      setMessage(globalMessage, errorMessage(error), "error");
    }
  }

  function bindControls() {
    el("simStartKind")?.addEventListener("change", toggleStartFields);
    toggleStartFields();

    el("simulationStartForm")?.addEventListener("submit", async (event) => {
      event.preventDefault();
      const kind = el("simStartKind").value;
      await mutate("/start", { method: "POST", json: {
        request_id: requestID(),
        name: el("simName").value.trim(),
        start_kind: kind,
        start_cue_id: kind === "RANGE" ? el("simStartCue").value : "",
        end_cue_id: kind === "RANGE" ? el("simEndCue").value : "",
        checkpoint_id: kind === "CHECKPOINT" ? el("simCheckpoint").value : "",
      } }, t("بدأت جلسة Simulation.", "Simulation started."));
    });

    el("simGo")?.addEventListener("click", () => mutate("/go", { method: "POST", json: { request_id: requestID() } }, t("تم تنفيذ GO افتراضياً.", "Virtual GO completed.")));
    el("simConfirmStart")?.addEventListener("click", () => mutate("/confirm-start", { method: "POST", json: {} }, t("تم تأكيد حالة البداية بدون ادعاء verification.", "Start state accepted without claiming verification.")));
    el("simCheckpointCapture")?.addEventListener("click", () => mutate("/checkpoints", { method: "POST", json: {} }, t("تم حفظ Checkpoint.", "Checkpoint captured.")));
    el("simResetTwin")?.addEventListener("click", () => mutate("/reset", { method: "POST", json: {} }, t("تم Reset للـDigital Twin فقط.", "Digital Twin reset.")));
    el("simStop")?.addEventListener("click", () => mutate("/stop", { method: "POST", json: { request_id: requestID() } }, t("تم إيقاف Simulation.", "Simulation stopped.")));

    el("simulationFaultForm")?.addEventListener("submit", async (event) => {
      event.preventDefault();
      await mutate("/faults", { method: "POST", json: {
        target_ref: el("simFaultTarget").value.trim(),
        capability: el("simFaultCapability").value.trim(),
        behavior: el("simFaultBehavior").value,
        delay_ms: Number(el("simFaultDelay").value || 0),
        uses: Number(el("simFaultUses").value || 0),
        error_code: el("simFaultCode").value.trim(),
      } }, t("تمت إضافة Fault افتراضية.", "Virtual fault configured."));
    });

    content.querySelectorAll(".sim-clear-fault").forEach((button) => button.addEventListener("click", () => mutate("/faults", { method: "DELETE", json: { target_ref: button.dataset.target, capability: button.dataset.capability } }, t("تم مسح Fault.", "Fault cleared."))));
    content.querySelectorAll(".sim-target-online").forEach((button) => button.addEventListener("click", () => mutate("/targets/state", { method: "POST", json: { target_ref: button.dataset.target, online: button.dataset.online === "true" } }, t("تغيرت الحالة الافتراضية.", "Virtual target state updated."))));
    content.querySelectorAll(".sim-restore-checkpoint").forEach((button) => button.addEventListener("click", () => mutate("/restore", { method: "POST", json: { checkpoint_id: button.dataset.checkpoint } }, t("تمت استعادة الـCheckpoint بدون Replay.", "Checkpoint restored without replay."))));
  }

  function installNavigation() {
    const nav = el("workspaceNav");
    if (!nav || el("simulationNav")) return;
    const button = document.createElement("button");
    button.id = "simulationNav";
    button.className = "nav-button";
    button.type = "button";
    button.dataset.page = SIM_PAGE;
    button.dataset.phase4Nav = "true";
    button.textContent = t("Simulation / المحاكاة", "Simulation / المحاكاة");
    button.addEventListener("click", async () => {
      if (!state.project) return;
      setPage(SIM_PAGE);
      setMessage(globalMessage, "");
      try {
        await refresh(false);
        startPolling();
      } catch (error) {
        setMessage(globalMessage, errorMessage(error), "error");
      }
    });
    const runtimeButton = nav.querySelector('[data-page="runtime"]');
    runtimeButton?.insertAdjacentElement("afterend", button);
  }

  installNavigation();
  window.addEventListener("stagecore:language-changed", () => {
    if (el("simulationNav")) el("simulationNav").textContent = t("Simulation / المحاكاة", "Simulation / المحاكاة");
    if (state.page === SIM_PAGE && latest) renderWorkspace();
  });
})();
