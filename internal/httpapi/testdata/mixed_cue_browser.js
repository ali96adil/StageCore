"use strict";

(async () => {
  const requestTrace = [];
  let offlineRecoveryRequests = 0;
  let confirmationRequests = 0;
  window.confirm = () => { confirmationRequests++; return true; };
  const browserFetch = window.fetch.bind(window);
  window.fetch = async (...args) => {
    const url = String(args[0]);
    if (localStorage.getItem("stagecore_test_offline_recovery") === "1" && url.includes("/cue-editor-recovery")) {
      offlineRecoveryRequests++;
      requestTrace.push(`${url} → simulated offline`);
      throw new TypeError("simulated offline Cue recovery endpoint");
    }
    const response = await browserFetch(...args);
    requestTrace.push(`${url} → ${response.status}`);
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
  const setViewport = async (width, height, mobile) => {
    const response = await fetch("/mixed-cue-viewport-ready", {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ width, height, mobile }),
    });
    if (!response.ok) throw new Error(`viewport gate failed at ${width}x${height}: ${response.status}`);
    await waitFor(() => window.innerWidth === width, `applying ${width}x${height} viewport`);
  };
  const touchElement = async (element, label) => {
    if (!element) throw new Error(`${label} touch target is missing`);
    element.scrollIntoView({ block: "center" });
    await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
    const bounds = element.getBoundingClientRect();
    const response = await fetch("/mixed-cue-touch-ready", {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ x: bounds.left + bounds.width / 2, y: bounds.top + bounds.height / 2 }),
    });
    if (!response.ok) throw new Error(`${label} native touch failed: ${response.status}, bounds=${JSON.stringify(bounds)}, body=${await response.text()}`);
  };
  const filterKey = async (key) => {
    const response = await fetch("/mixed-cue-filter-key-ready", {
      method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ key }),
    });
    if (!response.ok) throw new Error(`native Cue filter key ${key} failed: ${response.status}`);
  };
  const filterText = async (text) => {
    const response = await fetch("/mixed-cue-filter-key-ready", {
      method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ text }),
    });
    if (!response.ok) throw new Error(`native Cue search text failed: ${response.status}`);
  };
  const exerciseCueFiltersWithKeyboard = async (expectedLinkedIDs, locale) => {
    const allRows = [...document.querySelectorAll("#content tr[data-cue-id]")];
    const originalOrder = allRows.map((row) => row.dataset.cueId);
    const visibleRows = () => [...document.querySelectorAll("#content tr[data-cue-id]")].filter((row) => !row.hidden);
    if (allRows.length !== 123) throw new Error(`${locale} keyboard filter fixture has ${allRows.length} Cues`);
    const search = document.getElementById("cueListSearch");
    search?.focus();
    if (document.activeElement !== search) throw new Error(`${locale} Cue search did not receive keyboard focus`);
    await filterText("Navigation Cue 120");
    await waitFor(() => visibleRows().length === 1 && visibleRows()[0].innerText.includes("Navigation Cue 120"),
      `${locale} native keyboard search`);
    const searchCharacters = search.value.length;
    for (let index = 0; index < searchCharacters; index++) await filterKey("Backspace");
    await waitFor(() => search.value === "" && visibleRows().length === 123, `${locale} native keyboard search clear`)
      .catch((error) => { throw new Error(`${error?.message || error}; search=${search.value}, visible=${visibleRows().length}, active=${document.activeElement?.id}`); });
    await filterKey("Tab");
    const typeFilter = document.getElementById("cueListTypeFilter");
    if (document.activeElement !== typeFilter) throw new Error(`${locale} Tab did not move from Search to Action type`);
    await filterKey("Home");
    await filterKey("ArrowDown");
    await filterKey("Enter");
    if (typeFilter.value !== "mixed") throw new Error(`${locale} keyboard Action type selection=${typeFilter.value}`);
    const linkedFilter = document.getElementById("cueListLinkedFilter");
    for (let attempt = 0; attempt < 4 && document.activeElement !== linkedFilter; attempt++) {
      await filterKey("Tab");
    }
    if (document.activeElement !== linkedFilter) {
      throw new Error(`${locale} Tab sequence skipped the Linked Cues filter after Action type selection; active=${document.activeElement?.id}`);
    }
    await filterKey("Home");
    await filterKey("ArrowDown");
    await filterKey("Enter");
    await waitFor(() => visibleRows().length === expectedLinkedIDs.length &&
      visibleRows().every((row) => expectedLinkedIDs.includes(row.dataset.cueId)), `${locale} combined keyboard filters`)
      .catch((error) => { throw new Error(`${error?.message || error}; search=${search.value}, type=${typeFilter.value}, enabled=${document.getElementById("cueListEnabledFilter").value}, linked=${linkedFilter.value}, visible=${visibleRows().map((row) => row.dataset.cueId).join(",")}, expected=${expectedLinkedIDs.join(",")}`); });
    const clear = document.getElementById("clearCueListFilters");
    for (let attempt = 0; attempt < 4 && document.activeElement !== clear; attempt++) {
      await filterKey("Tab");
    }
    if (document.activeElement !== clear) throw new Error(`${locale} Tab did not move to Clear filters`);
    await filterKey("Enter");
    await waitFor(() => visibleRows().length === 123 && document.activeElement === search,
      `${locale} keyboard clear and focus return`);
    if (JSON.stringify(visibleRows().map((row) => row.dataset.cueId)) !== JSON.stringify(originalOrder)) {
      throw new Error(`${locale} keyboard search/filter changed Cue sequence order`);
    }
    return true;
  };
  const measureInspectorViewports = async (linkedParentID) => {
    const measurements = [];
    for (const [width, height, mobile] of [
      [390, 844, true], [720, 900, false], [768, 1024, true], [1024, 768, false], [1440, 900, false],
    ]) {
      await setViewport(width, height, mobile);
      await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
      const navigation = document.querySelector(".cue-inspection-navigation");
      if (!navigation) throw new Error(`Cue Inspector navigation disappeared at ${width}x${height}`);
      navigation.scrollIntoView({ block: "nearest" });
      const bounds = navigation.getBoundingClientRect();
      const pageWidth = Math.max(document.documentElement.scrollWidth, document.body.scrollWidth);
      const pageOverflow = pageWidth > window.innerWidth;
      const navigationVisible = bounds.width > 0 && bounds.left >= -1 && bounds.right <= window.innerWidth + 1;
      if (pageOverflow || !navigationVisible) {
        throw new Error(`Cue Inspector responsive geometry failed at ${width}x${height}: pageWidth=${pageWidth}, viewport=${window.innerWidth}, navigation=${JSON.stringify({left: bounds.left, right: bounds.right, width: bounds.width})}`);
      }
      const table = navigation.closest(".table-wrap");
      const cueList = document.querySelector(".cue-list-table-wrap");
      const cueListTable = cueList?.querySelector(".cue-list-table");
      const cueListRow = cueList?.querySelector("tr[data-cue-id]");
      const cueListOverflow = !!cueList && cueList.scrollWidth > cueList.clientWidth + 1;
      const linkedGroup = document.querySelector(`#content tr[data-cue-id="${linkedParentID}"] .cue-link-cell`);
      measurements.push({ width, height, mobile, pageOverflow,
        navigationVisible, navigationWidth: Math.round(bounds.width), tableClientWidth: table?.clientWidth || 0,
        tableScrollWidth: table?.scrollWidth || 0, cueListOverflow,
        cueListClientWidth: cueList?.clientWidth || 0, cueListScrollWidth: cueList?.scrollWidth || 0,
        cueListRole: cueListTable?.getAttribute("role") || "",
        cueListCardLayout: !!cueListRow && getComputedStyle(cueListRow).display === "grid",
        cueListDataLabels: cueListRow?.querySelectorAll("td[data-label]").length || 0,
        linkedGroupVisible: !!linkedGroup && linkedGroup.innerText.trim().length > 0 });
    }
    await setViewport(390, 844, true);
    return measurements;
  };

  try {
    if (document.readyState === "loading") {
      await new Promise((resolve) => document.addEventListener("DOMContentLoaded", resolve, { once: true }));
    }
    const configResponse = await fetch("/mixed-cue-config", { cache: "no-store" });
    if (!configResponse.ok) throw new Error(`test configuration returned HTTP ${configResponse.status}`);
    const config = await configResponse.json();
    await setViewport(390, 844, true);
    state.user = { user_id: config.user, username: "owner", role: "OWNER" };
    state.csrf = config.csrf;
    state.project = { project_id: config.project, name: config.name };
    sessionStorage.setItem("stagecore_csrf", state.csrf);
    if (!localStorage.getItem("stagecore_mixed_cue_ltr_evidence")) f016SetMode("dark");
    showApp();
    await navigate("cues");

    const ltrEvidence = localStorage.getItem("stagecore_mixed_cue_ltr_evidence");
    const restartMarker = "stagecore_mixed_cue_process_restart";
    const restartSessionMarker = "stagecore_mixed_cue_process_restart_session";
    const processRestarted = localStorage.getItem(restartMarker) === "1" &&
      sessionStorage.getItem(restartSessionMarker) !== "1";
    const staleReviewKey = "stagecore_mixed_cue_stale_review";
    if (sessionStorage.getItem(staleReviewKey) === "1") {
      const previous = JSON.parse(ltrEvidence || "null");
      if (!previous?.ids?.[2]) throw new Error("stale-review evidence from the prior browser page is missing");
      const parentID = previous.ids[2];
      await waitFor(() => !!document.querySelector(`#content tr[data-cue-id="${parentID}"]`),
        "loading the concurrently saved Cue before stale recovery review");
      document.querySelector(`#content tr[data-cue-id="${parentID}"] .cue-edit`)?.click();
      await waitFor(() => document.getElementById("cueDialog")?.open, "opening a Cue with a stale local recovery draft");
      const reviewButton = document.getElementById("sc009LocalRestore");
      await waitFor(() => reviewButton && !reviewButton.hidden, "stale local recovery review control");
      const recoveryStatus = document.querySelector("#sc009EditorRecovery [role='status']:nth-of-type(2)");
      if (!recoveryStatus?.textContent.includes("Cue changed since") ||
          !reviewButton.textContent.includes("Review local draft")) {
        throw new Error(`changed Cue baseline was not marked for review: ${recoveryStatus?.textContent || "missing status"}`);
      }
      const serverLink = document.querySelector(`#linkedCuesEditor .linked-cue-checkbox[value="${previous.ids[1]}"]`);
      const serverAction = document.querySelector("#actionsEditor .action-editor");
      if (!serverLink || !serverAction) throw new Error("saved Cue fields are missing from stale-review editor");
      const reviewPayload = document.getElementById("sc009LocalReview");
      reviewButton.click();
      await waitFor(() => !reviewPayload.hidden, "opening read-only stale local draft details");
      const reviewedDraft = JSON.parse(reviewPayload.value);
      const reviewedAction = reviewedDraft.actions?.[0];
      const currentPayload = JSON.parse(serverAction.querySelector(".action-parameters")?.value || "null");
      if (reviewedDraft.fields?.[1] !== "Offline restored Mixed Parent" ||
          reviewedDraft.links?.length !== 0 || reviewedAction?.action_id !== previous.action_ids[0] ||
          reviewedAction?.parameters?.scene_id !== "offline-sim" ||
          reviewedAction?.parameters?.preserve?.vendor !== "mixed-cue" ||
          document.getElementById("cueName")?.value !== "Concurrent saved parent" ||
          document.querySelector("#actionsEditor .action-editor .action-mode")?.value !== "PARALLEL" ||
          !serverLink.checked || currentPayload?.scene_id !== "opening-sim") {
        throw new Error(`stale recovery review changed or omitted data: reviewed=${reviewPayload.value}, name=${document.getElementById("cueName")?.value}, mode=${document.querySelector("#actionsEditor .action-editor .action-mode")?.value}, linked=${serverLink.checked}, payload=${JSON.stringify(currentPayload)}`);
      }
      if (offlineRecoveryRequests < 1 || confirmationRequests) {
        throw new Error(`stale recovery review unexpectedly used the server or confirmation (${offlineRecoveryRequests}/${confirmationRequests})`);
      }
      sessionStorage.removeItem(staleReviewKey);
      localStorage.removeItem("stagecore_mixed_cue_ltr_evidence");
      localStorage.removeItem(restartMarker);
      localStorage.removeItem("stagecore_test_offline_recovery");
      await report(JSON.stringify({ ...previous, stale_review_available: true, stale_review_not_applied: true,
        stale_review_name: document.getElementById("cueName").value }));
      return;
    }
    if (ltrEvidence) {
      if (!processRestarted) throw new Error("the browser profile reopened without a fresh session after process restart");
      const previous = JSON.parse(ltrEvidence);
      const parentID = previous.ids[2];
      await waitFor(() => document.documentElement.dir === "ltr" && document.documentElement.dataset.theme === "light" &&
        !!document.querySelector(`#content tr[data-cue-id="${parentID}"] .cue-label-cell`),
      "reloading the Draft Cue list in English LTR and light appearance");
      const filterKeyboardLTR = await exerciseCueFiltersWithKeyboard(previous.ids, "LTR");
      const ltrInspector = document.querySelector(`#content tr[data-cue-id="${parentID}"] details`);
      ltrInspector?.querySelector("summary")?.click();
      await waitFor(() => ltrInspector?.querySelector(".cue-inspection-navigation"),
        "opening the Cue Inspector for LTR viewport measurements");
      const responsiveLayoutsLTR = await measureInspectorViewports(parentID);
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
      const localRestore = document.getElementById("sc009LocalRestore");
      await waitFor(() => localRestore && !localRestore.hidden, "offline Mixed Cue recovery candidate after page reload");
      const localStatus = document.querySelector("#sc009EditorRecovery [role='status']:nth-of-type(2)");
      if (!localStatus?.textContent.includes("recovery draft is available")) {
        throw new Error(`offline recovery status was not actionable: ${localStatus?.textContent || "missing"}`);
      }
      localRestore.click();
      const recoveredLink = document.querySelector(`#linkedCuesEditor .linked-cue-checkbox[value="${previous.ids[1]}"]`);
      if (!recoveredLink) throw new Error("linked child control is missing during local recovery");
      const recoveredActionParameters = document.querySelector("#actionsEditor .action-editor .action-parameters");
      const recoveredActionID = document.querySelector("#actionsEditor .action-editor .action-id")?.value;
      let recoveredPayload;
      try { recoveredPayload = JSON.parse(recoveredActionParameters?.value || "null"); }
      catch (_) { throw new Error("recovered Mixed Action payload is not valid JSON"); }
      await waitFor(() => document.getElementById("cueName")?.value === "Offline restored Mixed Parent" &&
        document.querySelector("#actionsEditor .action-editor .action-mode")?.value === "SEQUENTIAL" && !recoveredLink.checked &&
        recoveredPayload?.scene_id === "offline-sim" && recoveredPayload?.preserve?.vendor === "mixed-cue" &&
        recoveredActionID === previous.action_ids[0],
      "restoring Cue fields, Action mode and linked child from browser-local recovery").catch(() => {
        throw new Error(`local recovery state mismatch: name=${document.getElementById("cueName")?.value}, mode=${document.querySelector("#actionsEditor .action-editor .action-mode")?.value}, linked=${recoveredLink?.checked}, payload=${recoveredActionParameters?.value}, actionID=${recoveredActionID}, reviewVisible=${!document.getElementById("sc009LocalReview")?.hidden}, confirms=${confirmationRequests}`);
      });
      if (offlineRecoveryRequests < 1) throw new Error("server recovery endpoint was not offline during local recovery");
      if (confirmationRequests) throw new Error(`matching local recovery unexpectedly prompted for confirmation (${confirmationRequests})`);
      const ltrProof = { ...previous, ltr_label: labelText, ltr_palette_visible: paletteVisible,
        responsive_layout_ltr: responsiveLayoutsLTR,
        filter_keyboard_ltr: filterKeyboardLTR,
        ltr_label_readable: readable, ltr_marker_contrast: colorContrast, ltr_text_contrast: textContrast,
        ltr_focus_visible: focusVisible, ltr_focus_contrast: focusContrast, ltr_page_overflow: overflow,
        browser_process_restarted: processRestarted,
        local_recovery_available: true, local_recovery_restored: true,
        local_action_payload_restored: true, local_action_identity_restored: true,
        theme_qualification: { rtl: previous.theme, ltr: document.documentElement.dataset.theme } };
      localStorage.setItem("stagecore_mixed_cue_ltr_evidence", JSON.stringify(ltrProof));
      sessionStorage.setItem(staleReviewKey, "1");
      const staleReady = await fetch("/mixed-cue-stale-ready", {method: "POST",
        headers: {"Content-Type": "application/json"}, body: JSON.stringify({cue_id: parentID})});
      if (!staleReady.ok) throw new Error(`stale baseline synchronization returned HTTP ${staleReady.status}`);
      window.location.reload();
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
    const filterKeyboardRTL = await exerciseCueFiltersWithKeyboard(createdIDs, "RTL");
    const typeFilter = document.getElementById("cueListTypeFilter");
    const linkedFilter = document.getElementById("cueListLinkedFilter");
    typeFilter.value = "mixed";
    typeFilter.dispatchEvent(new Event("change", { bubbles: true }));
    linkedFilter.value = "linked";
    linkedFilter.dispatchEvent(new Event("change", { bubbles: true }));
    await waitFor(() => visibleCueRows().length === 3 &&
      visibleCueRows().every((row) => createdIDs.includes(row.dataset.cueId)),
    "combining Mixed and linked Cue filters");
    await touchElement(document.getElementById("clearCueListFilters"), "clearing Cue filters");
    await waitFor(() => visibleCueRows().length === 123, "restoring the full sequence after clearing filters");
    if (JSON.stringify(visibleCueRows().map((row) => row.dataset.cueId)) !== JSON.stringify(originalCueOrder)) {
      throw new Error("search and filters changed Cue execution order");
    }

    const navigationRows = visibleCueRows();
    if (navigationRows.length !== 123) throw new Error(`Cue navigation list=${navigationRows.length}, want 123`);
    const firstNavigationDetails = navigationRows[0].querySelector("details");
    await touchElement(firstNavigationDetails?.querySelector("summary"), "opening the first Cue Inspector");
    await waitFor(() => firstNavigationDetails?.querySelector(".cue-inspection-navigation"),
      "opening the first Cue in the long-list Inspector by touch");
    const responsiveLayoutsRTL = await measureInspectorViewports(parentID);
    const inspectorShortcutTrace = [];
    firstNavigationDetails.addEventListener("keydown", (event) => inspectorShortcutTrace.push({
      key: event.key, target: event.target?.tagName, targetClass: event.target?.className,
      composing: event.isComposing, prevented: event.defaultPrevented,
    }));
    const requestInspectorKey = async (key) => {
      const response = await fetch("/mixed-cue-inspector-key-ready", {
        method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ key })
      });
      if (!response.ok) throw new Error(`Inspector keyboard gate failed for ${key}: ${response.status}`);
    };
    firstNavigationDetails.querySelector("summary")?.focus();
    await requestInspectorKey("j");
    try {
      await waitFor(() => navigationRows[1].querySelector("details")?.open,
        "moving to the next Cue with the Inspector J shortcut");
    } catch (error) {
      throw new Error(`${error?.message || error}; shortcutTrace=${JSON.stringify(inspectorShortcutTrace)}, firstOpen=${firstNavigationDetails.open}, nextOpen=${navigationRows[1].querySelector("details")?.open}, active=${document.activeElement?.outerHTML?.slice(0, 180)}`);
    }
    navigationRows[1].querySelector("summary")?.focus();
    await requestInspectorKey("k");
    await waitFor(() => firstNavigationDetails?.open,
      "moving to the previous Cue with the Inspector K shortcut");
    for (let index = 1; index < navigationRows.length; index++) {
      const current = navigationRows[index - 1];
      const currentNavigation = current.querySelector(".cue-inspection-navigation");
      const nextButton = currentNavigation?.querySelectorAll("button")[1];
      if (!nextButton || nextButton.disabled) throw new Error(`Next Cue control disabled at list position ${index}`);
      if (index === 1) {
        nextButton.focus();
        await requestInspectorKey("Enter");
      } else {
        nextButton.click();
      }
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
    const localUndo = document.getElementById("sc009UndoDraft");
    const localRedo = document.getElementById("sc009RedoDraft");
    const firstActionMode = () => document.querySelector("#actionsEditor .action-editor .action-mode");
    if (!localUndo || !localRedo || firstActionMode()?.value !== "PARALLEL") {
      throw new Error("Cue editor local history or the original Mixed Action mode is missing");
    }
    const temporaryName = "Temporary Mixed Cue history state";
    setValue(document.getElementById("cueName"), temporaryName);
    setValue(firstActionMode(), "SEQUENTIAL");
    selectedMiddle.checked = false;
    selectedMiddle.dispatchEvent(new Event("change", { bubbles: true }));
    await waitFor(() => !localUndo.disabled, "Cue editor Undo after changing Mixed fields, Action mode and link");
    localUndo.click();
    await waitFor(() => document.getElementById("cueName").value === "Native Visual and MIDI Parent" &&
      firstActionMode()?.value === "PARALLEL" && selectedMiddle.checked,
    "restoring the full Mixed Cue form with local Undo");
    if (localRedo.disabled) throw new Error("Cue editor local Redo was not enabled after Undo");
    localRedo.click();
    await waitFor(() => document.getElementById("cueName").value === temporaryName &&
      firstActionMode()?.value === "SEQUENTIAL" && !selectedMiddle.checked,
    "restoring the full Mixed Cue form with local Redo");
    localUndo.click();
    await waitFor(() => document.getElementById("cueName").value === "Native Visual and MIDI Parent" &&
      firstActionMode()?.value === "PARALLEL" && selectedMiddle.checked,
    "returning the linked Mixed Cue form to its saved baseline");
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
    const requestsBeforeDMXPreview = requestTrace.length;
    setValue(scenarioSelect, "missing-dmx");
    previewButton.click();
    const dmxTimeline = inspector.querySelector(".cue-inspection-result .cue-inspection-timeline");
    await waitFor(() => dmxTimeline?.querySelectorAll("li").length === 6,
      "rendering the missing-DMX simulation for all linked Actions");
    const dmxOutcomes = [...dmxTimeline.querySelectorAll("li")].map((item) => item.innerText);
    const dmxBlocked = dmxOutcomes.filter((item) => item.includes("DMX_NODE_MISSING") && item.includes("SIMULATED_BLOCKED"));
    if (dmxBlocked.length !== 1) {
      throw new Error(`missing-DMX simulation should block only the Lighting Action: ${JSON.stringify(dmxOutcomes)}`);
    }
    if (requestTrace.length !== requestsBeforeDMXPreview) {
      throw new Error(`missing-DMX simulation made network requests: ${requestTrace.slice(requestsBeforeDMXPreview).join("; ")}`);
    }
    const requestsBeforeCompanionPreview = requestTrace.length;
    const originalTargets = state.f002Targets || [];
    state.f002Targets = [{logical_name: "SIM", logical_type: "MACHINE_ROLE"},
      ...originalTargets.filter((target) => target.logical_name !== "SIM")];
    setValue(scenarioSelect, "missing-companion");
    previewButton.click();
    const companionTimeline = inspector.querySelector(".cue-inspection-result .cue-inspection-timeline");
    await waitFor(() => companionTimeline?.querySelectorAll("li").length === 6,
      "rendering the missing-Companion simulation for all linked Actions");
    const companionOutcomes = [...companionTimeline.querySelectorAll("li")].map((item) => item.innerText);
    const companionBlocked = companionOutcomes.filter((item) =>
      item.includes("COMPANION_MISSING_FOR_MACHINE_ROLE") && item.includes("SIMULATED_BLOCKED"));
    state.f002Targets = originalTargets;
    if (companionBlocked.length !== 6) {
      throw new Error(`missing-Companion simulation should block every Machine Role Action: ${JSON.stringify(companionOutcomes)}`);
    }
    if (requestTrace.length !== requestsBeforeCompanionPreview) {
      throw new Error(`missing-Companion simulation made network requests: ${requestTrace.slice(requestsBeforeCompanionPreview).join("; ")}`);
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
    quickLabelInput.focus();
    const labelBeforeShortcutKey = quickLabelInput.value;
    await requestInspectorKey("j");
    if (document.activeElement !== quickLabelInput || quickLabelInput.value !== `${labelBeforeShortcutKey}j` || !inspector.open) {
      throw new Error("Inspector J shortcut intercepted text entered in a quick-edit field");
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
    localStorage.setItem("stagecore_test_offline_recovery", "1");
    const recoveryDraftName = "Offline restored Mixed Parent";
    setValue(document.getElementById("cueName"), recoveryDraftName);
    const recoveryActionMode = document.querySelector("#actionsEditor .action-editor .action-mode");
    setValue(recoveryActionMode, "SEQUENTIAL");
    const recoveryActionParameters = document.querySelector("#actionsEditor .action-editor .action-parameters");
    setValue(recoveryActionParameters, JSON.stringify({scene_id: "offline-sim", preserve: {vendor: "mixed-cue"}}));
    const recoveryLinkedCue = document.querySelector(`#linkedCuesEditor .linked-cue-checkbox[value="${middleID}"]`);
    if (!recoveryLinkedCue?.checked) throw new Error("saved parent-to-middle link was unavailable for recovery draft");
    recoveryLinkedCue.checked = false;
    recoveryLinkedCue.dispatchEvent(new Event("change", { bubbles: true }));
    const recoveryPrefix = "stagecore.cue-recovery.v2:" + JSON.stringify([config.user, config.project, parentID]) + ":";
    await waitFor(() => Array.from({length: localStorage.length}, (_, index) => localStorage.key(index))
      .some((key) => key?.startsWith(recoveryPrefix)), "browser-local full Cue recovery write");
    const firstPass = { status: "PASS", ids: createdIDs, action_ids: editedActionIDs,
      edited_name: editedName, quick_edited_label: quickEditedLabel, quick_edited_notes: quickEditedNotes,
      mixed_form_undo: true, mixed_form_redo: true,
      filter_keyboard_rtl: filterKeyboardRTL,
      responsive_layout_rtl: responsiveLayoutsRTL,
      theme: document.documentElement.dataset.theme,
      cue_color: cueColor, cue_label_text: cueLabelText, color_marker_visible: colorMarkerVisible,
      palette_visible: paletteVisible, label_color_readable: labelColorReadable,
      marker_contrast: colorContrast, text_contrast: textContrast,
      focus_ring_visible: focusRingVisible, focus_contrast: focusContrast, page_overflow: pageOverflow };
    localStorage.setItem("stagecore_mixed_cue_ltr_evidence", JSON.stringify(firstPass));
    localStorage.setItem("stagecore_locale", "en");
    f016SetMode("light");
    sessionStorage.setItem(restartSessionMarker, "1");
    localStorage.setItem(restartMarker, "1");
    await fetch("/mixed-cue-restart-ready", { method: "POST" });
  } catch (error) {
    await report(JSON.stringify({ status: "FAIL", error: error?.stack || String(error), requests: requestTrace.slice(-16) }));
  }
})();
