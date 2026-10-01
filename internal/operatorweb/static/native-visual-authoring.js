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
      <label>Bottom-right Y<input class="f037-map-br-y" type="number" min="-4" max="4" step="any" value="${point("bottom_right","y",1)}" required></label>
      <label>Bottom-left X<input class="f037-map-bl-x" type="number" min="-4" max="4" step="any" value="${point("bottom_left","x",0)}" required></label>
      <label>Bottom-left Y<input class="f037-map-bl-y" type="number" min="-4" max="4" step="any" value="${point("bottom_left","y",1)}" required></label>
    </div>`;
  }
  if (capability === "visual.preload") {
    const t = p.transform || {};
    return `<div class="form-grid two">
      <label>Layer ID<input class="f037-layer" value="${layer}" required></label>
      <label>Content version ID<input class="f037-content-version" value="${esc(p.content_version_id || "")}" required></label>
      <label>Content SHA-256<input class="f037-content-hash mono" value="${esc(p.content_hash || "")}" pattern="[0-9a-f]{64}" required></label>
      <label>Content mode<select class="f037-content-mode">${["FIT","FILL","CROP"].map((v) => `<option value="${v}" ${p.content_mode === v ? "selected" : ""}>${v}</option>`).join("")}</select></label>
      <label>Opacity (optional 0-1)<input class="f037-opacity" type="number" min="0" max="1" step="0.01" value="${esc(p.opacity ?? "")}"></label>
      <label>Output ID (optional)<input class="f037-output" value="${esc(p.output_id || "")}"></label>
      <label>Z index (optional)<input class="f037-z" type="number" min="-4096" max="4096" step="1" value="${esc(p.z_index ?? "")}"></label>
    </div>
    <details><summary>Optional initial transform</summary>
      <div class="form-grid three">
        <label>X<input class="f037-x" type="number" step="any" value="${esc(t.x ?? "")}"></label>
        <label>Y<input class="f037-y" type="number" step="any" value="${esc(t.y ?? "")}"></label>
        <label>Scale X<input class="f037-scale-x" type="number" min="0.000001" step="any" value="${esc(t.scale_x ?? "")}"></label>
        <label>Scale Y<input class="f037-scale-y" type="number" min="0.000001" step="any" value="${esc(t.scale_y ?? "")}"></label>
        <label>Rotation  deg<input class="f037-rotation" type="number" step="any" value="${esc(t.rotation_degrees ?? "")}"></label>
      </div>
    </details>`;
  }
  if (capability === "visual.state.inspect") {
    return `<p class="muted">No command parameters. The assigned Native Visual Companion returns its current state.</p>`;
  }
  return `<div class="message warn">Use Advanced action settings for this Visual capability.</div>`;
}

function f037ValidateQuad(mapping) {
  const points = [mapping.top_left, mapping.top_right, mapping.bottom_right, mapping.bottom_left];
  let orientation = 0;
  for (let i = 0; i < points.length; i++) {
    const a = points[i], b = points[(i + 1) % 4], c = points[(i + 2) % 4];
    const cross = (b.x - a.x) * (c.y - b.y) - (b.y - a.y) * (c.x - b.x);
    if (Math.abs(cross) <= 1e-9) throw new Error("Output mapping must form a non-degenerate convex quad.");
    const sign = Math.sign(cross);
    if (!orientation) orientation = sign;
    else if (sign !== orientation) throw new Error("Output mapping corners must remain in convex perimeter order.");
  }
}

function f037BuildVisualPayload(panel) {
  const capability = panel.querySelector(".f037-capability")?.value || "";
  const targetRef = panel.querySelector(".f037-target")?.value || "";
  if (!targetRef) throw new Error("Choose a compatible Native Visual Machine Role target.");
  const q = (selector) => panel.querySelector(selector);
  const layer = () => f037Identifier(q(".f037-layer")?.value, "Layer ID");
  const output = () => f037Identifier(q(".f037-output")?.value, "Output ID");
  const p = { contract_version: 1 };

  switch (capability) {
    case "visual.preload": {
      p.layer_id = layer();
      p.content_version_id = f037Identifier(q(".f037-content-version")?.value, "Content version ID", 256);
      const hash = String(q(".f037-content-hash")?.value || "").trim();
      if (!/^[0-9a-f]{64}$/.test(hash)) throw new Error("Content SHA-256 must be 64 lowercase hexadecimal characters.");
      p.content_hash = hash;
      p.content_mode = q(".f037-content-mode")?.value || "FIT";
      const opacity = f037OptionalNumber(panel, ".f037-opacity", "Opacity", 0, 1);
      if (opacity !== undefined) p.opacity = opacity;
      const outputID = String(q(".f037-output")?.value || "").trim();
      if (outputID) p.output_id = f037Identifier(outputID, "Output ID");
      const z = f037OptionalNumber(panel, ".f037-z", "Z index", -4096, 4096, true);
      if (z !== undefined) p.z_index = z;
      const transform = {};
      const fields = [
        [".f037-x", "x", -Number.MAX_VALUE],
        [".f037-y", "y", -Number.MAX_VALUE],
        [".f037-scale-x", "scale_x", 0.000001],
        [".f037-scale-y", "scale_y", 0.000001],
        [".f037-rotation", "rotation_degrees", -Number.MAX_VALUE],
      ];
      for (const [selector, key, min] of fields) {
        const value = f037OptionalNumber(panel, selector, key, min, Number.MAX_VALUE);
        if (value !== undefined) transform[key] = value;
      }
      if (Object.keys(transform).length) p.transform = transform;
      break;
    }
    case "visual.play":
    case "visual.pause":
    case "visual.stop":
      p.layer_id = layer();
      break;
    case "visual.seek":
      p.layer_id = layer();
      p.position_ms = f037Number(q(".f037-position")?.value, "Position", 0, Number.MAX_SAFE_INTEGER, true);
      break;
    case "visual.loop":
      p.layer_id = layer();
      p.enabled = q(".f037-enabled")?.value === "true";
      break;
    case "visual.blackout":
      p.enabled = q(".f037-enabled")?.value === "true";
      break;
    case "visual.layer.opacity":
      p.layer_id = layer();
      p.opacity = f037Number(q(".f037-opacity")?.value, "Opacity", 0, 1);
      break;
    case "visual.layer.transform":
      p.layer_id = layer();
      p.transform = {
        x: f037Number(q(".f037-x")?.value, "X"),
        y: f037Number(q(".f037-y")?.value, "Y"),
        scale_x: f037Number(q(".f037-scale-x")?.value, "Scale X", 0.000001),
        scale_y: f037Number(q(".f037-scale-y")?.value, "Scale Y", 0.000001),
        rotation_degrees: f037Number(q(".f037-rotation")?.value, "Rotation"),
      };
      break;
    case "visual.transition": {
      const kind = q(".f037-transition-kind")?.value || "CUT";
      const from = f037Identifier(q(".f037-from-layer")?.value, "From layer");
      const to = f037Identifier(q(".f037-to-layer")?.value, "To layer");
      if (from === to) throw new Error("Transition source and destination layers must differ.");
      const duration = f037Number(q(".f037-duration")?.value, "Transition duration", 0, 30000, true);
      if (kind === "CUT" && duration !== 0) throw new Error("CUT transition duration must be 0 ms.");
      if (kind !== "CUT" && duration < 1) throw new Error("FADE/CROSSFADE duration must be at least 1 ms.");
      p.kind = kind;
      p.from_layer_id = from;
      p.to_layer_id = to;
      p.duration_ms = duration;
      p.target_opacity = f037Number(q(".f037-target-opacity")?.value, "Target opacity", 0, 1);
      break;
    }
    case "visual.layer.crop": {
      p.layer_id = layer();
      const x = f037Number(q(".f037-rect-x")?.value, "Crop X", 0, 1);
      const y = f037Number(q(".f037-rect-y")?.value, "Crop Y", 0, 1);
      const width = f037Number(q(".f037-rect-width")?.value, "Crop width", 0.000001, 1);
      const height = f037Number(q(".f037-rect-height")?.value, "Crop height", 0.000001, 1);
      if (x + width > 1.000000001 || y + height > 1.000000001) throw new Error("Crop rectangle must stay inside normalized layer bounds.");
      p.rect = { x, y, width, height };
      break;
    }
    case "visual.layer.mask":
      p.layer_id = layer();
      p.kind = q(".f037-mask-kind")?.value || "NONE";
      break;
    case "visual.layer.effect": {
      p.layer_id = layer();
      const brightness = f037OptionalNumber(panel, ".f037-brightness", "Brightness", -1, 1);
      const contrast = f037OptionalNumber(panel, ".f037-contrast", "Contrast", 0, 4);
      const saturation = f037OptionalNumber(panel, ".f037-saturation", "Saturation", 0, 2);
      if (brightness === undefined && contrast === undefined && saturation === undefined) throw new Error("Layer effect requires brightness, contrast or saturation.");
      if (brightness !== undefined) p.brightness = brightness;
      if (contrast !== undefined) p.contrast = contrast;
      if (saturation !== undefined) p.saturation = saturation;
      break;
    }
    case "visual.layer.order":
      p.layer_id = layer();
      p.z_index = f037Number(q(".f037-z")?.value, "Z index", -4096, 4096, true);
      break;
    case "visual.layer.output":
      p.layer_id = layer();
      p.output_id = output();
      break;
    case "visual.output.configure":
      p.output_id = output();
      p.width = f037Number(q(".f037-width")?.value, "Output width", 1, 16384);
      p.height = f037Number(q(".f037-height")?.value, "Output height", 1, 16384);
      break;
    case "visual.output.mapping":
      p.output_id = output();
      p.mapping = {
        top_left: { x: f037Number(q(".f037-map-tl-x")?.value, "Top-left X", -4, 4), y: f037Number(q(".f037-map-tl-y")?.value, "Top-left Y", -4, 4) },
        top_right: { x: f037Number(q(".f037-map-tr-x")?.value, "Top-right X", -4, 4), y: f037Number(q(".f037-map-tr-y")?.value, "Top-right Y", -4, 4) },
        bottom_right: { x: f037Number(q(".f037-map-br-x")?.value, "Bottom-right X", -4, 4), y: f037Number(q(".f037-map-br-y")?.value, "Bottom-right Y", -4, 4) },
        bottom_left: { x: f037Number(q(".f037-map-bl-x")?.value, "Bottom-left X", -4, 4), y: f037Number(q(".f037-map-bl-y")?.value, "Bottom-left Y", -4, 4) },
      };
      f037ValidateQuad(p.mapping);
      break;
    case "visual.state.inspect":
      break;
    default:
      throw new Error("Choose a supported Native Visual command or use Advanced action settings.");
  }
  return { capability, targetRef, parameters: p };
}

function f037DisablePanelControls(panel, disabled) {
  panel.querySelectorAll("input, select, textarea, button").forEach((control) => {
    control.disabled = disabled;
  });
}

function f037RenderVisualPanel(card, parsedParameters = null) {
  const panel = card.querySelector(".f037-visual-action");
  if (!panel) return;
  const capabilitySelect = panel.querySelector(".f037-capability");
  const targetSelect = panel.querySelector(".f037-target");
  const fields = panel.querySelector(".f037-fields");
  const currentTarget = card.querySelector(".action-target")?.value || "";
  const capability = capabilitySelect.value;
  const targets = f037TargetsForCapability(capability, currentTarget);
  targetSelect.innerHTML = `<option value="">Choose compatible Native Visual role...</option>` + targets.map(({target, legacy}) =>
    `<option value="${esc(target.logical_name)}" ${target.logical_name === currentTarget ? "selected" : ""}>${esc(target.logical_name)}${legacy ? " - legacy / capability mismatch" : ""}</option>`
  ).join("");
  if (!targetSelect.value && targets.length === 1) targetSelect.value = targets[0].target.logical_name;
  fields.innerHTML = f037FieldMarkup(capability, parsedParameters || {});
  const kind = fields.querySelector(".f037-transition-kind");
  const duration = fields.querySelector(".f037-duration");
  if (kind && duration) {
    kind.addEventListener("change", () => {
      if (kind.value === "CUT") duration.value = "0";
      else if (Number(duration.value || 0) < 1) duration.value = "500";
    });
  }
}

function f037EnhanceNativeVisual(card) {
  if (!card || card.dataset.f037VisualEnhanced) return;
  card.dataset.f037VisualEnhanced = "true";
  const builder = card.querySelector(".f002-action-builder");
  const kind = builder?.querySelector(".f002-action-kind");
  const capability = card.querySelector(".action-capability");
  const params = card.querySelector(".action-parameters");
  const target = card.querySelector(".action-target");
  if (!builder || !kind || !capability || !params || !target) return;

  const option = document.createElement("option");
  option.value = "visual";
  option.textContent = "Native Visual Engine";
  kind.insertBefore(option, kind.querySelector('option[value="advanced"]'));

  const rawParameters = params.value.trim();
  const parsedVisual = f037VisualCapabilitySet.has(capability.value)
    ? f037ParseVisualParameters(capability.value, params.value)
    : null;
  const panel = document.createElement("div");
  panel.className = "f037-visual-action hidden";
  panel.innerHTML = `
    <div class="form-grid two">
      <label>Native Visual command<select class="f037-capability">${f037VisualCapabilities.map(([value, label]) => `<option value="${value}">${esc(label)}</option>`).join("")}</select></label>
      <label>Native Visual Machine Role<select class="f037-target"></select></label>
    </div>
    <div class="f037-fields"></div>
    <p class="muted">StageCore Native Visual only. External VDMX stays on OSC / Execution Environments. Unknown legacy fields stay in Advanced and are not rewritten.</p>`;
  const midiPanel = builder.querySelector(".f002-midi-action");
  (midiPanel || builder.querySelector(".f002-osc-action"))?.insertAdjacentElement("afterend", panel);

  const capabilitySelect = panel.querySelector(".f037-capability");
  if (parsedVisual) {
    capabilitySelect.value = capability.value;
    kind.value = "visual";
  }

  const previousApplyKind = () => {
    const active = kind.value === "visual";
    panel.classList.toggle("hidden", !active);
    f037DisablePanelControls(panel, !active);
    if (active) {
      builder.querySelector(".f002-osc-action")?.classList.add("hidden");
      builder.querySelector(".f002-midi-action")?.classList.add("hidden");
      capability.value = capabilitySelect.value;
    }
  };

  capabilitySelect.addEventListener("change", () => {
    capability.value = capabilitySelect.value;
    f037RenderVisualPanel(card, null);
  });
  panel.querySelector(".f037-target")?.addEventListener("change", () => {
    target.value = panel.querySelector(".f037-target").value;
  });
  kind.addEventListener("change", previousApplyKind);

  f037RenderVisualPanel(card, parsedVisual);
  if (kind.value === "visual") {
    target.value = panel.querySelector(".f037-target")?.value || target.value;
  }
  previousApplyKind();
  if (f037VisualCapabilitySet.has(capability.value) && !parsedVisual && !["", "{}"].includes(rawParameters)) {
    kind.value = "advanced";
    previousApplyKind();
  }
}

const f037BaseEnhanceActionCard = f002EnhanceActionCard;
f002EnhanceActionCard = function f037EnhanceActionCard(card) {
  f037BaseEnhanceActionCard(card);
  f037EnhanceNativeVisual(card);
};

const f037BaseSyncActionCard = f002SyncActionCard;
f002SyncActionCard = function f037SyncActionCard(card) {
  f037BaseSyncActionCard(card);
  if (card?.querySelector(".f002-action-kind")?.value !== "visual") return;
  const panel = card.querySelector(".f037-visual-action");
  const capability = card.querySelector(".action-capability");
  const target = card.querySelector(".action-target");
  const params = card.querySelector(".action-parameters");
  if (!panel || !capability || !target || !params) return;
  const built = f037BuildVisualPayload(panel);
  capability.value = built.capability;
  target.value = built.targetRef;
  params.value = JSON.stringify(built.parameters, null, 2);
}

cueForm.addEventListener("submit", (event) => {
  try {
    actionsEditor.querySelectorAll(".action-editor").forEach((card) => {
      if (card.querySelector(".f002-action-kind")?.value === "visual") {
        f037BuildVisualPayload(card.querySelector(".f037-visual-action"));
      }
    });
  } catch (error) {
    event.preventDefault();
    event.stopImmediatePropagation();
    setMessage(globalMessage, error.message || errorMessage(error), "error");
  }
}, true);
