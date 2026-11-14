(() => {
  const current = {session_id: "current", name: "Current Rehearsal", runtime_snapshot_id: "snap-a"};
  const compared = {session_id: "previous", name: "Previous Rehearsal", runtime_snapshot_id: "snap-b"};
  const currentCues = [
    {cue_id: "cue-open", display_label: "1", name: "Opening", actions: [
      {action_id: "midi", capability_key: "midi.send", target_ref: "SIM MIDI", result: "COMPLETED", latency_ms: 12, ack_level: "TRANSPORT_ONLY", delivery_evidence: "EXTERNAL_EXECUTION_UNKNOWN", response_summary: "private adapter response"},
      {action_id: "light", capability_key: "lighting.channels.fade", target_ref: "SIM DMX", result: "FAILED", error_code: "OUTPUT_TIMEOUT", latency_ms: 31},
    ]},
    {cue_id: "cue-open", display_label: "1", name: "Opening", actions: [
      {action_id: "midi", capability_key: "midi.send", target_ref: "SIM MIDI", result: "COMPLETED", latency_ms: 14, ack_level: "APPLICATION", delivery_evidence: "APPLICATION_ACKNOWLEDGED"},
    ]},
  ];
  const comparedCues = [
    {cue_id: "cue-open", display_label: "1", name: "Opening", actions: [
      {action_id: "midi", capability_key: "midi.send", target_ref: "SIM MIDI", result: "FAILED", latency_ms: 25, ack_level: "NONE", delivery_evidence: "UNKNOWN"},
      {action_id: "visual", capability_key: "visual.scene.activate", target_ref: "SIM VDMX", result: "COMPLETED", latency_ms: 9},
    ]},
    {cue_id: "cue-open", display_label: "1", name: "Opening", actions: [
      {action_id: "midi", capability_key: "midi.send", target_ref: "SIM MIDI", result: "COMPLETED", latency_ms: 18, ack_level: "VERIFIED_STATE", delivery_evidence: "STATE_VERIFIED"},
    ]},
  ];
  const markup = sessionComparisonMarkup(current, currentCues, [], compared, comparedCues, []);
  document.querySelector("#comparison").innerHTML = markup;
  const tables = document.querySelectorAll("#comparison .table-wrap table");
  const actionTable = tables[tables.length - 1];
  const wrapper = actionTable?.closest(".table-wrap");
  const cells = [...(actionTable?.querySelectorAll("tbody") || [])].flatMap((body) => [...body.rows].flatMap((row) => [...row.cells]));
  const bodyText = document.querySelector("#comparison").textContent;
  const report = {
    status: "PASS",
    action_rows: actionTable?.tBodies[0]?.rows.length || 0,
    action_columns: actionTable?.tHead?.rows[0]?.cells.length || 0,
    horizontal_scroll: Boolean(wrapper && wrapper.scrollWidth > wrapper.clientWidth && wrapper.scrollWidth >= actionTable.scrollWidth),
    private_excluded: !bodyText.includes("private adapter response"),
    external_unknown: bodyText.includes("Transport sent; external execution unknown") && bodyText.includes("Output state verified"),
    cell_count: cells.length,
  };
  fetch("/comparison-result", {method: "POST", body: JSON.stringify(report)});
})();
