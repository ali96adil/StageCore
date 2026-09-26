"use strict";

const stagecoreConfigurationNavigateBase = navigate;

function configurationEditable() {
  return ["OWNER", "TECHNICIAN"].includes(state.user?.role);
}

async function loadConfiguration() {
  const projectID = encodeURIComponent(state.project.project_id);
  const configuration = await api(`/api/v1/projects/${projectID}/configuration`);
  let machineRoles = { roles: [], companions: [] };
  if (configurationEditable()) {
    try {
      machineRoles = await api(`/api/v1/projects/${projectID}/machine-roles`);
    } catch (_) {}
  }
  return {
    ...configuration,
    machine_roles: machineRoles.roles || [],
    companions: machineRoles.companions || [],
  };
}

function companionSupportsRole(companion, role) {
  if (!companion || companion.trust_state !== "TRUSTED") return false;
  const available = new Set(companion.capabilities || []);
  return (role.required_capabilities || []).every((capability) => available.has(capability));
}

function machineRoleTarget(role, targets) {
  return (targets || []).find((target) =>
    target.logical_type === "machine_role" &&
    target.configuration?.machine_role_id === role.machine_role_id
  ) || null;
}

function optionList(items, valueKey, labeler) {
  return `<option value="">Select…</option>` + items.map((item) => `<option value="${esc(item[valueKey])}">${esc(labeler(item))}</option>`).join("");
}

