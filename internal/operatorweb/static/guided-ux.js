"use strict";

const f002BaseRenderConfiguration = renderConfiguration;
const f002BaseRenderCues = renderCues;
const f002BaseAddActionEditor = addActionEditor;
const f002BaseOpenCueEditor = openCueEditor;

function f002SetText(element, key, text) {
  if (!element) return;
  element.dataset.i18n = key;
  element.textContent = text;
}

function f002ApplyNavigationLabels() {
  f002SetText(el("projectsNav"), "nav.projects", "Projects");
  const labels = {
    dashboard: ["nav.home", "Home"],
    configuration: ["nav.setup", "Setup"],
    cues: ["nav.cues", "Cues"],
    runtime: ["nav.run", "Run"],
    preflight: ["nav.check", "Check"],
    sessions: ["nav.history", "History"],
    notes: ["nav.notes", "Notes"],
  };
  for (const [page, value] of Object.entries(labels)) {
    f002SetText(document.querySelector(`[data-page="${page}"]`), value[0], value[1]);
  }
  f002SetText(el("securityNav"), "nav.security", "Security");
}

function f002StatusPill(text, kind = "neutral") {
  return pill(text, kind);
}

function f002Step(label, detail, stateName, kind, page) {
  const action = page ? ` data-f002-page="${esc(page)}" tabindex="0" role="button"` : "";
  return `<article class="f002-step ${esc(kind)}"${action}>
    <span class="f002-step-state">${f002StatusPill(stateName, kind === "done" ? "good" : kind === "attention" ? "warn" : "neutral")}</span>
    <strong>${esc(label)}</strong>
    <small>${esc(detail)}</small>
  </article>`;
}

function f002Task(title, detail, page, primary = false) {
  return `<button class="f002-task ${primary ? "primary" : ""}" type="button" data-f002-page="${esc(page)}">
    <strong>${esc(title)}</strong><span>${esc(detail)}</span>
  </button>`;
}

async function f002Optional(path, fallback) {
  try { return await api(path); }
  catch (_) { return fallback; }
}

