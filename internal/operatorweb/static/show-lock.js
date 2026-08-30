"use strict";

const stagecoreBaseLoadConfiguration = loadConfiguration;
const stagecoreBaseConfigurationEditable = configurationEditable;
const stagecoreBaseRenderConfiguration = renderConfiguration;

loadConfiguration = async function stagecoreLoadConfigurationWithShowLock() {
  const [model, lockPayload] = await Promise.all([
    stagecoreBaseLoadConfiguration(),
    api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/configuration/lock`),
  ]);
  state.showConfigurationLock = lockPayload.show_configuration_lock || { locked: false };
  model.show_configuration_lock = state.showConfigurationLock;
  return model;
};

configurationEditable = function stagecoreConfigurationEditableWithShowLock() {
  return stagecoreBaseConfigurationEditable() && !state.showConfigurationLock?.locked;
};

renderConfiguration = async function stagecoreRenderConfigurationWithShowLock() {
  await stagecoreBaseRenderConfiguration();
  const lock = state.showConfigurationLock;
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
};
