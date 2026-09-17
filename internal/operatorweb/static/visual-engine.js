(() => {
  "use strict";

  const copy = {
    en: {
      nav: "Visual Engine",
      eyebrow: "VISUAL ENGINE",
      title: "Visual Engine",
      summary: "Choose who owns visual playback for this Project and review the canonical visual configuration already stored in StageCore.",
      refresh: "Refresh",
      mode: "Engine mode",
      modeSummary: "NATIVE keeps playback inside StageCore. EXTERNAL keeps StageCore as the cue and routing authority while an external visual engine performs playback.",
      native: "Native",
      nativeSummary: "Use the StageCore native Visual Engine renderer and Companion path.",
      external: "External",
      externalSummary: "Use an external visual engine while keeping StageCore Project, Cue and routing truth canonical.",
      current: "Current",
      choose: "Use this mode",
      readOnly: "Read only",
      readOnlySummary: "Your role can inspect the Visual Engine workspace but cannot change Project configuration.",
      showLocked: "SHOW is active. Visual Engine configuration is locked until the SHOW session ends.",
      saved: "Visual Engine mode updated.",
      revision: "Revision",
      configured: "Native configuration",
      configuredYes: "Configured",
      configuredNo: "Not configured",
      roles: "Visual machine roles",
      rolesSummary: "Machine Roles that advertise Visual Engine or live-source capabilities.",
      sources: "Live sources",
      sourcesSummary: "Canonical F-007 LiveSources available to this Project.",
      outputs: "Visual outputs",
      outputsSummary: "Outputs whose capability belongs to the Visual Engine contract.",
      actions: "Visual Cue actions",
      actionsSummary: "Cue Actions that target Visual Engine capabilities.",
      emptyRoles: "No visual Machine Roles configured yet.",
      emptySources: "No LiveSources configured yet.",
      emptyOutputs: "No visual Outputs configured yet.",
      emptyActions: "No visual Cue Actions configured yet.",
      openConfiguration: "Open Configuration",
      openCues: "Open Cues",
      canonicalHint: "Create devices, targets, outputs and routing in Configuration. Author playback actions in Cues. This page does not create a second visual configuration model.",
      capability: "Capability",
      required: "Required",
      optional: "Optional",
      enabled: "Enabled",
      disabled: "Disabled",
      cue: "Cue",
      target: "Target",
      sourceClass: "Class",
      executionRole: "Execution role",
    },
    ar: {
      nav: "المحرك المرئي",
      eyebrow: "المحرك المرئي",
      title: "المحرك المرئي",
      summary: "اختَر الجهة المسؤولة عن تشغيل الصورة في هذا المشروع وراجع إعدادات العرض المرئي الرسمية المخزنة داخل StageCore.",
      refresh: "تحديث",
      mode: "وضع المحرك",
      modeSummary: "وضع NATIVE يبقي تشغيل الصورة داخل StageCore، أما EXTERNAL فيبقي StageCore مسؤولاً عن الـCues والتوجيه ويترك التشغيل لمحرك مرئي خارجي.",
      native: "داخلي NATIVE",
      nativeSummary: "استخدم محرك StageCore المرئي ومسار الـCompanion للتشغيل.",
      external: "خارجي EXTERNAL",
      externalSummary: "استخدم محركاً مرئياً خارجياً مع بقاء المشروع والـCues والتوجيه تحت سلطة StageCore.",
      current: "الحالي",
      choose: "استخدم هذا الوضع",
      readOnly: "للقراءة فقط",
      readOnlySummary: "صلاحيتك تسمح بمراجعة مساحة المحرك المرئي لكنها لا تسمح بتغيير إعدادات المشروع.",
      showLocked: "العرض SHOW نشط حالياً، لذلك إعدادات المحرك المرئي مقفلة إلى أن تنتهي جلسة العرض.",
      saved: "تم تحديث وضع المحرك المرئي.",
      revision: "الإصدار",
      configured: "إعداد NATIVE",
      configuredYes: "مهيأ",
      configuredNo: "غير مهيأ",
      roles: "أدوار الأجهزة المرئية",
      rolesSummary: "Machine Roles التي تعلن قدرات المحرك المرئي أو المصادر الحية.",
      sources: "المصادر الحية",
      sourcesSummary: "مصادر F-007 الرسمية المتاحة لهذا المشروع.",
      outputs: "المخارج المرئية",
      outputsSummary: "المخارج التي تنتمي قدراتها إلى عقد المحرك المرئي.",
      actions: "أوامر الـCue المرئية",
      actionsSummary: "Cue Actions التي تستهدف قدرات المحرك المرئي.",
      emptyRoles: "لا توجد Machine Roles مرئية مهيأة بعد.",
      emptySources: "لا توجد LiveSources مهيأة بعد.",
      emptyOutputs: "لا توجد مخارج مرئية مهيأة بعد.",
      emptyActions: "لا توجد Cue Actions مرئية مهيأة بعد.",
      openConfiguration: "فتح الإعداد",
      openCues: "فتح الـCues",
      canonicalHint: "أنشئ الأجهزة والأهداف والمخارج والتوجيه من Configuration، وأنشئ أوامر التشغيل من Cues. هذه الصفحة لا تنشئ نموذج إعداد مرئي موازياً.",
      capability: "القدرة",
      required: "مطلوب",
      optional: "اختياري",
      enabled: "مفعّل",
      disabled: "معطّل",
      cue: "Cue",
      target: "الهدف",
      sourceClass: "النوع",
      executionRole: "دور التنفيذ",
    },
  };

  let model = null;

  function lang() {
    return document.documentElement.lang?.toLowerCase().startsWith("ar") ? "ar" : "en";
  }

  function t(key) {
    return copy[lang()][key] || copy.en[key] || key;
  }

  function projectID() {
    return state.project?.project_id || state.project?.id || "";
  }

  function shortID(value) {
    const text = String(value || "");
    if (!text) return "—";
    return text.length > 18 ? `${text.slice(0, 8)}…${text.slice(-7)}` : text;
  }

  function booleanPill(value) {
    return pill(value ? t("enabled") : t("disabled"), value ? "good" : "neutral");
  }

  function roleCard(role) {
    const capabilities = role.required_capabilities || role.capabilities || [];
    return `
      <article class="visual-inventory-item">
        <div class="visual-inventory-head"><strong>${esc(role.display_name || role.role_key || role.machine_role_id || role.id || "—")}</strong>${pill(role.required ? t("required") : t("optional"), role.required ? "warn" : "neutral")}</div>
        <p class="muted mono">${esc(role.role_key || role.machine_role_id || role.id || "—")}</p>
        ${capabilities.length ? `<div class="visual-chip-row">${capabilities.map((capability) => `<span class="pill neutral">${esc(capability)}</span>`).join("")}</div>` : ""}
      </article>`;
  }

  function sourceCard(source) {
    return `
      <article class="visual-inventory-item">
        <div class="visual-inventory-head"><strong>${esc(source.name || source.live_source_id || source.id || "—")}</strong>${booleanPill(source.desired_enabled !== false)}</div>
        <dl class="visual-kv">
          <div><dt>${esc(t("sourceClass"))}</dt><dd>${esc(source.class || source.source_class || "—")}</dd></div>
          <div><dt>${esc(t("executionRole"))}</dt><dd class="mono">${esc(shortID(source.execution_machine_role_id || source.machine_role_id))}</dd></div>
        </dl>
      </article>`;
  }

  function outputCard(output) {
    return `
      <article class="visual-inventory-item">
        <strong>${esc(output.name || output.output_id || "—")}</strong>
        <dl class="visual-kv">
          <div><dt>${esc(t("target"))}</dt><dd class="mono">${esc(output.target_ref || "—")}</dd></div>
          <div><dt>${esc(t("capability"))}</dt><dd class="mono">${esc(output.capability_key || "—")}</dd></div>
        </dl>
      </article>`;
  }

  function actionCard(item) {
    const action = item.action || {};
    return `
      <article class="visual-inventory-item">
        <div class="visual-inventory-head"><strong>${esc(item.cue_name || item.cue_id || "—")}</strong>${booleanPill(action.enabled !== false)}</div>
        <dl class="visual-kv">
          <div><dt>${esc(t("cue"))}</dt><dd class="mono">${esc(shortID(item.cue_id))}</dd></div>
          <div><dt>${esc(t("capability"))}</dt><dd class="mono">${esc(action.capability_key || "—")}</dd></div>
          <div><dt>${esc(t("target"))}</dt><dd class="mono">${esc(action.target_ref || "—")}</dd></div>
        </dl>
      </article>`;
  }

  function inventorySection(title, summary, items, renderer, emptyText) {
    return `
      <section class="card visual-inventory-section">
        <div><h2>${esc(title)}</h2><p class="muted">${esc(summary)}</p></div>
        <div class="visual-inventory-list">${items.length ? items.map(renderer).join("") : `<div class="empty">${esc(emptyText)}</div>`}</div>
      </section>`;
  }

  function modeCard(mode, title, summary) {
    const current = model?.engine_mode === mode;
    const editable = canEdit();
    return `
      <article class="visual-mode-card ${current ? "selected" : ""}" data-mode="${mode}">
        <div class="visual-mode-head">
          <div><p class="eyebrow">${mode}</p><h2>${esc(title)}</h2></div>
          ${current ? pill(t("current"), "good") : ""}
        </div>
        <p class="muted">${esc(summary)}</p>
        ${editable ? `<button class="button ${current ? "ghost" : "primary"} visual-mode-select" type="button" data-mode="${mode}" ${current ? "disabled" : ""}>${esc(current ? t("current") : t("choose"))}</button>` : ""}
      </article>`;
  }

  async function renderVisualEngine(message = "", kind = "") {
    if (!state.project) return;
    setPage("visual-engine");
    model = await api(`/api/v1/projects/${encodeURIComponent(projectID())}/visual-engine`);
    const editable = canEdit();
    const roles = model.machine_roles || [];
    const sources = model.live_sources || [];
    const outputs = model.outputs || [];
    const actions = model.actions || [];

    content.innerHTML = `
      <div class="page-head visual-engine-head">
        <div><p class="eyebrow">${esc(t("eyebrow"))}</p><h1>${esc(t("title"))}</h1><p>${esc(t("summary"))}</p></div>
        <div class="toolbar">${pill(model.engine_mode || "EXTERNAL", model.engine_mode === "NATIVE" ? "good" : "neutral")}<button id="visualEngineRefresh" class="button ghost" type="button">${esc(t("refresh"))}</button></div>
      </div>
      <div id="visualEngineMessage" class="message ${message ? kind : "hidden"}">${message ? esc(message) : ""}</div>
      ${editable ? "" : `<div class="message warn"><strong>${esc(t("readOnly"))}</strong> · ${esc(t("readOnlySummary"))}</div>`}

      <section class="card visual-mode-section">
        <div class="section-title-row"><div><p class="eyebrow">${esc(t("mode"))}</p><h2>${esc(t("mode"))}</h2><p class="muted">${esc(t("modeSummary"))}</p></div><div class="visual-meta">${pill(`${t("revision")} ${shortID(model.revision_id)}`, model.revision_status === "DRAFT" ? "warn" : "good")}${pill(`${t("configured")}: ${model.native_configured ? t("configuredYes") : t("configuredNo")}`, model.native_configured ? "good" : "neutral")}</div></div>
        <div class="visual-mode-grid">
          ${modeCard("NATIVE", t("native"), t("nativeSummary"))}
          ${modeCard("EXTERNAL", t("external"), t("externalSummary"))}
        </div>
      </section>

      <section class="card visual-authority-handoff">
        <p>${esc(t("canonicalHint"))}</p>
        <div class="toolbar"><button id="visualOpenConfiguration" class="button" type="button">${esc(t("openConfiguration"))}</button><button id="visualOpenCues" class="button" type="button">${esc(t("openCues"))}</button></div>
      </section>

      <div class="visual-inventory-grid">
        ${inventorySection(t("roles"), t("rolesSummary"), roles, roleCard, t("emptyRoles"))}
        ${inventorySection(t("sources"), t("sourcesSummary"), sources, sourceCard, t("emptySources"))}
        ${inventorySection(t("outputs"), t("outputsSummary"), outputs, outputCard, t("emptyOutputs"))}
        ${inventorySection(t("actions"), t("actionsSummary"), actions, actionCard, t("emptyActions"))}
      </div>`;

    bindVisualEngine();
  }

  function visualMessage(message, kind = "") {
    const target = document.getElementById("visualEngineMessage");
    if (target) setMessage(target, message, kind);
  }

  function bindVisualEngine() {
    document.getElementById("visualEngineRefresh")?.addEventListener("click", () => renderVisualEngine().catch(visualError));
    document.getElementById("visualOpenConfiguration")?.addEventListener("click", () => navigate("configuration"));
    document.getElementById("visualOpenCues")?.addEventListener("click", () => navigate("cues"));
    document.querySelectorAll(".visual-mode-select").forEach((button) => button.addEventListener("click", () => selectMode(button.dataset.mode)));
  }

  async function selectMode(mode) {
    if (!canEdit() || !["NATIVE", "EXTERNAL"].includes(mode)) return;
    try {
      await api(`/api/v1/projects/${encodeURIComponent(projectID())}/visual-engine/mode`, {
        method: "PUT",
        json: { engine_mode: mode },
      });
      const projectPayload = await api(`/api/v1/projects/${encodeURIComponent(projectID())}`);
      state.project = projectPayload.project;
      updateWorkspaceProject();
      await renderVisualEngine(t("saved"), "success");
    } catch (error) {
      if (error.status === 423 && error.payload?.error_code === "SHOW_CONFIGURATION_LOCKED") {
        visualMessage(t("showLocked"), "warn");
        return;
      }
      visualError(error);
    }
  }

  function visualError(error) {
    visualMessage(errorMessage(error), "error");
  }

  function installNavigation() {
    const nav = document.getElementById("workspaceNav");
    if (!nav || nav.querySelector('[data-visual-engine-nav="true"]')) return;
    const button = document.createElement("button");
    button.type = "button";
    button.className = "nav-button";
    button.dataset.page = "visual-engine";
    button.dataset.visualEngineNav = "true";
    button.textContent = t("nav");
    button.addEventListener("click", () => renderVisualEngine().catch((error) => setMessage(globalMessage, errorMessage(error), "error")));
    const before = nav.querySelector('[data-page="environments"]') || nav.querySelector('[data-page="cues"]');
    nav.insertBefore(button, before);
  }

  window.renderVisualEngine = renderVisualEngine;

  document.addEventListener("DOMContentLoaded", () => {
    installNavigation();
    document.getElementById("languageSelect")?.addEventListener("change", () => {
      document.querySelector('[data-visual-engine-nav="true"]')?.remove();
      installNavigation();
      if (state.page === "visual-engine") renderVisualEngine().catch(() => {});
    });
  });
})();
