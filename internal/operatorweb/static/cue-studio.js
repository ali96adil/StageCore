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
      detail = Object.keys(params.levels || {}).join(", ") || params.command_type || "";
    } else if (kind === "tablet" || kind === "laser" || kind === "visual") {
      detail = params.command_type || params.action || "";
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

  function duplicateCard(card) {
    try {
      // Translate visual OSC/MIDI fields into the existing canonical payload.
      f002SyncActionCard(card);
      const read = (selector) => cardField(card, selector)?.value;
      const copy = {
        action_id: "",
        target_ref: read(".action-target") || "",
        capability_key: read(".action-capability") || "",
        parameters: JSON.parse(read(".action-parameters") || "{}"),
        timeout_policy: JSON.parse(read(".action-timeout") || "{}"),
        error_policy: JSON.parse(read(".action-error") || "{}"),
        execution_mode: read(".action-mode") || "SEQUENTIAL",
        priority_class: read(".action-priority") || "P1",
        enabled: cardField(card, ".action-enabled")?.checked ?? true,
      };
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
    make("sc008-copy", tr("نسخ", "Copy"), () => duplicateCard(card));
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
    card.addEventListener("input", () => updateCard(card));
    card.addEventListener("change", () => updateCard(card));
    expandCard(card, !existing);
    updateCard(card);
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
    const eligible = (state.f002Targets || []).filter((target) => {
      const name = String(target.logical_name || "").toLowerCase();
      return kind === "midi" ? /audio|ableton|midi/.test(name) : /osc|video|vdmx/.test(name);
    });
    const targetName = eligible.length === 1 ? eligible[0].logical_name : "";
    const payload = kind === "osc" ? { address: "", arguments: [] }
      : kind === "visual" ? { contract_version: 1, layer_id: "" } : {};
    addActionEditor({ capability_key: type, target_ref: targetName, parameters: payload });
    const added = actionCards().at(-1);
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
    const visualConfigured = (state.f002MachineRoles || []).some((role) =>
      !role.retired && (role.required_capabilities || []).some((capability) =>
        String(capability).startsWith("visual.")
      )
    );
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
      }
    }, true);
    pick("cueDialog")?.addEventListener("cancel", (event) => {
      if (initialFormState !== formSignature() &&
          !window.confirm(tr("تجاهل التغييرات غير المحفوظة؟", "Discard unsaved changes?"))) {
        event.preventDefault();
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
    const checkCard = Array.from(content.querySelectorAll("section.card"))
      .find((node) => node.querySelector("h3")?.textContent.trim() === "Cue Check");
    if (checkCard && !checkCard.closest(".sc008-cue-check")) {
      const disclosure = document.createElement("details");
      disclosure.className = "sc008-cue-check";
      const summary = document.createElement("summary");
      summary.textContent = tr("اختبار الكيوهات المنشورة وإجراءات الإيقاف",
        "Published Cue Check & stop controls");
      checkCard.before(disclosure);
      disclosure.append(summary, checkCard);
      // Never conceal STOP/BLACKOUT while an operational Session is active.
      disclosure.open = !pick("cueCheckStopButton")?.disabled;
    }
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
      const types = Array.from(classify(cue)).slice(0, 3);
      const cell = row.children[5];
      if (!cell || !types.length) return;
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