renderDashboard = async function f002RenderProjectHome() {
  const projectID = encodeURIComponent(state.project.project_id);
  const [dashboard, configuration, lockPayload] = await Promise.all([
    api(`/api/v1/projects/${projectID}/dashboard`),
    f002Optional(`/api/v1/projects/${projectID}/configuration`, { targets: [], cues: [] }),
    f002Optional(`/api/v1/projects/${projectID}/configuration/lock`, { show_configuration_lock: { locked: false } }),
  ]);
  state.project = dashboard.project;
  state.showConfigurationLock = lockPayload.show_configuration_lock || { locked: false };
  updateWorkspaceProject();

  const targets = configuration.targets || [];
  const cues = configuration.cues || [];
  const published = dashboard.published_snapshot;
  const draft = dashboard.draft_revision;
  const readiness = dashboard.readiness?.status || "NOT_EVALUATED";
  const active = dashboard.active_session;
  const mode = dashboard.mode || active?.type || "IDLE";

  let recommendation = { title: "Open Setup", detail: "Add the first device or target used by this Project.", page: "configuration" };
  if (targets.length > 0 && cues.length === 0) {
    recommendation = { title: "Create the first Cue", detail: "Build what should happen on stage without touching runtime code.", page: "cues" };
  } else if (cues.length > 0 && (!published || dashboard.unpublished_changes)) {
    recommendation = { title: "Validate and publish", detail: "Check the Draft and create the immutable Runtime Snapshot.", page: "cues" };
  } else if (published && readiness !== "PASS") {
    recommendation = { title: "Run Preflight", detail: "Check readiness before rehearsal or SHOW.", page: "preflight" };
  } else if (active) {
    recommendation = { title: "Continue the active Session", detail: `Return to ${active.type || mode} controls.`, page: "runtime" };
  } else if (published && readiness === "PASS") {
    recommendation = { title: "Rehearse or enter SHOW", detail: "The published Project is ready for the runtime workflow.", page: "runtime" };
  }

  const setupKind = targets.length ? "done" : "attention";
  const cueKind = cues.length ? "done" : "attention";
  const publishKind = published && !dashboard.unpublished_changes ? "done" : "attention";
  const checkKind = readiness === "PASS" ? "done" : "attention";
  const runKind = active ? "done" : "next";

  content.innerHTML = `
    <div class="page-head">
      <div>
        <p class="eyebrow" data-i18n="home.eyebrow">PROJECT HOME</p>
        <h1>${esc(dashboard.project.name)}</h1>
        <p data-i18n="home.summary">Prepare, check, rehearse and run this Project from one guided workspace.</p>
      </div>
      <div class="toolbar">
        ${f002StatusPill(mode, mode === "SHOW" ? "bad" : mode === "REHEARSAL" ? "good" : "neutral")}
        ${state.showConfigurationLock.locked ? f002StatusPill("CONFIGURATION LOCKED", "bad") : ""}
        ${f002StatusPill(readiness, readiness === "PASS" ? "good" : "warn")}
      </div>
    </div>

    <section class="f002-recommendation">
      <div><p class="eyebrow" data-i18n="home.next">RECOMMENDED NEXT STEP</p><h2>${esc(recommendation.title)}</h2><p>${esc(recommendation.detail)}</p></div>
      <button class="button primary big" type="button" data-f002-page="${esc(recommendation.page)}">Continue</button>
    </section>

    <section class="card">
      <div class="section-title-row"><div><p class="eyebrow" data-i18n="home.journey">PROJECT JOURNEY</p><h2>From setup to review</h2></div></div>
      <div class="f002-journey">
        ${f002Step("Setup", targets.length ? `${targets.length} target${targets.length === 1 ? "" : "s"} configured` : "Add devices and targets", targets.length ? "Ready" : "Next", setupKind, "configuration")}
        ${f002Step("Cues", cues.length ? `${cues.length} Cue${cues.length === 1 ? "" : "s"} in the current revision` : "Create the show sequence", cues.length ? "Ready" : "Next", cueKind, "cues")}
        ${f002Step("Publish", published && !dashboard.unpublished_changes ? `Snapshot v${published.snapshot_version}` : "Validate the Draft and publish", published && !dashboard.unpublished_changes ? "Ready" : "Needed", publishKind, "cues")}
        ${f002Step("Check", readiness === "PASS" ? "Preflight is ready" : "Resolve blockers and warnings", readiness === "PASS" ? "Ready" : "Check", checkKind, "preflight")}
        ${f002Step("Run", active ? `${active.type} Session active` : "Rehearse or run SHOW", active ? "Active" : "Available", runKind, "runtime")}
        ${f002Step("Review", "Sessions, notes and timing history", "Available", "next", "sessions")}
      </div>
    </section>

    <section class="f002-task-grid">
      ${f002Task("Setup", "Devices, targets, triggers and routing", "configuration")}
      ${f002Task("Build Cues", "Create and arrange show actions", "cues")}
      ${f002Task("Check readiness", "Preflight before rehearsal or SHOW", "preflight")}
      ${f002Task("Run", "Current Cue, next Cue and GO controls", "runtime", true)}
      ${f002Task("History", "Review previous Sessions and execution results", "sessions")}
      ${f002Task("Notes", "Keep operator and rehearsal notes", "notes")}
    </section>

    <details class="f002-advanced card">
      <summary>Advanced Project details</summary>
      <div class="f002-detail-grid">
        <div><span>Draft revision</span><strong>r${esc(draft?.revision_number || "—")} · ${esc(draft?.status || "—")}</strong><code>${esc(draft?.revision_id || "—")}</code></div>
        <div><span>Published Snapshot</span><strong>${published ? `v${esc(published.snapshot_version)}` : "Not published"}</strong><code>${esc(published?.runtime_snapshot_id || "—")}</code></div>
        <div><span>Active Session</span><strong>${esc(active?.type || "None")}</strong><code>${esc(active?.session_id || "—")}</code></div>
      </div>
    </details>`;

  content.querySelectorAll("[data-f002-page]").forEach((node) => {
    const open = () => navigate(node.dataset.f002Page);
    node.addEventListener("click", open);
    if (node.getAttribute("role") === "button") {
      node.addEventListener("keydown", (event) => {
        if (event.key === "Enter" || event.key === " ") { event.preventDefault(); open(); }
      });
    }
  });
  if (state.showConfigurationLock.locked && typeof stagecoreShowConfigurationLockBanner === "function") {
    stagecoreShowConfigurationLockBanner(state.showConfigurationLock);
  }
};

