"use strict";

const bootstrapPanel = el("bootstrapPanel");
const loginPanel = el("loginPanel");
const bootstrapForm = el("bootstrapForm");

function applyBootstrapUI() {
  const unclaimed = state.hub?.bootstrap_state === "UNCLAIMED";
  bootstrapPanel?.classList.toggle("hidden", !unclaimed);
  loginPanel?.classList.toggle("hidden", unclaimed);
  if (unclaimed) {
    setMessage(loginMessage, "This fresh Hub needs its first OWNER. Generate a setup code locally with stagecore-setup, then claim it here.", "warn");
  } else if (loginMessage?.textContent?.includes("fresh Hub")) {
    setMessage(loginMessage, "");
  }
}

async function refreshBootstrapUI() {
  state.hub = await api("/api/v1/auth/status");
  updateHubIdentity();
  applyBootstrapUI();
}

bootstrapForm?.addEventListener("submit", async (event) => {
  event.preventDefault();
  const setupCode = el("bootstrapCode").value.trim();
  const username = el("bootstrapUsername").value.trim();
  const password = el("bootstrapPassword").value;
  const confirm = el("bootstrapPasswordConfirm").value;
  if (password !== confirm) {
    setMessage(loginMessage, "OWNER passwords do not match.", "error");
    return;
  }
  setMessage(loginMessage, "Claiming this Hub…");
  try {
    const response = await fetch("/api/v1/auth/bootstrap", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "same-origin",
      cache: "no-store",
      body: JSON.stringify({ setup_code: setupCode, username, password }),
    });
    const payload = await response.json().catch(() => ({}));
    if (!response.ok) {
      const error = new Error(`HTTP ${response.status}`);
      error.status = response.status;
      error.payload = payload;
      throw error;
    }

    state.hub.bootstrap_state = "CLAIMED";
    applyBootstrapUI();
    el("username").value = username;
    const login = await apiLogin(username, password);
    state.user = login.user;
    state.csrf = login.csrf_token;
    sessionStorage.setItem("stagecore_csrf", state.csrf);
    el("bootstrapPassword").value = "";
    el("bootstrapPasswordConfirm").value = "";
    showApp();
    await loadProjects();
    renderProjects();
    setMessage(loginMessage, "");
  } catch (error) {
    setMessage(loginMessage, errorMessage(error), "error");
  }
});

// app.js performs its own normal boot. This second local status read only
// selects the correct first-run/login panel and introduces no WAN dependency.
refreshBootstrapUI().catch(() => {});
