"use strict";

const stagecoreSecurityShowAppBase = showApp;
const stagecoreSecurityShowLoginBase = showLogin;

showApp = function stagecoreSecurityShowApp() {
  stagecoreSecurityShowAppBase();
  const button = el("securityNav");
  if (button) button.classList.toggle("hidden", state.user?.role !== "OWNER");
};

showLogin = function stagecoreSecurityShowLogin(message = "") {
  const button = el("securityNav");
  if (button) button.classList.add("hidden");
  stagecoreSecurityShowLoginBase(message);
};

function securityResultKind(result) {
  if (result === "SUCCESS") return "good";
  if (result === "REJECTED") return "warn";
  if (result === "FAILED") return "bad";
  return "neutral";
}

async function securityLoad() {
  const [secrets, users, permissions, audit, pairing, companions] = await Promise.all([
    api("/api/v1/security/secrets"),
    api("/api/v1/security/users"),
    api("/api/v1/security/plugins/permissions?plugin_id=stagecore.osc"),
    api("/api/v1/security/audit?limit=100"),
    api("/api/v1/security/companions/pairing/pending"),
    api("/api/v1/security/companions"),
  ]);
  let projectAccess = null;
  try {
    const preview = await api("/api/v1/project-access/cutover-preview");
    if (preview?.enforced === false) {
      const projectList = await api("/api/v1/projects");
      projectAccess = { projects: projectList.projects || [], enforced: false };
    }
  } catch (_) {
    // The grant routes stay absent from production until the complete Project
    // ACL cutover is approved. Keep Security usable when opt-in routes are off.
  }
  return {
    secrets: secrets.secrets || [],
    users: users.users || [],
    permissions: permissions.permissions || [],
    audit: audit.records || [],
    pendingPairing: pairing.requests || [],
    companions: companions.companions || [],
    projectAccess,
  };
}

function securityRoleOptions(selected) {
  return ["OWNER", "TECHNICIAN", "OPERATOR", "VIEWER"]
    .map((role) => `<option value="${role}" ${role === selected ? "selected" : ""}>${role}</option>`)
    .join("");
}

function securityProjectAccessLevelOptions(role, enabled, currentLevel = "") {
  const labels = {
    VIEW: "View",
    EDIT: "Edit",
    OPERATE: "Operate",
    MANAGE: "Manage",
    NONE: "No Project access",
    CUSTOM: "Custom fine-grained permissions",
  };
  const scopedOperator = role === "OPERATOR" || role === "TECHNICIAN";
  const allowed = !enabled || (!scopedOperator && role !== "VIEWER") ? ["NONE"]
    : role === "VIEWER" ? ["VIEW", "NONE"]
      : ["VIEW", "EDIT", "OPERATE", "MANAGE", "NONE"];
  const knownCurrent = Object.prototype.hasOwnProperty.call(labels, currentLevel) ? currentLevel : "";
  const currentUnavailable = knownCurrent && !allowed.includes(knownCurrent);
  const placeholder = currentUnavailable
    ? `<option value="${knownCurrent}" selected disabled>${labels[knownCurrent]} (current)</option>`
    : knownCurrent ? "" : `<option value="" selected disabled>Choose a level</option>`;
  return placeholder + allowed.map((level) =>
    `<option value="${level}" ${level === knownCurrent ? "selected" : ""}>${labels[level]}</option>`
  ).join("");
}

function securityProjectAccessLevelDescription(level) {
  const descriptions = {
    VIEW: "View Project information and read assigned media.",
    EDIT: "View, edit Project/Cues, and upload media.",
    OPERATE: "Run Rehearsal/SHOW, STOP and Emergency Blackout, and control assigned lighting/audio.",
    MANAGE: "Edit and operate; publish Snapshots, configure devices, manage media, rename Projects, and export Capsules. Only the Owner can grant access or manage memberships.",
    NONE: "Remove this account's Project membership and grants.",
    CUSTOM: "This account has fine-grained custom permissions. Use the qualified grant API to inspect or change individual capabilities.",
  };
  return descriptions[level] || "Load the current access level before making a change.";
}