function f002CreateDetails(summary, className = "") {
  const details = document.createElement("details");
  details.className = `f002-advanced ${className}`.trim();
  const summaryNode = document.createElement("summary");
  summaryNode.textContent = summary;
  details.appendChild(summaryNode);
  return details;
}

function f002EnhanceConfiguration() {
  const heading = content.querySelector(".page-head h1");
  if (heading) heading.textContent = "Setup";
  const description = content.querySelector(".page-head p:not(.eyebrow)");
  if (description) description.textContent = "Connect the Project to stage devices and define how inputs trigger Cues or outputs. Common setup stays visual; raw JSON is optional.";

  const targetForm = el("targetForm");
  if (targetForm && !targetForm.dataset.f002Enhanced) {
    targetForm.dataset.f002Enhanced = "true";
    const typeInput = el("targetType");
    const configInput = el("targetConfig");
    const typeLabel = typeInput?.closest("label");
    const configLabel = configInput?.closest("label");
    const builder = document.createElement("section");
    builder.className = "f002-builder";
    builder.innerHTML = `
      <div class="f002-builder-head"><div><strong>Quick target setup</strong><span>Use the common OSC path without writing JSON.</span></div>
        <label>Setup type<select id="f002TargetSetupType"><option value="osc">OSC device</option><option value="advanced">Advanced / other</option></select></label>
      </div>
      <div id="f002OscTargetFields" class="form-grid two">
        <label>Device address or hostname<input id="f002OscHost" placeholder="192.168.1.50" autocomplete="off"></label>
        <label>OSC port<input id="f002OscPort" type="number" min="1" max="65535" value="9000"></label>
      </div>`;
    configLabel?.parentNode?.insertBefore(builder, configLabel);

    const advanced = f002CreateDetails("Advanced target settings");
    if (typeLabel) advanced.appendChild(typeLabel);
    if (configLabel) advanced.appendChild(configLabel);
    targetForm.appendChild(advanced);

    const mode = el("f002TargetSetupType");
    const fields = el("f002OscTargetFields");
    const host = el("f002OscHost");
    const port = el("f002OscPort");
    const disabled = el("targetName")?.disabled || false;
    host.disabled = disabled;
    port.disabled = disabled;
    mode.disabled = disabled;

    try {
      const parsed = JSON.parse(configInput.value || "{}");
      if (parsed?.osc?.host) host.value = parsed.osc.host;
      if (parsed?.osc?.port) port.value = parsed.osc.port;
    } catch (_) { mode.value = "advanced"; }

    const applyMode = () => {
      const basic = mode.value === "osc";
      fields.classList.toggle("hidden", !basic);
      advanced.open = !basic;
      host.required = basic && !disabled;
      port.required = basic && !disabled;
      if (basic && typeInput) typeInput.value = "GENERIC";
    };
    mode.addEventListener("change", applyMode);
    applyMode();

    targetForm.addEventListener("submit", () => {
      if (mode.value !== "osc") return;
      const parsedPort = Number(port.value);
      configInput.value = JSON.stringify({ osc: { host: host.value.trim(), port: parsedPort } }, null, 2);
      if (typeInput) typeInput.value = "GENERIC";
    }, true);
  }

  f002HideConfigurationJSON("inputSchema", "Advanced input schema");
  f002HideConfigurationJSON("outputSchema", "Advanced output schema");

  const routeForm = el("routeForm");
  if (routeForm && !routeForm.dataset.f002Advanced) {
    routeForm.dataset.f002Advanced = "true";
    const details = f002CreateDetails("Advanced routing conditions and parameters");
    for (const id of ["routeCondition", "routeTransform", "routeParameters"]) {
      const label = el(id)?.closest("label");
      if (label) details.appendChild(label);
    }
    routeForm.insertBefore(details, routeForm.querySelector('button[type="submit"]'));
  }

  const capability = el("outputCapability");
  if (capability && !capability.getAttribute("list")) {
    capability.setAttribute("list", "f002CapabilityList");
    f002EnsureCapabilityDatalist();
  }
}

