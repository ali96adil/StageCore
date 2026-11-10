// Max [js] script, running inside an Ableton Live Max for Live device.
autowatch = 0;
inlets = 1;
outlets = 1;

var clipBinding = null;

function stagecore_live(phase, ticket, trackName, operation, requested, trackID, deadlineText) {
    var confirmedID = 0;
    // Millisecond deadlines travel as strings to preserve precision in Max atoms.
    var deadline = Number(String(deadlineText).slice(1));
    try {
        if ((phase !== "check" && phase !== "apply") || !isFinite(deadline) || Date.now() >= deadline) throw new Error("invalid request");
        var track = new LiveAPI(null, "this_device canonical_parent");
        var id = Number(track.id);
        var name = track.get("name");
        if (track.type !== "Track" || id <= 0 ||
            (Array.isArray(name) ? name.join(" ") : String(name)) !== trackName) throw new Error("wrong track");
        if (operation === "clip") {
            stagecoreClip(phase, ticket, track, id, trackName, requested, trackID, deadline);
            return;
        }
        if ((requested !== 0 && requested !== 1) || ["mute", "solo", "arm", "transport"].indexOf(operation) < 0) throw new Error("unmapped operation");
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

function clipTarget(track, index) {
    var slots = track.get("clip_slots");
    var slotID = childID(slots, index);
    var slot = new LiveAPI(null, "id " + slotID);
    if (slot.type !== "ClipSlot" || Number(slot.id) !== slotID || readFlag(slot, "has_clip") !== 1) {
        throw new Error("clip slot unavailable");
    }
    var clipID = childID(slot.get("clip"), 0);
    var clip = new LiveAPI(null, "id " + clipID);
    if (clip.type !== "Clip" || Number(clip.id) !== clipID) throw new Error("clip unavailable");
    return { slot: slot, slotID: slotID, clip: clip, clipID: clipID };
}

function childID(children, index) {
    var offset = index * 2;
    var id = Number(children[offset + 1]);
    if (children[offset] !== "id" || !isFinite(id) || id <= 0 || Math.floor(id) !== id) throw new Error("invalid child ID");
    return id;
}

function stagecoreClip(phase, ticket, track, id, trackName, index, trackID, deadline) {
    var previous = clipBinding;
    clipBinding = null;
    if (typeof index !== "number" || Math.floor(index) !== index || index < 0 || index > 255) throw new Error("invalid clip index");
    if (readFlag(track, "can_be_armed") === 1 && readFlag(track, "arm") === 1) throw new Error("track is armed");
    var target = clipTarget(track, index);
    if (readFlag(target.clip, "is_triggered") !== 0 || readFlag(target.clip, "is_playing") !== 0) {
        throw new Error("clip already triggered or playing");
    }
    if (Date.now() >= deadline) throw new Error("clip deadline expired");
    if (phase === "check") {
        clipBinding = { trackID: id, slotID: target.slotID, clipID: target.clipID, index: index, deadline: deadline };
        outlet(0, "stagecore_live_result", ticket, 1, id);
        return;
    }
    if (!previous || previous.trackID !== id || id !== trackID || previous.index !== index ||
        previous.slotID !== target.slotID || previous.clipID !== target.clipID || previous.deadline !== deadline) {
        throw new Error("clip binding changed");
    }

    // Firing queues the Clip. A later Live API read must independently
    // confirm the same Clip is playing in the requested Track slot.
    var observation = new Task(function () {
        var success = false;
        try {
            if (Date.now() >= deadline) throw new Error("clip deadline expired");
            var currentTrack = new LiveAPI(null, "this_device canonical_parent");
            var name = currentTrack.get("name");
            if (Number(currentTrack.id) !== id ||
                (Array.isArray(name) ? name.join(" ") : String(name)) !== trackName) throw new Error("track changed");
            var current = clipTarget(currentTrack, index);
            if (current.slotID !== target.slotID || current.clipID !== target.clipID ||
                readFlag(current.clip, "is_recording") !== 0) throw new Error("clip changed or recording");
            var playingSlot = currentTrack.get("playing_slot_index");
            success = Number(Array.isArray(playingSlot) ? playingSlot[0] : playingSlot) === index &&
                readFlag(current.clip, "is_playing") === 1;
            if (!success) {
                observation.schedule(25);
                return;
            }
        } catch (_) {
            // No evidence for stale bindings, recording, or expired launches.
        }
        outlet(0, "stagecore_live_result", ticket, success && Date.now() < deadline ? 1 : 0, id);
    }, this);
    target.slot.call("fire");
    observation.schedule(25);
}
