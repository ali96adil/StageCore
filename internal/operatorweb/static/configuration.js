"use strict";

const nativeVisualRoleCapabilities = [
  "visual.preload", "visual.play", "visual.pause", "visual.stop", "visual.seek", "visual.loop",
  "visual.blackout", "visual.layer.opacity", "visual.layer.transform",
  "visual.transition", "visual.layer.crop", "visual.layer.mask", "visual.layer.effect",
  "visual.layer.order", "visual.layer.output", "visual.output.configure", "visual.output.mapping", "visual.state.inspect",
  "video.source.open", "video.source.close", "video.source.select", "video.source.route", "video.source.inspect",
];

const executionEnvironmentOperationCapability = "execution.environment.operation";

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

function missingRoleCapabilities(companion, role) {
  const available = new Set(companion?.capabilities || []);
  return (role?.required_capabilities || []).filter((capability) => !available.has(capability));
}

function companionSupportsRole(companion, role) {
  if (!companion || companion.trust_state !== "TRUSTED") return false;
  return missingRoleCapabilities(companion, role).length === 0;
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
      <div><p class="eyebrow">PROJECT CONFIGURATION</p><h1>Routing & Machine Roles</h1><p>Build the Draft routing graph and assign stable Companion roles such as AUDIO-ABLETON or VIDEO-VDMX. Published Runtime Snapshots remain immutable.</p></div>
      <div class="toolbar">${pill(model.revision.status, model.revision.status === "DRAFT" ? "warn" : "good")}${startEdit}${roleCanEdit ? `<button id="jumpMachineRoles" class="button ghost" type="button">Machine Roles</button>` : ""}<button id="refreshConfiguration" class="button" type="button">Refresh</button></div>
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
        <form id="inputForm" data-input-id="" style="margin-top:14px">
          <div class="form-grid two">
            <label>Name<input id="inputName" placeholder="GO Button" ${disabled} required></label>
            <label>Source ref<input id="inputSource" placeholder="osc:/go" ${disabled} required></label>
            <label>Event type<input id="inputEvent" value="osc.message" ${disabled} required></label>
            <label class="check-row"><input id="inputEnabled" type="checkbox" checked ${disabled}> Enabled</label>
          </div>
          <label>Value schema JSON<textarea id="inputSchema" class="mono" rows="3" ${disabled}>{}</textarea></label>
          <div class="row-actions"><button id="inputSubmit" class="button primary" type="submit" ${disabled}>Add input</button><button id="inputCancelEdit" class="button ghost hidden" type="button" ${disabled}>Cancel edit</button></div>
        </form>
        <div class="actions-editor" style="margin-top:14px">${(model.inputs || []).length ? model.inputs.map((input) => `
          <div class="action-editor"><div class="section-title-row"><strong>${esc(input.name)}</strong>${editable ? `<div class="row-actions"><button class="button ghost input-edit" type="button" data-input-id="${esc(input.input_id)}">Edit</button><button class="button danger input-remove" type="button" data-input-id="${esc(input.input_id)}">Remove</button></div>` : ""}</div><p class="muted">${esc(input.source_ref)} · ${esc(input.event_type)} · ${input.enabled ? "ENABLED" : "DISABLED"}</p><p class="mono muted">${esc(input.input_id)}</p></div>`).join("") : `<div class="empty">No inputs yet.</div>`}</div>
      </article>

      <article class="card">
        <p class="eyebrow">OUTPUTS</p><h2>Capability outputs</h2>
        <form id="outputForm" data-output-id="" style="margin-top:14px">
          <div class="form-grid two">
            <label>Name<input id="outputName" placeholder="Main projector OSC" ${disabled} required></label>
            <label>Target<select id="outputTarget" ${disabled} required>${optionList(model.targets || [], "logical_name", (item) => `${item.logical_name} · ${item.logical_type}`)}</select></label>
            <label>Capability<input id="outputCapability" value="osc.send" ${disabled} required></label>
            <label>Criticality<select id="outputCriticality" ${disabled}><option value="NORMAL">NORMAL</option><option value="CRITICAL">CRITICAL</option></select></label>
          </div>
          <label>Value schema JSON<textarea id="outputSchema" class="mono" rows="3" ${disabled}>{}</textarea></label>
          <div class="row-actions">
            <button id="outputSubmit" class="button primary" type="submit" ${disabled}>Add output</button>
            <button id="outputCancelEdit" class="button ghost hidden" type="button" ${disabled}>Cancel edit</button>
          </div>
        </form>
        <div class="actions-editor" style="margin-top:14px">${(model.outputs || []).length ? model.outputs.map((output) => `
          <div class="action-editor">
            <div class="section-title-row"><strong>${esc(output.name)}</strong>${editable ? `<button class="button ghost output-edit" type="button" data-output-id="${esc(output.output_id)}">Edit</button>` : ""}</div>
            <p class="muted">${esc(output.target_ref)} · ${esc(output.capability_key)}</p><p class="mono muted">${esc(output.output_id)}</p>
          </div>`).join("") : `<div class="empty">No outputs yet.</div>`}</div>
      </article>

      <article class="card">
        <p class="eyebrow">ROUTES</p><h2>Input → Cue/Output</h2>
        <form id="routeForm" data-route-id="" style="margin-top:14px">
          <div class="form-grid two">
            <label>Name<input id="routeName" placeholder="GO to projector" ${disabled} required></label>
            <label>Input<select id="routeInput" ${disabled} required>${optionList(model.inputs || [], "input_id", (item) => `${item.name} · ${item.source_ref}`)}</select></label>
            <label>Action output<select id="routeOutput" ${disabled}>${optionList(model.outputs || [], "output_id", (item) => `${item.name} · ${item.capability_key}`)}</select></label>
            <label>Action Cue<select id="routeCue" ${disabled}>${optionList(model.cues || [], "cue_id", (item) => `${item.display_label} · ${item.name}`)}</select></label>
            <label>Priority<select id="routePriority" ${disabled}><option value="P2">P2</option><option value="P1">P1</option><option value="P0">P0</option><option value="P3">P3</option></select></label>
            <label>Delay (ms)<input id="routeDelay" type="number" min="0" step="1" value="0" ${disabled}></label>
            <label>Debounce (ms)<input id="routeDebounce" type="number" min="0" step="1" value="0" ${disabled}></label>
            <label class="check-row"><input id="routeEnabled" type="checkbox" checked ${disabled}> Enabled</label>
          </div>
          <label>Condition JSON<textarea id="routeCondition" class="mono" rows="3" ${disabled}>null</textarea></label>
          <label>Transform JSON<textarea id="routeTransform" class="mono" rows="3" ${disabled}>null</textarea></label>
          <label>Action parameters JSON<textarea id="routeParameters" class="mono" rows="3" ${disabled}>{}</textarea></label>
          <div class="row-actions"><button id="routeSubmit" class="button primary" type="submit" ${disabled}>Add route</button><button id="routeCancelEdit" class="button ghost hidden" type="button" ${disabled}>Cancel edit</button></div>
        </form>
        <div class="actions-editor" style="margin-top:14px">${(model.routes || []).length ? model.routes.map((route) => `
          <div class="action-editor"><div class="section-title-row"><strong>${esc(route.name)}</strong><div class="row-actions">${pill(route.enabled ? "ENABLED" : "DISABLED", route.enabled ? "good" : "neutral")}${editable && (route.actions || []).length === 1 ? `<button class="button ghost route-edit" type="button" data-route-id="${esc(route.route_id)}">Edit</button>` : ""}${editable ? `<button class="button ghost route-toggle" type="button" data-route-id="${esc(route.route_id)}">${route.enabled ? "Disable" : "Enable"}</button><button class="button danger route-remove" type="button" data-route-id="${esc(route.route_id)}">Remove</button>` : ""}</div></div><p class="muted">Input ${esc(route.input_id)} · ${esc(route.priority_class)} · ${(route.actions || []).length} action(s)</p><p class="mono muted">${esc(route.route_id)}</p></div>`).join("") : `<div class="empty">No routes yet.</div>`}</div>
      </article>

      ${roleCanEdit ? `
      <article id="machineRolesCard" class="card">
        <p class="eyebrow">COMPANION MACHINE ROLES</p><h2>Mac / Companion roles</h2>
        <p class="muted">Create stable show roles such as AUDIO-ABLETON or VIDEO-VDMX. Cues target the Role, not a Mac hostname.</p>
        <form id="machineRoleForm" data-role-id="" style="margin-top:14px">
          <div class="form-grid two">
            <label>Role key<input id="machineRoleKey" placeholder="AUDIO-ABLETON" required></label>
            <label>Display name<input id="machineRoleName" placeholder="Ableton Audio"></label>
          </div>
          <div class="form-grid two">
            <label class="check-row"><input id="machineRoleMIDI" type="checkbox" checked> MIDI send</label>
            <label class="check-row"><input id="machineRoleOSC" type="checkbox"> OSC send</label>
            <label class="check-row"><input id="machineRoleEnvironment" type="checkbox"> VDMX / execution-environment operations</label>
            <label class="check-row"><input id="machineRoleEcho" type="checkbox"> Local echo / diagnostics</label>
            <label class="check-row"><input id="machineRoleNativeVisual" type="checkbox"> StageCore Native Visual + Live Source</label>
            <label class="check-row"><input id="machineRoleRequired" type="checkbox" checked> Required for show readiness</label>
          </div>
          <p class="muted">For external VDMX cue control use OSC send; add VDMX / execution-environment operations when StageCore must open, capture or restore the VDMX environment. Native Visual + Live Source is only for a Companion running StageCore\'s native visual engine.</p>
          <div class="row-actions">
            <button id="machineRoleSubmit" class="button primary" type="submit">Create Machine Role</button>
            <button id="machineRoleCancelEdit" class="button ghost hidden" type="button">Cancel edit</button>
          </div>
          <p class="muted">Role key is the stable runtime identity and cannot be renamed after creation. Edit changes display name, capability requirements and readiness requirement only.</p>
          ${!editable ? `<p class="muted">Role assignment is available now. To add a new Role as a Cue target, start a routing Draft.</p>` : ""}
        </form>
        <div class="actions-editor" style="margin-top:14px">
          ${(model.machine_roles || []).length ? model.machine_roles.map((role) => {
            const target = machineRoleTarget(role, model.targets);
            const assigned = role.assignment?.companion_id || "";
            const assignedCompanion = (model.companions || []).find((item) => item.companion_id === assigned);
            const candidates = (model.companions || []).filter((item) => companionSupportsRole(item, role));
            const incompatibleTrusted = (model.companions || []).filter((item) =>
              item.trust_state === "TRUSTED" && !companionSupportsRole(item, role)
            );
            return `
            <div class="action-editor machine-role-row" data-role-id="${esc(role.machine_role_id)}">
              <div class="section-title-row">
                <div><strong>${esc(role.role_key)}</strong><p class="muted">${esc(role.display_name || role.role_key)}</p></div>
                <div class="row-actions">
                  ${pill(role.retired ? "RETIRED" : (role.required ? "REQUIRED" : "OPTIONAL"), role.retired ? "neutral" : (role.required ? "warn" : "neutral"))}
                  <button class="button ghost machine-role-edit" type="button" data-role-id="${esc(role.machine_role_id)}">Edit</button>
                  ${role.retired
                    ? `<button class="button machine-role-restore" type="button" data-role-id="${esc(role.machine_role_id)}">Restore</button><button class="button danger machine-role-remove" type="button" data-role-id="${esc(role.machine_role_id)}">Remove permanently</button>`
                    : `<button class="button ghost machine-role-retire" type="button" data-role-id="${esc(role.machine_role_id)}" ${assigned ? `disabled title="Release the assigned Companion first"` : ""}>Retire</button>`}
                </div>
              </div>
              <p class="muted">Capabilities: ${esc((role.required_capabilities || []).join(", ") || "none")}</p>
              <p class="muted">Cue target: ${target ? `<strong>${esc(target.logical_name)}</strong>` : "not created yet"}</p>
              ${!role.retired && !target && editable ? `<button class="button machine-role-target" type="button" data-role-id="${esc(role.machine_role_id)}" data-role-key="${esc(role.role_key)}">Add as Cue target</button>` : ""}
              ${role.retired ? `<div class="message warn">Retired roles stay in history and cannot be assigned or executed. Restore the Role before reuse.</div>` : `
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
              `}
              ${incompatibleTrusted.length ? `
                <details style="margin-top:10px">
                  <summary>Trusted Companions missing required capabilities</summary>
                  <div class="actions-editor" style="margin-top:8px">
                    ${incompatibleTrusted.map((item) => {
                      const missing = missingRoleCapabilities(item, role);
                      return `<div class="action-editor"><strong>${esc(item.display_name || item.hostname || item.companion_id)}</strong><p class="muted">Missing: ${esc(missing.join(", "))}</p><p class="mono muted">${esc(item.companion_id)}</p></div>`;
                    }).join("")}
                  </div>
                </details>` : ""}
            </div>`;
          }).join("") : `<div class="empty">No Machine Roles yet.</div>`}
        </div>
      </article>` : ""}
    </div>`;

  el("refreshConfiguration").addEventListener("click", () => renderConfiguration().catch(configurationError));
  el("jumpMachineRoles")?.addEventListener("click", () => document.getElementById("machineRolesCard")?.scrollIntoView({ behavior: "smooth", block: "start" }));
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
      if (el("machineRoleEnvironment").checked) requiredCapabilities.push(executionEnvironmentOperationCapability);
      if (el("machineRoleEcho").checked) requiredCapabilities.push("local.echo");
      if (el("machineRoleNativeVisual").checked) requiredCapabilities.push(...nativeVisualRoleCapabilities);
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
            target_ref: roleKey,
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
  const inputForm = el("inputForm");
  const resetInputEditor = () => {
    inputForm.dataset.inputId = "";
    inputForm.reset();
    el("inputEvent").value = "osc.message";
    el("inputEnabled").checked = true;
    el("inputSchema").value = "{}";
    el("inputSubmit").textContent = "Add input";
    el("inputCancelEdit").classList.add("hidden");
  };
  document.querySelectorAll(".input-edit").forEach((button) => {
    button.addEventListener("click", () => {
      const input = (model.inputs || []).find((item) => item.input_id === button.dataset.inputId);
      if (!input) return;
      inputForm.dataset.inputId = input.input_id;
      el("inputName").value = input.name || "";
      el("inputSource").value = input.source_ref || "";
      el("inputEvent").value = input.event_type || "";
      el("inputEnabled").checked = !!input.enabled;
      el("inputSchema").value = jsonText(input.value_schema || {});
      el("inputSubmit").textContent = "Save input";
      el("inputCancelEdit").classList.remove("hidden");
      inputForm.scrollIntoView({ behavior: "smooth", block: "center" });
    });
  });
  document.querySelectorAll(".input-remove").forEach((button) => {
    button.addEventListener("click", async () => {
      const input = (model.inputs || []).find((item) => item.input_id === button.dataset.inputId);
      if (!input || !window.confirm(`Remove input "${input.name}"? Removal is blocked while any Route still references it.`)) return;
      try {
        await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/inputs/${encodeURIComponent(input.input_id)}?confirm=true`, { method: "DELETE" });
        await refreshProjectAndConfiguration();
      } catch (error) { configurationError(error); }
    });
  });
  el("inputCancelEdit").addEventListener("click", resetInputEditor);
  inputForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    const inputID = inputForm.dataset.inputId || "";
    try {
      await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/inputs${inputID ? `/${encodeURIComponent(inputID)}` : ""}`, {
        method: inputID ? "PUT" : "POST",
        json: {
          name: el("inputName").value.trim(),
          source_ref: el("inputSource").value.trim(),
          event_type: el("inputEvent").value.trim(),
          value_schema: parseJSONField(el("inputSchema").value, "Input schema"),
          enabled: el("inputEnabled").checked,
        },
      });
      await refreshProjectAndConfiguration();
    } catch (error) { configurationError(error); }
  });
  const outputForm = el("outputForm");
  const resetOutputEditor = () => {
    outputForm.dataset.outputId = "";
    outputForm.reset();
    el("outputCapability").value = "osc.send";
    el("outputCriticality").value = "NORMAL";
    el("outputSchema").value = "{}";
    el("outputSubmit").textContent = "Add output";
    el("outputCancelEdit").classList.add("hidden");
  };
  document.querySelectorAll(".output-edit").forEach((button) => {
    button.addEventListener("click", () => {
      const output = (model.outputs || []).find((item) => item.output_id === button.dataset.outputId);
      if (!output) return;
      outputForm.dataset.outputId = output.output_id;
      el("outputName").value = output.name || "";
      el("outputTarget").value = output.target_ref || "";
      el("outputCapability").value = output.capability_key || "";
      el("outputCriticality").value = output.criticality || "NORMAL";
      el("outputSchema").value = jsonText(output.value_schema || {});
      el("outputSubmit").textContent = "Save output";
      el("outputCancelEdit").classList.remove("hidden");
      outputForm.scrollIntoView({ behavior: "smooth", block: "center" });
    });
  });
  el("outputCancelEdit").addEventListener("click", resetOutputEditor);
  outputForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    const outputID = outputForm.dataset.outputId || "";
    try {
      await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/outputs${outputID ? `/${encodeURIComponent(outputID)}` : ""}`, {
        method: outputID ? "PUT" : "POST",
        json: {
          name: el("outputName").value.trim(),
          target_ref: el("outputTarget").value,
          capability_key: el("outputCapability").value.trim(),
          value_schema: parseJSONField(el("outputSchema").value, "Output schema"),
          criticality: el("outputCriticality").value,
        },
      });
      await refreshProjectAndConfiguration();
    } catch (error) { configurationError(error); }
  });
  const routeForm = el("routeForm");
  const routePayload = (route, enabled = route.enabled) => ({
    name: route.name,
    input_id: route.input_id,
    condition_definition: route.condition_definition ?? null,
    transform_definition: route.transform_definition ?? null,
    delay_ms: route.delay_ms ?? undefined,
    debounce_ms: route.debounce_ms ?? undefined,
    priority_class: route.priority_class || "P2",
    error_policy: route.error_policy || {},
    enabled: !!enabled,
    actions: (route.actions || []).map((action) => ({
      ...(action.output_id ? { output_id: action.output_id } : {}),
      ...(action.cue_id ? { cue_id: action.cue_id } : {}),
      parameters: action.parameters || {},
    })),
  });
  const setExistingRouteAdvancedMode = () => {
    const controls = [
      ["f002RouteConditionKind", "advanced"],
      ["f002RouteTransformKind", "advanced"],
      ["f002RouteActionData", "advanced"],
    ];
    for (const [id, value] of controls) {
      const control = el(id);
      if (!control) continue;
      control.value = value;
      control.dispatchEvent(new Event("change", { bubbles: true }));
    }
  };
  const resetRouteEditor = () => {
    routeForm.dataset.routeId = "";
    routeForm.reset();
    el("routePriority").value = "P2";
    el("routeDelay").value = "0";
    el("routeDebounce").value = "0";
    el("routeEnabled").checked = true;
    el("routeCondition").value = "null";
    el("routeTransform").value = "null";
    el("routeParameters").value = "{}";
    el("routeSubmit").textContent = "Add route";
    el("routeCancelEdit").classList.add("hidden");
    for (const [id, value] of [["f002RouteConditionKind", "always"], ["f002RouteTransformKind", "identity"], ["f002RouteActionData", "transformed"]]) {
      const control = el(id);
      if (!control) continue;
      control.value = value;
      control.dispatchEvent(new Event("change", { bubbles: true }));
    }
  };
  document.querySelectorAll(".route-edit").forEach((button) => {
    button.addEventListener("click", () => {
      const route = (model.routes || []).find((item) => item.route_id === button.dataset.routeId);
      if (!route || (route.actions || []).length !== 1) return;
      const action = route.actions[0];
      routeForm.dataset.routeId = route.route_id;
      el("routeName").value = route.name || "";
      el("routeInput").value = route.input_id || "";
      el("routeOutput").value = action.output_id || "";
      el("routeCue").value = action.cue_id || "";
      el("routePriority").value = route.priority_class || "P2";
      el("routeDelay").value = route.delay_ms ?? 0;
      el("routeDebounce").value = route.debounce_ms ?? 0;
      el("routeEnabled").checked = !!route.enabled;
      el("routeCondition").value = jsonText(route.condition_definition ?? null);
      el("routeTransform").value = jsonText(route.transform_definition ?? null);
      el("routeParameters").value = jsonText(action.parameters || {});
      el("routeSubmit").textContent = "Save route";
      el("routeCancelEdit").classList.remove("hidden");
      setExistingRouteAdvancedMode();
      routeForm.scrollIntoView({ behavior: "smooth", block: "center" });
    });
  });
  document.querySelectorAll(".route-toggle").forEach((button) => {
    button.addEventListener("click", async () => {
      const route = (model.routes || []).find((item) => item.route_id === button.dataset.routeId);
      if (!route) return;
      try {
        await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/routes/${encodeURIComponent(route.route_id)}`, {
          method: "PUT",
          json: routePayload(route, !route.enabled),
        });
        await refreshProjectAndConfiguration();
      } catch (error) { configurationError(error); }
    });
  });
  document.querySelectorAll(".route-remove").forEach((button) => {
    button.addEventListener("click", async () => {
      const route = (model.routes || []).find((item) => item.route_id === button.dataset.routeId);
      if (!route || !window.confirm(`Remove Route "${route.name}" from this Draft?`)) return;
      try {
        await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/routes/${encodeURIComponent(route.route_id)}?confirm=true`, { method: "DELETE" });
        await refreshProjectAndConfiguration();
      } catch (error) { configurationError(error); }
    });
  });
  el("routeCancelEdit").addEventListener("click", resetRouteEditor);
  routeForm.addEventListener("submit", async (event) => {
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
    const routeID = routeForm.dataset.routeId || "";
    const existing = (model.routes || []).find((item) => item.route_id === routeID);
    try {
      const delayMS = Number.parseInt(el("routeDelay").value || "0", 10);
      const debounceMS = Number.parseInt(el("routeDebounce").value || "0", 10);
      await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/routes${routeID ? `/${encodeURIComponent(routeID)}` : ""}`, {
        method: routeID ? "PUT" : "POST",
        json: {
          name: el("routeName").value.trim(),
          input_id: el("routeInput").value,
          condition_definition: parseJSONField(el("routeCondition").value, "Route condition"),
          transform_definition: parseJSONField(el("routeTransform").value, "Route transform"),
          delay_ms: delayMS > 0 ? delayMS : undefined,
          debounce_ms: debounceMS > 0 ? debounceMS : undefined,
          priority_class: el("routePriority").value,
          error_policy: existing?.error_policy || {},
          enabled: el("routeEnabled").checked,
          actions: [action],
        },
      });
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