function f002HideConfigurationJSON(id, summary) {
  const field = el(id);
  const label = field?.closest("label");
  if (!field || !label || label.parentElement?.classList.contains("f002-advanced")) return;
  const details = f002CreateDetails(summary);
  label.parentNode.insertBefore(details, label);
  details.appendChild(label);
}

function f002EnsureCapabilityDatalist() {
  if (el("f002CapabilityList")) return;
  const list = document.createElement("datalist");
  list.id = "f002CapabilityList";
  list.innerHTML = '<option value="osc.send">OSC message</option><option value="local.echo">Local test / echo</option>';
  document.body.appendChild(list);
}

function f002InstallTargetDatalist(targets) {
  let list = el("f002TargetList");
  if (!list) {
    list = document.createElement("datalist");
    list.id = "f002TargetList";
    document.body.appendChild(list);
  }
  list.innerHTML = (targets || []).map((target) => `<option value="${esc(target.logical_name)}"></option>`).join("");
  document.querySelectorAll(".action-target").forEach((input) => input.setAttribute("list", "f002TargetList"));
}

renderConfiguration = async function f002RenderConfiguration(...args) {
  await f002BaseRenderConfiguration(...args);
  f002EnhanceConfiguration();
};

renderCues = async function f002RenderCues(message = "") {
  await f002BaseRenderCues(message);
  const projectID = encodeURIComponent(state.project.project_id);
  const configuration = await f002Optional(`/api/v1/projects/${projectID}/configuration`, { targets: [] });
  state.f002Targets = configuration.targets || [];
  f002InstallTargetDatalist(state.f002Targets);
};

function f002EnhanceCueDialog() {
  const policy = el("cueExecutionPolicy");
  const policyLabel = policy?.closest("label");
  if (policyLabel && !policyLabel.parentElement?.classList.contains("f002-advanced")) {
    const details = f002CreateDetails("Advanced Cue policy");
    policyLabel.parentNode.insertBefore(details, policyLabel);
    details.appendChild(policyLabel);
  }
}

openCueEditor = function f002OpenCueEditor(cue) {
  f002BaseOpenCueEditor(cue);
  f002EnhanceCueDialog();
  f002InstallTargetDatalist(state.f002Targets || []);
};

function f002ParseOSCParameters(raw) {
  let parsed;
  try { parsed = JSON.parse(raw || "{}"); }
  catch (_) { return null; }
  if (!parsed || typeof parsed.address !== "string" || !Array.isArray(parsed.arguments || [])) return null;
  const args = parsed.arguments || [];
  for (const arg of args) {
    if (!arg || !["string", "int32", "float32", "bool"].includes(arg.type)) return null;
  }
  return { address: parsed.address, arguments: args };
}

function f002ArgumentRow(argument = { type: "string", value: "" }) {
  const row = document.createElement("div");
  row.className = "f002-argument-row";
  row.innerHTML = `
    <select class="f002-argument-type" aria-label="OSC value type">
      <option value="string">Text</option>
      <option value="int32">Whole number</option>
      <option value="float32">Decimal number</option>
      <option value="bool">On / Off</option>
    </select>
    <span class="f002-argument-value"></span>
    <button class="button ghost f002-remove-argument" type="button">Remove</button>`;
  const type = row.querySelector(".f002-argument-type");
  type.value = argument.type || "string";

  const renderValue = (value) => {
    const holder = row.querySelector(".f002-argument-value");
    holder.innerHTML = "";
    let input;
    if (type.value === "bool") {
      input = document.createElement("select");
      input.innerHTML = '<option value="true">On / true</option><option value="false">Off / false</option>';
      input.value = value === false ? "false" : "true";
    } else {
      input = document.createElement("input");
      if (type.value === "int32") { input.type = "number"; input.step = "1"; input.min = "-2147483648"; input.max = "2147483647"; }
      else if (type.value === "float32") { input.type = "number"; input.step = "any"; }
      else { input.type = "text"; }
      input.value = value ?? "";
    }
    input.className = "f002-argument-input";
    input.required = true;
    holder.appendChild(input);
  };

  renderValue(argument.value);
  type.addEventListener("change", () => renderValue(type.value === "bool" ? true : ""));
  row.querySelector(".f002-remove-argument").addEventListener("click", () => row.remove());
  return row;
}

