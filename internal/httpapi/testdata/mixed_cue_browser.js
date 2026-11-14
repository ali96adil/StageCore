"use strict";

(async () => {
  const requestTrace = [];
  const browserFetch = window.fetch.bind(window);
  window.fetch = async (...args) => {
    const response = await browserFetch(...args);
    requestTrace.push(`${args[0]} → ${response.status}`);
    return response;
  };
  const report = async (value) => {
    await fetch("/mixed-cue-result", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: value,
    });
    window.close();
  };
  const waitFor = async (predicate, label) => {
    const deadline = Date.now() + 10000;
    while (Date.now() < deadline) {
      if (predicate()) return;
      await new Promise((resolve) => setTimeout(resolve, 20));
    }
    throw new Error(`Timed out waiting for ${label}`);
  };
  const waitForStable = async (predicate, label) => {
    const deadline = Date.now() + 10000;
    while (Date.now() < deadline) {
      if (predicate()) {
        await new Promise((resolve) => setTimeout(resolve, 100));
        if (predicate()) return;
      }
      await new Promise((resolve) => setTimeout(resolve, 20));
    }
    throw new Error(`Timed out waiting for stable ${label}`);
  };
  const setValue = (element, value) => {
    element.value = value;
    element.dispatchEvent(new Event("input", { bubbles: true }));
    element.dispatchEvent(new Event("change", { bubbles: true }));
  };
  const rgb = (value) => {
    const match = String(value).match(/rgba?\(([^)]+)\)/i);
    if (!match) return null;
    const parts = match[1].replace(/\s*\/\s*/, " ").split(/[\s,]+/).filter(Boolean);
    if (parts.length < 3) return null;
    const channel = (part) => part.endsWith("%") ? parseFloat(part) * 2.55 : parseFloat(part);
    return { channels: parts.slice(0, 3).map(channel), alpha: parts[3] ? parseFloat(parts[3]) : 1 };
  };
  const compositedBackground = (element) => {
    const ancestors = [];
    for (let current = element; current; current = current.parentElement) ancestors.push(current);
    let background = [0, 0, 0];
    for (const current of ancestors.reverse()) {
      const foreground = rgb(getComputedStyle(current).backgroundColor);
      if (!foreground) continue;
      background = foreground.channels.map((value, index) => value * foreground.alpha + background[index] * (1 - foreground.alpha));
    }
    return background;
  };
  const luminance = (channels) => channels.map((value) => {
    const normalized = value / 255;
    return normalized <= 0.04045 ? normalized / 12.92 : ((normalized + 0.055) / 1.055) ** 2.4;
  }).reduce((sum, value, index) => sum + value * [0.2126, 0.7152, 0.0722][index], 0);
  const contrastRatio = (first, second) => {
    const values = [luminance(first), luminance(second)].sort((a, b) => b - a);
    return (values[0] + 0.05) / (values[1] + 0.05);
  };
  const markerContrasts = () => Object.fromEntries(["blue", "green", "amber", "purple"].map((color) => {
    const marker = document.querySelector(`#content .cue-label-cell[data-cue-color="${color}"]`);
    const shadow = marker && rgb(getComputedStyle(marker).boxShadow);
    return [color, marker && shadow ? contrastRatio(shadow.channels, compositedBackground(marker)) : 0];
  }));
  const tabToCueLabel = async () => {
    const closeButton = document.getElementById("closeCueDialog");
    const labelInput = document.getElementById("cueLabel");
    if (!closeButton || !labelInput || !document.getElementById("cueDialog")?.open) {
      throw new Error("Cue editor is not ready for keyboard focus acceptance");
    }
    closeButton.focus();
    if (document.activeElement !== closeButton) throw new Error("Could not focus the Cue editor's first control");
    const response = await fetch("/mixed-cue-focus-ready", { method: "POST" });
    if (!response.ok) throw new Error(`native Tab key request returned HTTP ${response.status}`);
    return labelInput;
  };

  try {
    if (document.readyState === "loading") {
      await new Promise((resolve) => document.addEventListener("DOMContentLoaded", resolve, { once: true }));
    }
    const configResponse = await fetch("/mixed-cue-config", { cache: "no-store" });
    if (!configResponse.ok) throw new Error(`test configuration returned HTTP ${configResponse.status}`);
    const config = await configResponse.json();
    state.user = { username: "owner", role: "OWNER" };
    state.csrf = config.csrf;
    state.project = { project_id: config.project, name: config.name };
    sessionStorage.setItem("stagecore_csrf", state.csrf);
    if (!sessionStorage.getItem("stagecore_mixed_cue_ltr_evidence")) f016SetMode("dark");
    showApp();
    await navigate("cues");

    const ltrEvidence = sessionStorage.getItem("stagecore_mixed_cue_ltr_evidence");
    if (ltrEvidence) {
      const previous = JSON.parse(ltrEvidence);
      const parentID = previous.ids[2];
      await waitFor(() => document.documentElement.dir === "ltr" && document.documentElement.dataset.theme === "light" &&
        !!document.querySelector(`#content tr[data-cue-id="${parentID}"] .cue-label-cell`),
      "reloading the Draft Cue list in English LTR and light appearance");
      const labelCell = document.querySelector(`#content tr[data-cue-id="${parentID}"] .cue-label-cell`);
      const plainCell = document.querySelector(`#content tr[data-cue-id="${parentID}"] td:not(.cue-label-cell)`);
      const editButton = document.querySelector(`#content tr[data-cue-id="${parentID}"] .cue-edit`);
      editButton?.click();
      await waitFor(() => document.getElementById("cueDialog")?.open, "opening Cue editor for LTR focus acceptance");
      const focusTarget = await tabToCueLabel();
      const style = labelCell ? getComputedStyle(labelCell) : null;
      const focusStyle = focusTarget ? getComputedStyle(focusTarget) : null;
      const labelText = labelCell?.innerText.trim() || "";
      const markerVisible = !!style && style.boxShadow !== "none";
      const paletteVisible = ["blue", "green", "amber", "purple"].every((color) => {
        const marker = document.querySelector(`#content .cue-label-cell[data-cue-color="${color}"]`);
        return !!marker && getComputedStyle(marker).boxShadow !== "none";
      });
      const readable = !!style && !!plainCell && style.color === getComputedStyle(plainCell).color;
      const colorContrast = markerContrasts();
      const textContrast = style && rgb(style.color) ? contrastRatio(rgb(style.color).channels, compositedBackground(labelCell)) : 0;
      const focusVisible = !!focusTarget && document.activeElement === focusTarget && focusTarget.matches(":focus-visible") &&
        !!focusStyle && focusStyle.outlineStyle !== "none" && focusStyle.outlineWidth === "3px";
      const focusOutline = focusStyle && rgb(focusStyle.outlineColor);
      const focusContrast = focusOutline ? contrastRatio(focusOutline.channels, compositedBackground(focusTarget.parentElement)) : 0;
      const overflow = document.documentElement.scrollWidth > window.innerWidth;
      if (labelText !== previous.quick_edited_label || !markerVisible || !paletteVisible || !readable ||
        Object.values(colorContrast).some((value) => value < 3) || textContrast < 4.5 || !focusVisible || focusContrast < 3 || overflow) {
        throw new Error(`Cue label/color LTR acceptance failed: label=${labelText}, marker=${markerVisible}, palette=${paletteVisible}, readable=${readable}, contrast=${JSON.stringify(colorContrast)}, text=${textContrast}, focus=${focusVisible}, focusContrast=${focusContrast}, overflow=${overflow}`);
      }
      sessionStorage.removeItem("stagecore_mixed_cue_ltr_evidence");
      await report(JSON.stringify({ ...previous, ltr_label: labelText, ltr_palette_visible: paletteVisible,
        ltr_label_readable: readable, ltr_marker_contrast: colorContrast, ltr_text_contrast: textContrast,
        ltr_focus_visible: focusVisible, ltr_focus_contrast: focusContrast, ltr_page_overflow: overflow,
        theme_qualification: { rtl: previous.theme, ltr: document.documentElement.dataset.theme } }));
      return;
    }

    if (!document.getElementById("createCueButton")) {
      const createDraftButton = document.getElementById("createDraftButton");
      if (createDraftButton) {
        createDraftButton.click();
        await waitFor(() => !!document.getElementById("createCueButton"), "Draft creation");
      }
    }

    const createdIDs = [];
    const createCue = async ({ label, name, linkedCueID = "", notes = "", actions }) => {
      const createButton = document.getElementById("createCueButton");
      if (!createButton) throw new Error(`Create Cue button is missing (page=${state.page}, revision=${state.currentRevision?.status || "unknown"}, role=${state.user?.role || "unknown"}, message=${document.getElementById("globalMessage")?.innerText || ""}, content=${document.getElementById("content")?.innerText || ""})`);
      createButton.click();
      const dialog = document.getElementById("cueDialog");
      if (!dialog.open) throw new Error("Cue editor dialog did not open");
      setValue(document.getElementById("cueLabel"), label);
      setValue(document.getElementById("cueName"), name);
      setValue(document.getElementById("cueColor"), label === "3" ? "blue" : label === "2" ? "amber" : "green");
      setValue(document.getElementById("cueStartDelaySeconds"), "0.125");
      setValue(document.getElementById("cueNotes"), notes);

      if (linkedCueID) {
        const link = document.querySelector(`#linkedCuesEditor .linked-cue-checkbox[value="${linkedCueID}"]`);
        if (!link || link.disabled) throw new Error(`Nested child Cue ${linkedCueID} is unavailable`);
        link.checked = true;
        link.dispatchEvent(new Event("change", { bubbles: true }));
      }

      for (const action of actions) {
        const addActionButton = document.getElementById("addActionButton");
        const previousCount = document.querySelectorAll("#actionsEditor .action-editor").length;
        addActionButton.click();
        const actionType = action.capability === "osc.send" ? "osc" : action.capability === "midi.send" ? "midi" : "advanced";
        const choice = document.querySelector(`#sc008Picker .sc008-choice[data-sc008-kind="${actionType}"]`);
        if (!choice) throw new Error(`Cue Studio action choice ${actionType} is missing`);
        choice.click();
        const cards = [...document.querySelectorAll("#actionsEditor .action-editor")];
        const card = cards[cards.length - 1];
        if (cards.length !== previousCount + 1 || !card) throw new Error(`Cue Studio did not create the selected ${actionType} Action`);
        const kindSelect = card.querySelector(".f002-action-kind");
        if (action.capability !== "osc.send" && action.capability !== "midi.send" && kindSelect) {
          kindSelect.value = "advanced";
          kindSelect.dispatchEvent(new Event("change", { bubbles: true }));
        }
        setValue(card.querySelector(".action-target"), "SIM");
        setValue(card.querySelector(".action-capability"), action.capability);
        setValue(card.querySelector(".action-mode"), action.mode);
        setValue(card.querySelector(".action-priority"), action.priority || "P1");
        if (action.capability === "osc.send") {
          setValue(card.querySelector(".f002-osc-address"), action.parameters.address);
        } else if (action.capability === "midi.send") {
          setValue(card.querySelector(".f002-midi-destination-mode"), "name");
          setValue(card.querySelector(".f002-midi-destination-name"), action.parameters.destination_name);
          setValue(card.querySelector(".f002-midi-message"), "note_on");
          setValue(card.querySelector(".f002-midi-channel"), String(action.parameters.bytes[0] - 143));
          setValue(card.querySelector(".f002-midi-data1"), String(action.parameters.bytes[1]));
          setValue(card.querySelector(".f002-midi-data2"), String(action.parameters.bytes[2]));
        } else {
          setValue(card.querySelector(".action-parameters"), JSON.stringify(action.parameters));
        }
        setValue(card.querySelector(".f002-timeout-kind"), "advanced");
        setValue(card.querySelector(".f002-error-kind"), "advanced");
        setValue(card.querySelector(".action-timeout"), JSON.stringify(action.timeout));
        setValue(card.querySelector(".action-error"), JSON.stringify({ on_error: "CONTINUE" }));
      }

      document.getElementById("cueForm").requestSubmit();
      await waitFor(() => !dialog.open && state.cues.some((cue) => cue.name === name), `Draft save for ${name}`);
      const cue = state.cues.find((item) => item.name === name);
      if (!cue || cue.actions?.length !== 2) throw new Error(`${name} did not persist as a two-Action Mixed Cue`);
      createdIDs.push(cue.cue_id);
      return cue.cue_id;
    };

    const leafID = await createCue({
      label: "1", name: "Lighting and Tablet Leaf", notes: "Check tablet media clip on SIM.", actions: [
        { capability: "lighting.channels.fade", mode: "PARALLEL", parameters: { levels: { warm: 50 }, fade_ms: 300 }, timeout: { start_delay_ms: 20 } },
        { capability: "tablet.media.play", mode: "SEQUENTIAL", parameters: { media_id: "clip-sim-1" }, timeout: {} },
      ],
    });
    const middleID = await createCue({
      label: "2", name: "OSC and MIDI Middle", linkedCueID: leafID,
      notes: "Simulate OSC barrier before MIDI.", actions: [
        { capability: "osc.send", mode: "PARALLEL_BARRIER", parameters: { address: "/sim/open", arguments: [{ type: "int32", value: 7 }] }, timeout: { start_delay_ms: 40 } },
        { capability: "midi.send", mode: "SEQUENTIAL", priority: "P2", parameters: { destination_name: "SIM MIDI", bytes: [144, 60, 100] }, timeout: {} },
      ],
    });
    const parentID = await createCue({
      label: "3", name: "Native Visual and MIDI Parent", linkedCueID: middleID,
      notes: "Initial parent note before quick edit.", actions: [
        { capability: "visual.scene.activate", mode: "PARALLEL", parameters: { scene_id: "opening-sim" }, timeout: {} },
        { capability: "midi.send", mode: "SEQUENTIAL", parameters: { destination_name: "SIM MIDI", bytes: [145, 64, 95] }, timeout: { start_delay_ms: 15 } },
      ],
    });

    if (createdIDs.length !== 3 || createdIDs[2] !== parentID) throw new Error("nested Mixed Cue IDs were not retained");
    const linkedChainRows = () => [...document.querySelectorAll("#content tr[data-cue-id]")]
      .filter((row) => createdIDs.includes(row.dataset.cueId));
    await waitForStable(() => document.querySelectorAll("#content .sc008-type-badge").length >= 6 &&
      linkedChainRows().length === 3 && linkedChainRows().every((row) => row.querySelector(".cue-link-cell")),
    "Cue Studio Mixed Cue badges and linked-chain rows");

    const allCueRows = [...document.querySelectorAll("#content tr[data-cue-id]")];
    const originalCueOrder = allCueRows.map((row) => row.dataset.cueId);
    const visibleCueRows = () => [...document.querySelectorAll("#content tr[data-cue-id]")].filter((row) => !row.hidden);
    if (allCueRows.length !== 123) throw new Error(`Cue list=${allCueRows.length}, want 123`);

    const search = document.getElementById("cueListSearch");
    setValue(search, "Navigation Cue 120");
    await waitFor(() => visibleCueRows().length === 1 && visibleCueRows()[0].innerText.includes("Navigation Cue 120"),
      "searching a named Cue in a list over 100 rows");
    setValue(search, "lighting.channels.fade");
    await waitFor(() => visibleCueRows().length === 3 &&
      visibleCueRows().every((row) => createdIDs.includes(row.dataset.cueId)),
    "searching an Action nested in a three-Cue Mixed chain");
    setValue(search, "");
    const typeFilter = document.getElementById("cueListTypeFilter");
    const linkedFilter = document.getElementById("cueListLinkedFilter");
    typeFilter.value = "mixed";
    typeFilter.dispatchEvent(new Event("change", { bubbles: true }));
    linkedFilter.value = "linked";
    linkedFilter.dispatchEvent(new Event("change", { bubbles: true }));
    await waitFor(() => visibleCueRows().length === 3 &&
      visibleCueRows().every((row) => createdIDs.includes(row.dataset.cueId)),
    "combining Mixed and linked Cue filters");
    document.getElementById("clearCueListFilters").click();
    await waitFor(() => visibleCueRows().length === 123, "restoring the full sequence after clearing filters");
    if (JSON.stringify(visibleCueRows().map((row) => row.dataset.cueId)) !== JSON.stringify(originalCueOrder)) {
      throw new Error("search and filters changed Cue execution order");
    }

    const navigationRows = visibleCueRows();
    if (navigationRows.length !== 123) throw new Error(`Cue navigation list=${navigationRows.length}, want 123`);
    const firstNavigationDetails = navigationRows[0].querySelector("details");
    firstNavigationDetails?.querySelector("summary")?.click();
    await waitFor(() => firstNavigationDetails?.querySelector(".cue-inspection-navigation"),
      "opening the first Cue in the long-list Inspector");
    for (let index = 1; index < navigationRows.length; index++) {
      const current = navigationRows[index - 1];
      const currentNavigation = current.querySelector(".cue-inspection-navigation");
      const nextButton = currentNavigation?.querySelectorAll("button")[1];
      if (!nextButton || nextButton.disabled) throw new Error(`Next Cue control disabled at list position ${index}`);
      nextButton.click();
      const targetRow = navigationRows[index];
      const targetDetails = targetRow.querySelector("details");
      const targetCue = state.cues[index];
      const targetCapability = targetCue?.actions?.[0]?.capability_key || "";
      await waitFor(() => targetDetails?.open && targetDetails.querySelector(".cue-inspection-navigation") &&
        targetDetails.innerText.includes(targetCue?.name || "") && targetCapability &&
        targetDetails.innerText.includes(targetCapability),
      `navigating to Cue ${index + 1} of ${navigationRows.length} with its Action context`);
      const targetSummary = targetDetails.querySelector("summary");
      if (document.activeElement !== targetSummary) throw new Error(`Cue ${index + 1} navigation did not move keyboard focus`);
      const bounds = targetRow.getBoundingClientRect();
      if (bounds.bottom <= 0 || bounds.top >= window.innerHeight) {
        throw new Error(`Cue ${index + 1} was selected outside the viewport after navigation`);
      }
    }
    const lastNavigationButtons = navigationRows.at(-1).querySelectorAll(".cue-inspection-navigation button");
    if (lastNavigationButtons.length !== 2 || !lastNavigationButtons[1].disabled) {
      throw new Error("Next Cue control is not disabled at the end of the long list");
    }

    const parent = state.cues.find((cue) => cue.cue_id === parentID);
    const originalActionIDs = (parent?.actions || []).map((action) => action.action_id);
    if (originalActionIDs.length !== 2 || originalActionIDs.some((id) => !id)) {
      throw new Error("Saved parent Mixed Cue did not retain Action identities before editing");
    }
    const parentEditButton = document.querySelector(`.cue-edit[data-id="${parentID}"]`);
    if (!parentEditButton) throw new Error("Parent Mixed Cue Edit control is missing");
    parentEditButton.click();
    const editor = document.getElementById("cueDialog");
    await waitFor(() => editor.open, "opening parent Mixed Cue for editing");
    if (document.querySelectorAll("#actionsEditor .action-editor").length !== 2) {
      throw new Error("Parent Mixed Cue editor did not reopen both saved Actions");
    }
    const selectedMiddle = document.querySelector(`#linkedCuesEditor .linked-cue-checkbox[value="${middleID}"]`);
    if (!selectedMiddle?.checked) throw new Error("Parent-to-middle Cue link was not restored in the editor");
    const editedName = "Native Visual and MIDI Parent · Browser Edit";
    setValue(document.getElementById("cueName"), editedName);
    document.getElementById("cueForm").requestSubmit();
    await waitFor(() => !editor.open && state.cues.find((cue) => cue.cue_id === parentID)?.name === editedName,
      "saving parent Mixed Cue edit");
    const editedParent = state.cues.find((cue) => cue.cue_id === parentID);
    const editedActionIDs = (editedParent?.actions || []).map((action) => action.action_id);
    if (JSON.stringify(editedActionIDs) !== JSON.stringify(originalActionIDs)) {
      throw new Error("Editing Cue metadata changed existing Action identities");
    }
    await waitFor(() => document.querySelector(`#content [data-cue-id="${parentID}"]`)?.innerText.includes(editedName),
      "rendering saved parent Cue name");
    if (!document.body.innerText.includes("Cue updated in Draft")) {
      throw new Error("Draft edit confirmation was not rendered");
    }

    await waitFor(() => document.querySelector(`#content tr[data-cue-id="${parentID}"] details`),
      "rerendering the parent Cue Inspector after editing");
    const parentRow = document.querySelector(`#content tr[data-cue-id="${parentID}"]`);
    const inspector = parentRow?.querySelector("details");
    if (!inspector) throw new Error("Parent Cue Inspector disclosure is missing");
    inspector.querySelector("summary")?.click();
    await waitFor(() => inspector.open && inspector.querySelector(".cue-inspection-scenario"),
      "opening the parent Cue Inspector");
    const inspectorText = inspector.innerText;
    for (const expected of ["Lighting and Tablet Leaf", "OSC and MIDI Middle", editedName,
      "Check tablet media clip on SIM.", "Simulate OSC barrier before MIDI.",
      "lighting.channels.fade", "tablet.media.play", "osc.send", "midi.send", "visual.scene.activate"]) {
      if (!inspectorText.includes(expected)) throw new Error(`Cue Inspector omitted linked-chain detail: ${expected}`);
    }
    const controls = inspector.querySelectorAll(".cue-inspection-controls select");
    if (controls.length !== 3) throw new Error(`Cue Inspector simulation controls=${controls.length}, want 3`);
    const [scenarioSelect, targetSelect] = controls;
    setValue(scenarioSelect, "offline-target");
    if (targetSelect.hidden) throw new Error("offline-target scenario did not reveal its target selector");
    setValue(targetSelect, "SIM");
    const previewButton = inspector.querySelector(".cue-inspection-controls button");
    if (!previewButton) throw new Error("Cue Inspector simulation preview button is missing");
    const requestsBeforePreview = requestTrace.length;
    previewButton.click();
    const timeline = inspector.querySelector(".cue-inspection-result .cue-inspection-timeline");
    await waitFor(() => timeline?.querySelectorAll("li").length === 6,
      "rendering all nested Mixed Cue Actions in local simulation");
    const outcomes = [...timeline.querySelectorAll("li")].map((item) => item.innerText);
    if (outcomes.some((item) => !item.includes("VIRTUAL_TARGET_OFFLINE")) ||
        outcomes.some((item) => !item.includes("SIMULATED_BLOCKED"))) {
      throw new Error(`offline-target simulation did not mark every Action blocked: ${JSON.stringify(outcomes)}`);
    }
    if (requestTrace.length !== requestsBeforePreview) {
      throw new Error(`Cue Inspector simulation made network requests: ${requestTrace.slice(requestsBeforePreview).join("; ")}`);
    }
    const forbiddenRequests = requestTrace.filter((item) => /\/go(?:\?|\s|$)|\/stop(?:\?|\s|$)|blackout/i.test(item));
    if (forbiddenRequests.length) throw new Error(`browser authoring/inspection attempted runtime commands: ${forbiddenRequests.join("; ")}`);

    const quickEditButton = inspector.querySelector(".sc014-quick-edit-open");
    if (!quickEditButton) throw new Error("Draft Cue Inspector quick-edit control is missing");
    quickEditButton.click();
    const quickEditForm = inspector.querySelector(".sc014-quick-edit-form");
    if (!quickEditForm || quickEditForm.hidden) throw new Error("Cue Inspector quick-edit form did not open");
    const quickEditedLabel = "3E";
    const quickEditedNotes = "Simulate visual readiness before rehearsal.";
    const quickLabelInput = quickEditForm.querySelector("input");
    const quickNotesInput = quickEditForm.querySelector("textarea");
    if (!quickLabelInput?.getAttribute("aria-label") || !quickLabelInput.closest("label") ||
        !quickNotesInput?.getAttribute("aria-label") || !quickNotesInput.closest("label")) {
      throw new Error("Cue Inspector quick-edit fields lack accessible labels");
    }
    setValue(quickLabelInput, quickEditedLabel);
    setValue(quickNotesInput, quickEditedNotes);
    quickEditForm.requestSubmit();
    await waitFor(() => state.cues.find((cue) => cue.cue_id === parentID)?.display_label === quickEditedLabel &&
      state.cues.find((cue) => cue.cue_id === parentID)?.notes_summary === quickEditedNotes,
    "saving quick label and notes edits to the Draft");
    const quickEditedParent = state.cues.find((cue) => cue.cue_id === parentID);
    if (JSON.stringify(quickEditedParent.actions) !== JSON.stringify(parent.actions)) {
      throw new Error("Cue Inspector quick edit changed one or more Mixed Cue Actions");
    }
    await waitFor(() => document.querySelector(`#content tr[data-cue-id="${parentID}"] details`)?.innerText.includes(quickEditedNotes),
      "showing saved quick-edit notes in the Cue Inspector");
    const labelCell = document.querySelector(`#content tr[data-cue-id="${parentID}"] .cue-label-cell`);
    const plainCell = document.querySelector(`#content tr[data-cue-id="${parentID}"] td:not(.cue-label-cell)`);
    const editButton = document.querySelector(`#content tr[data-cue-id="${parentID}"] .cue-edit`);
    editButton?.click();
    await waitFor(() => document.getElementById("cueDialog")?.open, "opening Cue editor for keyboard focus acceptance");
    const focusTarget = await tabToCueLabel();
    const labelStyle = labelCell ? getComputedStyle(labelCell) : null;
    const focusStyle = focusTarget ? getComputedStyle(focusTarget) : null;
    const cueColor = labelCell?.dataset.cueColor || "";
    const cueLabelText = labelCell?.innerText.trim() || "";
    const colorMarkerVisible = !!labelStyle && labelStyle.boxShadow !== "none";
    const paletteVisible = ["blue", "green", "amber", "purple"].every((color) => {
      const marker = document.querySelector(`#content .cue-label-cell[data-cue-color="${color}"]`);
      return !!marker && getComputedStyle(marker).boxShadow !== "none";
    });
    const labelColorReadable = !!labelStyle && !!plainCell && labelStyle.color === getComputedStyle(plainCell).color;
    const colorContrast = markerContrasts();
    const textContrast = labelStyle && rgb(labelStyle.color) ? contrastRatio(rgb(labelStyle.color).channels, compositedBackground(labelCell)) : 0;
    const focusRingVisible = !!focusTarget && document.activeElement === focusTarget && focusTarget.matches(":focus-visible") &&
      !!focusStyle && focusStyle.outlineStyle !== "none" && focusStyle.outlineWidth === "3px";
    const focusOutline = focusStyle && rgb(focusStyle.outlineColor);
    const focusContrast = focusOutline ? contrastRatio(focusOutline.channels, compositedBackground(focusTarget.parentElement)) : 0;
    const pageOverflow = document.documentElement.scrollWidth > window.innerWidth;
    if (cueColor !== "blue" || cueLabelText !== quickEditedLabel || !colorMarkerVisible || !paletteVisible || !labelColorReadable ||
      Object.values(colorContrast).some((value) => value < 3) || textContrast < 4.5 || !focusRingVisible || focusContrast < 3 || pageOverflow) {
      throw new Error(`Cue label/color mobile accessibility failed: color=${cueColor}, label=${cueLabelText}, marker=${colorMarkerVisible}, palette=${paletteVisible}, readable=${labelColorReadable}, contrast=${JSON.stringify(colorContrast)}, text=${textContrast}, focus=${focusRingVisible}, focusContrast=${focusContrast}, overflow=${pageOverflow}`);
    }
    const runtimeRequestsAfterEdit = requestTrace.filter((item) => /\/go(?:\?|\s|$)|\/stop(?:\?|\s|$)|blackout/i.test(item));
    if (!requestTrace.some((item) => item.includes(`/cues/${parentID} → 200`)) || runtimeRequestsAfterEdit.length) {
      throw new Error(`quick metadata edit did not use the Draft update path safely: ${requestTrace.slice(-10).join("; ")}`);
    }
    const firstPass = { status: "PASS", ids: createdIDs, action_ids: editedActionIDs,
      edited_name: editedName, quick_edited_label: quickEditedLabel, quick_edited_notes: quickEditedNotes,
      theme: document.documentElement.dataset.theme,
      cue_color: cueColor, cue_label_text: cueLabelText, color_marker_visible: colorMarkerVisible,
      palette_visible: paletteVisible, label_color_readable: labelColorReadable,
      marker_contrast: colorContrast, text_contrast: textContrast,
      focus_ring_visible: focusRingVisible, focus_contrast: focusContrast, page_overflow: pageOverflow };
    sessionStorage.setItem("stagecore_mixed_cue_ltr_evidence", JSON.stringify(firstPass));
    localStorage.setItem("stagecore_locale", "en");
    f016SetMode("light");
    window.location.reload();
  } catch (error) {
    await report(JSON.stringify({ status: "FAIL", error: error?.stack || String(error), requests: requestTrace.slice(-16) }));
  }
})();