async function renderSecurity() {
  if (state.user?.role !== "OWNER") {
    content.innerHTML = `<div class="empty">OWNER authorization is required for Security administration.</div>`;
    return;
  }
  const model = await securityLoad();
  const trustedCompanions = model.companions.filter((companion) => companion.trust_state === "TRUSTED");
  const historicalCompanions = model.companions.filter((companion) => companion.trust_state !== "TRUSTED");
  const projectAccessAvailable = model.projectAccess?.enforced === false;
  const projectAccessUsers = model.users.filter((user) => user.role !== "OWNER");
  content.innerHTML = `
    <div class="page-head">
      <div><p class="eyebrow">HUB SECURITY</p><h1>Security operations</h1><p>Secrets, users, first-party Plugin permissions and append-oriented audit records stay local to this Hub.</p></div>
      <div class="toolbar"><button id="renewSession" class="button" type="button">Renew local session</button><button id="refreshSecurity" class="button" type="button">Refresh</button></div>
    </div>

    <div class="grid cards">
      <article class="card">
        <p class="eyebrow">SECRET STORE</p><h2>Credential references</h2>
        <p class="muted">Values are encrypted at rest and never displayed after entry.</p>
        <form id="createSecretForm" class="form-grid two" style="margin-top:14px">
          <label>Logical name<input id="newSecretName" placeholder="pjlink-main-password" required></label>
          <label>Secret value<input id="newSecretValue" type="password" autocomplete="new-password" required></label>
          <button class="button primary" type="submit">Create secret</button>
        </form>
        <div class="actions-editor" style="margin-top:14px">
          ${model.secrets.length ? model.secrets.map((secret) => `
            <div class="action-editor secret-row" data-name="${esc(secret.logical_name)}">
              <div class="section-title-row"><div><strong>${esc(secret.reference)}</strong><p class="muted">Updated ${esc(fmtDate(secret.updated_at))}</p></div><div class="toolbar"><button class="button rotate-secret" type="button">Rotate</button><button class="button ghost delete-secret" type="button">Delete</button></div></div>
            </div>`).join("") : `<div class="empty">No secrets stored.</div>`}
        </div>
      </article>

      <article class="card">
        <p class="eyebrow">FIRST-PARTY PLUGIN</p><h2>OSC permissions</h2>
        <p class="muted">Revoking a required permission blocks new executions and SHOW Preflight without changing the published Snapshot.</p>
        <div class="actions-editor" style="margin-top:14px">
          ${model.permissions.length ? model.permissions.map((grant) => `
            <label class="action-editor check-row permission-row" data-plugin="${esc(grant.plugin_id)}" data-permission="${esc(grant.permission)}">
              <input class="permission-toggle" type="checkbox" ${grant.granted ? "checked" : ""}> <span><strong>${esc(grant.permission)}</strong><br><span class="muted">${grant.granted ? "Granted" : "Revoked"} · ${esc(grant.updated_by || "system")}</span></span>
            </label>`).join("") : `<div class="empty">No first-party permission records.</div>`}
        </div>
      </article>
    </div>

    <article class="card" style="margin-top:16px">
      <div class="section-title-row"><div><p class="eyebrow">LOCAL USERS</p><h2>Accounts and emergency session revocation</h2></div></div>
      <form id="createUserForm" class="form-grid three" style="margin-top:14px">
        <label>Username<input id="newUsername" required></label>
        <label>Password<input id="newUserPassword" type="password" autocomplete="new-password" minlength="12" required></label>
        <label>Role<select id="newUserRole">${securityRoleOptions("VIEWER")}</select></label>
        <button class="button primary" type="submit">Create user</button>
      </form>
      <div class="actions-editor" style="margin-top:14px">
        ${model.users.map((user) => `
          <div class="action-editor user-row" data-user-id="${esc(user.user_id)}">
            <div class="section-title-row"><div><strong>${esc(user.username)}</strong><p class="muted mono">${esc(user.user_id)}</p></div>${pill(user.enabled ? "ENABLED" : "DISABLED", user.enabled ? "good" : "bad")}</div>
            <div class="form-grid three" style="margin-top:10px">
              <label>Role<select class="user-role">${securityRoleOptions(user.role)}</select></label>
              <label class="check-row"><input class="user-enabled" type="checkbox" ${user.enabled ? "checked" : ""}> Enabled</label>
              <button class="button ghost revoke-user-sessions" type="button">Emergency revoke sessions</button>
            </div>
          </div>`).join("")}
      </div>
      ${projectAccessAvailable ? `
        <section class="action-editor" style="margin-top:16px">
          <div class="section-title-row"><div><p class="eyebrow">PROJECT ACCESS · QUALIFICATION ONLY</p><h3>Project permissions</h3></div>${pill("PREPARED · NOT ENFORCED", "warn")}</div>
          <p class="muted">These grants are stored by the opt-in qualification API but are not enforced by this Hub. Do not rely on them to restrict Project, Cue, Vault, device, or Show access. A separate reviewed cutover is required.</p>
          ${model.projectAccess.projects.length && projectAccessUsers.length ? `
            <form id="projectAccessGrantForm" class="form-grid three" style="margin-top:12px">
              <label>Project<select id="projectAccessProject" required><option value="" selected disabled>Choose a Project</option>${model.projectAccess.projects.map((project) => `<option value="${esc(project.project_id)}">${esc(project.name)}</option>`).join("")}</select></label>
              <label>Account<select id="projectAccessUser" required><option value="" selected disabled>Choose an account</option>${projectAccessUsers.map((user) => `<option value="${esc(user.user_id)}" data-role="${esc(user.role)}" data-enabled="${String(!!user.enabled)}">${esc(user.username)} · ${esc(user.role)}${user.enabled ? "" : " · disabled"}</option>`).join("")}</select></label>
              <label>Access level<select id="projectAccessLevel" disabled required><option value="" selected disabled>Load current access first</option></select></label>
              <div class="toolbar"><button id="loadProjectAccess" class="button ghost" type="button">Load current access</button><button id="saveProjectAccess" class="button primary" type="submit" disabled>Save prepared access</button></div>
            </form>
            <p id="projectAccessLevelDescription" class="muted" aria-live="polite" style="margin-top:8px">Load the current access level before making a change.</p>
            <p id="projectAccessCurrent" class="muted" role="status" aria-live="polite" style="margin-top:10px">Select a Project and account, then load current access.</p>
          ` : `<div class="empty" style="margin-top:12px">An account and Project are required to review prepared grants.</div>`}
        </section>` : ""}
    </article>

    <article class="card" style="margin-top:16px">
      <div class="section-title-row">
        <div><p class="eyebrow">COMPANION TRUST</p><h2>Trusted Companions</h2></div>
        <div class="toolbar">${pill(`TRUSTED ${trustedCompanions.length}`, trustedCompanions.length ? "good" : "neutral")}</div>
      </div>
      <p class="muted">Only currently trusted Companion identities count here. Revoked and untrusted identities remain preserved in Security history.</p>
      <h3 style="margin-top:16px">Pending pairing</h3>
      <p class="muted">Pairing request IDs stay internal. Match the code shown on the physical device before approving trust.</p>
      <div class="actions-editor" style="margin-top:14px">
        ${model.pendingPairing.length ? model.pendingPairing.map((request) => `
          <div class="action-editor pairing-request-row" data-pairing-request="${esc(request.request_id)}">
            <div class="section-title-row">
              <div>
                <strong>${esc(request.display_name || request.companion_id)}</strong>
                <p class="muted">${esc(request.platform || "unknown")} ${esc(request.version || "")} · expires ${esc(fmtDate(request.expires_at))}</p>
                <p class="muted mono">${esc(request.companion_id)}</p>
              </div>
              ${pill("PENDING", "warn")}
            </div>
            <div class="form-grid two" style="margin-top:10px">
              <label>Pairing code<input class="pairing-code" autocomplete="off" spellcheck="false" required></label>
              <button class="button approve-pairing-request" type="button">Approve pairing</button>
            </div>
          </div>`).join("") : `<div class="empty">No pending Companion pairing requests.</div>`}
      </div>
      <div class="actions-editor" style="margin-top:14px">
        ${trustedCompanions.length ? trustedCompanions.map((companion) => `
          <div class="action-editor companion-inventory-row" data-companion-id="${esc(companion.companion_id)}">
            <div class="section-title-row">
              <div>
                <strong>${esc(companion.display_name || companion.hostname || companion.companion_id)}</strong>
                <p class="muted">${esc(companion.hostname || "—")} · ${esc(companion.platform || "unknown")} ${esc(companion.version || "")} · last seen ${esc(fmtDate(companion.last_seen_at))}</p>
                <p class="muted mono">${esc(companion.companion_id)}</p>
              </div>
              <div class="toolbar">${pill(companion.trust_state || "UNKNOWN", companion.trust_state === "TRUSTED" ? "good" : companion.trust_state === "REVOKED" ? "bad" : "warn")}${pill(companion.readiness || "UNKNOWN", companion.readiness === "READY" ? "good" : companion.readiness === "OFFLINE" ? "bad" : "warn")}</div>
            </div>
            <p class="muted">Capabilities: ${esc((companion.capabilities || []).join(", ") || "—")}</p>
            ${companion.machine_role ? `<p class="muted">Machine Role: <strong>${esc(companion.machine_role.display_name || companion.machine_role.role_key)}</strong> · ${esc(companion.machine_role.role_key)} · ${esc(companion.assignment?.state || "")}</p>` : '<p class="muted">Machine Role: unassigned</p>'}
          </div>`).join("") : '<div class="empty">No trusted Companion identities.</div>'}
      </div>

      <details class="action-editor" style="margin-top:14px">
        <summary><strong>History / removed devices</strong> · ${historicalCompanions.length}</summary>
        <p class="muted" style="margin-top:10px">Revoked and untrusted identities are excluded from the trusted count but retained for pairing, key and security audit history.</p>
        <div class="actions-editor" style="margin-top:10px">
          ${historicalCompanions.length ? historicalCompanions.map((companion) => `
            <div class="action-editor companion-history-row" data-companion-id="${esc(companion.companion_id)}">
              <div class="section-title-row">
                <div>
                  <strong>${esc(companion.display_name || companion.hostname || companion.companion_id)}</strong>
                  <p class="muted">${esc(companion.hostname || "—")} · ${esc(companion.platform || "unknown")} ${esc(companion.version || "")} · last seen ${esc(fmtDate(companion.last_seen_at))}</p>
                  <p class="muted mono">${esc(companion.companion_id)}</p>
                </div>
                <div class="toolbar">${pill(companion.trust_state || "UNKNOWN", companion.trust_state === "REVOKED" ? "bad" : "warn")}</div>
              </div>
            </div>`).join("") : '<div class="empty">No revoked or untrusted Companion history.</div>'}
        </div>
      </details>

      <form id="companionRevokeForm" class="form-grid two" style="margin-top:14px">
        <label>Companion
          <select id="revokeCompanionID" required>
            <option value="">Choose a Companion…</option>
            ${trustedCompanions.map((companion) => `<option value="${esc(companion.companion_id)}">${esc(companion.display_name || companion.hostname || companion.companion_id)} · ${esc(companion.hostname || companion.companion_id)}</option>`).join("")}
          </select>
        </label>
        <label>Emergency reason<input id="revokeCompanionReason" required></label>
        <button class="button ghost" type="submit" ${trustedCompanions.length ? "" : "disabled"}>Emergency revoke Companion</button>
      </form>
    </article>

    <article class="card" style="margin-top:16px">
      <div class="section-title-row"><div><p class="eyebrow">SECURITY AUDIT</p><h2>Latest security records</h2></div><span class="muted">${model.audit.length} shown</span></div>
      <div class="actions-editor" style="margin-top:14px">
        ${model.audit.length ? model.audit.map((record) => `
          <div class="action-editor">
            <div class="section-title-row"><div><strong>${esc(record.event_type)}</strong><p class="muted">${esc(fmtDate(record.occurred_at))} · ${esc(record.actor_username || "system")}</p></div>${pill(record.result, securityResultKind(record.result))}</div>
            <p class="muted" style="margin-top:8px">${esc(record.resource_type || "")}${record.resource_id ? ` · ${esc(record.resource_id)}` : ""}${record.reason ? ` · ${esc(record.reason)}` : ""}</p>
          </div>`).join("") : `<div class="empty">No security audit records yet.</div>`}
      </div>
    </article>`;

  el("refreshSecurity").addEventListener("click", () => renderSecurity().catch(securityError));
  el("renewSession").addEventListener("click", async () => {
    try {
      const renewed = await api("/api/v1/auth/renew", { method: "POST" });
      state.csrf = renewed.csrf_token;
      sessionStorage.setItem("stagecore_csrf", state.csrf);
      state.user = renewed.user;
      setMessage(globalMessage, "Local session renewed; the previous session credential is revoked.", "good");
    } catch (error) { securityError(error); }
  });
  const projectAccessForm = el("projectAccessGrantForm");
  if (projectAccessForm) {
    const projectSelect = el("projectAccessProject");
    const userSelect = el("projectAccessUser");
    const levelSelect = el("projectAccessLevel");
    const loadButton = el("loadProjectAccess");
    const saveButton = el("saveProjectAccess");
    const levelDescription = el("projectAccessLevelDescription");
    const currentStatus = el("projectAccessCurrent");
    let loadedTarget = "";
    const targetKey = () => `${projectSelect.value}/${userSelect.value}`;
    const canSaveLoadedLevel = () => {
      const selectedUser = projectAccessUsers.find((user) => user.user_id === userSelect.value);
      return loadedTarget === targetKey() && !!levelSelect.value && levelSelect.value !== "CUSTOM" &&
        (!!selectedUser?.enabled || levelSelect.value === "NONE");
    };
    const resetLoadedAccess = () => {
      loadedTarget = "";
      levelSelect.disabled = true;
      levelSelect.innerHTML = `<option value="" selected disabled>Load current access first</option>`;
      saveButton.disabled = true;
      levelDescription.textContent = securityProjectAccessLevelDescription("");
      currentStatus.textContent = "Select a Project and account, then load current access.";
    };
    projectSelect.addEventListener("change", resetLoadedAccess);
    userSelect.addEventListener("change", resetLoadedAccess);
    levelSelect.addEventListener("change", () => {
      levelDescription.textContent = securityProjectAccessLevelDescription(levelSelect.value);
      saveButton.disabled = !canSaveLoadedLevel();
    });
    loadButton.addEventListener("click", async () => {
      const projectID = projectSelect.value;
      const userID = userSelect.value;
      const requestedTarget = targetKey();
      if (!projectID || !userID) {
        currentStatus.textContent = "Choose both a Project and account first.";
        return;
      }
      loadButton.disabled = true;
      saveButton.disabled = true;
      try {
        const access = await api(`/api/v1/projects/${encodeURIComponent(projectID)}/access/users/${encodeURIComponent(userID)}`);
        if (access.enforced !== false || access.phase !== "PREPARED_NOT_ENFORCED") {
          throw new Error("The Project grant API did not confirm its not-enforced qualification state.");
        }
        if (targetKey() !== requestedTarget) {
          resetLoadedAccess();
          currentStatus.textContent = "Selection changed while loading. Load access again.";
          return;
        }
        const user = projectAccessUsers.find((candidate) => candidate.user_id === userID);
        levelSelect.innerHTML = securityProjectAccessLevelOptions(user?.role || "VIEWER", !!user?.enabled, access.access_level || "CUSTOM");
        levelSelect.disabled = false;
        loadedTarget = requestedTarget;
        levelDescription.textContent = securityProjectAccessLevelDescription(levelSelect.value);
        currentStatus.textContent = `Current: ${access.access_level || "CUSTOM"}. Fine-grained permissions: ${(access.permissions || []).join(", ") || "none"}. Stored grants are not enforced.`;
        saveButton.disabled = !canSaveLoadedLevel();
      } catch (error) {
        resetLoadedAccess();
        currentStatus.textContent = errorMessage(error);
      } finally {
        loadButton.disabled = false;
      }
    });
    projectAccessForm.addEventListener("submit", async (event) => {
      event.preventDefault();
      if (!canSaveLoadedLevel()) return;
      const projectID = projectSelect.value;
      const userID = userSelect.value;
      const level = levelSelect.value;
      if (!confirm(`Store ${level} access for this account? These grants remain PREPARED_NOT_ENFORCED and do not restrict this Hub.`)) return;
      saveButton.disabled = true;
      try {
        const result = await api(`/api/v1/projects/${encodeURIComponent(projectID)}/access/users/${encodeURIComponent(userID)}`, {
          method: "PUT", json: { access_level: level },
        });
        if (result.enforced !== false || result.phase !== "PREPARED_NOT_ENFORCED") {
          throw new Error("The Project grant API did not confirm its not-enforced qualification state.");
        }
        if (targetKey() !== `${projectID}/${userID}`) {
          const projectName = model.projectAccess.projects.find((project) => project.project_id === projectID)?.name || projectID;
          const username = projectAccessUsers.find((user) => user.user_id === userID)?.username || userID;
          currentStatus.textContent = `Saved ${result.access_level} for ${projectName} / ${username}. Selection changed; load access for the current account before editing.`;
          return;
        }
        currentStatus.textContent = `Saved ${result.access_level}. PREPARED_NOT_ENFORCED. Fine-grained permissions: ${(result.permissions || []).join(", ") || "none"}.`;
      } catch (error) {
        currentStatus.textContent = errorMessage(error);
        securityError(error);
      } finally {
        saveButton.disabled = !canSaveLoadedLevel();
      }
    });
  }
  el("createSecretForm").addEventListener("submit", async (event) => {
    event.preventDefault();
    try {
      await api("/api/v1/security/secrets", { method: "POST", json: { logical_name: el("newSecretName").value.trim(), value: el("newSecretValue").value } });
      el("newSecretValue").value = "";
      await renderSecurity();
    } catch (error) { securityError(error); }
  });
  content.querySelectorAll(".rotate-secret").forEach((button) => button.addEventListener("click", async () => {
    const name = button.closest(".secret-row").dataset.name;
    const value = prompt(`New value for secret:${name}`);
    if (!value) return;
    try { await api(`/api/v1/security/secrets/${encodeURIComponent(name)}`, { method: "PUT", json: { value } }); await renderSecurity(); }
    catch (error) { securityError(error); }
  }));
  content.querySelectorAll(".delete-secret").forEach((button) => button.addEventListener("click", async () => {
    const name = button.closest(".secret-row").dataset.name;
    if (!confirm(`Delete secret:${name}? Published configuration will keep the reference and Preflight will BLOCK until it is restored.`)) return;
    try { await api(`/api/v1/security/secrets/${encodeURIComponent(name)}`, { method: "DELETE" }); await renderSecurity(); }
    catch (error) { securityError(error); }
  }));
  content.querySelectorAll(".permission-toggle").forEach((toggle) => toggle.addEventListener("change", async () => {
    const row = toggle.closest(".permission-row");
    try { await api(`/api/v1/security/plugins/${encodeURIComponent(row.dataset.plugin)}/permissions/${encodeURIComponent(row.dataset.permission)}`, { method: "PUT", json: { granted: toggle.checked } }); await renderSecurity(); }
    catch (error) { toggle.checked = !toggle.checked; securityError(error); }
  }));
  el("createUserForm").addEventListener("submit", async (event) => {
    event.preventDefault();
    try {
      await api("/api/v1/security/users", { method: "POST", json: { username: el("newUsername").value.trim(), password: el("newUserPassword").value, role: el("newUserRole").value } });
      el("newUserPassword").value = "";
      await renderSecurity();
    } catch (error) { securityError(error); }
  });
  content.querySelectorAll(".user-role").forEach((select) => select.addEventListener("change", async () => {
    const userID = select.closest(".user-row").dataset.userId;
    try { await api(`/api/v1/security/users/${encodeURIComponent(userID)}/role`, { method: "PUT", json: { role: select.value } }); await renderSecurity(); }
    catch (error) { securityError(error); }
  }));
  content.querySelectorAll(".user-enabled").forEach((toggle) => toggle.addEventListener("change", async () => {
    const userID = toggle.closest(".user-row").dataset.userId;
    try { await api(`/api/v1/security/users/${encodeURIComponent(userID)}/enabled`, { method: "PUT", json: { enabled: toggle.checked } }); await renderSecurity(); }
    catch (error) { securityError(error); }
  }));
  content.querySelectorAll(".revoke-user-sessions").forEach((button) => button.addEventListener("click", async () => {
    const row = button.closest(".user-row");
    const reason = prompt("Emergency revocation reason");
    if (!reason || !confirm("Revoke all active sessions for this user now? This remains available during SHOW.")) return;
    try { await api(`/api/v1/security/users/${encodeURIComponent(row.dataset.userId)}/revoke-sessions`, { method: "POST", json: { reason, confirm: "REVOKE" } }); await renderSecurity(); }
    catch (error) { securityError(error); }
  }));
  content.querySelectorAll(".approve-pairing-request").forEach((button) => button.addEventListener("click", async () => {
    const row = button.closest(".pairing-request-row");
    const requestID = row?.dataset.pairingRequest || "";
    const code = row?.querySelector(".pairing-code")?.value.trim() || "";
    if (!requestID || !code) {
      setMessage(globalMessage, "Enter the pairing code shown on the physical device.", "warn");
      return;
    }
    button.disabled = true;
    try {
      await api("/api/v1/security/companions/pairing/approve", { method: "POST", json: { request_id: requestID, pairing_code: code } });
      await renderSecurity();
    } catch (error) {
      button.disabled = false;
      securityError(error);
    }
  }));
  el("companionRevokeForm").addEventListener("submit", async (event) => {
    event.preventDefault();
    const companionID = el("revokeCompanionID").value.trim();
    const reason = el("revokeCompanionReason").value.trim();
    if (!companionID || !reason || !confirm("Emergency revoke this Companion identity? Affected Machine Roles will lose READY state.")) return;
    try { await api(`/api/v1/security/companions/${encodeURIComponent(companionID)}/revoke`, { method: "POST", json: { reason, confirm: "REVOKE" } }); await renderSecurity(); }
    catch (error) { securityError(error); }
  });
}

function securityError(error) {
  setMessage(globalMessage, errorMessage(error), "error");
}

const securityNav = el("securityNav");
if (securityNav) {
  securityNav.addEventListener("click", async () => {
    setPage("security");
    document.querySelectorAll(".nav-button").forEach((button) => button.classList.remove("active"));
    securityNav.classList.add("active");
    setMessage(globalMessage, "");
    try { await renderSecurity(); }
    catch (error) { securityError(error); }
  });
}