async function renderConfiguration() {
  const model = await loadConfiguration();
  const roleCanEdit = configurationEditable();
  const editable = roleCanEdit && model.revision.status === "DRAFT";
  const disabled = editable ? "" : "disabled";
  const startEdit = roleCanEdit && !editable ? `<button id="startRoutingEdit" class="button primary" type="button">Start routing edit</button>` : "";
  content.innerHTML = `
    <div class="page-head">
      <div><p class="eyebrow">PROJECT CONFIGURATION</p><h1>Targets and Routing</h1><p>Build the Draft routing graph through the supported Operator interface. Published Runtime Snapshots remain immutable.</p></div>
      <div class="toolbar">${pill(model.revision.status, model.revision.status === "DRAFT" ? "warn" : "good")}${startEdit}<button id="refreshConfiguration" class="button" type="button">Refresh</button></div>
    </div>
    ${roleCanEdit && !editable ? `<div class="message warn">This revision backs a published Runtime Snapshot. Start a routing edit to fork a new Draft and refresh all revision-bound IDs before changing configuration.</div>` : ""}

    <div class="grid cards">
      <article class="card">
        <p class="eyebrow">TARGETS</p><h2>Logical target aliases</h2>
        <form id="targetForm" style="margin-top:14px">
          <div class="form-grid two">
            <label>Logical name<input id="targetName" placeholder="PROJECTOR-MAIN" ${disabled} required></label>
            <label>Logical type<input id="targetType" value="GENERIC" ${disabled} required></label>
          </div>
          <label>Configuration JSON<textarea id="targetConfig" class="mono" rows="5" ${disabled}>{}</textarea></label>
          <button class="button primary" type="submit" ${disabled}>Add target</button>
        </form>
        <div class="actions-editor" style="margin-top:14px">${(model.targets || []).length ? model.targets.map((target) => `
          <div class="action-editor"><div class="section-title-row"><strong>${esc(target.logical_name)}</strong>${editable ? `<button class="button danger target-remove" type="button" data-alias-id="${esc(target.alias_id)}" data-target-name="${esc(target.logical_name)}">Remove</button>` : ""}</div><p class="muted">${esc(target.logical_type)}</p><pre class="mono muted">${esc(jsonText(target.configuration))}</pre></div>`).join("") : `<div class="empty">No logical targets yet.</div>`}</div>
      </article>

      <article class="card">
        <p class="eyebrow">INPUTS</p><h2>Runtime input definitions</h2>
        <form id="inputForm" style="margin-top:14px">
          <div class="form-grid two">
            <label>Name<input id="inputName" placeholder="GO Button" ${disabled} required></label>
            <label>Source ref<input id="inputSource" placeholder="osc:/go" ${disabled} required></label>
            <label>Event type<input id="inputEvent" value="osc.message" ${disabled} required></label>
            <label class="check-row"><input id="inputEnabled" type="checkbox" checked ${disabled}> Enabled</label>
          </div>
          <label>Value schema JSON<textarea id="inputSchema" class="mono" rows="3" ${disabled}>{}</textarea></label>
          <button class="button primary" type="submit" ${disabled}>Add input</button>
        </form>
        <div class="actions-editor" style="margin-top:14px">${(model.inputs || []).length ? model.inputs.map((input) => `
          <div class="action-editor"><strong>${esc(input.name)}</strong><p class="muted">${esc(input.source_ref)} · ${esc(input.event_type)} · ${input.enabled ? "ENABLED" : "DISABLED"}</p><p class="mono muted">${esc(input.input_id)}</p></div>`).join("") : `<div class="empty">No inputs yet.</div>`}</div>
      </article>

      <article class="card">
        <p class="eyebrow">OUTPUTS</p><h2>Capability outputs</h2>
        <form id="outputForm" style="margin-top:14px">
          <div class="form-grid two">
            <label>Name<input id="outputName" placeholder="Main projector OSC" ${disabled} required></label>
            <label>Target<select id="outputTarget" ${disabled} required>${optionList(model.targets || [], "logical_name", (item) => `${item.logical_name} · ${item.logical_type}`)}</select></label>
            <label>Capability<input id="outputCapability" value="osc.send" ${disabled} required></label>
            <label>Criticality<select id="outputCriticality" ${disabled}><option value="NORMAL">NORMAL</option><option value="CRITICAL">CRITICAL</option></select></label>
          </div>
          <label>Value schema JSON<textarea id="outputSchema" class="mono" rows="3" ${disabled}>{}</textarea></label>
          <button class="button primary" type="submit" ${disabled}>Add output</button>
        </form>
        <div class="actions-editor" style="margin-top:14px">${(model.outputs || []).length ? model.outputs.map((output) => `
          <div class="action-editor"><strong>${esc(output.name)}</strong><p class="muted">${esc(output.target_ref)} · ${esc(output.capability_key)}</p><p class="mono muted">${esc(output.output_id)}</p></div>`).join("") : `<div class="empty">No outputs yet.</div>`}</div>
      </article>

      <article class="card">
        <p class="eyebrow">ROUTES</p><h2>Input → Cue/Output</h2>
        <form id="routeForm" style="margin-top:14px">
          <div class="form-grid two">
            <label>Name<input id="routeName" placeholder="GO to projector" ${disabled} required></label>
            <label>Input<select id="routeInput" ${disabled} required>${optionList(model.inputs || [], "input_id", (item) => `${item.name} · ${item.source_ref}`)}</select></label>
            <label>Action output<select id="routeOutput" ${disabled}>${optionList(model.outputs || [], "output_id", (item) => `${item.name} · ${item.capability_key}`)}</select></label>
            <label>Action Cue<select id="routeCue" ${disabled}>${optionList(model.cues || [], "cue_id", (item) => `${item.display_label} · ${item.name}`)}</select></label>
            <label>Priority<select id="routePriority" ${disabled}><option value="P2">P2</option><option value="P1">P1</option><option value="P0">P0</option><option value="P3">P3</option></select></label>
            <label class="check-row"><input id="routeEnabled" type="checkbox" checked ${disabled}> Enabled</label>
          </div>
          <label>Condition JSON<textarea id="routeCondition" class="mono" rows="3" ${disabled}>null</textarea></label>
          <label>Transform JSON<textarea id="routeTransform" class="mono" rows="3" ${disabled}>null</textarea></label>
          <label>Action parameters JSON<textarea id="routeParameters" class="mono" rows="3" ${disabled}>{}</textarea></label>
          <button class="button primary" type="submit" ${disabled}>Add route</button>
        </form>
        <div class="actions-editor" style="margin-top:14px">${(model.routes || []).length ? model.routes.map((route) => `
          <div class="action-editor"><div class="section-title-row"><strong>${esc(route.name)}</strong>${pill(route.enabled ? "ENABLED" : "DISABLED", route.enabled ? "good" : "neutral")}</div><p class="muted">Input ${esc(route.input_id)} · ${esc(route.priority_class)}</p><p class="mono muted">${esc(route.route_id)}</p></div>`).join("") : `<div class="empty">No routes yet.</div>`}</div>
      </article>

      ${roleCanEdit ? `
      <article class="card">
        <p class="eyebrow">COMPANION MACHINE ROLES</p><h2>Mac / Companion roles</h2>
        <p class="muted">Create stable show roles such as AUDIO-ABLETON or VIDEO-VDMX. Cues target the Role, not a Mac hostname.</p>
        <form id="machineRoleForm" style="margin-top:14px">
          <div class="form-grid two">
            <label>Role key<input id="machineRoleKey" placeholder="AUDIO-ABLETON" required></label>
            <label>Display name<input id="machineRoleName" placeholder="Ableton Audio"></label>
          </div>
          <div class="form-grid two">
            <label class="check-row"><input id="machineRoleMIDI" type="checkbox" checked> MIDI send</label>
            <label class="check-row"><input id="machineRoleOSC" type="checkbox"> OSC send</label>
            <label class="check-row"><input id="machineRoleEcho" type="checkbox"> Local echo / diagnostics</label>
            <label class="check-row"><input id="machineRoleRequired" type="checkbox" checked> Required for show readiness</label>
          </div>
          <button class="button primary" type="submit">Create Machine Role</button>
          ${!editable ? `<p class="muted">Role assignment is available now. To add a new Role as a Cue target, start a routing Draft.</p>` : ""}
        </form>
        <div class="actions-editor" style="margin-top:14px">
          ${(model.machine_roles || []).length ? model.machine_roles.map((role) => {
            const target = machineRoleTarget(role, model.targets);
            const assigned = role.assignment?.companion_id || "";
            const assignedCompanion = (model.companions || []).find((item) => item.companion_id === assigned);
            const candidates = (model.companions || []).filter((item) => companionSupportsRole(item, role));
            return `
            <div class="action-editor machine-role-row" data-role-id="${esc(role.machine_role_id)}">
              <div class="section-title-row">
                <div><strong>${esc(role.role_key)}</strong><p class="muted">${esc(role.display_name || role.role_key)}</p></div>
                ${pill(role.required ? "REQUIRED" : "OPTIONAL", role.required ? "warn" : "neutral")}
              </div>
              <p class="muted">Capabilities: ${esc((role.required_capabilities || []).join(", ") || "none")}</p>
              <p class="muted">Cue target: ${target ? `<strong>${esc(target.logical_name)}</strong>` : "not created yet"}</p>
              ${!target && editable ? `<button class="button machine-role-target" type="button" data-role-id="${esc(role.machine_role_id)}" data-role-key="${esc(role.role_key)}">Add as Cue target</button>` : ""}
              <div class="form-grid two" style="margin-top:10px">
                <label>Assigned Companion
                  <select class="machine-role-companion">
                    <option value="">Select trusted Companion…</option>
                    ${candidates.map((item) => `<option value="${esc(item.companion_id)}" ${item.companion_id === assigned ? "selected" : ""}>${esc(item.display_name || item.hostname || item.companion_id)} · ${esc(item.readiness || "UNKNOWN")}</option>`).join("")}
                  </select>
                </label>
                <div class="row-actions" style="align-self:end">
                  ${assigned ? `<button class="button danger machine-role-release" type="button">Release ${esc(assignedCompanion?.display_name || "Companion")}</button>` : `<button class="button primary machine-role-assign" type="button">Assign</button>`}
                </div>
              </div>
              ${!candidates.length ? `<p class="muted">No TRUSTED Companion currently advertises every required capability.</p>` : ""}
            </div>`;
          }).join("") : `<div class="empty">No Machine Roles yet.</div>`}
        </div>
      </article>` : ""}
    </div>`;

  el("refreshConfiguration").addEventListener("click", () => renderConfiguration().catch(configurationError));
  const startButton = el("startRoutingEdit");
  if (startButton) {
    startButton.addEventListener("click", async () => {
      try {
        await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/configuration/draft`, { method: "POST" });
        await refreshProjectAndConfiguration();
        setMessage(globalMessage, "New routing Draft created. The published Runtime Snapshot remains unchanged.", "good");
      } catch (error) { configurationError(error); }
    });
  }
  const machineRoleForm = el("machineRoleForm");
  if (machineRoleForm) {
    machineRoleForm.addEventListener("submit", async (event) => {
      event.preventDefault();
      const requiredCapabilities = [];
      if (el("machineRoleMIDI").checked) requiredCapabilities.push("midi.send");
      if (el("machineRoleOSC").checked) requiredCapabilities.push("osc.send");
      if (el("machineRoleEcho").checked) requiredCapabilities.push("local.echo");
      if (!requiredCapabilities.length) {
        configurationError(new Error("Choose at least one Machine Role capability."));
        return;
      }
      try {
        await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/machine-roles`, {
          method: "POST",
          json: {
            role_key: el("machineRoleKey").value.trim(),
            display_name: el("machineRoleName").value.trim(),
            required_capabilities: requiredCapabilities,
            required: el("machineRoleRequired").checked,
          },
        });
        await renderConfiguration();
        setMessage(globalMessage, "Machine Role created. Add it as a Cue target from the Role card when a routing Draft is open.", "good");
      } catch (error) { configurationError(error); }
    });
  }

  document.querySelectorAll(".machine-role-assign").forEach((button) => {
    button.addEventListener("click", async () => {
      const row = button.closest(".machine-role-row");
      const roleID = row?.dataset.roleId || "";
      const companionID = row?.querySelector(".machine-role-companion")?.value || "";
      if (!roleID || !companionID) {
        configurationError(new Error("Choose a trusted Companion before assignment."));
        return;
      }
      try {
        await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/machine-roles/${encodeURIComponent(roleID)}/assignment`, {
          method: "POST",
          json: { companion_id: companionID },
        });
        await renderConfiguration();
        setMessage(globalMessage, "Companion assigned to Machine Role.", "good");
      } catch (error) { configurationError(error); }
    });
  });

  document.querySelectorAll(".machine-role-release").forEach((button) => {
    button.addEventListener("click", async () => {
      const row = button.closest(".machine-role-row");
      const roleID = row?.dataset.roleId || "";
      if (!roleID || !window.confirm("Release this Companion from the Machine Role?")) return;
      try {
        await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/machine-roles/${encodeURIComponent(roleID)}/assignment`, {
          method: "DELETE",
        });
        await renderConfiguration();
        setMessage(globalMessage, "Machine Role assignment released.", "good");
      } catch (error) { configurationError(error); }
    });
  });

  document.querySelectorAll(".machine-role-target").forEach((button) => {
    button.addEventListener("click", async () => {
      const roleID = button.dataset.roleId || "";
      const roleKey = button.dataset.roleKey || "";
      if (!roleID || !roleKey) return;
      try {
        await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/targets`, {
          method: "POST",
          json: {
            logical_name: roleKey,
            logical_type: "machine_role",
            configuration: { machine_role_id: roleID },
          },
        });
        await refreshProjectAndConfiguration();
        setMessage(globalMessage, `Machine Role ${roleKey} is now available as a Cue target.`, "good");
      } catch (error) { configurationError(error); }
    });
  });

  if (!editable) return;
  el("targetForm").addEventListener("submit", async (event) => {
    event.preventDefault();
    try {
      await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/targets`, { method: "POST", json: { logical_name: el("targetName").value.trim(), logical_type: el("targetType").value.trim(), configuration: parseJSONField(el("targetConfig").value, "Target configuration") } });
      await refreshProjectAndConfiguration();
    } catch (error) { configurationError(error); }
  });
  document.querySelectorAll(".target-remove").forEach((button) => {
    button.addEventListener("click", async () => {
      const targetName = button.dataset.targetName || "this target";
      if (!window.confirm(`Remove ${targetName}? Published Runtime Snapshots remain unchanged.`)) return;
      try {
        await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/targets/${encodeURIComponent(button.dataset.aliasId)}?confirm=true`, { method: "DELETE" });
        await refreshProjectAndConfiguration();
        setMessage(globalMessage, `Removed target ${targetName}.`, "good");
      } catch (error) { configurationError(error); }
    });
  });
  el("inputForm").addEventListener("submit", async (event) => {
    event.preventDefault();
    try {
      await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/inputs`, { method: "POST", json: { name: el("inputName").value.trim(), source_ref: el("inputSource").value.trim(), event_type: el("inputEvent").value.trim(), value_schema: parseJSONField(el("inputSchema").value, "Input schema"), enabled: el("inputEnabled").checked } });
      await refreshProjectAndConfiguration();
    } catch (error) { configurationError(error); }
  });
  el("outputForm").addEventListener("submit", async (event) => {
    event.preventDefault();
    try {
      await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/outputs`, { method: "POST", json: { name: el("outputName").value.trim(), target_ref: el("outputTarget").value, capability_key: el("outputCapability").value.trim(), value_schema: parseJSONField(el("outputSchema").value, "Output schema"), criticality: el("outputCriticality").value } });
      await refreshProjectAndConfiguration();
    } catch (error) { configurationError(error); }
  });
  el("routeForm").addEventListener("submit", async (event) => {
    event.preventDefault();
    const outputID = el("routeOutput").value;
    const cueID = el("routeCue").value;
    if ((outputID ? 1 : 0) + (cueID ? 1 : 0) !== 1) {
      configurationError(new Error("Choose exactly one Route action: an Output or a Cue."));
      return;
    }
    const action = { parameters: parseJSONField(el("routeParameters").value, "Route Action parameters") };
    if (outputID) action.output_id = outputID;
    if (cueID) action.cue_id = cueID;
    try {
      await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/routes`, { method: "POST", json: { name: el("routeName").value.trim(), input_id: el("routeInput").value, condition_definition: parseJSONField(el("routeCondition").value, "Route condition"), transform_definition: parseJSONField(el("routeTransform").value, "Route transform"), priority_class: el("routePriority").value, error_policy: {}, enabled: el("routeEnabled").checked, actions: [action] } });
      await refreshProjectAndConfiguration();
    } catch (error) { configurationError(error); }
  });
}

async function refreshProjectAndConfiguration() {
  const projectPayload = await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}`);
  state.project = projectPayload.project;
  await renderConfiguration();
}

function configurationError(error) {
  setMessage(globalMessage, errorMessage(error), "error");
}

navigate = async function stagecoreConfigurationNavigate(page) {
  if (page !== "configuration") return stagecoreConfigurationNavigateBase(page);
  if (!state.project) return;
  setPage(page);
  setMessage(globalMessage, "");
  try { await renderConfiguration(); }
  catch (error) { configurationError(error); }
};