"use strict";

Object.assign(f025Strings, {
  "f025.operations": {"en":"Runtime operations","ar-IQ":"عمليات التشغيل"},
  "f025.open_environment": {"en":"Open environment","ar-IQ":"فتح بيئة التشغيل"},
  "f025.capture_snapshot": {"en":"Capture snapshot","ar-IQ":"التقاط Snapshot"},
  "f025.operation_unbound": {"en":"Bind this environment to a Machine Role before runtime operations.","ar-IQ":"اربط بيئة التشغيل بدور جهاز قبل تنفيذ عمليات التشغيل."},
  "f025.operation_running": {"en":"Running execution-environment operation…","ar-IQ":"جارٍ تنفيذ عملية بيئة التشغيل…"},
  "f025.operation_completed": {"en":"Operation completed.","ar-IQ":"اكتملت العملية."},
  "f025.operation_unsupported": {"en":"This adapter does not support that operation.","ar-IQ":"هذا الـAdapter لا يدعم هذه العملية."},
  "f025.snapshot_partial": {"en":"PARTIAL snapshot — reconstruction guidance, not a complete VDMX backup.","ar-IQ":"Snapshot جزئي — إرشادات لإعادة البناء وليست نسخة كاملة من VDMX."},
  "f025.snapshot_result": {"en":"Latest captured snapshot metadata","ar-IQ":"آخر بيانات Snapshot ملتقطة"}
});

const f025OperationResults = new Map();

function f025OperationPath(environmentID) {
  return `${f025CollectionPath()}/${encodeURIComponent(environmentID)}/operations`;
}

function f025OperationResultMarkup(environmentID) {
  const result = f025OperationResults.get(environmentID);
  if (!result) return "";
  const completed = result.status === "COMPLETED";
  const unsupported = result.status === "UNSUPPORTED";
  const summary = result.response_summary
    || (completed ? f025T("f025.operation_completed") : unsupported ? f025T("f025.operation_unsupported") : result.error_code || result.status);
  const kind = completed ? "success" : unsupported ? "warn" : "error";
  const snapshot = result.snapshot ? `<details style="margin-top:10px" open>
    <summary>${esc(f025T("f025.snapshot_result"))}</summary>
    ${result.snapshot.capture_status === "PARTIAL" ? `<div class="message warn">${esc(f025T("f025.snapshot_partial"))}</div>` : ""}
    <pre class="mono muted">${esc(JSON.stringify(result.snapshot, null, 2))}</pre>
  </details>` : "";
  return `<div class="message ${kind}" style="margin-top:10px">${esc(summary)}${result.error_code ? ` · ${esc(result.error_code)}` : ""}</div>${snapshot}`;
}

function f025OperationMarkup(environment) {
  if (!canRuntime()) return "";
  const bound = Boolean(environment.machine_role_id);
  return `<div data-f025-operation-card data-environment-id="${esc(environment.execution_environment_id)}" style="margin-top:14px">
    <p class="eyebrow">${esc(f025T("f025.operations"))}</p>
    ${bound ? "" : `<div class="message warn">${esc(f025T("f025.operation_unbound"))}</div>`}
    <div class="toolbar">
      <button class="button primary f025-operation" data-kind="OPEN" type="button" ${bound ? "" : "disabled"}>${esc(f025T("f025.open_environment"))}</button>
      <button class="button f025-operation" data-kind="CAPTURE_SNAPSHOT" type="button" ${bound ? "" : "disabled"}>${esc(f025T("f025.capture_snapshot"))}</button>
    </div>
    ${f025OperationResultMarkup(environment.execution_environment_id)}
  </div>`;
}

const f025OperationEnvironmentCardBase = f025EnvironmentCard;
f025EnvironmentCard = function f025EnvironmentCardWithOperations(environment, roles, editable) {
  const base = f025OperationEnvironmentCardBase(environment, roles, editable);
  const operations = f025OperationMarkup(environment);
  if (!operations) return base;
  return base.replace("</article>", `${operations}</article>`);
};

async function f025RunOperation(environmentID, kind, button) {
  button.disabled = true;
  setMessage(globalMessage, f025T("f025.operation_running"));
  try {
    const result = await api(f025OperationPath(environmentID), {
      method: "POST",
      json: {
        operation_id: requestID(),
        kind,
        timeout_ms: 10000,
      },
    });
    f025OperationResults.set(environmentID, result);
    setMessage(globalMessage, "");
    await renderExecutionEnvironments();
  } catch (error) {
    setMessage(globalMessage, errorMessage(error), "error");
    button.disabled = false;
  }
}

function f025BindOperationControls() {
  content.querySelectorAll(".f025-operation").forEach((button) => {
    button.addEventListener("click", async () => {
      const card = button.closest("[data-f025-operation-card]");
      if (!card?.dataset.environmentId) return;
      await f025RunOperation(card.dataset.environmentId, button.dataset.kind, button);
    });
  });
}

const f025OperationRenderBase = renderExecutionEnvironments;
renderExecutionEnvironments = async function renderExecutionEnvironmentsWithOperations(message = "") {
  await f025OperationRenderBase(message);
  f025BindOperationControls();
};
