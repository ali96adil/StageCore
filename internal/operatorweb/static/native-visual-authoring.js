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
  if (["visual.play", "visual.pause", "visual.stop"].includes(capability)) {
    return `<div class="form-grid two"><label>Layer ID<input class="f037-layer" value="${layer}" required></label></div>`;
  }
  if (capability === "visual.blackout") {
    return `<div class="form-grid two"><label>Blackout<select class="f037-enabled"><option value="true" ${p.enabled !== false ? "selected" : ""}>ON</option><option value="false" ${p.enabled === false ? "selected" : ""}>OFF</option></select></label></div>`;
  }
  if (capability === "visual.seek") {
    return `<div class="form-grid two"><label>Layer ID<input class="f037-layer" value="${layer}" required></label><label>Position (ms)<input class="f037-position" type="number" min="0" step="1" value="${esc(p.position_ms ?? 0)}" required></label></div>`;
  }
  if (capability === "visual.loop") {
    return `<div class="form-grid two"><label>Layer ID<input class="f037-layer" value="${layer}" required></label><label>Loop<select class="f037-enabled"><option value="true" ${p.enabled !== false ? "selected" : ""}>ON</option><option value="false" ${p.enabled === false ? "selected" : ""}>OFF</option></select></label></div>`;
  }
  if (capability === "visual.layer.opacity") {
    return `<div class="form-grid two"><label>Layer ID<input class="f037-layer" value="${layer}" required></label><label>Opacity (0-1)<input class="f037-opacity" type="number" min="0" max="1" step="0.01" value="${esc(p.opacity ?? 1)}" required></label></div>`;
  }
  if (capability === "visual.layer.transform") {
    const t = p.transform || {};
    return `<div class="form-grid three">
      <label>Layer ID<input class="f037-layer" value="${layer}" required></label>
      <label>X<input class="f037-x" type="number" step="any" value="${esc(t.x ?? 0)}" required></label>
      <label>Y<input class="f037-y" type="number" step="any" value="${esc(t.y ?? 0)}" required></label>
      <label>Scale X<input class="f037-scale-x" type="number" min="0.000001" step="any" value="${esc(t.scale_x ?? 1)}" required></label>
      <label>Scale Y<input class="f037-scale-y" type="number" min="0.000001" step="any" value="${esc(t.scale_y ?? 1)}" required></label>
      <label>Rotation  deg<input class="f037-rotation" type="number" step="any" value="${esc(t.rotation_degrees ?? 0)}" required></label>
    </div>`;
  }
  if (capability === "visual.transition") {
    return `<div class="form-grid three">
      <label>Transition<select class="f037-transition-kind">${["CUT","FADE","CROSSFADE"].map((v) => `<option value="${v}" ${p.kind === v ? "selected" : ""}>${v}</option>`).join("")}</select></label>
      <label>From layer<input class="f037-from-layer" value="${esc(p.from_layer_id || "")}" required></label>
      <label>To layer<input class="f037-to-layer" value="${esc(p.to_layer_id || "")}" required></label>
      <label>Duration (ms)<input class="f037-duration" type="number" min="0" max="30000" step="1" value="${esc(p.duration_ms ?? 0)}" required></label>
      <label>Target opacity<input class="f037-target-opacity" type="number" min="0" max="1" step="0.01" value="${esc(p.target_opacity ?? 1)}" required></label>
    </div>`;
  }
  if (capability === "visual.layer.crop") {
    const rect = p.rect || {};
    return `<div class="form-grid three">
      <label>Layer ID<input class="f037-layer" value="${layer}" required></label>
      <label>X (0-1)<input class="f037-rect-x" type="number" min="0" max="1" step="0.01" value="${esc(rect.x ?? 0)}" required></label>
      <label>Y (0-1)<input class="f037-rect-y" type="number" min="0" max="1" step="0.01" value="${esc(rect.y ?? 0)}" required></label>
      <label>Width (0-1)<input class="f037-rect-width" type="number" min="0.000001" max="1" step="0.01" value="${esc(rect.width ?? 1)}" required></label>
      <label>Height (0-1)<input class="f037-rect-height" type="number" min="0.000001" max="1" step="0.01" value="${esc(rect.height ?? 1)}" required></label>
    </div>`;
  }
  if (capability === "visual.layer.mask") {
    return `<div class="form-grid two"><label>Layer ID<input class="f037-layer" value="${layer}" required></label><label>Mask<select class="f037-mask-kind">${["NONE","RECT","ELLIPSE"].map((v) => `<option value="${v}" ${p.kind === v ? "selected" : ""}>${v}</option>`).join("")}</select></label></div>`;
  }
  if (capability === "visual.layer.effect") {
    return `<div class="form-grid three">
      <label>Layer ID<input class="f037-layer" value="${layer}" required></label>
      <label>Brightness (-1...1)<input class="f037-brightness" type="number" min="-1" max="1" step="0.01" value="${esc(p.brightness ?? "")}"></label>
      <label>Contrast (0...4)<input class="f037-contrast" type="number" min="0" max="4" step="0.01" value="${esc(p.contrast ?? "")}"></label>
      <label>Saturation (0...2)<input class="f037-saturation" type="number" min="0" max="2" step="0.01" value="${esc(p.saturation ?? "")}"></label>
    </div>`;
  }
  if (capability === "visual.layer.order") {
    return `<div class="form-grid two"><label>Layer ID<input class="f037-layer" value="${layer}" required></label><label>Z index<input class="f037-z" type="number" min="-4096" max="4096" step="1" value="${esc(p.z_index ?? 0)}" required></label></div>`;
  }
  if (capability === "visual.layer.output") {
    return `<div class="form-grid two"><label>Layer ID<input class="f037-layer" value="${layer}" required></label><label>Output ID<input class="f037-output" value="${esc(p.output_id || "")}" required></label></div>`;
  }
  if (capability === "visual.output.configure") {
    return `<div class="form-grid three"><label>Output ID<input class="f037-output" value="${esc(p.output_id || "")}" required></label><label>Width<input class="f037-width" type="number" min="1" max="16384" step="any" value="${esc(p.width ?? 1920)}" required></label><label>Height<input class="f037-height" type="number" min="1" max="16384" step="any" value="${esc(p.height ?? 1080)}" required></label></div>`;
  }
  if (capability === "visual.output.mapping") {
    const m = p.mapping || {};
    const point = (key, axis, fallback) => esc(m?.[key]?.[axis] ?? fallback);
    return `<div class="form-grid three">
      <label>Output ID<input class="f037-output" value="${esc(p.output_id || "")}" required></label>
      <label>Top-left X<input class="f037-map-tl-x" type="number" min="-4" max="4" step="any" value="${point("top_left","x",0)}" required></label>
      <label>Top-left Y<input class="f037-map-tl-y" type="number" min="-4" max="4" step="any" value="${point("top_left","y",0)}" required></label>
      <label>Top-right X<input class="f037-map-tr-x" type="number" min="-4" max="4" step="any" value="${point("top_right","x",1)}" required></label>
      <label>Top-right Y<input class="f037-map-tr-y" type="number" min="-4" max="4" step="any" value="${point("top_right","y",0)}" required></label>
      <label>Bottom-right X<input class="f037-map-br-x" type="number" min="-4" max="4" step="any" value="${point("bottom_right","x",1)}" required></label>
      <lab