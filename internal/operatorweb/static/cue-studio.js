"use strict";

/*
 * SC-008 Cue Studio, progressive enhancement on top of the existing Cue API.
 * This file does not change runtime execution, snapshots or the Cue schema.
 */
(function () {
  const tr = (ar, en) => document.documentElement.lang.startsWith("ar") ? ar : en;
  const sc008Strings = {
    "cue.studio.choose": {"en": "Choose an action", "ar-IQ": "اختر نوع الإجراء"},
    "cue.studio.optional": {"en": "Timing, links and notes", "ar-IQ": "التوقيت، الربط والملاحظات"},
    "cue.studio.close_warning": {"en": "Discard unsaved Cue changes?", "ar-IQ": "تجاهل تغييرات الكيو غير المحفوظة؟"},
  };
  const phrase = (key) => sc008Strings[key][document.documentElement.lang.startsWith("ar") ? "ar-IQ" : "en"];
  const pick = (id) => document.getElementById(id);
  const actionCards = () => Array.from(pick("actionsEditor")?.querySelectorAll(".action-editor") || []);
  const cardField = (card, selector) => card.querySelector(selector);
  let initialFormState = "";
  let actionClipboard = null;

  function safeObject(raw) {
    try { return JSON.parse(raw || "{}"); } catch (_) { return {}; }
  }

  function actionKind(card) {
    const cap = cardField(card, ".action-capability")?.value || "";
    if (cap === "osc.send") return "osc";
    if (cap === "midi.send") return "midi";
    if (cap.startsWith("lighting.")) return "lighting";
    if (cap.startsWith("tablet.")) return "tablet";
    if (cap.startsWith("laser.")) return "laser";
    if (cap.startsWith("visual.")) return "visual";
    return "advanced";
  }

  function summarize(card) {
    const kind = actionKind(card);
    const target = cardField(card, ".action-target")?.value.trim() || tr("بلا هدف", "No target");
    const params = safeObject(cardField(card, ".action-parameters")?.value);
    let detail = "";
    if (kind === "osc") {
      const addr = cardField(card, ".f002-osc-address")?.value || params.address || "";
      detail = addr;
    } else if (kind === "midi") {
      const message = cardField(card, ".f002-midi-message")?.value || "MIDI";
      const ch = cardField(card, ".f002-midi-channel")?.value || "";
      detail = message + (ch ? " · Ch " + ch : "");
    } else if (kind === "lighting") {
      detail = Object.keys(params.levels || {}).join(", ") || params.command_type ||
        cardField(card, ".action-capability")?.value || "";
    } else if (kind === "tablet" || kind === "laser" || kind === "visual") {
      detail = params.command_type || params.action ||
        cardField(card, ".action-capability")?.value || "";
    }
    const labels = {
      osc: tr("VDMX / OSC", "VDMX / OSC"),
      midi: tr("Ableton / MIDI", "Ableton / MIDI"),
      lighting: tr("إضاءة / DMX", "Lighting / DMX"),
      tablet: tr("تابلت / فيديو", "Tablet / Video"),
      laser: "StageLaser",
      visual: tr("محرك مرئي", "Native Visual"),
      advanced: tr("إجراء متقدم", "Advanced action"),
    };
    return { kind, title: labels[kind], summary: [target, detail].filter(Boolean).join(" · ") };
  }

  function updateCard(card) {
    if (!card?.classList.contains("sc008-action-card")) return;
    const data = summarize(card);
    const title = card.querySelector(".sc008-action-name");
    const summary = card.querySelector(".sc008-action-summary");
    const badge = card.querySelector(".sc008-action-type");
    if (title) title.textContent = data.title;
    if (summary) summary.textContent = data.summary;
    if (badge) badge.textContent = data.kind.toUpperCase();
    card.dataset.actionKind = data.kind;
  }

  function refreshActions() {
    const cards = actionCards();
    cards.forEach((card, index) => {
      const number = card.querySelector(".action-title");
      if (number) number.textContent = String(index + 1).padStart(2, "0");
      card.querySelector(".sc008-action-select")?.setAttribute("aria-label",
        tr(`تحديد الإجراء ${index + 1} للنسخ`, `Select action ${index + 1} to copy`));
      updateCard(card);
      const up = card.querySelector(".sc008-up");
      const down = card.querySelector(".sc008-down");
      if (up) up.disabled = index === 0;
      if (down) down.disabled = index === cards.length - 1;
    });
    const total = pick("sc008ActionCount");
    if (total) total.textContent = String(cards.length);
    const empty = pick("sc008Empty");
    if (empty) empty.hidden = cards.length > 0;
    refreshClipboardButtons();
  }

  function expandCard(card, shouldOpen) {
    const body = card.querySelector(".sc008-action-body");
    const button = card.querySelector(".sc008-expand");
    if (!body || !button) return;
    body.hidden = !shouldOpen;
    button.setAttribute("aria-expanded", String(shouldOpen));
    button.textContent = shouldOpen ? tr("طي", "Collapse") : tr("تعديل", "Edit");
  }

  function moveCard(card, delta) {
    const parent = card.parentNode;
    const cards = actionCards();
    const old = cards.indexOf(card);
    const next = old + delta;
    if (!parent || old < 0 || next < 0 || next >= cards.length) return;
    if (delta < 0) parent.insertBefore(card, cards[next]);
    else parent.insertBefore(cards[next], card);
    refreshActions();
    card.querySelector(".sc008-expand")?.focus();
  }

  function serializeActionCopy(card, raw = null) {
    // Translate visual OSC/MIDI fields into the existing canonical payload.
    f002SyncActionCard(card);
    const read = (selector) => cardField(card, selector)?.value;
    const parse = (selector) => {
      const value = read(selector) || "{}";
      try { return JSON.parse(value); }
      catch (error) {
        if (!raw) throw error;
        raw[selector] = value;
        return {};
      }
    };
    return {
      action_id: "",
      target_ref: read(".action-target") || "",
      capability_key: read(".action-capability") || "",
      parameters: parse(".action-parameters"),
      timeout_policy: parse(".action-timeout"),
      error_policy: parse(".action-error"),
      execution_mode: read(".action-mode") || "SEQUENTIAL",
      priority_class: read(".action-priority") || "P1",
      enabled: cardField(card, ".action-enabled")?.checked ?? true,
    };
  }

  function clipboardAction() {
    return clipboardActions()[0] || null;
  }

  function clipboardActions() {
    if (actionClipboard && actionClipboard.user === state.user && actionClipboard.projectID === state.project?.project_id) {
      return actionClipboard.actions || (actionClipboard.action ? [actionClipboard.action] : []);
    }
    actionClipboard = null;
    return [];
  }

  function refreshClipboardButtons() {
    const selectedCount = actionCards().filter((card) => cardField(card, ".sc008-action-select")?.checked).length;
    const copy = pick("sc008CopySelectedActions");
    if (copy) copy.disabled = !state.user || !state.project?.project_id || selectedCount === 0;
    const actions = clipboardActions();
    const paste = pick("sc008PasteAction");
    if (paste) {
      paste.disabled = actions.length === 0;
      paste.textContent = actions.length > 1
        ? tr(`لصق ${actions.length} إجراءات`, `Paste ${actions.length} actions`)
        : tr("لصق إجراء", "Paste action");
    }
  }

  function warnIfActionTargetUnavailable(action) {
    const targets = state.f002Targets;
    if (!Array.isArray(targets)) return;
    const targetRef = String(action?.target_ref || "").trim();
    if (targetRef && targets.some((target) => String(target.logical_name || "").trim() === targetRef)) return;
    setMessage(globalMessage,
      tr("الهدف غير متاح في إعدادات هذا المشروع. أضف الهدف أو أعد تعيينه قبل الاعتماد على هذا الكيو.", "Target is not available in this Project configuration. Add or remap the target before relying on this Cue."),
      "warn");
  }

  function copyActionToClipboard(card) {
    if (!state.user || !state.project?.project_id) return;
    try {
      const action = serializeActionCopy(card);
      actionClipboard = { user: state.user, projectID: state.project.project_id, action, actions: [action] };
      refreshClipboardButtons();
    } catch (_) {
      window.alert(tr("راجع قيم هذا الإجراء قبل نسخه.", "Review this action's fields before copying."));
      expandCard(card, true);
    }
  }

  function copySelectedActionsToClipboard() {
    if (!state.user || !state.project?.project_id) return;
    const selected = actionCards().filter((card) => cardField(card, ".sc008-action-select")?.checked);
    if (!selected.length) return;
    const actions = [];
    let invalid;
    try {
      for (const card of selected) {
        try { actions.push(serializeActionCopy(card)); }
        catch (error) { invalid = card; throw error; }
      }
      actionClipboard = { user: state.user, projectID: state.project.project_id, actions };
      refreshClipboardButtons();
    } catch (_) {
      window.alert(tr("راجع قيم الإجراءات المحددة قبل نسخها.", "Review the selected actions before copying."));
      if (invalid) expandCard(invalid, true);
    }
  }

  function pasteClipboardAction() {
    const actions = clipboardActions();
    if (!actions.length) return;
    for (const action of actions) {
      warnIfActionTargetUnavailable(action);
      addActionEditor(structuredClone(action));
    }
    const inserted = actionCards().slice(-actions.length);
    actionCards().forEach((item) => expandCard(item, item === inserted.at(-1)));
    refreshActions();
    inserted.at(-1)?.querySelector(".sc008-expand")?.focus();
  }

  function duplicateCard(card) {
    try {
      const copy = serializeActionCopy(card);
      addActionEditor(copy);
      const inserted = actionCards().at(-1);
      card.parentNode.insertBefore(inserted, card.nextSibling);
      actionCards().forEach((item) => expandCard(item, item === inserted));
      refreshActions();
    } catch (_) {
      window.alert(tr("راجع قيم هذا الإجراء قبل نسخه.", "Review this action's fields before duplicating."));
      expandCard(card, true);
    }
  }

  function uiSignature(card) {
    const builder = card.querySelector(".f002-action-builder");
    if (!builder) return "";
    return Array.from(builder.querySelectorAll("input, select, textarea"))
      .map((node) => [node.className, node.value, node.type === "checkbox" ? node.checked : null]);
  }

  const previousSync = f002SyncActionCard;
  f002SyncActionCard = function sc008SafeSync(card) {
    // A no-op edit must not rewrite recognized OSC/MIDI payloads and discard
    // extension fields that the visual builder does not understand.
    if (card.dataset.sc008Existing === "true" &&
        card.dataset.sc008OriginalVisual === JSON.stringify(uiSignature(card))) return;
    previousSync(card);
  };

  function enhanceTargetPicker(card) {
    if (!["osc", "midi"].includes(actionKind(card))) return;
    const input = cardField(card, ".action-target");
    const label = input?.closest("label");
    const targets = state.f002Targets || [];
    if (!input || !label || !targets.length || label.querySelector(".sc008-target-picker")) return;

    // Keep the existing canonical input and legacy/freeform targets intact.
    // A friendly picker only mirrors its value; it never changes cue schema.
    if (label.firstChild?.nodeType === Node.TEXT_NODE) {
      label.firstChild.textContent = tr("الجهاز / الهدف", "Device / target");
    }
    const select = document.createElement("select");
    select.className = "sc008-target-picker";
    select.required = true;
    select.setAttribute("aria-label", tr("اختر الهدف", "Choose target"));
    const placeholder = document.createElement("option");
    placeholder.value = "";
    placeholder.textContent = tr("اختر من الأهداف المعرفة…", "Choose a configured target…");
    select.append(placeholder);
    for (const target of targets) {
      const name = String(target.logical_name || "").trim();
      if (!name) continue;
      const option = document.createElement("option");
      option.value = name;
      option.textContent = name;
      select.append(option);
    }
    const manual = document.createElement("option");
    manual.value = "__manual__";
    manual.textContent = tr("هدف آخر (متقدم)…", "Other target (advanced)…");
    select.append(manual);
    const current = input.value.trim();
    select.value = Array.from(select.options).some((option) => option.value === current)
      ? current : current ? "__manual__" : "";

    const apply = () => {
      const other = select.value === "__manual__";
      input.type = other ? "text" : "hidden";
      input.required = other;
      if (!other) input.value = select.value;
      input.dispatchEvent(new Event("change", { bubbles: true }));
    };
    select.addEventListener("change", apply);
    input.after(select);
    if (current) {
      if (select.value !== "__manual__") apply();
    } else apply();
  }

  function decorateAction(card, existing) {
    if (!card || card.classList.contains("sc008-action-card")) return;
    card.classList.add("sc008-action-card");
    card.dataset.sc008Existing = String(!!existing);
    card.dataset.sc008OriginalVisual = JSON.stringify(uiSignature(card));
    const header = card.querySelector(".action-head");
    if (!header) return;
    const originalDelete = header.querySelector(".remove-action");
    const name = document.createElement("span");
    name.className = "sc008-action-name";
    const summary = document.createElement("small");
    summary.className = "sc008-action-summary";
    const type = document.createElement("span");
    type.className = "sc008-action-type";
    const titleGroup = document.createElement("div");
    titleGroup.className = "sc008-action-identity";
    titleGroup.append(name, summary, type);
    header.querySelector(".action-title")?.after(titleGroup);

    const tools = document.createElement("div");
    tools.className = "sc008-action-tools";
    const selectionLabel = document.createElement("label");
    selectionLabel.className = "sc008-select-action";
    const selection = document.createElement("input");
    selection.type = "checkbox";
    selection.className = "sc008-action-select";
    selection.setAttribute("aria-label", tr("تحديد الإجراء للنسخ", "Select action to copy"));
    selection.addEventListener("change", refreshClipboardButtons);
    const selectionText = document.createElement("span");
    selectionText.textContent = tr("تحديد", "Select");
    selectionLabel.append(selection, selectionText);
    tools.append(selectionLabel);
    const make = (cls, label, fn) => {
      const button = document.createElement("button");
      button.type = "button";
      button.className = "button ghost " + cls;
      button.textContent = label;
      button.setAttribute("aria-label", label);
      button.addEventListener("click", fn);
      tools.appendChild(button);
      return button;
    };
    make("sc008-up", "↑", () => moveCard(card, -1));
    make("sc008-down", "↓", () => moveCard(card, 1));
    make("sc008-copy", tr("تكرار", "Duplicate"), () => duplicateCard(card));
    make("sc008-copy-clipboard", tr("نسخ للكيوهات", "Copy to other Cues"), () => copyActionToClipboard(card));
    make("sc008-expand", tr("تعديل", "Edit"), () => expandCard(card, card.querySelector(".sc008-action-body")?.hidden));
    if (originalDelete) {
      originalDelete.textContent = "×";
      originalDelete.title = tr("حذف الإجراء", "Remove action");
      tools.append(originalDelete);
      originalDelete.addEventListener("click", refreshActions);
    }
    header.append(tools);
    const body = document.createElement("div");
    body.className = "sc008-action-body";
    Array.from(card.children).forEach((child) => {
      if (child !== header && !child.matches("input.action-id")) body.appendChild(child);
    });
    card.append(body);
    enhanceTargetPicker(card);
    card.addEventListener("input", () => updateCard(card));
    card.addEventListener("change", () => updateCard(card));
    expandCard(card, !existing);
    updateCard(card);
  }

  function finishAddonComposer(previousCount) {
    const cards = actionCards();
    if (cards.length <= previousCount) return;
    const composer = pick("f002CueComposer");
    if (composer) composer.hidden = true;
    const latest = cards.at(-1);
    cards.forEach((card) => expandCard(card, card === latest));
    refreshActions();
    latest?.scrollIntoView({ block: "nearest" });
  }

  // Reuse the existing canonical Add-on action builders and keep their
  // validation/error messages. Collapse the setup composer ONLY on a
  // successful creation, so error messages stay visible to the operator.
  for (const functionName of [
    "f002AddTabletAction",
    "f002AddLightingAction",
    "f002AddStageLaserAction",
  ]) {
    const original = ({
      f002AddTabletAction,
      f002AddLightingAction,
      f002AddStageLaserAction,
    })[functionName];
    const wrapped = async (...args) => {
      const oldCount = actionCards().length;
      await original(...args);
      finishAddonComposer(oldCount);
    };
    if (functionName === "f002AddTabletAction") f002AddTabletAction = wrapped;
    if (functionName === "f002AddLightingAction") f002AddLightingAction = wrapped;
    if (functionName === "f002AddStageLaserAction") f002AddStageLaserAction = wrapped;
  }

  const previousAdd = addActionEditor;
  addActionEditor = function sc008AddAction(action = null) {
    previousAdd(action);
    const card = actionCards().at(-1);
    decorateAction(card, !!action?.action_id);
    refreshActions();
  };

  function showCreator(kind) {
    const composer = pick("f002CueComposer");
    if (!composer) return false;
    const sectionKey = {
      tablet: "stagecore.tablet-controller",
      lighting: "stagecore.lighting-controller",
      laser: "stagecore.stagelaser-controller",
    }[kind];
    let selected = null;
    composer.querySelectorAll("[data-f002-extension], [data-f002-import]").forEach((node) => {
      const match = sectionKey
        ? node.dataset.f002Extension === sectionKey
        : kind === "import" && node.hasAttribute("data-f002-import");
      node.hidden = !match;
      if (match) selected = node;
    });
    composer.hidden = !selected;
    selected?.scrollIntoView({ block: "nearest", behavior: "smooth" });
    return !!selected;
  }

  function createCoreAction(kind) {
    const type = kind === "midi" ? "midi.send"
      : kind === "visual" ? "visual.play"
      : kind === "advanced" ? "local.echo" : "osc.send";
    const eligible = kind === "visual" && typeof f037TargetsForCapability === "function"
      ? f037TargetsForCapability("visual.play").map(({ target }) => target)
      : (state.f002Targets || []).filter((target) => {
        const name = String(target.logical_name || "").toLowerCase();
        return kind === "midi" ? /audio|ableton|midi/.test(name) : /osc|vdmx|video/.test(name);
      });
    const targetName = eligible.length === 1 ? eligible[0].logical_name : "";
    const payload = kind === "osc" ? { address: "", arguments: [] }
      : kind === "visual" ? { contract_version: 1, layer_id: "" } : {};
    addActionEditor({ capability_key: type, target_ref: targetName, parameters: payload });
    const added = actionCards().at(-1);
    // A newly created MIDI action has no destination yet, so it cannot pass
    // the existing parseMIDIParameters round-trip parser on construction.
    // Open its typed MIDI editor explicitly; the operator must choose a
    // destination before the submit serializer builds canonical MIDI bytes.
    if (kind === "midi") {
      const select = added?.querySelector(".f002-action-kind");
      if (select) {
        select.value = "midi";
        select.dispatchEvent(new Event("change", { bubbles: true }));
      }
    }
    actionCards().forEach((card) => expandCard(card, card === added));
    const picker = pick("sc008Picker");
    picker?.classList.remove("is-open");
    pick("f002CueComposer")?.setAttribute("hidden", "");
    added?.scrollIntoView({ block: "nearest", behavior: "smooth" });
    const focus = added?.querySelector(".action-target");
    focus?.focus();
  }

  function pickerCards() {
    const addons = state.f002InstalledAddons || new Set();
    const list = [
      { key: "osc", icon: "◉", name: "VDMX / OSC", desc: tr("مسار OSC وقيمه، مع اكتشاف VDMX", "OSC path, typed values, VDMX discovery") },
      { key: "midi", icon: "♫", name: "Ableton / MIDI", desc: tr("وجهة MIDI والنوتة والقناة", "MIDI destination, notes, channel") },
    ];
    if (addons.has("stagecore.lighting-controller")) list.push({
      key: "lighting", icon: "☼", name: tr("إضاءة / DMX", "Lighting / DMX"),
      desc: tr("مستوى الإضاءة والفيد", "Channels, levels and fade"),
    });
    if (addons.has("stagecore.tablet-controller")) list.push({
      key: "tablet", icon: "▣", name: tr("تابلت / فيديو", "Tablet / Video"),
      desc: tr("ميديا، لايف، اتجاه وسُطوع", "Media, live, orientation and brightness"),
    });
    if (addons.has("stagecore.stagelaser-controller")) list.push({
      key: "laser", icon: "◇", name: "StageLaser",
      desc: tr("تشغيل آمن وإطفاء وفلاش", "Safe on/off and flash"),
    });
    // Native Visual is built into the Hub, not an Add-on. Only advertise
    // the focused editor when the Project has a visual-capable Machine Role.
    const visualConfigured = typeof f037TargetsForCapability === "function" &&
      f037TargetsForCapability("visual.play").length > 0;
    if (visualConfigured) list.push({
      key: "visual", icon: "◩", name: tr("محرك الفيديو الداخلي", "Native Visual Engine"),
      desc: tr("تشغيل الطبقات والانتقالات والمؤثرات", "Layers, transitions and effects"),
    });
    if (pick("f002CueComposer")?.querySelector("[data-f002-import]")) list.push({
      key: "import", icon: "⊞", name: tr("تركيب من كيو موجود", "Compose from Cue"),
      desc: tr("نسخ الإجراءات بدون تغيير الأصل", "Copy actions without altering source"),
    });
    list.push({ key: "advanced", icon: "⚙", name: tr("إجراء متقدم", "Advanced action"),
      desc: tr("قدرات أخرى مع الحفاظ على JSON", "Other capabilities, preserve raw JSON") });
    return list;
  }

  function ensurePicker(workspace) {
    let picker = pick("sc008Picker");
    if (!picker) {
      picker = document.createElement("section");
      picker.id = "sc008Picker";
      picker.className = "sc008-picker";
      workspace.appendChild(picker);
    }
    picker.classList.toggle("is-open", actionCards().length === 0);
    const heading = document.createElement("div");
    heading.className = "sc008-picker-head";
    const title = document.createElement("h3");
    title.textContent = phrase("cue.studio.choose");
    const description = document.createElement("p");
    description.textContent = tr("تظهر لك إعدادات النوع المختار فقط. تقدر تضيف أكثر من نوع بنفس الكيو.",
      "See only the selected editor. Mix action types in the same Cue.");
    heading.append(title, description);
    const choices = document.createElement("div");
    choices.className = "sc008-picker-grid";
    picker.replaceChildren(heading, choices);
    for (const option of pickerCards()) {
      const button = document.createElement("button");
      button.type = "button";
      button.className = "sc008-choice";
      button.dataset.sc008Kind = option.key;
      const icon = document.createElement("span");
      icon.className = "sc008-choice-icon";
      icon.setAttribute("aria-hidden", "true");
      icon.textContent = option.icon;
      const label = document.createElement("strong");
      label.textContent = option.name;
      const detail = document.createElement("small");
      detail.textContent = option.desc;
      button.append(icon, label, detail);
      button.addEventListener("click", () => {
        if (["osc", "midi", "visual", "advanced"].includes(option.key)) createCoreAction(option.key);
        else {
          picker.classList.remove("is-open");
          showCreator(option.key);
        }
      });
      choices.appendChild(button);
    }
  }

  function createOptionalDetails(form) {
    let details = pick("sc008Optional");
    if (details) return details;
    details = document.createElement("details");
    details.id = "sc008Optional";
    details.className = "sc008-optional";
    const summary = document.createElement("summary");
    summary.textContent = phrase("cue.studio.optional");
    details.append(summary);
    const critical = pick("cueCriticality")?.closest("label");
    const enabled = pick("cueEnabled")?.closest("label");
    const settings = document.createElement("div");
    settings.className = "form-grid two sc008-optional-flags";
    if (critical) settings.append(critical);
    if (enabled) settings.append(enabled);
    details.append(settings);
    // Keep original inputs and linked-cue checkbox listeners unchanged.
    const delay = pick("cueStartDelaySeconds")?.closest("section");
    if (delay) {
      const help = delay.querySelector("p");
      if (help) {
        const explainer = document.createElement("details");
        const explainSummary = document.createElement("summary");
        explainSummary.textContent = tr("شرح التوقيت", "Timing details");
        explainer.append(explainSummary, help);
        delay.append(explainer);
      }
      details.append(delay);
    }
    const linked = form.querySelector(".cue-link-editor");
    if (linked) details.append(linked);
    const notes = pick("cueNotes")?.closest("label");
    if (notes) details.append(notes);
    // Move the *outer* Advanced execution policy disclosure; F-002 may
    // have wrapped its label in a nested details element. Moving only that
    // inner element leaves an empty duplicate panel in the Cue form.
    const policyField = pick("cueExecutionPolicy");
    const policy = policyField?.closest("details.card") ||
      policyField?.closest("details");
    if (policy) details.append(policy);
    return details;
  }

  // Dialog-local history only. Save continues through the existing Draft API.
  function createDraftHistory(limit) {
    let entries = [];
    let position = -1;
    return {
      reset(value) {
        entries = [value];
        position = 0;
      },
      record(value) {
        if (value === null || value === entries[position]) return;
        entries = entries.slice(0, position + 1);
        entries.push(value);
        if (entries.length > limit) entries.shift();
        position = entries.length - 1;
      },
      current() { return entries[position]; },
      canMove(delta) { return position + delta >= 0 && position + delta < entries.length; },
      move(delta) {
        if (!this.canMove(delta)) return undefined;
        position += delta;
        return entries[position];
      },
    };
  }

  const draftHistory = createDraftHistory(30);
  const historyFields = ["cueLabel", "cueName", "cueColor", "cueCriticality", "cueEnabled",
    "cueExecutionPolicy", "cueStartDelaySeconds", "cueNotes"];
  const timecodeFields = ["f018CueTimecodeMode", "f018CueBindingID", "f018CueAt", "f018CueExpiry"];
  let historyTimer;
  let restoringDraft = false;

  function readDraftHistory(includeIncomplete = false) {
    try {
      const fields = historyFields.map((id) => {
        const field = pick(id);
        return field.type === "checkbox" ? field.checked : field.value;
      });
      const links = Array.from(pick("linkedCuesEditor").querySelectorAll(".linked-cue-checkbox:checked"), (node) => node.value);
      const actions = actionCards().map((card) => {
        // Guided synchronization writes canonical JSON. Run it on a clone
        // so recording history never changes the visible edit or raw text.
        const clone = card.cloneNode(true);
        const controls = card.querySelectorAll("input,select,textarea");
        clone.querySelectorAll("input,select,textarea").forEach((node, index) => {
          node.value = controls[index].value;
          if (node.type === "checkbox") node.checked = controls[index].checked;
        });
        const raw = includeIncomplete ? {} : null;
        const action = serializeActionCopy(clone, raw);
        if (raw && Object.keys(raw).length) action.recovery_raw = raw;
        action.action_id = cardField(card, ".action-id").value;
        return action;
      });
      const timecode = timecodeFields.map((id) => pick(id)?.value);
      return JSON.stringify({fields, links, actions, timecode});
    } catch (_) {
      return null; // keep invalid in-progress JSON editable
    }
  }

  function refreshHistoryButtons(current = draftHistory.current()) {
    const undo = pick("sc009UndoDraft");
    const redo = pick("sc009RedoDraft");
    if (undo) undo.disabled = current !== null && !draftHistory.canMove(-1);
    if (redo) redo.disabled = current === null || !draftHistory.canMove(1);
  }

  function recordDraftHistory() {
    if (restoringDraft || !pick("cueDialog")?.open) return;
    const current = readDraftHistory();
    draftHistory.record(current);
    refreshHistoryButtons(current);
  }

  function restoreDraftHistory(delta) {
    clearTimeout(historyTimer);
    const current = readDraftHistory();
    draftHistory.record(current);
    // Invalid text can be discarded explicitly by Undo to recover the
    // nearest valid checkpoint. Redo never records invalid JSON.
    const stored = current === null && delta < 0 ? draftHistory.current() : draftHistory.move(delta);
    if (typeof stored !== "string") return;
    applyEditorDraft(JSON.parse(stored));
    refreshHistoryButtons();
  }

  function applyEditorDraft(draft) {
    restoringDraft = true;
    try {
      historyFields.forEach((id, index) => {
        const field = pick(id);
        if (field.type === "checkbox") field.checked = draft.fields[index];
        else field.value = draft.fields[index];
      });
      pick("linkedCuesEditor").querySelectorAll(".linked-cue-checkbox").forEach((node) => {
        node.checked = draft.links.includes(node.value);
      });
      updateLinkedCuesEditorState();
      if (Array.isArray(draft.timecode)) {
        timecodeFields.forEach((id, index) => {
          const field = pick(id);
          if (field && typeof draft.timecode[index] === "string") field.value = draft.timecode[index];
        });
        if (typeof f018SyncCueBindingEnabled === "function") f018SyncCueBindingEnabled();
      }
      pick("actionsEditor").replaceChildren();
      draft.actions.forEach((action) => {
        addActionEditor(action);
        const card = actionCards().at(-1);
        for (const [field, mode] of [
          [".action-parameters", ".f002-action-kind"],
          [".action-timeout", ".f002-timeout-kind"],
          [".action-error", ".f002-error-kind"],
        ]) {
          if (typeof action.recovery_raw?.[field] !== "string") continue;
          const select = cardField(card, mode);
          if (select) {
            select.value = "advanced";
            select.dispatchEvent(new Event("change", { bubbles: true }));
          }
          cardField(card, field).value = action.recovery_raw[field];
        }
      });
      refreshActions();
    } finally {
      restoringDraft = false;
    }
  }

  function bindDraftHistory(form, anchor) {
    if (!anchor) return;
    for (const [id, delta, label] of [
      ["sc009UndoDraft", -1, tr("تراجع", "Undo")],
      ["sc009RedoDraft", 1, tr("إعادة", "Redo")],
    ]) {
      const button = pick(id) || document.createElement("button");
      button.id = id;
      button.type = "button";
      button.className = "button ghost";
      button.textContent = label;
      button.onclick = () => restoreDraftHistory(delta);
      if (!button.isConnected) anchor.after(button);
    }
    if (!form.dataset.sc009History) {
      form.dataset.sc009History = "true";
      const schedule = () => {
        clearTimeout(historyTimer);
        historyTimer = setTimeout(recordDraftHistory, 300);
      };
      form.addEventListener("input", schedule);
      form.addEventListener("change", schedule);
      new MutationObserver(schedule).observe(pick("actionsEditor"), {childList:true, subtree:true});
      pick("cueDialog")?.addEventListener("close", (event) => {
        if (event.currentTarget.open) return;
        clearTimeout(historyTimer);
        draftHistory.reset(null);
      });
    }
    clearTimeout(historyTimer);
    draftHistory.reset(readDraftHistory());
    refreshHistoryButtons();
  }

  function bindEditorRecovery(form, cue, workspace) {
    const dialog = pick("cueDialog");
    const projectID = state.project?.project_id;
    const user = state.user;
    const base = readDraftHistory(true);
    if (!dialog || !projectID || !user || !base) return;
    const cueID = cue?.cue_id || "";
    const path = `/api/v1/projects/${encodeURIComponent(projectID)}/cue-editor-recovery`;
    const panel = document.createElement("div");
    panel.id = "sc009EditorRecovery";
    const status = document.createElement("p");
    status.className = "muted";
    status.setAttribute("role", "status");
    const restore = document.createElement("button");
    const discard = document.createElement("button");
    for (const button of [restore, discard]) {
      button.type = "button";
      button.className = "button ghost";
      button.hidden = true;
    }
    restore.textContent = tr("استرجع المسودة", "Recover draft");
    discard.textContent = tr("احذف النسخة المحفوظة", "Discard recovery");
    panel.append(status, restore, discard);
    pick(panel.id)?.remove();
    workspace.prepend(panel);

    const localStatus = document.createElement("p");
    localStatus.className = "muted";
    localStatus.setAttribute("role", "status");
    const localRestore = restore.cloneNode();
    localRestore.id = "sc009LocalRestore";
    const localDiscard = discard.cloneNode();
    localDiscard.id = "sc009LocalDiscard";
    localDiscard.textContent = tr("احذف المسودة المحلية", "Discard local draft");
    const localReview = document.createElement("textarea");
    localReview.id = "sc009LocalReview";
    localReview.hidden = true;
    localReview.readOnly = true;
    localReview.setAttribute("aria-label", tr("تعديلات المسودة المحلية", "Local draft edits"));
    const localChoices = document.createElement("select");
    localChoices.setAttribute("aria-label", tr("المسودات المحلية", "Local recovery drafts"));
    panel.append(localStatus, localChoices, localRestore, localDiscard, localReview);

    const scope = user.user_id && JSON.stringify([user.user_id, projectID, cueID]);
    const prefix = scope && "stagecore.cue-recovery.v2:" + scope + ":";
    const legacyKey = scope && "stagecore.cue-recovery.v1:" + scope;
    const localMessage = (ar, en) => { localStatus.textContent = tr(ar, en); };
    const { timecode, ...previousBaseline } = JSON.parse(base);
    const compatible = (saved) => saved?.version === 1 && saved.cue_id === cueID &&
      (saved.base === base || saved.base === JSON.stringify(previousBaseline)) &&
      saved.draft?.fields?.length === historyFields.length &&
      Array.isArray(saved.draft.links) && Array.isArray(saved.draft.actions);
    const candidates = [];
    let localPending = null;
    let ownedKey = "";
    let ownedDraft = base;
    const recoveryDocument = (current) => ({ version: 1, cue_id: cueID, base, draft: JSON.parse(current) });
    const addCandidate = (storage, key) => {
      const saved = JSON.parse(storage.getItem(key) || "null");
      if (saved?.version === 1 && saved.cue_id === cueID && saved.draft) candidates.push({ storage, key, saved });
    };
    try {
      if (prefix) {
        for (let i = 0; i < window.localStorage.length; i++) {
          const key = window.localStorage.key(i);
          if (key?.startsWith(prefix)) {
            try { addCandidate(window.localStorage, key); } catch (_) { /* A damaged draft does not hide valid copies. */ }
          }
        }
        addCandidate(window.sessionStorage, legacyKey);
      }
    } catch (_) {
      localMessage("تخزين المسودات في المتصفح غير متاح.", "Browser draft storage is unavailable.");
    }
    function writeLocal(current) {
      if (!prefix) return false;
      if (current === ownedDraft) return true;
      try {
        // Each revision has its own key. A different tab can remove only the
        // revision it reviewed, even if the original editor has since edited it.
        const key = current === base ? "" : prefix + window.crypto.getRandomValues(new Uint32Array(4)).join("-");
        if (key) window.localStorage.setItem(key, JSON.stringify(recoveryDocument(current)));
        const previous = ownedKey;
        ownedKey = key;
        ownedDraft = current;
        if (previous) window.localStorage.removeItem(previous);
        return true;
      } catch (_) {
        localMessage("تعذّر تخزين المسودة في المتصفح.", "The browser could not store this draft.");
        return false;
      }
    }
    const showLocalCandidate = () => {
      localChoices.hidden = candidates.length < 2;
      localReview.hidden = true;
      localRestore.hidden = localDiscard.hidden = !localPending;
      if (!localPending) return;
      const matches = compatible(localPending.saved);
      localRestore.textContent = matches ? tr("استرجع المسودة المحلية", "Recover local draft") :
        tr("راجع المسودة المحلية", "Review local draft");
      localMessage(matches ? "توجد مسودة محفوظة في المتصفح." : "تغيّر الكيو منذ حفظ المسودة. راجع التعديلات قبل نقلها.",
        matches ? "A recovery draft is available in this browser." : "The Cue changed since this draft was saved. Review the edits before transferring them.");
    };
    const refreshLocalChoices = () => {
      localChoices.textContent = "";
      candidates.forEach((_, index) => {
        const option = document.createElement("option");
        option.value = String(index);
        option.textContent = tr("مسودة ", "Draft ") + (index + 1);
        localChoices.append(option);
      });
      localChoices.value = "0";
      localPending = candidates[0] || null;
      showLocalCandidate();
    };
    localChoices.onchange = () => {
      localPending = candidates[Number(localChoices.value)] || null;
      showLocalCandidate();
    };
    function removeLocalCandidate() {
      try {
        localPending.storage.removeItem(localPending.key);
      } catch (_) {
        localMessage("تعذّر حذف المسودة المحلية.", "The browser could not remove this draft.");
        return false;
      }
      candidates.splice(candidates.indexOf(localPending), 1);
      refreshLocalChoices();
      return true;
    }
    refreshLocalChoices();

    let generation = "";
    let blocked = true;
    let ownsRecovery = false;
    let lastSaved = base;
    let timer;
    let queue = Promise.resolve();
    const lifetime = new AbortController();
    const active = () => dialog.open && state.user === user && state.project?.project_id === projectID && panel.isConnected;
    const message = (ar, en) => { if (active()) status.textContent = tr(ar, en); };
    const checkpointsPath = `/api/v1/projects/${encodeURIComponent(projectID)}/cues/${encodeURIComponent(cueID)}/editor-checkpoints`;
    const checkpointSection = document.createElement("details");
    checkpointSection.id = "sc009NamedCheckpoints";
    checkpointSection.hidden = !cueID;
    const checkpointSummary = document.createElement("summary");
    checkpointSummary.textContent = tr("نقاط حفظ المسودة", "Draft checkpoints");
    const checkpointStatus = document.createElement("p");
    checkpointStatus.className = "muted";
    checkpointStatus.setAttribute("role", "status");
    const checkpointName = document.createElement("input");
    checkpointName.maxLength = 64;
    checkpointName.placeholder = tr("اسم نقطة الحفظ", "Checkpoint name");
    checkpointName.setAttribute("aria-label", checkpointName.placeholder);
    const checkpointSave = document.createElement("button");
    checkpointSave.type = "button";
    checkpointSave.className = "button";
    checkpointSave.textContent = tr("احفظ نقطة", "Save checkpoint");
    const checkpointList = document.createElement("select");
    checkpointList.setAttribute("aria-label", tr("نقاط الحفظ", "Saved checkpoints"));
    const checkpointPreview = document.createElement("textarea");
    checkpointPreview.hidden = true;
    checkpointPreview.readOnly = true;
    checkpointPreview.setAttribute("aria-label", tr("معاينة نقطة الحفظ", "Checkpoint preview"));
    const checkpointRestore = document.createElement("button");
    checkpointRestore.type = "button";
    checkpointRestore.className = "button ghost";
    checkpointRestore.hidden = true;
    checkpointRestore.textContent = tr("استرجع إلى المحرر", "Restore into editor");
    const checkpointDelete = document.createElement("button");
    checkpointDelete.type = "button";
    checkpointDelete.className = "button ghost";
    checkpointDelete.hidden = true;
    checkpointDelete.textContent = tr("احذف نقطة الحفظ", "Delete checkpoint");
    checkpointSection.append(checkpointSummary, checkpointStatus, checkpointName,
      checkpointSave, checkpointList, checkpointPreview, checkpointRestore, checkpointDelete);
    panel.append(checkpointSection);
    let checkpointRecords = [];
    let selectedCheckpoint = null;
    const checkpointCanRestore = (record) => {
      const saved = record?.document;
      const draft = saved?.draft;
      return saved?.version === 1 && saved.cue_id === cueID &&
        draft?.fields?.length === historyFields.length && Array.isArray(draft.links) &&
        draft.links.every((id) => typeof id === "string") && Array.isArray(draft.actions) &&
        draft.actions.every((action) => action && typeof action === "object" && !Array.isArray(action)) &&
        (!draft.timecode || (Array.isArray(draft.timecode) && draft.timecode.length === timecodeFields.length));
    };
    const renderCheckpointSelection = () => {
      selectedCheckpoint = checkpointRecords.find((record) => record.checkpoint_id === checkpointList.value) || null;
      checkpointPreview.hidden = !selectedCheckpoint;
      checkpointRestore.hidden = !selectedCheckpoint || !checkpointCanRestore(selectedCheckpoint);
      checkpointDelete.hidden = !selectedCheckpoint;
      checkpointPreview.value = selectedCheckpoint ? JSON.stringify(selectedCheckpoint.document?.draft, null, 2) : "";
    };
    const renderCheckpointList = (preferredID = "") => {
      checkpointList.textContent = "";
      checkpointRecords.forEach((record) => {
        const option = document.createElement("option");
        option.value = record.checkpoint_id;
        option.textContent = `${record.name} · ${record.created_at}`;
        checkpointList.append(option);
      });
      checkpointList.hidden = checkpointRecords.length === 0;
      checkpointList.value = checkpointRecords.some((record) => record.checkpoint_id === preferredID) ?
        preferredID : (checkpointRecords[0]?.checkpoint_id || "");
      renderCheckpointSelection();
    };
    checkpointList.onchange = renderCheckpointSelection;
    checkpointSave.onclick = async () => {
      if (!active()) return;
      const name = checkpointName.value.trim();
      const current = readDraftHistory(true);
      if (!name || !current) {
        checkpointStatus.textContent = tr("أدخل اسماً وتأكد من صلاحية حقول المسودة.", "Enter a name and make sure the draft fields are valid.");
        return;
      }
      checkpointSave.disabled = true;
      try {
        const created = await api(checkpointsPath, {
          method: "POST", json: { name, document: recoveryDocument(current) },
        });
        if (!active()) return;
        checkpointRecords = [created, ...checkpointRecords.filter((record) => record.checkpoint_id !== created.checkpoint_id)].slice(0, 20);
        checkpointName.value = "";
        renderCheckpointList(created.checkpoint_id);
        checkpointStatus.textContent = tr("حُفظت نقطة المسودة. لم يتغير الكيو المنشور.", "Draft checkpoint saved. The authored Cue was not changed.");
      } catch (_) {
        if (active()) checkpointStatus.textContent = tr("تعذّر حفظ نقطة المسودة.", "Could not save the draft checkpoint.");
      } finally {
        checkpointSave.disabled = false;
      }
    };
    checkpointRestore.onclick = () => {
      if (!active() || !checkpointCanRestore(selectedCheckpoint)) return;
      const current = readDraftHistory(true);
      const stale = !compatible(selectedCheckpoint.document);
      if ((stale || current !== base) && !window.confirm(tr(
        stale ? "تغيّر الكيو منذ نقطة الحفظ. استرجاعها إلى المحرر؟" : "استبدال التعديلات الحالية بمسودة نقطة الحفظ؟",
        stale ? "The Cue changed since this checkpoint. Restore it into the editor?" : "Replace the current edits with this checkpoint draft?",
      ))) return;
      applyEditorDraft(selectedCheckpoint.document.draft);
      recordDraftHistory();
      checkpointStatus.textContent = tr("استُرجعت المسودة في المحرر فقط. احفظ الكيو عند الجاهزية.", "Draft restored into the editor only. Save the Cue when ready.");
    };
    checkpointDelete.onclick = async () => {
      if (!active() || !selectedCheckpoint || !window.confirm(tr("حذف نقطة الحفظ المحددة؟", "Delete this checkpoint?"))) return;
      const deleting = selectedCheckpoint;
      checkpointDelete.disabled = true;
      try {
        await api(`${checkpointsPath}/${encodeURIComponent(deleting.checkpoint_id)}`, { method: "DELETE" });
        if (!active()) return;
        checkpointRecords = checkpointRecords.filter((record) => record.checkpoint_id !== deleting.checkpoint_id);
        renderCheckpointList();
        checkpointStatus.textContent = tr("حُذفت نقطة الحفظ.", "Checkpoint deleted.");
      } catch (_) {
        if (active()) checkpointStatus.textContent = tr("تعذّر حذف نقطة الحفظ.", "Could not delete the checkpoint.");
      } finally {
        checkpointDelete.disabled = false;
      }
    };
    const loadCheckpoints = async () => {
      if (!cueID) return;
      try {
        const result = await api(checkpointsPath);
        if (!active()) return;
        checkpointRecords = Array.isArray(result.checkpoints) ?
          result.checkpoints.filter((record) => record?.cue_id === cueID && record?.checkpoint_id && record?.name) : [];
        renderCheckpointList();
        if (checkpointRecords.length) checkpointStatus.textContent = tr("تُحفظ هذه النقاط لهذا المستخدم والكيو فقط.", "These checkpoints are saved for this user and Cue only.");
      } catch (_) {
        if (active()) checkpointStatus.textContent = tr("تعذّر تحميل نقاط الحفظ.", "Could not load checkpoints.");
      }
    };
    const clearRecovery = async () => {
      if (generation) {
        await api(path, { method: "DELETE", headers: { "If-Match": generation } });
        generation = "";
      }
    };
    const reportFailure = (error) => {
      if (error?.status === 409) blocked = true;
      message("تعذّر حفظ الاسترجاع. أبقِ المحرر مفتوحاً وأعد الاتصال، أو راجع التبويب الآخر.",
        "Recovery could not be saved. Keep the editor open and reconnect or check the other tab.");
    };
    const cacheLocal = () => {
      if (!active() || restoringDraft) return;
      const current = readDraftHistory(true);
      if (current === null) return;
      if (writeLocal(current) && !localPending) {
        localMessage(current === base ? "" : "حُفظت مسودة في المتصفح.",
          current === base ? "" : "Draft stored in this browser.");
      }
    };
    const flush = (keepalive = false) => {
      if (!active() || blocked || localPending || restoringDraft) return;
      const current = readDraftHistory(true);
      if (current === null) {
        message("تعذّر قراءة حقول المحرر. آخر نسخة محفوظة تبقى متاحة.",
          "The editor fields could not be read. The last saved recovery is still available.");
        return;
      }
      queue = queue.then(async () => {
        if (blocked || localPending || current === lastSaved || !active()) return;
        if (current === base && !generation) return;
        const document = recoveryDocument(current);
        const saved = await api(path, { method: "PUT", json: { generation, document }, keepalive });
        generation = saved.generation;
        ownsRecovery = true;
        lastSaved = current;
        message("حُفظت نسخة للاسترجاع.", "Recovery saved.");
      }).catch(reportFailure);
    };
    const schedule = () => {
      cacheLocal();
      clearTimeout(timer);
      timer = setTimeout(flush, 400);
    };
    const options = { signal: lifetime.signal };
    form.addEventListener("input", schedule, options);
    form.addEventListener("change", schedule, options);
    const changes = new MutationObserver(schedule);
    changes.observe(pick("actionsEditor"), { childList: true, subtree: true });
    window.addEventListener("online", () => { if (loaded) flush(); else load(); }, options);
    window.addEventListener("pagehide", () => { cacheLocal(); flush(true); }, options);
    let loaded = false;

    localRestore.onclick = () => {
      if (!active() || !localPending) return;
      if (!compatible(localPending.saved)) {
        localReview.value = JSON.stringify(localPending.saved.draft, null, 2);
        localReview.hidden = false;
        return;
      }
      if (readDraftHistory(true) !== base &&
          !window.confirm(tr("استبدال التعديلات الحالية بمسودة التبويب؟", "Replace current edits with this tab's draft?"))) return;
      const recovered = localPending.saved.draft;
      if (!writeLocal(JSON.stringify(recovered))) return;
      applyEditorDraft(recovered);
      recordDraftHistory();
      removeLocalCandidate();
      schedule();
    };
    localDiscard.onclick = () => {
      if (!active() || !window.confirm(tr("حذف المسودة المحلية؟", "Delete this local recovery draft?"))) return;
      if (!localPending || !removeLocalCandidate()) return;
      schedule();
    };
    const closeRecovery = () => {
      if (active()) writeLocal(base);
      clearTimeout(timer);
      blocked = true;
      queue = queue.then(async () => {
        // A save/discard event from an obsolete editor must not delete the
        // previous Project's server recovery after the operator switched
        // projects. Its local copy remains available in the original scope.
        if (ownsRecovery && state.user === user && state.project?.project_id === projectID) {
          await clearRecovery();
        }
      }).catch(reportFailure);
    };
    dialog.addEventListener("stagecore:cue-saved", closeRecovery, options);
    dialog.addEventListener("stagecore:cue-discard", closeRecovery, options);
    dialog.addEventListener("close", () => {
      if (dialog.open) return;
      clearTimeout(timer);
      changes.disconnect();
      lifetime.abort();
      panel.remove();
    }, options);
    async function load() {
      try {
        const saved = await api(path);
        if (!active()) return;
        loaded = true;
        generation = saved.generation || "";
        const document = saved.document;
        if (!generation) {
          blocked = false;
          flush();
          return;
        }
        const draft = document?.draft;
        const matches = compatible(document);
        message(matches ? "توجد مسودة محفوظة لهذا الكيو." : "توجد مسودة لكيو آخر أو لنسخة أقدم.",
          matches ? "A recovery draft exists for this Cue." : "Recovery belongs to another Cue or an older version.");
        restore.hidden = !matches;
        discard.hidden = false;
        restore.onclick = () => {
          if (!active() || (readDraftHistory(true) !== base &&
              !window.confirm(tr("استبدال التعديلات الحالية بالمسودة المحفوظة؟", "Replace these edits with the saved draft?")))) return;
          applyEditorDraft(draft);
          lastSaved = readDraftHistory(true);
          recordDraftHistory();
          ownsRecovery = true;
          blocked = false;
          restore.hidden = discard.hidden = true;
          cacheLocal();
          message("استُرجعت المسودة.", "Draft recovered.");
        };
        discard.onclick = async () => {
          if (!active() || !window.confirm(tr("حذف نسخة الاسترجاع؟", "Delete the saved recovery?"))) return;
          try {
            await clearRecovery();
            if (!active()) return;
            blocked = false;
            restore.hidden = discard.hidden = true;
            message("حُذفت نسخة الاسترجاع.", "Recovery discarded.");
            flush();
          } catch (error) {
            reportFailure(error);
          }
        };
      } catch (error) {
        reportFailure(error);
      }
    }
    load();
    loadCheckpoints();
  }

  function bindActionPresets(workspace) {
    const user = state.user;
    const projectID = state.project?.project_id;
    if (!user || !projectID) return;
    const path = `/api/v1/projects/${encodeURIComponent(projectID)}/cue-action-presets`;
    const panel = document.createElement("details");
    panel.id = "sc009ActionPresets";
    const heading = document.createElement("summary");
    heading.textContent = tr("قوالب الإجراءات", "Action presets");
    const status = document.createElement("p");
    status.className = "muted";
    status.setAttribute("role", "status");
    const name = document.createElement("input");
    name.placeholder = tr("اسم القالب", "Preset name");
    name.setAttribute("aria-label", name.placeholder);
    name.maxLength = 64;
    const search = document.createElement("input");
    search.type = "search";
    search.className = "sc010-preset-search";
    search.placeholder = tr("ابحث في أسماء القوالب والأهداف والإجراءات", "Search preset names, targets, and actions");
    search.setAttribute("aria-label", search.placeholder);
    const button = (text) => {
      const result = document.createElement("button");
      result.type = "button";
      result.className = "button ghost";
      result.textContent = text;
      return result;
    };
    const save = button(tr("احفظ الإجراء المنسوخ", "Save copied action"));
    const list = document.createElement("ul");
    panel.append(heading, name, save, status, search, list);
    pick(panel.id)?.remove();
    workspace.append(panel);
    const active = () => pick("cueDialog")?.open && panel.isConnected &&
      state.user === user && state.project?.project_id === projectID;
    const filterRows = () => {
      const query = String(search.value || "").trim().toLocaleLowerCase();
      for (const row of list.children) {
        row.hidden = Boolean(query) && !String(row.dataset.searchText || "").includes(query);
      }
    };
    const refresh = async () => {
      const { presets } = await api(path);
      if (!active()) return;
      list.replaceChildren();
      for (const preset of presets) {
        const row = document.createElement("li");
        const actions = Array.isArray(preset.actions) ? preset.actions : [preset.action].filter(Boolean);
        row.dataset.searchText = `${preset.name || ""} ${JSON.stringify(actions)}`.toLocaleLowerCase();
        const targets = state.f002Targets || [];
        const targetPresent = actions.length > 0 && actions.every((action) => {
          const targetRef = String(action?.target_ref || "").trim();
          return targetRef && targets.some((target) => target.logical_name === targetRef);
        });
        const targetStatus = document.createElement("p");
        targetStatus.className = targetPresent ? "muted" : "message warn";
        targetStatus.setAttribute("role", targetPresent ? "status" : "alert");
        const missingTargetNotice = tr(
          "الهدف غير موجود في إعدادات هذا المشروع. أضف هدفاً أو أعد تعيينه قبل الاعتماد على هذا الكيو.",
          "Target is not present in this Project configuration. Add or remap a target before relying on this Cue.",
        );
        targetStatus.textContent = targetPresent
          ? tr("الهدف موجود في إعدادات هذا المشروع.", "Target is available in this Project configuration.")
          : missingTargetNotice;
        const remaps = document.createElement("details");
        remaps.className = "sc010-preset-remaps";
        const remapHeading = document.createElement("summary");
        remapHeading.textContent = tr("إعادة تعيين الأهداف المفقودة", "Remap missing targets for this Cue");
        remaps.append(remapHeading);
        const mappedTargets = new Map();
        const missingTargets = new Map();
        actions.forEach((action, index) => {
          const originalTarget = String(action?.target_ref || "").trim();
          if (originalTarget && targets.some((target) => target.logical_name === originalTarget)) return;
          if (!missingTargets.has(originalTarget)) missingTargets.set(originalTarget, []);
          missingTargets.get(originalTarget).push(index);
        });
        const renderRemaps = () => {
          if (remaps.dataset.ready || !remaps.open) return;
          remaps.dataset.ready = "true";
          for (const [originalTarget, indexes] of missingTargets) {
            const label = document.createElement("label");
            label.className = "sc010-preset-remap";
            const caption = document.createElement("span");
            const actionNumbers = indexes.map((index) => index + 1).join(", ");
            caption.textContent = tr(
              `الإجراءات ${actionNumbers} — الهدف الحالي: ${originalTarget || "غير محدد"}`,
              `Actions ${actionNumbers} — current target: ${originalTarget || "unset"}`,
            );
            const select = document.createElement("select");
            select.className = "sc010-target-remap";
            select.setAttribute("aria-label", caption.textContent);
            const empty = document.createElement("option");
            empty.value = "";
            empty.textContent = tr("أبقِ الهدف كما هو", "Keep current target");
            select.append(empty);
            const choices = new Set();
            for (const target of targets) {
              const logicalName = String(target?.logical_name || "").trim();
              if (!logicalName || choices.has(logicalName)) continue;
              choices.add(logicalName);
              const option = document.createElement("option");
              option.value = logicalName;
              option.textContent = `${logicalName} · ${target.logical_type || ""}`;
              select.append(option);
            }
            select.addEventListener("change", () => {
              const replacement = String(select.value || "").trim();
              if (replacement) mappedTargets.set(originalTarget, replacement);
              else mappedTargets.delete(originalTarget);
              targetStatus.className = "message warn";
              if (Array.from(missingTargets.keys()).every((target) => mappedTargets.has(target))) {
                targetStatus.textContent = tr(
                  "ستُطبّق إعادة التعيين على نسخة المسودة فقط. تحقّق من توافق الهدف قبل النشر.",
                  "Targets will be remapped in this Draft Cue only. Verify target compatibility before publishing.",
                );
              } else {
                targetStatus.textContent = missingTargetNotice;
              }
            });
            label.append(caption, select);
            remaps.append(label);
          }
        };
        remaps.addEventListener("toggle", renderRemaps);
        const preview = document.createElement("details");
        const previewHeading = document.createElement("summary");
        previewHeading.textContent = actions.length > 1
          ? tr(`معاينة ${actions.length} إجراءات`, `Preview ${actions.length} actions`)
          : tr("معاينة تفاصيل الإجراء", "Preview action details");
        const previewContent = document.createElement("pre");
        previewContent.className = "mono muted";
        previewContent.textContent = JSON.stringify(actions.length > 1 ? actions : actions[0] || {}, null, 2);
        preview.append(previewHeading, previewContent);
        const insert = button(actions.length > 1
          ? tr(`أضف ${actions.length} إجراءات إلى الكيو`, `Add ${actions.length} actions to Cue`)
          : tr("أضف إلى الكيو", "Add to Cue"));
        insert.onclick = () => {
          if (!active()) return;
          actions.forEach((action, index) => {
            const copy = structuredClone(action);
            const originalTarget = String(action?.target_ref || "").trim();
            if (mappedTargets.has(originalTarget)) copy.target_ref = mappedTargets.get(originalTarget);
            addActionEditor({ ...copy, action_id: "" });
          });
          refreshActions();
        };
        const remove = button(tr("احذف القالب", "Delete preset"));
        remove.onclick = async () => {
          if (!active() || !window.confirm(tr("حذف قالب «" + preset.name + "»؟", "Delete preset “" + preset.name + "”?"))) return;
          remove.disabled = true;
          try {
            await api(path + "/" + encodeURIComponent(preset.preset_id), { method: "DELETE" });
            if (active()) await refresh();
          } catch (error) {
            if (active()) status.textContent = errorMessage(error);
          } finally {
            remove.disabled = false;
          }
        };
        row.append(preset.name, targetStatus);
        if (missingTargets.size) row.append(remaps);
        row.append(preview, insert, " ", remove);
        list.append(row);
      }
      filterRows();
    };
    search.addEventListener("input", filterRows);
    panel.addEventListener("toggle", () => {
      if (panel.open && active()) refresh().catch((error) => {
        if (active()) status.textContent = errorMessage(error);
      });
    });
    save.onclick = async () => {
      if (!active()) return;
      const actions = clipboardActions();
      const label = name.value.trim();
      if (!actions.length || !label) {
        status.textContent = tr("انسخ إجراءً أو مجموعة إجراءات واكتب اسم القالب.", "Copy one or more actions and enter a preset name.");
        return;
      }
      save.disabled = true;
      try {
        const preset = actions.length > 1 ? { name: label, actions } : { name: label, action: actions[0] };
        await api(path, { method: "POST", json: preset });
        if (!active()) return;
        name.value = "";
        status.textContent = tr("حُفظ القالب.", "Preset saved.");
        await refresh();
      } catch (error) {
        if (active()) status.textContent = error.status === 409
          ? tr("الاسم موجود أو وصلت إلى ٣٢ قالباً.", "The name exists or the 32-preset limit was reached.")
          : errorMessage(error);
      } finally {
        save.disabled = false;
      }
    };
  }

  function formSignature() {
    return JSON.stringify(Array.from(pick("cueForm")?.querySelectorAll("input,select,textarea") || [])
      .map((node) => [node.id || node.className, node.value, node.type === "checkbox" ? node.checked : null]));
  }

  function bindFormGuards() {
    const form = pick("cueForm");
    if (form.dataset.sc008Guarded) return;
    form.dataset.sc008Guarded = "true";
    form.addEventListener("click", (event) => {
      if (!event.target.closest("#closeCueDialog, #cancelCueButton")) return;
      if (initialFormState === formSignature()) return;
      if (!window.confirm(phrase("cue.studio.close_warning"))) {
        event.preventDefault();
        event.stopImmediatePropagation();
      } else {
        pick("cueDialog")?.dispatchEvent(new Event("stagecore:cue-discard"));
      }
    }, true);
    pick("cueDialog")?.addEventListener("cancel", (event) => {
      if (initialFormState !== formSignature() &&
          !window.confirm(tr("تجاهل التغييرات غير المحفوظة؟", "Discard unsaved changes?"))) {
        event.preventDefault();
      } else if (initialFormState !== formSignature()) {
        pick("cueDialog")?.dispatchEvent(new Event("stagecore:cue-discard"));
      }
    });
    form.addEventListener("invalid", (event) => {
      const card = event.target.closest(".sc008-action-card");
      if (card) expandCard(card, true);
      const optional = event.target.closest("#sc008Optional");
      if (optional) optional.open = true;
    }, true);
  }

  function initializeEditor(cue) {
    const form = pick("cueForm");
    if (!form || !pick("actionsEditor")) return;
    form.classList.add("sc008-form");
    pick("cueDialog")?.classList.add("sc008-dialog");
    const meta = pick("cueName")?.closest(".form-grid.two");
    const workspace = pick("sc008Workarea") || document.createElement("section");
    workspace.id = "sc008Workarea";
    workspace.className = "sc008-workarea";
    if (!workspace.isConnected && meta) meta.after(workspace);

    const heading = pick("addActionButton")?.closest(".section-title-row");
    if (heading) workspace.append(heading);
    ensurePicker(workspace);
    const composer = pick("f002CueComposer");
    if (composer) workspace.append(composer);
    workspace.append(pick("actionsEditor"));

    const empty = pick("sc008Empty") || document.createElement("p");
    empty.id = "sc008Empty";
    empty.className = "sc008-empty";
    empty.textContent = tr("اختَر نوع إجراء حتى تبني الكيو.", "Choose an action to start building this Cue.");
    workspace.append(empty);

    const counter = pick("sc008ActionCount") || document.createElement("span");
    counter.id = "sc008ActionCount";
    counter.className = "sc008-count";
    heading?.querySelector("h3")?.append(" ", counter);
    const addButton = pick("addActionButton");
    if (addButton) addButton.textContent = tr("+ إضافة إجراء", "+ Add action");
    const paste = pick("sc008PasteAction") || document.createElement("button");
    paste.id = "sc008PasteAction";
    paste.className = "button ghost";
    paste.type = "button";
    paste.textContent = tr("لصق إجراء", "Paste action");
    paste.disabled = !clipboardAction();
    paste.onclick = pasteClipboardAction;
    if (!paste.isConnected) addButton?.after(paste);
    const copySelected = pick("sc008CopySelectedActions") || document.createElement("button");
    copySelected.id = "sc008CopySelectedActions";
    copySelected.className = "button ghost";
    copySelected.type = "button";
    copySelected.textContent = tr("نسخ المحدد للكيوهات", "Copy selected to other Cues");
    copySelected.disabled = true;
    copySelected.onclick = copySelectedActionsToClipboard;
    if (!copySelected.isConnected) paste.after(copySelected);
    const optional = createOptionalDetails(form);
    if (!optional.isConnected) workspace.after(optional);
    optional.open = !!(cue?.notes_summary || cue?.criticality === "CRITICAL" ||
      cue?.enabled === false || (cue?.execution_policy?.linked_cue_ids || []).length ||
      Number(cue?.execution_policy?.start_delay_ms || 0) > 0);

    if (composer) {
      composer.hidden = true;
      composer.querySelectorAll("[data-f002-extension],[data-f002-import]")
        .forEach((section) => { section.hidden = true; });
    }
    actionCards().forEach((card, index) => {
      decorateAction(card, !!cue?.actions?.[index]?.action_id);
      expandCard(card, index === 0);
    });
    refreshActions();
    bindFormGuards();
    initialFormState = formSignature();
    bindDraftHistory(form, paste);
    bindEditorRecovery(form, cue, workspace);
    bindActionPresets(workspace);
  }

  // Base Cue creation attaches a click listener. Capture before that listener
  // so the Add action button opens the type picker rather than a raw OSC card.
  pick("addActionButton")?.addEventListener("click", (event) => {
    event.preventDefault();
    event.stopImmediatePropagation();
    const picker = pick("sc008Picker");
    picker?.classList.toggle("is-open");
    pick("f002CueComposer")?.setAttribute("hidden", "");
    if (picker?.classList.contains("is-open")) picker.scrollIntoView({ block: "nearest" });
  }, true);

  const previousOpen = openCueEditor;
  openCueEditor = function sc008OpenCueEditor(cue) {
    previousOpen(cue);
    initializeEditor(cue);
  };

  async function moveCueToPosition(cue) {
    if (!cue || !(state.cues || []).length) return;
    const ordered = state.cues || [];
    const sourceIndex = ordered.findIndex((item) => item.cue_id === cue.cue_id);
    if (sourceIndex < 0) return;
    const raw = window.prompt(
      tr("انقل كيو «" + cue.name + "» إلى أي رقم؟ (1–" + ordered.length + ")",
        "Move Cue “" + cue.name + "” to position (1–" + ordered.length + ")"),
      String(sourceIndex + 1)
    );
    if (raw === null) return;
    const value = raw.trim();
    const number = Number(value);
    if (!/^[0-9]+$/.test(value) || !Number.isSafeInteger(number) ||
        number < 1 || number > ordered.length) {
      window.alert(tr("أدخل رقماً صحيحاً ضمن نطاق الكيوهات.",
        "Enter an integer position within the Cue list."));
      return;
    }
    if (number === sourceIndex + 1) return;
    const moved = [...ordered];
    const [item] = moved.splice(sourceIndex, 1);
    moved.splice(number - 1, 0, item);
    try {
      await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/cues/reorder`, {
        method: "POST",
        json: { cue_ids: moved.map((entry) => entry.cue_id) },
      });
      await renderCues(tr("تم تحديث ترتيب الكيوهات في المسودة.", "Draft Cue order updated."));
    } catch (error) {
      setMessage(globalMessage, errorMessage(error), error?.status === 409 ? "warn" : "error");
    }
  }

  function cueActionExecutionModeText(mode) {
    switch (String(mode || "SEQUENTIAL").toUpperCase()) {
      case "SEQUENTIAL":
        return tr("تتابعي؛ ينتظر هذا الإجراء، وقد تستمر إجراءات متوازية سابقة بالتزامن.",
          "Sequential; waits for this Action while earlier parallel Actions may continue.");
      case "PARALLEL":
        return tr("متوازٍ؛ يُنتظر عند الحاجز التالي أو نهاية الكيو، وقد ينتهي مبكراً عند الإيقاف أو الفشل.",
          "Parallel; joined at the next barrier or end of Cue, and may end earlier on STOP or failure.");
      case "PARALLEL_BARRIER":
        return tr("حاجز متوازٍ؛ يبدأ بالتزامن ثم ينتظر كل الإجراءات المعلقة.",
          "Parallel barrier; starts concurrently, then waits for all pending Actions.");
      default:
        return tr("نمط تنفيذ غير مدعوم: ", "Unsupported execution mode: ") + String(mode);
    }
  }

  // Read-only planning from the loaded revision. No runtime API is used.
  function planCueInspection(root, cues) {
    const byID = new Map(cues.map((cue) => [cue.cue_id, cue]));
    const parents = new Map();
    const ambiguous = new Set();
    for (const cue of cues) {
      for (const child of cueLinkedIDs(cue)) {
        if (parents.has(child) && parents.get(child) !== cue.cue_id) {
          ambiguous.add(child);
        } else {
          parents.set(child, cue.cue_id);
        }
      }
    }
    const parent = ambiguous.has(root.cue_id) ? undefined : parents.get(root.cue_id);
    const active = new Set();
    const seen = new Set();
    const stack = [{ id: root.cue_id }];
    const group = [];
    const issues = [];
    while (stack.length) {
      const { id, done } = stack.pop();
      if (done) {
        active.delete(id);
        continue;
      }
      if (active.has(id)) {
        issues.push({ kind: "cycle", id });
        continue;
      }
      if (seen.has(id)) continue;
      if (ambiguous.delete(id)) issues.push({ kind: "multiple-parents", id });
      const cue = byID.get(id);
      if (!cue || (cue.enabled === false && id !== root.cue_id)) {
        issues.push({ kind: cue ? "disabled" : "missing", id });
        continue;
      }
      seen.add(id);
      active.add(id);
      group.push({
        cue,
        delay: cuePolicyObject(cue).start_delay_ms || 0,
        actions: [...(cue.actions || [])].sort((a, b) => a.order_index - b.order_index),
      });
      stack.push({ id, done: true });
      for (const child of cueLinkedIDs(cue).reverse()) stack.push({ id: child });
    }
    return { group, issues, parent, linkedCuesConcurrent: group.length > 1 };
  }

  function cueInspectionTarget(targets, targetRef) {
    return (Array.isArray(targets) ? targets : []).find((target) =>
      target.logical_name === targetRef || target.target_ref === targetRef || target.alias_id === targetRef) || null;
  }

  function cueInspectionParameters(action) {
    const value = action?.parameters;
    if (value && typeof value === "object" && !Array.isArray(value)) return value;
    if (typeof value !== "string") return {};
    try {
      const parsed = JSON.parse(value);
      return parsed && typeof parsed === "object" && !Array.isArray(parsed) ? parsed : {};
    } catch (_) {
      return {};
    }
  }

  // This is a local, hypothetical dispatch timeline. With no latency
  // assumption, offsets are lower bounds because capability completion time
  // is unknown. It deliberately never calls an execution API.
  function cueInspectionTimeline(plan, options = {}) {
    const targets = Array.isArray(options.targets) ? options.targets : [];
    const scenario = options.scenario || "baseline";
    const offlineTarget = String(options.offlineTarget || "");
    const latencyMS = Math.max(0, Number(options.latencyMS) || 0);
    const rows = [];

    for (const member of plan.group || []) {
      let cursorMS = 0;
      let gate = [];
      let pending = [];
      let blockedReason = "";
      const actions = [...(member.actions || [])].sort((a, b) => a.order_index - b.order_index);
      for (const action of actions) {
        const mode = String(action.execution_mode || "SEQUENTIAL").toUpperCase();
        const actionDelayMS = Math.max(0, Number(action.timeout_policy?.start_delay_ms) || 0);
        const waitFor = [...gate];
        const dispatchMS = cursorMS + actionDelayMS;
        const timeoutMS = Number(action.timeout_policy?.timeout_ms) || 0;
        const target = cueInspectionTarget(targets, action.target_ref);
        const logicalType = String(target?.logical_type || "").toUpperCase();
        const capability = String(action.capability_key || "").toLowerCase();
        const onError = String(action.error_policy?.on_error || "CONTINUE").toUpperCase();
        let outcome = action.enabled === false ? "SKIPPED_DISABLED"
          : blockedReason ? "SIMULATED_NOT_REACHED" : "PLANNED_ONLY";
        let reason = action.enabled === false ? "ACTION_DISABLED" : blockedReason;

        if (action.enabled !== false && !blockedReason) {
          if (scenario === "offline-target" && action.target_ref && action.target_ref === offlineTarget) {
            outcome = "SIMULATED_BLOCKED";
            reason = "VIRTUAL_TARGET_OFFLINE";
          } else if (scenario === "missing-companion" && logicalType === "MACHINE_ROLE") {
            outcome = "SIMULATED_BLOCKED";
            reason = "COMPANION_MISSING_FOR_MACHINE_ROLE";
          } else if (scenario === "missing-companion" && !target) {
            outcome = "UNVERIFIED_TARGET";
            reason = "TARGET_METADATA_UNAVAILABLE_COMPANION_IMPACT_UNKNOWN";
          } else if (scenario === "missing-dmx" &&
              (capability.startsWith("lighting.") || logicalType.includes("LIGHTING") || logicalType.includes("DMX"))) {
            outcome = "SIMULATED_BLOCKED";
            reason = "DMX_NODE_MISSING";
          } else if (action.target_ref && !target) {
            outcome = "UNVERIFIED_TARGET";
            reason = "TARGET_MAPPING_NOT_CONFIRMED_IN_LOADED_CONFIGURATION";
          }
          if (outcome === "PLANNED_ONLY" && latencyMS > 0 && timeoutMS > 0 && latencyMS > timeoutMS) {
            outcome = "SIMULATED_TIMEOUT";
            reason = "ASSUMED_LATENCY_EXCEEDS_ACTION_TIMEOUT";
          }
        }

        const parameters = cueInspectionParameters(action);
        const rawFade = Number(parameters.fade_ms);
        const fadeMS = Number.isFinite(rawFade) && rawFade >= 0 ? rawFade : null;
        const row = {
          cueID: member.cue.cue_id,
          cueDelayMS: Number(member.delay) || 0,
          action,
          mode,
          actionDelayMS,
          timeoutMS,
          fadeMS,
          dispatchMS: outcome === "SIMULATED_NOT_REACHED" ? null : (Number(member.delay) || 0) + dispatchMS,
          waitFor,
          outcome,
          reason,
          latencyMS: action.enabled === false ? 0 : latencyMS,
          completionMS: outcome === "SIMULATED_TIMEOUT" ? timeoutMS
            : outcome === "SIMULATED_BLOCKED" || action.enabled === false ? 0 : latencyMS,
        };
        rows.push(row);
        if (action.enabled === false || blockedReason) continue;

        // Match Cue Engine scheduling: PARALLEL dispatches in the background;
        // SEQUENTIAL waits for its own completion; PARALLEL_BARRIER joins all
        // pending Actions, including itself, before later Actions are started.
        if (mode === "PARALLEL") {
          pending.push({
            id: action.action_id,
            completionMS: dispatchMS + row.completionMS,
            fatal: (row.outcome === "SIMULATED_BLOCKED" || row.outcome === "SIMULATED_TIMEOUT") && onError !== "CONTINUE",
          });
        } else if (mode === "SEQUENTIAL") {
          cursorMS = dispatchMS + row.completionMS;
          gate = action.action_id ? [action.action_id] : [];
          if ((row.outcome === "SIMULATED_BLOCKED" || row.outcome === "SIMULATED_TIMEOUT") && onError !== "CONTINUE") {
            blockedReason = "ACTION_POLICY_FAIL_CUE:" + (action.action_id || "UNKNOWN_ACTION");
          }
        } else if (mode === "PARALLEL_BARRIER") {
          pending.push({
            id: action.action_id,
            completionMS: dispatchMS + row.completionMS,
            fatal: (row.outcome === "SIMULATED_BLOCKED" || row.outcome === "SIMULATED_TIMEOUT") && onError !== "CONTINUE",
          });
          cursorMS = Math.max(cursorMS, ...pending.map((item) => item.completionMS));
          gate = pending.map((item) => item.id).filter(Boolean);
          if (pending.some((item) => item.fatal)) blockedReason = "PARALLEL_BARRIER_FATAL_ACTION_FAILURE";
          pending = [];
        } else {
          row.outcome = "PLAN_UNSUPPORTED";
          row.reason = "UNSUPPORTED_EXECUTION_MODE";
          blockedReason = "PREVIOUS_UNSUPPORTED_EXECUTION_MODE:" + (action.action_id || "UNKNOWN_ACTION");
        }
      }
    }
    return rows;
  }

  function renderCueInspectionTimeline(parent, rows, options) {
    const line = (tag, value, className = "") => {
      const node = document.createElement(tag);
      node.textContent = value;
      if (className) node.className = className;
      parent.append(node);
      return node;
    };
    line("p", tr(
      "محاكاة محلية فقط؛ لا تُرسل أوامر. الزمن حد أدنى مبني على التأخيرات المحفوظة، واكتمال الجهاز غير معلوم.",
      "Local simulation only; no commands are sent. Timing is a lower bound from authored delays; device completion is unknown."
    ), "muted");
    line("p", tr(
      "المعاينة لا تثبت إرسال الأمر أو إقرار التطبيق أو ملاحظة خرج فعلي.",
      "This preview does not establish command dispatch, application acknowledgment, or physically observed output."
    ), "muted");
    if (options.latencyMS > 0) line("p", tr(
      "زمن افتراضي مفترض لكل إجراء: ", "Assumed virtual latency per Action: "
    ) + options.latencyMS + " ms · " + tr("ليس قياساً للجهاز.", "not measured from a device."), "muted");
    const list = document.createElement("ol");
    list.className = "cue-inspection-timeline";
    for (const row of rows) {
      const time = row.dispatchMS === null
        ? tr("لن يصل إليه المسار: ", "Not reached in this scenario: ") + row.reason
        : row.waitFor.length
        ? tr("بعد اكتمال ", "after completion of ") + row.waitFor.join(", ") + " · ≥ GO + " + row.dispatchMS + " ms"
        : "≥ GO + " + row.dispatchMS + " ms";
      const values = [
        row.cueID,
        row.action.action_id || tr("إجراء بلا معرّف", "Action without ID"),
        row.action.capability_key || tr("قدرة غير معروفة", "Unknown capability"),
        row.action.target_ref || tr("هدف غير محدد", "No target selected"),
        time,
        cueActionExecutionModeText(row.mode),
        row.fadeMS === null ? "" : tr("Fade ", "Fade ") + row.fadeMS + " ms",
        row.timeoutMS ? tr("مهلة ", "Timeout ") + row.timeoutMS + " ms" : "",
        row.latencyMS ? tr("زمن افتراضي ", "Assumed latency ") + row.latencyMS + " ms" : "",
        row.outcome,
        row.reason,
      ].filter(Boolean).join(" · ");
      const item = document.createElement("li");
      item.textContent = values;
      list.append(item);
    }
    parent.append(list);
    if (!rows.length) line("p", tr("لا توجد إجراءات في هذه السلسلة.", "This Cue group contains no Actions."));
  }

  function cueInspector(cue, cues, projectID, findings) {
    const details = document.createElement("details");
    const title = document.createElement("summary");
    title.textContent = tr("فاحص الكيو · تشغيل جاف", "Cue Inspector · Dry-run");
    details.append(title);
    details.addEventListener("toggle", () => {
      if (!details.open || details.childElementCount > 1) return;
      const plan = planCueInspection(cue, cues);
      const line = (parent, tag, value) => {
        const item = document.createElement(tag);
        item.textContent = value;
        parent.append(item);
        return item;
      };
      line(details, "p", tr("خطة لنسخة التأليف المحمّلة، بدون نشر أو إرسال أي أمر. أوقات التنفيذ الفعلية تعتمد على الأجهزة.", "Plan for the loaded authoring revision. Nothing is published or dispatched. Actual execution time depends on the devices."));
      if (cue.enabled === false) line(details, "p", tr("هذا الكيو معطّل.", "This Cue is disabled."));
      if (plan.parent) line(details, "p", tr("يُشغّل من الكيو الأب ", "Triggered by parent Cue ") + plan.parent);
      if (plan.linkedCuesConcurrent) line(details, "p", tr(
        "الكيوهات المرتبطة تبدأ بالتوازي؛ تأخير بدء كل كيو محسوب من GO وليس من انتهاء الكيو السابق.",
        "Linked Cues start concurrently; each Cue start delay is measured from GO, not from the previous Cue completing."
      ));
      const refs = new Set();
      for (const { cue: member, delay, actions } of plan.group) {
        refs.add(member.cue_id);
        line(details, "strong", [member.display_label, member.name,
          tr("بدء من GO + ", "Start from GO + ") + delay + " ms"].filter(Boolean).join(" · "));
        if (member.notes_summary) line(details, "p", member.notes_summary);
        const list = document.createElement("ol");
        for (const action of actions) {
          refs.add(action.action_id);
          refs.add(action.target_ref);
          const delay = action.timeout_policy?.start_delay_ms || 0;
          const timeout = action.timeout_policy?.timeout_ms;
          const errorPolicy = action.error_policy || {};
          line(list, "li", [action.capability_key, action.target_ref,
            cueActionExecutionModeText(action.execution_mode),
            delay ? tr("انتظار الإجراء قبل الإرسال ", "Action delay before dispatch ") + delay + " ms" : "",
            timeout ? tr("مهلة ", "Timeout ") + timeout + " ms" : "",
            errorPolicy.require_application_ack ? tr("يتطلب إقرار التطبيق", "Requires application ACK") : "",
            errorPolicy.retry_before_dispatch ? tr("إعادة واحدة قبل الإرسال فقط", "One retry before dispatch only") : "",
            action.enabled === false ? tr("معطّل", "Disabled") : ""].filter(Boolean).join(" · "));
        }
        if (list.childElementCount) details.append(list);
      }
      for (const { kind, id } of plan.issues) {
        const labels = {
          cycle: tr("حلقة ربط", "Link cycle"),
          disabled: tr("رابط لكيو معطّل", "Link to disabled Cue"),
          missing: tr("رابط لكيو مفقود", "Link to missing Cue"),
          "multiple-parents": tr("الكيو مرتبط بأكثر من كيو أب", "Cue is linked by more than one parent"),
        };
        line(details, "p", labels[kind] + " · " + id);
      }
      for (const finding of findings) {
        if (finding.ref && refs.has(finding.ref)) {
          line(details, "p", [finding.severity, finding.code, finding.message].filter(Boolean).join(" · "));
        }
      }
      const scenarioPanel = document.createElement("section");
      scenarioPanel.className = "cue-inspection-scenario";
      const scenarioTitle = document.createElement("strong");
      scenarioTitle.textContent = tr("محاكاة مسار المسودة", "Draft path simulation");
      scenarioPanel.append(scenarioTitle);
      const scenarioControls = document.createElement("div");
      scenarioControls.className = "cue-inspection-controls";
      const scenarioSelect = document.createElement("select");
      scenarioSelect.setAttribute("aria-label", tr("سيناريو افتراضي", "Virtual scenario"));
      [
        ["baseline", tr("المسار الأساسي", "Baseline path")],
        ["offline-target", tr("هدف افتراضي غير متصل", "Virtual target offline")],
        ["missing-companion", tr("Companion مفقود", "Companion missing")],
        ["missing-dmx", tr("عقدة DMX مفقودة", "DMX node missing")],
      ].forEach(([value, label]) => {
        const option = document.createElement("option");
        option.value = value;
        option.textContent = label;
        scenarioSelect.append(option);
      });
      scenarioControls.append(scenarioSelect);
      const targetRefs = [...new Set(plan.group.flatMap((member) => member.actions || [])
        .map((action) => String(action.target_ref || "").trim()).filter(Boolean))];
      const targetSelect = document.createElement("select");
      targetSelect.setAttribute("aria-label", tr("الهدف الافتراضي غير المتصل", "Virtual offline target"));
      const targetPlaceholder = document.createElement("option");
      targetPlaceholder.value = "";
      targetPlaceholder.textContent = tr("اختر هدفاً", "Choose a target");
      targetSelect.append(targetPlaceholder);
      targetRefs.forEach((ref) => {
        const option = document.createElement("option");
        option.value = ref;
        option.textContent = ref;
        targetSelect.append(option);
      });
      targetSelect.hidden = true;
      scenarioControls.append(targetSelect);
      const latencySelect = document.createElement("select");
      latencySelect.setAttribute("aria-label", tr("زمن المحاكاة الافتراضي", "Assumed virtual latency"));
      [[0, "0 ms"], [100, "100 ms"], [250, "250 ms"], [1000, "1000 ms"]].forEach(([value, label]) => {
        const option = document.createElement("option");
        option.value = String(value);
        option.textContent = label;
        latencySelect.append(option);
      });
      scenarioControls.append(latencySelect);
      scenarioSelect.addEventListener("change", () => {
        targetSelect.hidden = scenarioSelect.value !== "offline-target";
      });
      const previewButton = document.createElement("button");
      previewButton.type = "button";
      previewButton.className = "button ghost";
      previewButton.textContent = tr("اعرض المحاكاة", "Preview simulation");
      scenarioControls.append(previewButton);
      const simulationResult = document.createElement("div");
      simulationResult.className = "cue-inspection-result";
      simulationResult.setAttribute("role", "status");
      previewButton.addEventListener("click", () => {
        simulationResult.replaceChildren();
        const options = {
          scenario: scenarioSelect.value,
          offlineTarget: targetSelect.value,
          latencyMS: Number(latencySelect.value),
          targets: state.f002Targets || [],
        };
        const rows = cueInspectionTimeline(plan, options);
        renderCueInspectionTimeline(simulationResult, rows, options);
      });
      scenarioPanel.append(scenarioControls, simulationResult);
      details.append(scenarioPanel);
      const button = line(details, "button", tr("افحص جاهزية النسخة المنشورة", "Check published snapshot readiness"));
      button.type = "button";
      button.className = "button ghost";
      const result = document.createElement("div");
      result.setAttribute("role", "status");
      details.append(result);
      button.addEventListener("click", async () => {
        if (state.project?.project_id !== projectID) return;
        button.disabled = true;
        result.textContent = tr("جاري فحص الجاهزية…", "Checking readiness…");
        try {
          const report = await api(`/api/v1/projects/${encodeURIComponent(projectID)}/preflight`);
          if (!result.isConnected || state.project?.project_id !== projectID) return;
          result.replaceChildren();
          line(result, "strong", tr("النسخة المنشورة ", "Published snapshot ") +
            [report.snapshot_version, report.runtime_snapshot_id, report.status].filter(Boolean).join(" · "));
          const list = document.createElement("ul");
          for (const check of report.checks || []) {
            if (check.status !== "PASS") {
              line(list, "li", [check.status, check.category, check.summary, check.detail].filter(Boolean).join(" · "));
            }
          }
          if (list.childElementCount) result.append(list);
        } catch (error) {
          if (result.isConnected && state.project?.project_id === projectID) result.textContent = errorMessage(error);
        } finally {
          if (button.isConnected) button.disabled = false;
        }
      });
    });
    return details;
  }

  function enhanceCueList() {
    const tableWrap = content.querySelector(".table-wrap");
    const tbody = tableWrap?.querySelector("tbody");
    if (!tbody || pick("sc008CueFilter")) return;
    const toolbar = document.createElement("section");
    toolbar.id = "sc008CueFilter";
    toolbar.className = "sc008-list-toolbar";
    const label = document.createElement("label");
    label.textContent = tr("بحث في الكيوهات", "Find Cues");
    const input = document.createElement("input");
    input.type = "search";
    input.placeholder = tr("رقم، اسم أو نوع الإجراء…", "Number, name or action type…");
    label.append(input);
    const select = document.createElement("select");
    select.setAttribute("aria-label", tr("تصفية الكيوهات", "Filter Cues"));
    [
      ["all", tr("كل الكيوهات", "All Cues")],
      ["osc", "OSC / VDMX"],
      ["midi", "MIDI / Ableton"],
      ["lighting", "DMX"],
      ["tablet", "Tablet"],
      ["laser", "StageLaser"],
      ["visual", "Native Visual"],
      ["mixed", tr("مركب", "Mixed")],
      ["disabled", tr("معطّل", "Disabled")],
    ].forEach(([value, text]) => {
      const option = document.createElement("option");
      option.value = value;
      option.textContent = text;
      select.append(option);
    });
    const counter = document.createElement("span");
    counter.className = "sc008-list-count";
    toolbar.append(label, select, counter);
    tableWrap.before(toolbar);
    const rows = Array.from(tbody.querySelectorAll("tr"));
    const cues = state.cues || [];
    const classify = (cue) => {
      const types = new Set((cue?.actions || []).map((a) => {
        const cap = a.capability_key || "";
        if (cap === "osc.send") return "osc";
        if (cap === "midi.send") return "midi";
        if (cap.startsWith("lighting.")) return "lighting";
        if (cap.startsWith("tablet.")) return "tablet";
        if (cap.startsWith("laser.")) return "laser";
        if (cap.startsWith("visual.")) return "visual";
        return "advanced";
      }));
      return types;
    };
    const apply = () => {
      const query = input.value.trim().toLocaleLowerCase();
      let visible = 0;
      rows.forEach((row, index) => {
        const cue = cues[index];
        if (!cue) return;
        const types = classify(cue);
        const matchKind = select.value === "all" ||
          (select.value === "disabled" && !cue.enabled) ||
          (select.value === "mixed" && types.size > 1) ||
          types.has(select.value);
        const text = [cue.order_index, cue.display_label, cue.name,
          ...(cue.actions || []).map((a) => a.capability_key)].join(" ").toLocaleLowerCase();
        const show = matchKind && text.includes(query);
        row.hidden = !show;
        if (show) visible++;
      });
      counter.textContent = tr("ظاهر ", "Showing ") + visible + " / " + cues.length;
    };
    input.addEventListener("input", apply);
    select.addEventListener("change", apply);
    apply();

    rows.forEach((row, index) => {
      const cue = cues[index];
      if (!cue) return;
      const cell = row.children[5];
      if (!cell) return;
      cell.append(cueInspector(cue, cues, state.project.project_id, state.validation?.findings || []));

      const types = Array.from(classify(cue)).slice(0, 3);
      const badges = document.createElement("div");
      badges.className = "sc008-type-badges";
      types.forEach((kind) => {
        const item = document.createElement("small");
        item.className = "sc008-type-badge";
        item.textContent = kind.toUpperCase();
        badges.append(item);
      });
      cell.append(badges);
    });

    // Keep the daily actions visible; place destructive and infrequent
    // controls in an accessible per-Cue disclosure, preserving click handlers.
    rows.forEach((row) => {
      const actions = row.querySelector(".row-actions");
      if (!actions) return;
      const secondary = Array.from(actions.querySelectorAll(
        ".cue-up,.cue-down,.cue-toggle,.cue-duplicate,.cue-delete"));
      if (!secondary.length) return;
      // Moving to a position uses the existing atomic reorder endpoint and
      // stable Cue IDs, not repeated one-step reorders.
      if (actions.querySelector(".cue-up,.cue-down")) {
        const cue = cues[rows.indexOf(row)];
        const move = document.createElement("button");
        move.type = "button";
        move.className = "button sc008-move-position";
        move.textContent = tr("نقل إلى رقم…", "Move to position…");
        move.addEventListener("click", () => moveCueToPosition(cue));
        secondary.push(move);
      }
      const menu = document.createElement("details");
      menu.className = "sc008-row-more";
      const title = document.createElement("summary");
      title.textContent = tr("المزيد", "More");
      menu.append(title);
      const buttons = document.createElement("div");
      buttons.className = "sc008-row-more-actions";
      secondary.forEach((button) => buttons.append(button));
      menu.append(buttons);
      actions.append(menu);
    });
  }

  const previousRender = renderCues;
  renderCues = async function sc008RenderCues(...args) {
    await previousRender(...args);
    enhanceCueList();
  };
})();
