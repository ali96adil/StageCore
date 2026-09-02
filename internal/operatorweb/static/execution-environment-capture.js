"use strict";

Object.assign(f025Strings, {
  "f025.assets": {"en":"Environment assets","ar-IQ":"ملفات بيئة التشغيل"},
  "f025.capture_file": {"en":"Capture local file to Vault","ar-IQ":"حفظ ملف محلي داخل الـ Vault"},
  "f025.capture_hint": {"en":"Choose the exact local file. StageCore streams it into the existing Vault, verifies SHA-256 and size, then marks this asset CONTENT_BOUND.","ar-IQ":"اختر الملف المحلي نفسه. يقوم StageCore بتمريره إلى الـ Vault الموجود، ويتحقق من SHA-256 والحجم، ثم يحوّل الملف إلى CONTENT_BOUND."},
  "f025.capture_success": {"en":"Asset captured and verified in Vault.","ar-IQ":"تم حفظ الملف والتحقق منه داخل الـ Vault."},
  "f025.choose_file": {"en":"Choose a file before capture.","ar-IQ":"اختر ملفاً قبل بدء الحفظ."},
  "f025.vault_available": {"en":"Vault copy available","ar-IQ":"نسخة الـ Vault متوفرة"},
  "f025.vault_missing": {"en":"Identity declared — Vault copy missing","ar-IQ":"الهوية معلنة — نسخة الـ Vault غير موجودة"},
  "f025.vault_reference": {"en":"Reference only — bytes are not in Vault","ar-IQ":"مرجع فقط — البايتات غير موجودة في الـ Vault"},
  "f025.vault_unavailable": {"en":"Vault status unavailable","ar-IQ":"حالة الـ Vault غير متوفرة"},
  "f025.asset_hash": {"en":"Content SHA-256","ar-IQ":"بصمة المحتوى SHA-256"},
  "f025.asset_size": {"en":"Exact bytes","ar-IQ":"الحجم الدقيق بالبايت"},
  "f025.guided_capture_note": {"en":"Guided setup starts reference-only. After creation, capture the real file from the asset card so StageCore can verify and store the bytes itself.","ar-IQ":"يبدأ الإعداد الموجّه كمرجع فقط. بعد الإنشاء، احفظ الملف الحقيقي من بطاقة الملف حتى يتحقق StageCore من البايتات ويخزنها بنفسه."}
});

function f025CapturePath(environmentID, assetKey, suffix = "capture") {
  return `${f025CollectionPath()}/${encodeURIComponent(environmentID)}/assets/${encodeURIComponent(assetKey)}/${suffix}`;
}

function f025CaptureAssetMarkup(environment, asset, editable) {
  const contentBound = asset.capture_policy === "CONTENT_BOUND";
  const hash = contentBound ? (asset.content_hash || "—") : "—";
  const size = contentBound && asset.size_bytes !== undefined && asset.size_bytes !== null ? String(asset.size_bytes) : "—";
  const status = contentBound ? f025T("f025.vault_unavailable") : f025T("f025.vault_reference");
  return `<div class="card" style="margin-top:12px" data-f025-asset-card data-environment-id="${esc(environment.execution_environment_id)}" data-asset-key="${esc(asset.key)}" data-capture-policy="${esc(asset.capture_policy)}">
    <div class="section-title-row">
      <div>
        <p class="eyebrow">${esc(asset.kind || "ASSET")} · ${esc(asset.key)}</p>
        <h3>${esc(asset.name || asset.key)}</h3>
        ${asset.locator ? `<p class="mono muted">${esc(asset.locator)}</p>` : ""}
      </div>
      <div class="toolbar">${contentBound ? pill(f025T("f025.content_badge"), "good") : pill(f025T("f025.reference_badge"), "warn")}</div>
    </div>
    <div class="form-grid two" style="margin-top:10px">
      <div><span class="label">${esc(f025T("f025.asset_hash"))}</span><p class="mono muted">${esc(hash)}</p></div>
      <div><span class="label">${esc(f025T("f025.asset_size"))}</span><p class="mono muted">${esc(size)}</p></div>
    </div>
    <div class="message ${contentBound ? "warn" : ""} f025-vault-status">${esc(status)}</div>
    ${editable ? `<div style="margin-top:10px">
      <p class="muted">${esc(f025T("f025.capture_hint"))}</p>
      <div class="toolbar">
        <input class="f025-capture-file" type="file" aria-label="${esc(f025T("f025.capture_file"))}">
        <button class="button primary f025-capture-button" type="button">${esc(f025T("f025.capture_file"))}</button>
      </div>
    </div>` : ""}
  </div>`;
}

