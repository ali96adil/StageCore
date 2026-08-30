"use strict";

const stagecoreBaseLoadConfiguration = loadConfiguration;
const stagecoreBaseConfigurationEditable = configurationEditable;
const stagecoreBaseRenderConfiguration = renderConfiguration;
const stagecoreBaseRenderCues = renderCues;
const stagecoreBaseCanEdit = canEdit;

async function stagecoreLoadShowConfigurationLock() {
  const payload = await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/configuration/lock`);
  state.showConfigurationLock = payload.show_configuration_lock || { locked: false };
  return state.showConfigurationLock;
}

function stagecoreShowConfigurationLockBanner(lock) {
  if (!lock?.locked) return;
  const pageHead = content.querySelector(".page-head");
  if (!pageHead || content.querySelector("[data-show-configuration-lock]")) return;
  pageHead.insertAdjacentHTML("afterend", `
    <div class="message error" data-show-configuration-lock>
      <strong>SHOW MODE — CONFIGURATION LOCKED</strong><br>
      Structural Project configuration cannot be changed while the active SHOW is running.
      Exit SHOW through the authorized Runtime controls before editing configuration.
      <span class="mono">Session ${esc(lock.active_show_session_id || "")} · Snapshot ${esc(lock.runtime_snapshot_id || "")}</span>
    </div>`);
}

loadConfiguration = async function stagecoreLoadConfigurationWithShowLock() {
  const [model, lock] = await Promise.all([
    stagecoreBaseLoadConfiguration(),
    stagecoreLoadShowConfigurationLock(),
  ]);
  model.show_configuration_lock = lock;
  return model;
};

configurationEditable = function stagecoreConfigurationEditableWithShowLock() {
  return stagecoreBaseConfigurationEditable() && !state.showConfigurationLock?.locked;
};

renderConfiguration = async function stagecoreRenderConfigurationWithShowLock() {
  await stagecoreBaseRenderConfiguration();
  stagecoreShowConfigurationLockBanner(state.showConfigurationLock);
};

renderCues = async function stagecoreRenderCuesWithShowLock(message = "") {
  const lock = await stagecoreLoadShowConfigurationLock();
  if (!lock.locked) {
    await stagecoreBaseRenderCues(message);
    return;
  }
  canEdit = () => false;
  try {
    await stagecoreBaseRenderCues(message);
  } finally {
    canEdit = stagecoreBaseCanEdit;
  }
  stagecoreShowConfigurationLockBanner(lock);
};
