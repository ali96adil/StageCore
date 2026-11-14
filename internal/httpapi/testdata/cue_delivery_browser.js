"use strict";

(async () => {
  function check(condition, description) {
    if (!condition) throw new Error(description);
  }

  async function until(condition, description) {
    for (let i = 0; i < 200; i++) {
      if (condition()) return;
      await new Promise((resolve) => setTimeout(resolve, 25));
    }
    throw new Error(description);
  }

  try {
    const config = await api("/cue-delivery-config");
    state.user = { role: "OWNER" };
    state.csrf = config.csrf;
    state.project = { project_id: config.project };
    state.page = "runtime";
    window.confirm = () => true;

    await renderRuntime();
    el("goButton").click();
    const first = state.runtimeCueReceipt;
    check((await fetch("/cue-delivery-go-stored")).ok, "GO was not persisted");
    await renderRuntime();
    el("jumpCueSelect").value = config.jump;
    el("jumpButton").click();
    await until(() => state.runtimeCueReceipt.result?.payload?.cue_execution_id &&
      el("runtimeCueTrace") && content.textContent.includes("JUMP"), "Jump receipt missing");
    const jump = state.runtimeCueReceipt;
    check((await fetch("/cue-delivery-release-go", { method: "POST" })).ok, "GO release failed");
    await until(() => first.result?.payload?.cue_execution_id, "delayed GO receipt missing");
    check(state.runtimeCueReceipt === jump, "delayed GO replaced Jump");
    // A duplicate request must produce exactly the original execution.
    const replay = await api(`/api/v1/projects/${config.project}/runtime/go`, {
      method: "POST",
      json: {request_id:first.requestID, expected_session_id:config.session, expected_current_cue_id:null}
    });
    check(replay.result?.payload?.cue_execution_id === first.result.payload.cue_execution_id,
      "replayed GO created a different execution");
    el("runtimeCueTrace").click();
    await until(() => content.textContent.includes(jump.result.payload.cue_execution_id), "selected Jump trace missing");
    check(!content.textContent.includes(first.result.payload.cue_execution_id), "trace showed unrelated GO");

    setPage("runtime");
    await renderRuntime();
    el("goButton").click();
    await until(() => content.textContent.includes("Delivery unconfirmed"), "lost response was not marked unconfirmed");
    setPage("sessions");
    await renderSessionDetail(config.session);
    check(content.textContent.includes("Lost response cue"), "committed GO missing from persisted history");
    check(content.textContent.includes(jump.result.payload.cue_execution_id), "Jump missing from persisted history");
    await fetch("/cue-delivery-result", { method: "POST", body: "PASS" });
  } catch (error) {
    await fetch("/cue-delivery-result", { method: "POST", body: error.stack || String(error) });
  }
})();