const f025CaptureEnvironmentCardBase = f025EnvironmentCard;
f025EnvironmentCard = function f025EnvironmentCardWithCapture(environment, roles, editable) {
  const base = f025CaptureEnvironmentCardBase(environment, roles, editable);
  const assets = environment.manifest?.assets || [];
  if (!assets.length) return base;
  const assetSection = `<div style="margin-top:14px"><p class="eyebrow">${esc(f025T("f025.assets"))}</p>${assets.map((asset) => f025CaptureAssetMarkup(environment, asset, editable)).join("")}</div>`;
  return base.replace('<details style="margin-top:12px">', `${assetSection}<details style="margin-top:12px">`);
};

function f025CaptureEnforceGuidedFlow() {
  const policy = document.getElementById("f025CapturePolicy");
  if (!policy) return;
  policy.value = "REFERENCE_ONLY";
  for (const option of [...policy.options]) {
    if (option.value === "CONTENT_BOUND") option.remove();
  }
  document.getElementById("f025ContentHashLabel")?.classList.add("hidden");
  document.getElementById("f025SizeBytesLabel")?.classList.add("hidden");
  if (!document.getElementById("f025GuidedCaptureNote")) {
    const note = document.createElement("div");
    note.id = "f025GuidedCaptureNote";
    note.className = "message";
    note.textContent = f025T("f025.guided_capture_note");
    document.getElementById("f025GuidedForm")?.appendChild(note);
  }
}

async function f025CaptureRefreshStatuses() {
  const cards = [...content.querySelectorAll("[data-f025-asset-card]")];
  await Promise.all(cards.map(async (card) => {
    const target = card.querySelector(".f025-vault-status");
    if (!target) return;
    if (card.dataset.capturePolicy !== "CONTENT_BOUND") {
      target.textContent = f025T("f025.vault_reference");
      target.className = "message warn f025-vault-status";
      return;
    }
    try {
      const status = await api(f025CapturePath(card.dataset.environmentId, card.dataset.assetKey, "vault-status"));
      target.textContent = status.vault_available ? f025T("f025.vault_available") : f025T("f025.vault_missing");
      target.className = `message ${status.vault_available ? "success" : "warn"} f025-vault-status`;
    } catch (_) {
      target.textContent = f025T("f025.vault_unavailable");
      target.className = "message warn f025-vault-status";
    }
  }));
}

function f025CaptureBindControls() {
  content.querySelectorAll(".f025-capture-button").forEach((button) => {
    button.addEventListener("click", async () => {
      const card = button.closest("[data-f025-asset-card]");
      const input = card?.querySelector(".f025-capture-file");
      const file = input?.files?.[0];
      if (!card || !file) {
        setMessage(globalMessage, f025T("f025.choose_file"), "warn");
        return;
      }
      button.disabled = true;
      try {
        await api(f025CapturePath(card.dataset.environmentId, card.dataset.assetKey), {
          method: "POST",
          headers: {"Content-Type": "application/octet-stream"},
          body: file,
        });
        await renderExecutionEnvironments(f025T("f025.capture_success"));
      } catch (error) {
        button.disabled = false;
        f025Error(error);
      }
    });
  });
}

const f025CaptureRenderBase = renderExecutionEnvironments;
renderExecutionEnvironments = async function renderExecutionEnvironmentsWithCapture(message = "") {
  await f025CaptureRenderBase(message);
  f025CaptureEnforceGuidedFlow();
  f025CaptureBindControls();
  await f025CaptureRefreshStatuses();
};
