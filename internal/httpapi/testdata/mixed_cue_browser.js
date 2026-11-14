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
  const setValue = (element, value) => {
    element.value = value;
    element.dispatchEvent(new Event("input", { bubbles: true }));
    element.dispatchEvent(new Event("change", { bubbles: true }));
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
    showApp();
    await navigate("cues");

    if (!document.getElementById("createCueButton")) {
      const createDraftButton = document.getElementById("createDraftButton");
      if (createDraftButton) {
        createDraftButton.click();
        await waitFor(() => !!document.getElementById("createCueButton"), "Draft creation");
      }
    }

    const createdIDs = [];
    const createCue = async ({ label, name, linkedCueID = "", actions }) => {
      const createButton = document.getElementById("createCueButton");
      if (!createButton) throw new Error(`Create Cue button is missing (page=${state.page}, revision=${state.currentRevision?.status || "unknown"}, role=${state.user?.role || "unknown"}, message=${document.getElementById("globalMessage")?.innerText || ""}, content=${document.getElementById("content")?.innerText || ""})`);
      createButton.click();
      const dialog = document.getElementById("cueDialog");
      if (!dialog.open) throw new Error("Cue editor dialog did not open");
      setValue(document.getElementById("cueLabel"), label);
      setValue(document.getElementById("cueName"), name);
      setValue(document.getElementById("cueStartDelaySeconds"), "0.125");

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
      label: "1", name: "Lighting and Tablet Leaf", actions: [
        { capability: "lighting.channels.fade", mode: "PARALLEL", parameters: { levels: { warm: 50 }, fade_ms: 300 }, timeout: { start_delay_ms: 20 } },
        { capability: "tablet.media.play", mode: "SEQUENTIAL", parameters: { media_id: "clip-sim-1" }, timeout: {} },
      ],
    });
    const middleID = await createCue({
      label: "2", name: "OSC and MIDI Middle", linkedCueID: leafID, actions: [
        { capability: "osc.send", mode: "PARALLEL_BARRIER", parameters: { address: "/sim/open", arguments: [{ type: "int32", value: 7 }] }, timeout: { start_delay_ms: 40 } },
        { capability: "midi.send", mode: "SEQUENTIAL", priority: "P2", parameters: { destination_name: "SIM MIDI", bytes: [144, 60, 100] }, timeout: {} },
      ],
    });
    const parentID = await createCue({
      label: "3", name: "Native Visual and MIDI Parent", linkedCueID: middleID, actions: [
        { capability: "visual.scene.activate", mode: "PARALLEL", parameters: { scene_id: "opening-sim" }, timeout: {} },
        { capability: "midi.send", mode: "SEQUENTIAL", parameters: { destination_name: "SIM MIDI", bytes: [145, 64, 95] }, timeout: { start_delay_ms: 15 } },
      ],
    });

    if (createdIDs.length !== 3 || createdIDs[2] !== parentID) throw new Error("nested Mixed Cue IDs were not retained");
    await waitFor(() => document.querySelectorAll("#content .sc008-type-badge").length >= 6,
      "Cue Studio rendering the saved Mixed Cue action types");
    await waitFor(() => document.querySelectorAll("#content .cue-link-cell").length === 3,
      "Cue Studio rendering the linked Cue chain");
    if (document.querySelectorAll("#content .sc008-type-badge").length < 6) {
      throw new Error("Cue Studio did not render the saved Mixed Cue action type badges");
    }
    if (document.querySelectorAll("#content .cue-link-cell").length !== 3) {
      throw new Error("Cue Studio did not render all linked-chain rows");
    }
    if (document.body.innerText.includes("Cue created in Draft") === false) {
      throw new Error("Draft save confirmation was not rendered");
    }
    await report(JSON.stringify({ status: "PASS", ids: createdIDs }));
  } catch (error) {
    await report(JSON.stringify({ status: "FAIL", error: error?.stack || String(error), requests: requestTrace.slice(-16) }));
  }
})();
