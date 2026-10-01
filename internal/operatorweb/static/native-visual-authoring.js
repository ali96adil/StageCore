"use strict";

// Guided Native Visual authoring layered on top of the common F-002 Action builder.
// It writes only the existing visual.* contract; the raw capability/parameters
// fields remain available under Advanced action settings as a lossless escape hatch.

const f037VisualCapabilities = [
  ["visual.preload", "Preload media layer"],
  ["visual.play", "Play layer"],
  ["visual.pause", "Pause layer"],
  ["visual.stop", "Stop layer"],
  ["visual.seek", "Seek layer"],
  ["visual.loop", "Set layer loop"],
  ["visual.blackout", "Set visual blackout"],
  ["visual.layer.opacity", "Layer opacity"],
  ["visual.layer.transform", "Layer transform"],
  ["visual.transition", "Layer transition"],
  ["visual.layer.crop", "Layer crop"],
  ["visual.layer.mask", "Layer mask"],
  ["visual.layer.effect", "Layer effect"],
  ["visual.layer.order", "Layer order"],
  ["visual.layer.output", "Assign layer output"],
  ["visual.output.configure", "Configure output"],
  ["visual.output.mapping", "Map output quad"],
  ["visual.state.inspect", "Inspect visual state"],
];

const f037VisualCapabilitySet = new Set(f037VisualCapabilities.map(([key]) => key));

function f037MachineRoleForTargetRef(targetRef) {
  const target = (state.f002Targets || []).find((item) => item.logical_name === targetRef);
  if (!target || String(target.logical_type || "").toLowerCase() !== "machine_role") return null;
  let roleID = "";
  try {
    const cfg = typeof target.configuration === "string"
      ? JSON.parse(target.configuration || "{}")
      : (target.configuration || {});
    roleID = String(cfg.machine_role_id || "").trim();
  } catch (_) {
    return null;
  }
  return (state.f002MachineRoles || []).find((role) => role.machine_role_id === roleID) || null;
}

function f037TargetsForCapability(capability, currentTargetRef = "") {
  const candidates = (state.f002Targets || []).filter((target) => {
    if (String(target.logical_type || "").toLowerCase() !== "machine_role") return false;
    const role = f037MachineRoleForTargetRef(target.logical_name);
    return !!role && !role.retired && (role.required_capabilities || []).includes(capability);
  }).map((target) => ({ target, legacy: false }));

  if (currentTargetRef && !candidates.some((item) => item.target.logical_name === currentTargetRef)) {
    const current = (state.f002Targets || []).find((item) => item.logical_name === currentTargetRef);
    if (current && String(current.logical_type || "").toLowerCase() === "machine_role") {
      candidates.unshift({ target: current, legacy: true });
    }
  }
  return candidates;
}

function f037AllowedKeys(capability) {
  switch (capability) {
    case "visual.preload":
      return ["contract_version", "layer_id", "content_version_id", "content_hash", "content_mode", "opacity", "transform", "output_id", "z_index"];
    case "visual.play":
    case "visual.pause":
    case "visual.stop":
      return ["contract_version", "layer_id"];
    case "visual.seek":
      return ["contract_version", "layer_id", "position_ms"];
    case "visual.loop":
      return ["contract_version", "layer_id", "enabled"];
    case "visual.blackout":
      return ["contract_version", "enabled"];
    case "visual.layer.opacity":
      return ["contract_version", "layer_id", "opacity"];
    case "visual.layer.transform":
      return ["contract_version", "layer_id", "transform"];
    case "visual.transition":
      return ["contract_version", "kind", "from_layer_id", "to_layer_id", "duration_ms", "target_opacity"];
    case "visual.layer.crop":
      return ["contract_version", "layer_id", "rect"];
    case "visual.layer.mask":
      return ["contract_version", "layer_id", "kind"];
    case "visual.layer.effect":
      return ["contract_version", "layer_id", "brightness", "contrast", "saturation"];
    case "visual.layer.order":
      return ["contract_version", "layer_id", "z_index"];
    case "visual.layer.output":
      return ["contract_version", "layer_id", "output_id"];
    case "visual.output.configure":
      return ["contract_version", "output_id", "width", "height"];
    case "visual.output.mapping":
      return ["contract_version", "output_id", "mapping"];
    case "visual.state.inspect":
      return ["contract_version"];
    default:
      return null;
  }
}

function f037ObjectHasOnly(value, allowed) {
  return !!value && typeof value === "object" && !Array.isArray(value) &&
    Object.keys(value).every((key) => allowed.includes(key));
}

function f037NestedShapeValid(capability, parsed) {
  if (capability === "visual.preload" && parsed.transform !== undefined &&
      !f037ObjectHasOnly(parsed.transform, ["x", "y", "scale_x", "scale_y", "rotation_degrees"])) {
    return false;
  }
  if (capability === "visual.layer.transform" &&
      !f037ObjectHasOnly(parsed.transform, ["x", "y", "scale_x", "scale_y", "rotation_degrees"])) {
    return false;
  }
  if (capability === "visual.layer.crop" &&
      !f037ObjectHasOnly(parsed.rect, ["x", "y", "width", "height"])) {
    return false;
  }
  if (capability === "visual.output.mapping") {
    if (!f037ObjectHasOnly(parsed.mapping, ["top_left", "top_right", "bottom_right", "bottom_left"])) return false;
    for (const key of ["top_left", "top_right", "bottom_right", "bottom_left"]) {
      if (!f037ObjectHasOnly(parsed.mapping?.[key], ["x", "y"])) return false;
    }
  }
  return true;
}

function f037ParseVisualParameters(capability, raw) {
  const allowed = f037AllowedKeys(capability);
  if (!allowed) return null;
  let parsed;
  try { parsed = JSON.parse(String(raw || "{}")); }
  catch (_) { return null; }
  if (!parsed || Array.isArray(parsed) || typeof parsed !== "object" || parsed.contract_version !== 1) return null;
  if (!Object.keys(parsed).every((key) => allowed.includes(key))) return null;
  if (!f037NestedShapeValid(capability, parsed)) return null;
  return parsed;
}

function f037Identifier(value, label, maxLength = 64) {
  const text = String(value || "").trim();
  if (!text || text.length > maxLength) {
    throw new Error(`${label} is required and must be at most ${maxLength} characters.`);
  }
  return text;
}

function f037Number(value, label, min = -Number.MAX_VALUE, max = Number.MAX_VALUE, integer = false, optional = false) {
  const raw = String(value ?? "").trim();
  if (optional && raw === "") return undefined;
  const number = Number(raw);
  if (!Number.isFinite(number) || (integer && !Number.isInteger(number)) || number < min || number > max) {
    throw new Error(`${label} must be ${integer ? "a whole number" : "a number"} between ${min} and ${max}.`);
  }
  return number;
}

function f037OptionalNumber(panel, selector, label, min, max, integer = false) {
  const input = panel.querySelector(selector);
  if (!input || String(input.value || "").trim() === "") return undefined;
  return f037Number(input.value, label, min, max, integer, true);
}

function f037FieldMarkup(capability, p = {}) {
  const layer = esc(p.layer_id || "");
  if (["visual.play",