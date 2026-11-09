// Max [js] script, running inside an Ableton Live Max for Live device.
autowatch = 0;
inlets = 1;
outlets = 1;

function stagecore_live(phase, ticket, trackName, mute, trackID, deadlineText) {
    var confirmedID = 0;
    // Millisecond deadlines travel as strings to preserve precision in Max atoms.
    var deadline = Number(String(deadlineText).slice(1));
    try {
        if ((phase !== "check" && phase !== "apply") || (mute !== 0 && mute !== 1) ||
            !isFinite(deadline) || Date.now() >= deadline) throw new Error("invalid request");
        var track = new LiveAPI(null, "this_device canonical_parent");
        var id = Number(track.id);
        var name = track.get("name");
        if (track.type !== "Track" || id <= 0 ||
            (Array.isArray(name) ? name.join(" ") : String(name)) !== trackName) throw new Error("wrong track");
        var value = readMute(track);
        if (phase === "apply") {
            if (id !== trackID || Date.now() >= deadline) throw new Error("stale binding");
            if (value !== mute) track.set("mute", mute);
            if (readMute(track) !== mute) throw new Error("Live did not apply mute");
        }
        if (Date.now() < deadline) confirmedID = id;
    } catch (_) {
        // Keep the failure local and release no application evidence.
    }
    outlet(0, "stagecore_live_result", ticket, confirmedID ? 1 : 0, confirmedID);
}

function readMute(track) {
    var value = track.get("mute");
    value = Array.isArray(value) ? value[0] : value;
    if (value !== 0 && value !== 1) throw new Error("invalid Live mute state");
    return value;
}