function f002ReadArgument(row) {
  const type = row.querySelector(".f002-argument-type").value;
  const raw = row.querySelector(".f002-argument-input").value;
  if (type === "bool") return { type, value: raw === "true" };
  if (type === "int32") return { type, value: Number.parseInt(raw, 10) };
  if (type === "float32") return { type, value: Number(raw) };
  return { type, value: raw };
}

function f002EnhanceActionCard(card) {
  if (!card || card.dataset.f002Enhanced) return;
  card.dataset.f002Enhanced = "true";
  const capability = card.querySelector(".action-capability");
  const params = card.querySelector(".action-parameters");
  const target = card.querySelector(".action-target");
  if (!capability || !params || !target) return;

  target.setAttribute("list", "f002TargetList");
  f002EnsureCapabilityDatalist();
  capability.setAttribute("list", "f002CapabilityList");

  const parsedOSC = capability.value === "osc.send" ? f002ParseOSCParameters(params.value) : null;
  const builder = document.createElement("section");
  builder.className = "f002-builder f002-action-builder";
  builder.innerHTML = `
    <div class="f002-builder-head"><div><strong>Action builder</strong><span>Choose the common visual path or keep the expert capability unchanged.</span></div>
      <label>Action type<select class="f002-action-kind"><option value="osc">Send OSC message</option><option value="advanced">Advanced capability</option></select></label>
    </div>
    <div class="f002-osc-action">
      <label>OSC address<input class="f002-osc-address" placeholder="/stagecore/go"></label>
      <div class="section-title-row"><div><strong>Values</strong><p class="muted">Optional OSC arguments in send order.</p></div><button class="button ghost f002-add-argument" type="button">+ Value</button></div>
      <div class="f002-arguments"></div>
    </div>`;
  params.closest("label").parentNode.insertBefore(builder, params.closest("label"));

  const advanced = f002CreateDetails("Advanced action settings");
  const labels = [
    capability.closest("label"),
    card.querySelector(".action-mode")?.closest("label"),
    card.querySelector(".action-priority")?.closest("label"),
    params.closest("label"),
    card.querySelector(".action-timeout")?.closest("label"),
    card.querySelector(".action-error")?.closest("label"),
  ].filter(Boolean);
  card.appendChild(advanced);
  labels.forEach((label) => advanced.appendChild(label));

  const kind = builder.querySelector(".f002-action-kind");
  const oscPanel = builder.querySelector(".f002-osc-action");
  const address = builder.querySelector(".f002-osc-address");
  const argumentsNode = builder.querySelector(".f002-arguments");
  kind.value = parsedOSC ? "osc" : "advanced";
  if (parsedOSC) {
    address.value = parsedOSC.address;
    parsedOSC.arguments.forEach((argument) => argumentsNode.appendChild(f002ArgumentRow(argument)));
  }

  const applyKind = () => {
    const basic = kind.value === "osc";
    oscPanel.classList.toggle("hidden", !basic);
    advanced.open = !basic;
    address.required = basic;
    if (basic) capability.value = "osc.send";
  };
  kind.addEventListener("change", applyKind);
  builder.querySelector(".f002-add-argument").addEventListener("click", () => argumentsNode.appendChild(f002ArgumentRow()));
  applyKind();
}

function f002SyncActionCard(card) {
  if (!card?.dataset.f002Enhanced) return;
  const kind = card.querySelector(".f002-action-kind");
  if (!kind || kind.value !== "osc") return;
  const capability = card.querySelector(".action-capability");
  const params = card.querySelector(".action-parameters");
  const address = card.querySelector(".f002-osc-address").value.trim();
  const args = [...card.querySelectorAll(".f002-argument-row")].map(f002ReadArgument);
  capability.value = "osc.send";
  params.value = JSON.stringify({ address, arguments: args }, null, 2);
}

addActionEditor = function f002AddActionEditor(action = null) {
  f002BaseAddActionEditor(action);
  const cards = actionsEditor.querySelectorAll(".action-editor");
  f002EnhanceActionCard(cards[cards.length - 1]);
  f002InstallTargetDatalist(state.f002Targets || []);
};

cueForm.addEventListener("submit", () => {
  actionsEditor.querySelectorAll(".action-editor").forEach(f002SyncActionCard);
}, true);

f002ApplyNavigationLabels();
f002EnhanceCueDialog();
