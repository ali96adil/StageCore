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
  const [secrets, users, permissions, audit] = await Promise.all([
    api("/api/v1/security/secrets"),
    api("/api/v1/security/users"),
    api("/api/v1/security/plugins/permissions?plugin_id=stagecore.osc"),
    api("/api/v1/security/audit?limit=100"),
  ]);
  return {
    secrets: secrets.secrets || [],
    users: users.users || [],
    permissions: permissions.permissions || [],
    audit: audit.records || [],
  };
}

function securityRoleOptions(selected) {
  return ["OWNER", "TECHNICIAN", "OPERATOR", "VIEWER"]
    .map((role) => `<option value="${role}" ${role === selected ? "selected" : ""}>${role}</option>`)
    .join("");
}

async function renderSecurity() {
  if (state.user?.role !== "OWNER") {
    content.innerHTML = `<div class="empty">OWNER authorization is required for Security administration.</div>`;
    return;
  }
  const model = await securityLoad();
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
    </article>

    <article class="card" style="margin-top:16px">
      <p class="eyebrow">COMPANION TRUST</p><h2>Pair or emergency revoke</h2>
      <div class="form-grid two" style="margin-top:14px">
        <form id="pairingApproveForm">
          <label>Pairing request ID<input id="pairingRequestID" required></label>
          <label>Pairing code<input id="pairingCode" required></label>
          <button class="button" type="submit">Approve pairing</button>
        </form>
        <form id="companionRevokeForm">
          <label>Companion ID<input id="revokeCompanionID" required></label>
          <label>Emergency reason<input id="revokeCompanionReason" required></label>
          <button class="button ghost" type="submit">Emergency revoke Companion</button>
        </form>
      </div>
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
  el("pairingApproveForm").addEventListener("submit", async (event) => {
    event.preventDefault();
    try { await api("/api/v1/security/companions/pairing/approve", { method: "POST", json: { request_id: el("pairingRequestID").value.trim(), pairing_code: el("pairingCode").value.trim() } }); await renderSecurity(); }
    catch (error) { securityError(error); }
  });
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
