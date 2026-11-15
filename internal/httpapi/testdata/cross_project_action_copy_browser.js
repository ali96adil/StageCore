"use strict";

(async () => {
  const requestTrace = [];
  const confirmMessages = [];
  let targetMapLayout = null;
  let targetMapDebug = null;
  window.confirm = (message) => { confirmMessages.push(String(message)); return true; };
  const browserFetch = window.fetch.bind(window);
  window.fetch = async (...args) => {
    const response = await browserFetch(...args);
    requestTrace.push(`${args[0]} → ${response.status}`);
    return response;
  };
  const report = async (value) => {
    await fetch("/cross-project-copy-result", {
      method: "POST", headers: {"Content-Type": "application/json"}, body: JSON.stringify(value),
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

  try {
    if (document.readyState === "loading") {
      await new Promise((resolve) => document.addEventListener("DOMContentLoaded", resolve, {once: true}));
    }
    const configResponse = await fetch("/cross-project-copy-config", {cache: "no-store"});
    if (!configResponse.ok) throw new Error(`test configuration returned HTTP ${configResponse.status}`);
    const config = await configResponse.json();
    state.user = {username: "owner", role: "OWNER"};
    state.csrf = config.csrf;
    state.project = {project_id: config.source, name: "Cross Project Copy Source"};
    sessionStorage.setItem("stagecore_csrf", state.csrf);
    showApp();
    await navigate("cues");

    const sourceButton = document.querySelector(`.cue-edit[data-id="${config.source_cue}"]`);
    if (!sourceButton) throw new Error("source Mixed Cue edit control is missing");
    sourceButton.click();
    const dialog = document.getElementById("cueDialog");
    await waitFor(() => dialog.open && document.querySelectorAll("#actionsEditor .action-editor").length === 2,
      "opening the two-Action source Mixed Cue");
    const authoringFields = () => [...document.querySelectorAll("#cueForm input, #cueForm select, #cueForm textarea")]
      .filter((field) => !field.matches(".sc008-action-select"))
      .map((field) => [field.id || field.className, field.value, field.type === "checkbox" ? field.checked : null]);
    const sourceBaseline = JSON.stringify(authoringFields());
    const sourceCards = [...document.querySelectorAll("#actionsEditor .action-editor")];
    const sourceActionIDs = sourceCards.map((card) => card.querySelector(".action-id").value);
    const sourceTargets = sourceCards.map((card) => card.querySelector(".action-target").value);
    const sourceModes = sourceCards.map((card) => card.querySelector(".action-mode").value);
    const sourceCapabilities = sourceCards.map((card) => card.querySelector(".action-capability").value);
    for (const card of sourceCards) {
      const checkbox = card.querySelector(".sc008-action-select");
      checkbox.checked = true;
      checkbox.dispatchEvent(new Event("change", {bubbles: true}));
    }
    const copySelected = document.getElementById("sc008CopySelectedActions");
    if (!copySelected || copySelected.disabled) throw new Error("copy-selected command did not enable for both Actions");
    copySelected.click();
    if (JSON.stringify(authoringFields()) !== sourceBaseline) {
      throw new Error(`clipboard selection mutated Cue editor fields: ${JSON.stringify({sourceBaseline, current: authoringFields()})}`);
    }
    if (confirmMessages.length !== 0) throw new Error(`clipboard selection triggered an unsaved-edit confirmation: ${confirmMessages.join(" | ")}`);
    const presetPanel = document.getElementById("sc009ActionPresets");
    if (!presetPanel) throw new Error("Cue Action preset panel is missing");
    const presetFields = [...presetPanel.querySelectorAll("input")].filter((field) => field.type !== "search");
    if (presetFields.length !== 2) throw new Error(`preset fields=${presetFields.length}, want name and description`);
    const [presetName, presetDescription] = presetFields;
    const sendPresetKeys = async (keys) => fetch("/cross-project-copy-preset-keyboard-ready", {
      method: "POST", headers: {"Content-Type": "application/json"}, body: JSON.stringify({keys}),
    });
    presetPanel.open = true;
    try {
      await waitFor(() => requestTrace.some((request) =>
        request.includes(`/api/v1/projects/${config.source}/cue-action-presets`)), "loading source Project Action presets");
    } catch (error) {
      throw new Error(`${error}; source=${config.source}; state_project=${state.project?.project_id}; panel_open=${presetPanel.open}; requests=${JSON.stringify(requestTrace)}`);
    }
    presetName.value = "Reusable mixed pair";
    presetName.dispatchEvent(new Event("input", {bubbles: true}));
    presetDescription.value = "Reusable lighting and OSC";
    presetDescription.dispatchEvent(new Event("input", {bubbles: true}));
    const savePreset = presetPanel.querySelector("button");
    presetName.focus();
    const presetTabs = await sendPresetKeys(["Tab", "Tab"]);
    if (!presetTabs.ok) throw new Error(`native preset Tab traversal returned HTTP ${presetTabs.status}`);
    if (document.activeElement !== savePreset) {
      throw new Error(`two native Tabs from the preset name did not focus Save: ${document.activeElement?.outerHTML?.slice(0, 180)}`);
    }
    const presetEnter = await sendPresetKeys(["Enter"]);
    if (!presetEnter.ok) throw new Error(`native preset Enter returned HTTP ${presetEnter.status}`);
    await waitFor(() => presetPanel.querySelectorAll("li").length === 1, "saving and rendering a Mixed Action preset");
    const presetRow = presetPanel.querySelector("li");
    const presetSearch = presetPanel.querySelector("input[type=search]");
    presetSearch.value = "missing-preset-query";
    presetSearch.dispatchEvent(new Event("input", {bubbles: true}));
    if (!presetRow.hidden) throw new Error("Action preset search did not hide a nonmatching row");
    presetSearch.value = "SOURCE-OSC";
    presetSearch.dispatchEvent(new Event("input", {bubbles: true}));
    if (presetRow.hidden) throw new Error("Action preset search did not match a nested target");
    const presetPreview = presetRow.querySelector("pre");
    if (!presetPreview?.textContent.includes("/source/open") || !presetPreview.textContent.includes("SOURCE-LIGHT")) {
      throw new Error(`Mixed Action preset preview lost saved content: ${presetPreview?.textContent}`);
    }
    const previewDetails = [...presetRow.querySelectorAll("details")].find((details) => details.contains(presetPreview));
    const previewSummary = previewDetails?.querySelector("summary");
    const insertPreset = presetRow.querySelector("button");
    if (!previewSummary || !insertPreset) throw new Error("Action preset keyboard preview or insert control is missing");
    previewSummary.focus();
    const previewEnter = await sendPresetKeys(["Enter"]);
    if (!previewEnter.ok) throw new Error(`native preset preview Enter returned HTTP ${previewEnter.status}`);
    await waitFor(() => previewDetails.open, "opening the preset preview with Enter");
    const insertTab = await sendPresetKeys(["Tab"]);
    if (!insertTab.ok) throw new Error(`native preset insert Tab returned HTTP ${insertTab.status}`);
    if (document.activeElement !== insertPreset) {
      throw new Error(`Tab from preset preview did not focus Insert: ${document.activeElement?.outerHTML?.slice(0, 180)}`);
    }
    const insertEnter = await sendPresetKeys(["Enter"]);
    if (!insertEnter.ok) throw new Error(`native preset insert Enter returned HTTP ${insertEnter.status}`);
    await waitFor(() => document.querySelectorAll("#actionsEditor .action-editor").length === 4,
      "inserting both preset Actions into the current Draft Cue");
    const presetCopies = [...document.querySelectorAll("#actionsEditor .action-editor")].slice(2);
    if (presetCopies.some((card) => !card.classList.contains("sc008-action-card") ||
        !card.querySelector(".sc008-action-select") || card.querySelector(".action-id").value !== "") ||
        JSON.stringify(presetCopies.map((card) => card.querySelector(".action-target").value)) !== JSON.stringify(sourceTargets) ||
        JSON.stringify(presetCopies.map((card) => card.querySelector(".action-capability").value)) !== JSON.stringify(sourceCapabilities) ||
        JSON.stringify(presetCopies.map((card) => card.querySelector(".action-mode").value)) !== JSON.stringify(sourceModes) ||
        presetCopies.some((card, index) => JSON.stringify(JSON.parse(card.querySelector(".action-parameters").value)) !==
          JSON.stringify(JSON.parse(sourceCards[index].querySelector(".action-parameters").value)))) {
      throw new Error(`preset insertion did not create fresh, fully enhanced Mixed Actions: ${JSON.stringify({
        classes: presetCopies.map((card) => card.className),
        targets: presetCopies.map((card) => card.querySelector(".action-target").value),
        capabilities: presetCopies.map((card) => card.querySelector(".action-capability").value),
        modes: presetCopies.map((card) => card.querySelector(".action-mode").value),
        action_ids: presetCopies.map((card) => card.querySelector(".action-id").value),
      })}`);
    }
    document.getElementById("cancelCueButton").click();
    await waitFor(() => !dialog.open, "discarding the unsaved preset insertion");
    if (confirmMessages.length !== 1) throw new Error(`preset insertion should trigger one discard confirmation: ${confirmMessages.join(" | ")}`);

    sourceButton.click();
    await waitFor(() => dialog.open, "reopening the source Cue for an unsaved-edit guard check");
    const cueName = document.getElementById("cueName");
    cueName.value = `${cueName.value} unsaved`;
    cueName.dispatchEvent(new Event("input", {bubbles: true}));
    document.getElementById("cancelCueButton").click();
    await waitFor(() => !dialog.open, "closing after accepting the unsaved-edit warning");
    if (confirmMessages.length !== 2) throw new Error(`Cue edit and preset insertion should each trigger a discard confirmation: ${confirmMessages.join(" | ")}`);

    document.getElementById("projectsNav").click();
    await waitFor(() => state.page === "projects" &&
      document.querySelector(`.open-project[data-project-id="${config.destination}"]`), "loading Projects");
    document.querySelector(`.open-project[data-project-id="${config.destination}"]`).click();
    await waitFor(() => state.project?.project_id === config.destination && state.page === "dashboard",
      "opening the destination Project");
    await navigate("cues");
    const destinationButton = document.querySelector(`.cue-edit[data-id="${config.destination_cue}"]`);
    if (!destinationButton) throw new Error("destination Draft Cue edit control is missing");
    destinationButton.click();
    await waitFor(() => dialog.open, "opening the destination Draft Cue");
    const paste = document.getElementById("sc008PasteAction");
    if (!paste || paste.disabled) throw new Error("same-session Action clipboard did not survive Project navigation");
    paste.click();
    await waitFor(() => document.querySelectorAll("#actionsEditor .action-editor").length === 2,
      "pasting both Mixed Actions into the destination Draft");
    const pastedCards = [...document.querySelectorAll("#actionsEditor .action-editor")];
    const pastedTargets = pastedCards.map((card) => card.querySelector(".action-target").value);
    const pastedActionIDs = pastedCards.map((card) => card.querySelector(".action-id").value);
    if (JSON.stringify(pastedTargets) !== JSON.stringify(sourceTargets) || pastedActionIDs.some(Boolean)) {
      throw new Error(`paste changed source fields or retained identities: ${JSON.stringify({pastedTargets, pastedActionIDs})}`);
    }
    const warning = document.getElementById("globalMessage").innerText;
    if (!/(not configured here|غير معرفة هنا)/i.test(warning) || !/(remap|أعد تعيينها)/i.test(warning)) {
      throw new Error(`missing-target warning was not shown: ${warning}`);
    }

    const mapButton = document.getElementById("sc008MapCopiedTargets");
    if (!mapButton || mapButton.disabled) throw new Error("bulk target mapping did not enable for copied Actions");
    mapButton.click();
    const mapDialog = document.getElementById("sc008TargetMapDialog");
    await waitFor(() => mapDialog?.open, "opening bulk target mapping dialog");
    await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
    const mapApply = mapDialog.querySelector(".button.primary");
    const measureMap = async (direction) => {
      document.documentElement.dir = direction;
      mapApply.scrollIntoView({block: "nearest", inline: "nearest"});
      await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
      const bounds = mapApply.getBoundingClientRect();
      const dialogBounds = mapDialog.getBoundingClientRect();
      return {
        direction: getComputedStyle(mapDialog).direction,
        page_width: Math.max(document.documentElement.scrollWidth, document.body.scrollWidth),
        dialog_overflow: mapDialog.scrollWidth > mapDialog.clientWidth + 1,
        apply_visible: bounds.width > 0 && bounds.left >= -1 && bounds.right <= window.innerWidth + 1 &&
          bounds.top >= -1 && bounds.bottom <= window.innerHeight + 1 &&
          bounds.top >= dialogBounds.top + 1 && bounds.bottom <= dialogBounds.bottom - 1 &&
          dialogBounds.left >= 0 && dialogBounds.right <= window.innerWidth + 1,
      };
    };
    const rtlLayout = await measureMap("rtl");
    const ltrLayout = await measureMap("ltr");
    document.documentElement.dir = "rtl";
    await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
    targetMapLayout = {
      zoom_percent: Math.round(window.devicePixelRatio * 100),
      viewport_width: window.innerWidth,
      page_width: rtlLayout.page_width,
      dialog_overflow: rtlLayout.dialog_overflow,
      apply_visible: rtlLayout.apply_visible,
      direction: rtlLayout.direction,
      ltr_direction: ltrLayout.direction,
      ltr_page_width: ltrLayout.page_width,
      ltr_dialog_overflow: ltrLayout.dialog_overflow,
      ltr_apply_visible: ltrLayout.apply_visible,
    };
    if (targetMapLayout.zoom_percent !== 200 || targetMapLayout.direction !== "rtl" || targetMapLayout.viewport_width !== 720 ||
        targetMapLayout.page_width > 720 || targetMapLayout.dialog_overflow || !targetMapLayout.apply_visible ||
        targetMapLayout.ltr_direction !== "ltr" || targetMapLayout.ltr_page_width > 720 ||
        targetMapLayout.ltr_dialog_overflow || !targetMapLayout.ltr_apply_visible) {
      throw new Error(`200% cross-Project target-map RTL/LTR layout failed: ${JSON.stringify(targetMapLayout)}`);
    }
    const targetSelects = [...mapDialog.querySelectorAll("select[aria-label]")];
    if (targetSelects.length !== 2) throw new Error(`target map rows=${targetSelects.length}, want one per source target`);
    const mapping = {"SOURCE-LIGHT": "DEST-LIGHT", "SOURCE-OSC": "DEST-OSC"};
    for (const select of targetSelects) {
      const sourceTarget = Object.keys(mapping).find((source) => select.getAttribute("aria-label").includes(source));
      if (!sourceTarget || ![...select.options].some((option) => option.value === mapping[sourceTarget])) {
        throw new Error(`target map lacks configured destination for ${select.getAttribute("aria-label")}`);
      }
    }
    if (document.activeElement !== targetSelects[0]) throw new Error("opening target map did not focus its first destination selector");
    const keyboardReady = await fetch("/cross-project-copy-map-ready", {method: "POST"});
    if (!keyboardReady.ok) throw new Error(`native keyboard handshake returned HTTP ${keyboardReady.status}`);
    await new Promise((resolve) => setTimeout(resolve, 100));
    if (!mapDialog.open || targetSelects.some((select) => select.value !== "")) {
      throw new Error("Enter on an incomplete target selector closed or changed the mapping dialog");
    }
    for (const select of targetSelects) {
      const sourceTarget = Object.keys(mapping).find((source) => select.getAttribute("aria-label").includes(source));
      const destination = mapping[sourceTarget];
      if (!sourceTarget) throw new Error("target map row does not match a copied source target");
      select.value = destination;
      select.dispatchEvent(new Event("change", {bubbles: true}));
    }
    mapApply.scrollIntoView({block: "nearest", inline: "nearest"});
    await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
    const touchBounds = mapApply.getBoundingClientRect();
    if (touchBounds.left < 0 || touchBounds.top < 0 || touchBounds.right > 720 || touchBounds.bottom > 450) {
      throw new Error(`target-map Apply control is outside the emulated 720x450 touch viewport: ${touchBounds.toJSON()}`);
    }
    const touchReady = await fetch("/cross-project-copy-touch-ready", {
      method: "POST", headers: {"Content-Type": "application/json"},
      body: JSON.stringify({x: touchBounds.left + touchBounds.width / 2, y: touchBounds.top + touchBounds.height / 2}),
    });
    if (!touchReady.ok) throw new Error(`emulated target-map Apply touch returned HTTP ${touchReady.status}`);
    await waitFor(() => !mapDialog.open && JSON.stringify([...document.querySelectorAll("#actionsEditor .action-target")]
      .map((input) => input.value)) === JSON.stringify(["DEST-LIGHT", "DEST-OSC"]), "touch-applied Draft target mapping");
    const touchApplied = !mapDialog.open;
    targetMapDebug = {
      dialog_open: mapDialog.open,
      selected_targets: targetSelects.map((select) => select.value),
      touch_applied: touchApplied,
      action_targets: [...document.querySelectorAll("#actionsEditor .action-target")].map((input) => input.value),
      focused_control: document.activeElement?.outerHTML?.slice(0, 180) || "none",
    };
    if (mapDialog.open || JSON.stringify(targetMapDebug.action_targets) !== JSON.stringify(["DEST-LIGHT", "DEST-OSC"])) {
      throw new Error(`touch did not apply the Draft-only target map: ${JSON.stringify(targetMapDebug)}`);
    }
    if (targetSelects[0].value !== "DEST-LIGHT" || targetSelects[1].value !== "DEST-OSC") {
      throw new Error(`target-map fixture selected unexpected targets: ${targetSelects.map((select) => select.value).join(",")}`);
    }

    document.getElementById("cueForm").requestSubmit();
    await waitFor(() => !dialog.open && state.cues.find((cue) => cue.cue_id === config.destination_cue)?.actions?.length === 2,
      "saving mapped Actions to the destination Draft");
    const savedCue = state.cues.find((cue) => cue.cue_id === config.destination_cue);
    const unsafeCall = requestTrace.some((request) =>
      /\/publish(?:\?|\s|$)|\/runtime\/(?:go|jump|stop)(?:\?|\s|$)|emergency-blackout/i.test(request));
    const configurationCopied = requestTrace.some((request) => /\/stage-devices\/(?:assignments|aliases|configuration)/i.test(request));
    if (!savedCue || unsafeCall || configurationCopied || savedCue.actions.some((action, index) =>
      action.target_ref !== mapping[sourceTargets[index]] || action.action_id === sourceActionIDs[index] ||
      action.execution_mode !== sourceModes[index] || action.capability_key !== sourceCapabilities[index])) {
      throw new Error(`saved destination Draft crossed a safety/identity boundary: ${JSON.stringify({savedCue, unsafeCall, configurationCopied})}`);
    }
    await report({
      status: "PASS", targets: savedCue.actions.map((action) => action.target_ref),
      action_ids: savedCue.actions.map((action) => action.action_id), unsafe_call: unsafeCall,
      configuration_copied: configurationCopied, touch_applied: touchApplied,
      ...targetMapLayout, requests: requestTrace,
    });
  } catch (error) {
    await report({status: "FAIL", error: String(error.stack || error), target_map_debug: targetMapDebug, requests: requestTrace});
  }
})();
