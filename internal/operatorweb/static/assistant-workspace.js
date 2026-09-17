(() => {
  "use strict";

  const f023Strings = {
    "assistant.nav": { en: "Assistant", "ar-IQ": "المساعد" },
    "assistant.eyebrow": { en: "STAGECORE ASSISTANT", "ar-IQ": "مساعد StageCore" },
    "assistant.title": { en: "Assistant", "ar-IQ": "المساعد" },
    "assistant.summary": { en: "Ask for evidence-grounded explanations, diagnostics, or editable Draft proposals. StageCore remains the authority for every change.", "ar-IQ": "اطلب شرحاً موثقاً بالأدلة أو تشخيصاً أو مقترحات قابلة للمراجعة داخل Draft. يبقى StageCore هو المرجع لكل تغيير." },
    "assistant.refresh": { en: "Refresh", "ar-IQ": "تحديث" },
    "assistant.available": { en: "Provider available", "ar-IQ": "المزوّد متاح" },
    "assistant.unavailable": { en: "Provider unavailable", "ar-IQ": "المزوّد غير متاح" },
    "assistant.offline_title": { en: "Assistant is optional", "ar-IQ": "المساعد اختياري" },
    "assistant.offline_detail": { en: "No Assistant provider is configured on this Hub. Normal StageCore setup, cues, runtime, preflight, notes, and show control continue to work normally.", "ar-IQ": "لا يوجد مزوّد للمساعد مهيأ على هذا الـHub. تبقى إعدادات StageCore والـCues والتشغيل والفحص والملاحظات والتحكم بالعرض تعمل بشكل طبيعي." },
    "assistant.normal_operator": { en: "Normal Operator remains available", "ar-IQ": "واجهة التشغيل الاعتيادية تبقى متاحة" },
    "assistant.task": { en: "Task", "ar-IQ": "المهمة" },
    "assistant.explain": { en: "Explain", "ar-IQ": "اشرح" },
    "assistant.diagnose": { en: "Diagnose", "ar-IQ": "شخّص" },
    "assistant.draft": { en: "Draft", "ar-IQ": "اقترح تعديلاً" },
    "assistant.explain_detail": { en: "Explain current StageCore evidence without changing the Project.", "ar-IQ": "اشرح أدلة StageCore الحالية بدون تغيير المشروع." },
    "assistant.diagnose_detail": { en: "Investigate readiness, execution, timing, or rehearsal/simulation evidence without issuing runtime commands.", "ar-IQ": "حلّل أدلة الجاهزية أو التنفيذ أو التوقيت أو البروفة والمحاكاة بدون إرسال أوامر تشغيل." },
    "assistant.draft_detail": { en: "Prepare structured proposed changes for explicit Preview and Apply.", "ar-IQ": "جهّز تغييرات مقترحة ومنظمة للمراجعة ثم التطبيق الصريح." },
    "assistant.scope": { en: "Evidence scope", "ar-IQ": "نطاق الأدلة" },
    "assistant.readiness": { en: "Readiness", "ar-IQ": "الجاهزية" },
    "assistant.execution": { en: "Execution", "ar-IQ": "التنفيذ" },
    "assistant.timing": { en: "Timing", "ar-IQ": "التوقيت" },
    "assistant.rehearsal": { en: "Rehearsal / Simulation", "ar-IQ": "البروفة / المحاكاة" },
    "assistant.rehearsal_hint": { en: "Uses the selected SIMULATION session with the F-024 Simulation Report, Flight Recorder, and advisory F-028 timing evidence. This analysis cannot issue live show commands.", "ar-IQ": "يستخدم جلسة SIMULATION المختارة مع تقرير F-024 للمحاكاة وFlight Recorder وأدلة التوقيت الاستشارية F-028. هذا التحليل لا يستطيع إرسال أوامر للعرض الحي." },
    "assistant.session": { en: "Session", "ar-IQ": "الجلسة" },
    "assistant.select_session": { en: "Select a Session", "ar-IQ": "اختر جلسة" },
    "assistant.select_simulation": { en: "Select a Simulation Session", "ar-IQ": "اختر جلسة محاكاة" },
    "assistant.all_sessions": { en: "Use available timing history", "ar-IQ": "استخدم سجل التوقيت المتاح" },
    "assistant.no_sessions": { en: "No rehearsal or show Sessions are available yet.", "ar-IQ": "لا توجد جلسات بروفة أو عرض متاحة بعد." },
    "assistant.no_simulations": { en: "No Simulation Sessions are available yet. Run or open a simulation first.", "ar-IQ": "لا توجد جلسات محاكاة متاحة حالياً. شغّل أو افتح محاكاة أولاً." },
    "assistant.prompt": { en: "What do you want StageCore to help with?", "ar-IQ": "بماذا تريد من StageCore أن يساعدك؟" },
    "assistant.prompt_placeholder": { en: "Example: Why did the last video cue fail?", "ar-IQ": "مثال: لماذا فشل آخر Cue للفيديو؟" },
    "assistant.run": { en: "Run Assistant task", "ar-IQ": "نفّذ مهمة المساعد" },
    "assistant.working": { en: "Reviewing canonical StageCore context…", "ar-IQ": "تتم مراجعة سياق StageCore الرسمي…" },
    "assistant.answer": { en: "Answer", "ar-IQ": "الإجابة" },
    "assistant.evidence": { en: "Evidence used", "ar-IQ": "الأدلة المستخدمة" },
    "assistant.assumptions": { en: "Assumptions", "ar-IQ": "الافتراضات" },
    "assistant.missing": { en: "Missing context", "ar-IQ": "السياق الناقص" },
    "assistant.proposed_changes": { en: "Proposed changes", "ar-IQ": "التغييرات المقترحة" },
    "assistant.no_evidence": { en: "No evidence references were returned.", "ar-IQ": "لم تُرجع مراجع أدلة." },
    "assistant.no_assumptions": { en: "No assumptions declared.", "ar-IQ": "لا توجد افتراضات معلنة." },
    "assistant.no_missing": { en: "No missing context declared.", "ar-IQ": "لا يوجد سياق ناقص معلن." },
    "assistant.kind": { en: "Kind", "ar-IQ": "النوع" },
    "assistant.target": { en: "Target", "ar-IQ": "الهدف" },
    "assistant.reference": { en: "Local reference", "ar-IQ": "مرجع محلي" },
    "assistant.details": { en: "Details", "ar-IQ": "التفاصيل" },
    "assistant.preview": { en: "Preview proposal", "ar-IQ": "راجع المقترح" },
    "assistant.apply": { en: "Apply to Draft", "ar-IQ": "طبّق على Draft" },
    "assistant.discard": { en: "Discard proposal", "ar-IQ": "تجاهل المقترح" },
    "assistant.previewed": { en: "Proposal preview is sealed by StageCore. Review it, then Apply or Discard.", "ar-IQ": "ثبّت StageCore نسخة المراجعة للمقترح. راجعها ثم طبّقها أو تجاهلها." },
    "assistant.applied": { en: "Proposal applied through StageCore Draft authority.", "ar-IQ": "تم تطبيق المقترح عبر صلاحية Draft الرسمية في StageCore." },
    "assistant.discarded": { en: "Proposal discarded. No Project change was made by this action.", "ar-IQ": "تم تجاهل المقترح. لم يغيّر هذا الإجراء المشروع." },
    "assistant.expiry": { en: "Preview expires", "ar-IQ": "تنتهي المراجعة" },
    "assistant.no_auto_apply": { en: "The Assistant never auto-applies changes. Preview and Apply are explicit operator actions.", "ar-IQ": "المساعد لا يطبق التغييرات تلقائياً. المراجعة والتطبيق إجراءان صريحان من المشغّل." },
    "assistant.current_revision": { en: "Current revision", "ar-IQ": "الإصدار الحالي" },
    "assistant.request_failed": { en: "Assistant request failed.", "ar-IQ": "فشل طلب المساعد." },
    "assistant.execution_needs_session": { en: "Execution evidence requires a Session.", "ar-IQ": "أدلة التنفيذ تتطلب اختيار جلسة." },
    "assistant.rehearsal_needs_simulation": { en: "Rehearsal analysis requires a selected Simulation Session.", "ar-IQ": "تحليل البروفة يتطلب اختيار جلسة محاكاة." },
    "assistant.empty_prompt": { en: "Enter a question or drafting instruction first.", "ar-IQ": "اكتب السؤال أو تعليمات الاقتراح أولاً." },
    "assistant.provider_unavailable": { en: "Assistant provider is unavailable. Normal StageCore operation is unaffected.", "ar-IQ": "مزوّد المساعد غير متاح. تشغيل StageCore الاعتيادي غير متأثر." },
    "assistant.evidence_unavailable": { en: "The requested canonical evidence is not available yet.", "ar-IQ": "الأدلة الرسمية المطلوبة غير متاحة حالياً." },
    "assistant.ungrounded": { en: "The provider returned evidence StageCore could not verify, so the answer was rejected.", "ar-IQ": "أرجع المزوّد أدلة لم يستطع StageCore التحقق منها، لذلك تم رفض الإجابة." },
    "assistant.stale": { en: "The Project changed after this proposal was prepared. Ask the Assistant to prepare a fresh Draft.", "ar-IQ": "تغيّر المشروع بعد إعداد المقترح. اطلب من المساعد إعداد Draft جديد." },
    "assistant.show_locked": { en: "SHOW is active. Structural configuration changes are locked.", "ar-IQ": "وضع SHOW نشط. التغييرات الهيكلية على الإعداد مقفلة." },
    "assistant.forbidden": { en: "Your role cannot apply one or more proposed changes.", "ar-IQ": "صلاحيتك لا تسمح بتطبيق تغيير واحد أو أكثر من المقترح." },
  };

  const assistantModel = {
    status: null,
    sessions: [],
    response: null,
    sealedProposal: null,
    taskKind: "EXPLAIN",
    scope: "READINESS",
    selectedSessionID: "",
    message: "",
    messageKind: "",
  };

  function assistantLocale() {
    return document.documentElement.lang?.toLowerCase().startsWith("ar") ? "ar-IQ" : "en";
  }

  function assistantText(key) {
    const value = f023Strings[key];
    return value?.[assistantLocale()] || value?.en || key;
  }

  function assistantProjectID() {
    return state.project?.project_id || state.project?.id || "";
  }

  function assistantTaskAvailable(kind) {
    return !!assistantModel.status?.tasks?.[kind];
  }

  function assistantFriendlyError(error) {
    const code = error?.payload?.error_code || "";
    const mapped = {
      ASSISTANT_PROVIDER_UNAVAILABLE: "assistant.provider_unavailable",
      ASSISTANT_EVIDENCE_UNAVAILABLE: "assistant.evidence_unavailable",
      ASSISTANT_RESPONSE_UNGROUNDED: "assistant.ungrounded",
      ASSISTANT_PROPOSAL_STALE: "assistant.stale",
      SHOW_CONFIGURATION_LOCKED: "assistant.show_locked",
      FORBIDDEN: "assistant.forbidden",
    }[code];
    return mapped ? assistantText(mapped) : errorMessage(error) || assistantText("assistant.request_failed");
  }

  async function assistantLoadModel() {
    const projectID = assistantProjectID();
    if (!projectID) return;
    assistantModel.status = await api(`/api/v1/projects/${encodeURIComponent(projectID)}/assistant/status`);
    try {
      const payload = await api(`/api/v1/projects/${encodeURIComponent(projectID)}/sessions?limit=100`);
      assistantModel.sessions = payload.sessions || [];
    } catch (_) {
      assistantModel.sessions = [];
    }
  }

  function assistantScopeSessions() {
    if (assistantModel.scope !== "REHEARSAL") return assistantModel.sessions;
    return assistantModel.sessions.filter((session) => String(session.session_type || session.type || "").toUpperCase() === "SIMULATION");
  }

  function assistantSessionOptions() {
    const scopedSessions = assistantScopeSessions();
    const emptyLabel = assistantModel.scope === "TIMING"
      ? assistantText("assistant.all_sessions")
      : assistantModel.scope === "REHEARSAL"
        ? assistantText("assistant.select_simulation")
        : assistantText("assistant.select_session");
    return `<option value="">${esc(emptyLabel)}</option>` + scopedSessions.map((session) => {
      const sessionType = session.session_type || session.type || "SESSION";
      const label = `${sessionType} · ${session.name || fmtDate(session.started_at)}`;
      return `<option value="${esc(session.session_id)}" ${assistantModel.selectedSessionID === session.session_id ? "selected" : ""}>${esc(label)}</option>`;
    }).join("");
  }

  function assistantSelectedSession() {
    return assistantModel.sessions.find((session) => session.session_id === assistantModel.selectedSessionID) || null;
  }

  function assistantTaskDetail(kind) {
    if (kind === "DIAGNOSE") return assistantText("assistant.diagnose_detail");
    if (kind === "DRAFT") return assistantText("assistant.draft_detail");
    return assistantText("assistant.explain_detail");
  }

  function assistantList(titleKey, values, emptyKey) {
    const list = Array.isArray(values) ? values.filter((item) => String(item || "").trim()) : [];
    return `<section class="card assistant-result-card">
      <h2>${esc(assistantText(titleKey))}</h2>
      ${list.length ? `<ul class="assistant-text-list">${list.map((item) => `<li>${esc(item)}</li>`).join("")}</ul>` : `<div class="empty">${esc(assistantText(emptyKey))}</div>`}
    </section>`;
  }

  function assistantEvidence(response) {
    const evidence = Array.isArray(response?.evidence) ? response.evidence : [];
    return `<section class="card assistant-result-card">
      <h2>${esc(assistantText("assistant.evidence"))}</h2>
      ${evidence.length ? `<div class="assistant-evidence-grid">${evidence.map((item) => `
        <article class="assistant-evidence-item">
          <span class="pill neutral">${esc(item.kind || "EVIDENCE")}</span>
          <span class="mono">${esc(item.id || "—")}</span>
        </article>`).join("")}</div>` : `<div class="empty">${esc(assistantText("assistant.no_evidence"))}</div>`}
    </section>`;
  }

  function assistantHumanKey(value) {
    return String(value || "").replaceAll("_", " ").replace(/\b\w/g, (char) => char.toUpperCase());
  }

  function assistantStructuredValue(value, depth = 0) {
    if (depth > 4) return `<span class="muted">…</span>`;
    if (value === null || value === undefined || value === "") return `<span class="muted">—</span>`;
    if (Array.isArray(value)) {
      if (!value.length) return `<span class="muted">—</span>`;
      return `<ol class="assistant-structured-list">${value.map((item) => `<li>${assistantStructuredValue(item, depth + 1)}</li>`).join("")}</ol>`;
    }
    if (typeof value === "object") {
      const entries = Object.entries(value);
      if (!entries.length) return `<span class="muted">—</span>`;
      return `<dl class="assistant-structured">${entries.map(([key, item]) => `<div><dt>${esc(assistantHumanKey(key))}</dt><dd>${assistantStructuredValue(item, depth + 1)}</dd></div>`).join("")}</dl>`;
    }
    if (typeof value === "boolean") return pill(value ? "TRUE" : "FALSE", value ? "good" : "neutral");
    return `<span>${esc(value)}</span>`;
  }

  function assistantProposalOperation(operation, index) {
    return `<article class="assistant-operation-card">
      <div class="assistant-operation-head">
        <div><p class="eyebrow">${esc(assistantText("assistant.proposed_changes"))} ${index + 1}</p><h3>${esc(operation.summary || operation.kind || "Change")}</h3></div>
        ${pill(operation.kind || "CHANGE", "neutral")}
      </div>
      <div class="assistant-operation-meta">
        ${operation.target_id ? `<span><strong>${esc(assistantText("assistant.target"))}:</strong> <span class="mono">${esc(operation.target_id)}</span></span>` : ""}
        ${operation.ref ? `<span><strong>${esc(assistantText("assistant.reference"))}:</strong> <span class="mono">${esc(operation.ref)}</span></span>` : ""}
      </div>
      <div class="assistant-operation-details"><strong>${esc(assistantText("assistant.details"))}</strong>${assistantStructuredValue(operation.payload || {})}</div>
    </article>`;
  }

  function assistantProposal(response) {
    const proposal = assistantModel.sealedProposal || response?.proposal;
    if (!proposal) return "";
    const operations = Array.isArray(proposal.operations) ? proposal.operations : [];
    const sealed = !!assistantModel.sealedProposal;
    return `<section class="card assistant-proposal-card">
      <div class="section-title-row">
        <div><p class="eyebrow">DRAFT PROPOSAL</p><h2>${esc(assistantText("assistant.proposed_changes"))}</h2><p class="muted">${esc(assistantText("assistant.no_auto_apply"))}</p></div>
        ${pill(`${assistantText("assistant.current_revision")} ${proposal.base_revision_id || "—"}`, "warn")}
      </div>
      ${sealed ? `<div class="message success">${esc(assistantText("assistant.previewed"))}${proposal.expires_at ? ` · ${esc(assistantText("assistant.expiry"))} ${esc(fmtDate(proposal.expires_at))}` : ""}</div>` : ""}
      <div class="assistant-operation-list">${operations.map(assistantProposalOperation).join("")}</div>
      <div class="toolbar assistant-proposal-actions">
        ${sealed ? `<button id="assistantApplyProposal" class="button primary" type="button">${esc(assistantText("assistant.apply"))}</button>` : `<button id="assistantPreviewProposal" class="button primary" type="button">${esc(assistantText("assistant.preview"))}</button>`}
        <button id="assistantDiscardProposal" class="button ghost" type="button">${esc(assistantText("assistant.discard"))}</button>
      </div>
    </section>`;
  }

  function assistantResponse() {
    const response = assistantModel.response;
    if (!response) return "";
    return `<div class="assistant-results">
      <section class="card assistant-answer-card">
        <p class="eyebrow">${esc(assistantText("assistant.answer"))}</p>
        <p class="assistant-answer">${esc(response.answer || "—")}</p>
      </section>
      <div class="assistant-context-grid">
        ${assistantEvidence(response)}
        ${assistantList("assistant.assumptions", response.assumptions, "assistant.no_assumptions")}
        ${assistantList("assistant.missing", response.missing_context, "assistant.no_missing")}
      </div>
      ${assistantProposal(response)}
    </div>`;
  }

  function assistantProviderBanner() {
    if (assistantModel.status?.provider_state === "AVAILABLE") {
      return `<div class="message success assistant-provider-state">${esc(assistantText("assistant.available"))}</div>`;
    }
    return `<div class="message warn assistant-provider-state"><strong>${esc(assistantText("assistant.offline_title"))}</strong> · ${esc(assistantText("assistant.offline_detail"))}</div>`;
  }

  function assistantTaskOptions() {
    return [
      ["EXPLAIN", "assistant.explain"],
      ["DIAGNOSE", "assistant.diagnose"],
      ["DRAFT", "assistant.draft"],
    ].map(([kind, key]) => `<option value="${kind}" ${assistantModel.taskKind === kind ? "selected" : ""} ${assistantTaskAvailable(kind) ? "" : "disabled"}>${esc(assistantText(key))}</option>`).join("");
  }

  function assistantScopeControls() {
    if (assistantModel.taskKind === "DRAFT") return "";
    const needsSession = ["EXECUTION", "TIMING", "REHEARSAL"].includes(assistantModel.scope);
    const scopedSessions = assistantScopeSessions();
    const emptySessionKey = assistantModel.scope === "REHEARSAL" ? "assistant.no_simulations" : "assistant.no_sessions";
    return `<div class="form-grid two assistant-scope-grid">
      <label>${esc(assistantText("assistant.scope"))}
        <select id="assistantScope">
          <option value="READINESS" ${assistantModel.scope === "READINESS" ? "selected" : ""}>${esc(assistantText("assistant.readiness"))}</option>
          <option value="EXECUTION" ${assistantModel.scope === "EXECUTION" ? "selected" : ""}>${esc(assistantText("assistant.execution"))}</option>
          <option value="TIMING" ${assistantModel.scope === "TIMING" ? "selected" : ""}>${esc(assistantText("assistant.timing"))}</option>
          <option value="REHEARSAL" ${assistantModel.scope === "REHEARSAL" ? "selected" : ""}>${esc(assistantText("assistant.rehearsal"))}</option>
        </select>
      </label>
      ${needsSession ? `<label>${esc(assistantText("assistant.session"))}<select id="assistantSession">${assistantSessionOptions()}</select></label>` : ""}
    </div>
    ${assistantModel.scope === "REHEARSAL" ? `<p class="muted">${esc(assistantText("assistant.rehearsal_hint"))}</p>` : ""}
    ${needsSession && !scopedSessions.length ? `<p class="muted">${esc(assistantText(emptySessionKey))}</p>` : ""}`;
  }

  function assistantEnsureAvailableTask() {
    if (assistantTaskAvailable(assistantModel.taskKind)) return;
    for (const kind of ["EXPLAIN", "DIAGNOSE", "DRAFT"]) {
      if (assistantTaskAvailable(kind)) {
        assistantModel.taskKind = kind;
        return;
      }
    }
  }

  async function renderAssistantWorkspace(refresh = true) {
    if (!state.project) return;
    setPage("assistant");
    if (refresh || !assistantModel.status) {
      await assistantLoadModel();
      assistantEnsureAvailableTask();
    }
    const enabled = assistantTaskAvailable(assistantModel.taskKind);
    content.innerHTML = `
      <div class="page-head assistant-head">
        <div><p class="eyebrow">${esc(assistantText("assistant.eyebrow"))}</p><h1>${esc(assistantText("assistant.title"))}</h1><p>${esc(assistantText("assistant.summary"))}</p></div>
        <div class="toolbar">${pill(assistantModel.status?.provider_state || "UNAVAILABLE", assistantModel.status?.provider_state === "AVAILABLE" ? "good" : "warn")}<button id="assistantRefresh" class="button ghost" type="button">${esc(assistantText("assistant.refresh"))}</button></div>
      </div>
      ${assistantProviderBanner()}
      ${assistantModel.message ? `<div id="assistantMessage" class="message ${esc(assistantModel.messageKind)}">${esc(assistantModel.message)}</div>` : `<div id="assistantMessage" class="message hidden"></div>`}
      <section class="card assistant-task-card">
        <div class="section-title-row"><div><p class="eyebrow">${esc(assistantText("assistant.task"))}</p><h2>${esc(assistantText(`assistant.${assistantModel.taskKind.toLowerCase()}`))}</h2><p class="muted">${esc(assistantTaskDetail(assistantModel.taskKind))}</p></div>${pill(assistantText("assistant.normal_operator"), "neutral")}</div>
        <form id="assistantTaskForm" class="assistant-task-form">
          <div class="form-grid two">
            <label>${esc(assistantText("assistant.task"))}<select id="assistantTaskKind">${assistantTaskOptions()}</select></label>
            <div class="assistant-task-authority"><span class="label">Authority</span><strong>${assistantModel.taskKind === "DRAFT" ? "DRAFT_PROPOSAL" : "READ_ONLY"}</strong></div>
          </div>
          ${assistantScopeControls()}
          <label>${esc(assistantText("assistant.prompt"))}<textarea id="assistantPrompt" rows="5" maxlength="16384" placeholder="${esc(assistantText("assistant.prompt_placeholder"))}" ${enabled ? "" : "disabled"}></textarea></label>
          <div class="toolbar"><button id="assistantRun" class="button primary" type="submit" ${enabled ? "" : "disabled"}>${esc(assistantText("assistant.run"))}</button></div>
        </form>
      </section>
      ${assistantResponse()}`;
    bindAssistantWorkspace();
  }

  function bindAssistantWorkspace() {
    document.getElementById("assistantRefresh")?.addEventListener("click", () => {
      assistantModel.status = null;
      assistantModel.message = "";
      renderAssistantWorkspace(true).catch(assistantShowError);
    });
    document.getElementById("assistantTaskKind")?.addEventListener("change", (event) => {
      assistantModel.taskKind = event.target.value;
      if (assistantModel.taskKind === "DIAGNOSE" && assistantModel.scope === "READINESS" && assistantModel.sessions.length) assistantModel.scope = "EXECUTION";
      assistantModel.response = null;
      assistantModel.sealedProposal = null;
      assistantModel.message = "";
      renderAssistantWorkspace(false).catch(assistantShowError);
    });
    document.getElementById("assistantScope")?.addEventListener("change", (event) => {
      assistantModel.scope = event.target.value;
      assistantModel.selectedSessionID = "";
      renderAssistantWorkspace(false).catch(assistantShowError);
    });
    document.getElementById("assistantSession")?.addEventListener("change", (event) => {
      assistantModel.selectedSessionID = event.target.value;
    });
    document.getElementById("assistantTaskForm")?.addEventListener("submit", submitAssistantTask);
    document.getElementById("assistantPreviewProposal")?.addEventListener("click", previewAssistantProposal);
    document.getElementById("assistantApplyProposal")?.addEventListener("click", applyAssistantProposal);
    document.getElementById("assistantDiscardProposal")?.addEventListener("click", discardAssistantProposal);
  }

  async function submitAssistantTask(event) {
    event.preventDefault();
    const prompt = document.getElementById("assistantPrompt")?.value.trim() || "";
    if (!prompt) {
      assistantModel.message = assistantText("assistant.empty_prompt");
      assistantModel.messageKind = "warn";
      await renderAssistantWorkspace(false);
      return;
    }
    if (assistantModel.taskKind !== "DRAFT" && assistantModel.scope === "EXECUTION" && !assistantModel.selectedSessionID) {
      assistantModel.message = assistantText("assistant.execution_needs_session");
      assistantModel.messageKind = "warn";
      await renderAssistantWorkspace(false);
      return;
    }
    if (assistantModel.taskKind !== "DRAFT" && assistantModel.scope === "REHEARSAL" && !assistantModel.selectedSessionID) {
      assistantModel.message = assistantText("assistant.rehearsal_needs_simulation");
      assistantModel.messageKind = "warn";
      await renderAssistantWorkspace(false);
      return;
    }
    const session = assistantSelectedSession();
    const selectedSessionType = String(session?.session_type || session?.type || "").toUpperCase();
    if (assistantModel.taskKind !== "DRAFT" && assistantModel.scope === "REHEARSAL" && selectedSessionType !== "SIMULATION") {
      assistantModel.message = assistantText("assistant.rehearsal_needs_simulation");
      assistantModel.messageKind = "warn";
      await renderAssistantWorkspace(false);
      return;
    }
    assistantModel.message = assistantText("assistant.working");
    assistantModel.messageKind = "";
    assistantModel.response = null;
    assistantModel.sealedProposal = null;
    await renderAssistantWorkspace(false);
    try {
      const payload = await api(`/api/v1/projects/${encodeURIComponent(assistantProjectID())}/assistant/tasks`, {
        method: "POST",
        json: {
          request_id: requestID(),
          kind: assistantModel.taskKind,
          scope: assistantModel.taskKind === "DRAFT" ? undefined : assistantModel.scope,
          prompt,
          session_id: session?.session_id || undefined,
          runtime_snapshot_id: session?.runtime_snapshot_id || undefined,
        },
      });
      assistantModel.response = payload.response || null;
      assistantModel.message = "";
      assistantModel.messageKind = "";
      await renderAssistantWorkspace(false);
    } catch (error) {
      assistantModel.message = assistantFriendlyError(error);
      assistantModel.messageKind = "error";
      await renderAssistantWorkspace(false);
    }
  }

  async function previewAssistantProposal() {
    const proposal = assistantModel.response?.proposal;
    if (!proposal) return;
    try {
      const payload = await api(`/api/v1/projects/${encodeURIComponent(assistantProjectID())}/assistant/proposals/preview`, {
        method: "POST",
        json: { proposal },
      });
      assistantModel.sealedProposal = payload.proposal;
      assistantModel.message = assistantText("assistant.previewed");
      assistantModel.messageKind = "success";
      await renderAssistantWorkspace(false);
    } catch (error) {
      assistantModel.message = assistantFriendlyError(error);
      assistantModel.messageKind = "error";
      await renderAssistantWorkspace(false);
    }
  }

  async function applyAssistantProposal() {
    const proposal = assistantModel.sealedProposal;
    if (!proposal) return;
    const operations = Array.isArray(proposal.operations) ? proposal.operations : [];
    const path = operations.length > 1 ? "apply-batch" : "apply";
    try {
      await api(`/api/v1/projects/${encodeURIComponent(assistantProjectID())}/assistant/proposals/${path}`, {
        method: "POST",
        json: { proposal, apply: true },
      });
      assistantModel.response = assistantModel.response ? { ...assistantModel.response, proposal: null } : null;
      assistantModel.sealedProposal = null;
      assistantModel.message = assistantText("assistant.applied");
      assistantModel.messageKind = "success";
      await renderAssistantWorkspace(false);
    } catch (error) {
      assistantModel.message = assistantFriendlyError(error);
      assistantModel.messageKind = "error";
      await renderAssistantWorkspace(false);
    }
  }

  async function discardAssistantProposal() {
    assistantModel.response = assistantModel.response ? { ...assistantModel.response, proposal: null } : null;
    assistantModel.sealedProposal = null;
    assistantModel.message = assistantText("assistant.discarded");
    assistantModel.messageKind = "success";
    await renderAssistantWorkspace(false);
  }

  function assistantShowError(error) {
    setMessage(globalMessage, assistantFriendlyError(error), "error");
  }

  function assistantIntegrateWorkspaceProfiles() {
    if (typeof F017_PAGES === "undefined" || typeof F017_PRESETS === "undefined" || typeof f017Strings === "undefined") return;
    if (!F017_PAGES.includes("assistant")) F017_PAGES.push("assistant");
    f017Strings["workspace.page.assistant"] = f023Strings["assistant.nav"];
    for (const [presetID, preset] of Object.entries(F017_PRESETS)) {
      if (!preset.page_order.includes("assistant")) {
        const dashboardIndex = preset.page_order.indexOf("dashboard");
        const insertion = dashboardIndex >= 0 ? dashboardIndex + 1 : preset.page_order.length;
        preset.page_order.splice(insertion, 0, "assistant");
      }
      if (["stage-manager", "rehearsal"].includes(presetID) && !preset.visible_pages.includes("assistant")) {
        const dashboardIndex = preset.visible_pages.indexOf("dashboard");
        const insertion = dashboardIndex >= 0 ? dashboardIndex + 1 : preset.visible_pages.length;
        preset.visible_pages.splice(insertion, 0, "assistant");
      }
    }
    if (typeof f017ApplyProfile === "function") f017ApplyProfile({ navigateIfNeeded: false });
  }

  function installAssistantNavigation() {
    const nav = document.getElementById("workspaceNav");
    if (!nav || nav.querySelector('[data-assistant-nav="true"]')) return;
    const button = document.createElement("button");
    button.type = "button";
    button.className = "nav-button";
    button.dataset.page = "assistant";
    button.dataset.assistantNav = "true";
    button.textContent = assistantText("assistant.nav");
    button.addEventListener("click", () => renderAssistantWorkspace(true).catch(assistantShowError));
    const before = nav.querySelector('[data-page="configuration"]') || nav.querySelector('[data-page="cues"]');
    nav.insertBefore(button, before);
    assistantIntegrateWorkspaceProfiles();
  }

  window.renderAssistantWorkspace = renderAssistantWorkspace;

  document.addEventListener("DOMContentLoaded", () => {
    installAssistantNavigation();
    document.getElementById("languageSelect")?.addEventListener("change", () => {
      document.querySelector('[data-assistant-nav="true"]')?.remove();
      installAssistantNavigation();
      if (state.page === "assistant") renderAssistantWorkspace(false).catch(() => {});
    });
  });
})();
