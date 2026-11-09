// Max [js] script, running inside an Ableton Live Max for Live device.
autowatch = 0;
inlets = 1;
outlets = 1;

function stagecore_live(phase, ticket, trackName, operation, requested, trackID, deadlineText) {
    var confirmedID = 0;
    // Millisecond deadlines travel as strings to preserve precision in Max atoms.
    var deadline = Number(String(deadlineText).slice(1));
    try {
        if ((phase !== "check" && phase !== "apply") || (requested !== 0 && requested !== 1) ||
            !isFinite(deadline) || Date.now() >= deadline) throw new Error("invalid request");
        var track = new LiveAPI(null, "this_device canonical_parent");
        var id = Number(track.id);
        var name = track.get("name");
        if (track.type !== "Track" || id <= 0 ||
            (Array.isArray(name) ? name.join(" ") : String(name)) !== trackName) throw new Error("wrong track");
        if (["mute", "solo", "arm", "transport"].indexOf(operation) < 0) throw new Error("unmapped operation");
        if (operation === "arm" && readFlag(track, "can_be_armed") !== 1) throw new Error("track cannot be armed");
        var target = operation === "transport" ? new LiveAPI(null, "live_set") : track;
        if (operation === "transport" && (target.type !== "Song" || Number(target.id) <= 0)) throw new Error("invalid Live Set");
        var property = operation === "transport" ? "is_playing" : operation;
        var value = readFlag(target, property);
        if (phase === "apply") {
            if (id !== trackID || Date.now() >= deadline) throw new Error("stale binding");
            if (value !== requested) {
                if (operation === "transport") target.call(requested ? "start_playing" : "stop_playing");
                else target.set(property, requested);
            }
            if (readFlag(target, property) !== requested) throw new Error("Live did not apply the requested state");
        }
        if (Date.now() < deadline) confirmedID = id;
    } catch (_) {
        // Keep the failure local and release no application evidence.
    }
    outlet(0, "stagecore_live_result", ticket, confirmedID ? 1 : 0, confirmedID);
}

function readFlag(target, property) {
    var value = target.get(property);
    value = Array.isArray(value) ? value[0] : value;
    if (value !== 0 && value !== 1) throw new Error("invalid Live property state");
    return value;
}
