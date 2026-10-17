"use strict";

// SC-002A phase 1: read-only Hub-global clock settings; no privileged writes.
// Phone time is only a comparison reference, never an authoritative NTP source.
const clockUI = {
  refreshInFlight: false,
  lastSample: null,
};

function clockUIStrings() {
  return localStorage.getItem("stagecore_locale") === "en" ? {
    title: "Date & Time", subtitle: "System settings — Raspberry Pi Hub",
    description: "This page displays the Hub clock, not your phone clock. Works on the local theatre Wi-Fi without Mac or Internet.",
    server: "Hub time (Baghdad)", utc: "Hub UTC", phone: "This phone / browser",
    difference: "Hub − phone (estimate)", source: "Synchronization status",
    rtc: "RTC hardware", battery: "RTC battery", systemZone: "Host time zone",
    sample: "Uncertainty", refresh: "Refresh now", unknown: "Unknown",
    readonly: "Monitoring only — setting time from phone, manual entry and Mac Companion will come in a later safe phase.",
    wrongClock: "If the Pi time is wrong, keep SHOW disconnected until authorized recovery is available.",
    ago: "Last sampled", justNow: "Just now",
    previewTitle: "Preview proposed time correction", previewHelp: "This is a DRY RUN. No Pi clock change is possible from this version. Phone time can be wrong.",
    previewPhone: "Preview this phone's time", previewManual: "Preview manual Baghdad time", manualLabel: "Baghdad date and time (UTC+03:00)",
    previewStatus: "PREVIEW ONLY — no changes applied", current: "Current Hub UTC", proposed: "Proposed UTC", delta: "Proposed − current",
    activeSession: "Active runtime session", noSession: "None", blocked: "Active session: future clock correction must be blocked", unblocked: "No session is active, but applying remains unavailable.", invalidManual: "Enter a valid Baghdad date and time.",
  } : {
    title: "الساعة والتاريخ", subtitle: "إعدادات النظام — Raspberry Pi Hub",
    description: "الوقت المعروض هو وقت الـPi، وليس ساعة الهاتف. تعمل الصفحة من شبكة المسرح بدون ماك أو إنترنت.",
    server: "ساعة الـPi (بغداد)", utc: "وقت الـPi العالمي (UTC)", phone: "وقت هذا الهاتف / المتصفح",
    difference: "فرق الـPi عن الهاتف (تقديري)", source: "حالة المزامنة",
    rtc: "ساعة RTC", battery: "بطارية RTC", systemZone: "منطقة وقت النظام",
    sample: "هامش عدم اليقين", refresh: "تحديث الآن", unknown: "غير معروف",
    readonly: "مراقبة فقط حالياً — ضبط الوقت من الهاتف أو يدوياً أو من Mac Companion يضاف لاحقاً بعد فحوص الأمان.",
    wrongClock: "إذا ساعة الـPi غير صحيحة، لا تعتبرها متزامنة حتى يتم إصلاحها بطريقة مخوّلة.",
    ago: "آخر فحص", justNow: "الآن",
    previewTitle: "معاينة تعديل الساعة", previewHelp: "هذه معاينة فقط ولا تغيّر ساعة الـPi. ممكن ساعة الهاتف نفسها تكون غير صحيحة.",
    previewPhone: "معاينة الوقت من الهاتف", previewManual: "معاينة الوقت اليدوي", manualLabel: "التاريخ والوقت في بغداد (UTC+03:00)",
    previewStatus: "معاينة فقط — لم يتغير الوقت", current: "وقت الـPi الحالي UTC", proposed: "الوقت المقترح UTC", delta: "فرق الوقت المقترح",
    activeSession: "جلسة التشغيل الحالية", noSession: "لا توجد", blocked: "جلسة نشطة: تعديل الساعة مستقبلاً يكون محجوباً", unblocked: "لا توجد جلسة نشطة، لكن تطبيق الوقت غير متاح حالياً.", invalidManual: "أدخل تاريخاً ووقتاً صحيحين بتوقيت بغداد.",
  };
}

function clockLocalizedDate(ms, timeZone) {
  const locale = localStorage.getItem("stagecore_locale") === "en" ? "en-GB" : "ar-IQ";
  try {
    return new Intl.DateTimeFormat(locale, {
      timeZone, year: "numeric", month: "short", day: "numeric",
      hour: "2-digit", minute: "2-digit", second: "2-digit",
      hourCycle: "h23",
    }).format(new Date(ms));
  } catch (_) {
    return new Date(ms).toISOString();
  }
}

function clockStatusName(value) {
  const ar = localStorage.getItem("stagecore_locale") !== "en";
  const names = {
    SYNCHRONIZED: ar ? "متزامنة" : "Synchronized",
    NOT_SYNCHRONIZED: ar ? "غير متزامنة" : "Not synchronized",
    PRESENT: ar ? "موجودة" : "Detected",
    ABSENT: ar ? "غير موجودة" : "Not detected",
    UNKNOWN: ar ? "غير معروف" : "Unknown",
  };
  return names[value] || names.UNKNOWN;
}

async function clockOneSample() {
  const start = Date.now();
  const value = await api("/api/v1/system/clock/status");
  const finish = Date.now();
  if (!Number.isFinite(value?.server_unix_ms)) throw new Error("Invalid Hub time response.");
  return {
    ...value,
    offsetMS: value.server_unix_ms - (start + finish) / 2,
    uncertaintyMS: Math.max(1, Math.ceil((finish - start) / 2)),
    roundTripMS: finish - start,
    capturedAt: finish,
  };
}

async function clockMeasure() {
  // The lowest RTT sample reduces (not eliminates) scheduling/network noise.
  // A device whose own time is wrong will give a misleading offset: advisory.
  const attempts = [];
  for (let i = 0; i < 3; i++) {
    attempts.push(await clockOneSample());
  }
  attempts.sort((a, b) => a.roundTripMS - b.roundTripMS);
  return attempts[0];
}

function clockPaintValues() {
  if (state.page !== "clock" || !clockUI.lastSample) return;
  const s = clockUI.lastSample;
  const t = clockUIStrings();
  const now = Date.now();
  const hubNow = now + s.offsetMS;
  const gap = s.offsetMS / 1000;
  const plus = gap > 0 ? "+" : "";
  const set = (id, value) => { const target = el(id); if (target) target.textContent = value; };
  set("clockPiTime", clockLocalizedDate(hubNow, s.display_zone || "Asia/Baghdad"));
  set("clockUTCTime", clockLocalizedDate(hubNow, "UTC"));
  set("clockPhoneTime", clockLocalizedDate(now, s.display_zone || "Asia/Baghdad"));
  set("clockOffset", `${plus}${gap.toFixed(2)} s`);
  set("clockUncertainty", `±${s.uncertaintyMS} ms (RTT ${s.roundTripMS} ms)`);
  set("clockSync", clockStatusName(s.sync_status));
  set("clockRTC", clockStatusName(s.rtc_status));
  set("clockBattery", clockStatusName(s.rtc_battery_status));
  set("clockSystemZone", s.system_zone || t.unknown);
  set("clockLastCheck", `${t.ago}: ${Math.max(0, Math.round((now - s.capturedAt) / 1000))} s`);
}

async function refreshClockStatus() {
  if (clockUI.refreshInFlight || state.page !== "clock") return;
  clockUI.refreshInFlight = true;
  const refresh = el("clockRefresh");
  if (refresh) refresh.disabled = true;
  try {
    const sample = await clockMeasure();
    if (state.page !== "clock") return;
    clockUI.lastSample = sample;
    el("clockError")?.classList.add("hidden");
    clockPaintValues();
  } catch (error) {
    if (state.page !== "clock") return;
    const message = el("clockError");
    if (message) {
      message.textContent = errorMessage(error);
      message.classList.remove("hidden");
    }
  } finally {
    clockUI.refreshInFlight = false;
    const currentRefresh = el("clockRefresh");
    if (currentRefresh) currentRefresh.disabled = false;
  }
}

// Preview NEVER applies time: the Hub has no privileged clock-adjust endpoint.
async function clockPreviewProposal(source, unixMS) {
  if (state.user?.role !== "OWNER" || state.page !== "clock") return;
  const t = clockUIStrings();
  const region = el("clockPreviewResult");
  if (!region) return;
  region.textContent = "…";
  try {
    const preview = await api("/api/v1/system/clock/preview", {
      method: "POST", json: { source, proposed_unix_ms: unixMS },
    });
    if (state.page !== "clock" || !el("clockPreviewResult")) return;
    const line = (label, value) => `${label}: ${value}`;
    const difference = (preview.difference_ms / 1000).toFixed(1);
    region.textContent = [
      t.previewStatus,
      line(t.current, preview.server_utc),
      line(t.proposed, preview.proposed_utc),
      line(t.delta, `${difference} s`),
      line(t.activeSession, preview.active_session || t.noSession),
      preview.runtime_blocked ? t.blocked : t.unblocked,
    ].join("\n");
  } catch (error) {
    if (el("clockPreviewResult")) el("clockPreviewResult").textContent = errorMessage(error);
  }
}

// Explicit Asia/Baghdad UTC+03:00 interpretation of a manual local-clock value.
// This is a proposal preview, never clock authority; source is labelled.
function clockBaghdadManualMillis(value) {
  if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(value || "")) return null;
  const parsed = Date.parse(`${value}:00+03:00`);
  if (!Number.isFinite(parsed)) return null;
  const recheck = new Date(parsed + 3 * 3600000).toISOString().slice(0, 16);
  return recheck === value ? parsed : null;
}

function renderClockSettings() {
  const t = clockUIStrings();
  content.innerHTML = `
    <div class="page-head">
      <div>
        <p class="eyebrow">${esc(t.subtitle)}</p>
        <h1>${esc(t.title)}</h1>
        <p class="muted">${esc(t.description)}</p>
      </div>
      <div class="toolbar">
        <button type="button" id="clockRefresh" class="button primary">${esc(t.refresh)}</button>
      </div>
    </div>
    <div id="clockError" class="message error hidden" role="alert"></div>
    <div class="clock-status-grid">
      <article class="card clock-primary">
        <p class="eyebrow">${esc(t.server)}</p>
        <strong class="clock-big-value" id="clockPiTime">—</strong>
        <p class="muted">${esc(t.utc)}: <span id="clockUTCTime">—</span></p>
        <p class="muted">${esc(t.ago)}: <span id="clockLastCheck">—</span></p>
      </article>
      <article class="card">
        <p class="eyebrow">${esc(t.phone)}</p>
        <strong id="clockPhoneTime">—</strong>
        <p class="muted">${esc(t.difference)}: <span id="clockOffset" class="mono">—</span></p>
        <p class="muted">${esc(t.sample)}: <span id="clockUncertainty" class="mono">—</span></p>
      </article>
      <article class="card">
        <p class="eyebrow">${esc(t.source)}</p>
        <strong id="clockSync">—</strong>
        <p class="muted">${esc(t.rtc)}: <span id="clockRTC">—</span></p>
        <p class="muted">${esc(t.battery)}: <span id="clockBattery">—</span></p>
        <p class="muted">${esc(t.systemZone)}: <span id="clockSystemZone">—</span></p>
      </article>
    </div>
    <article class="card clock-read-only">
      <p>${esc(t.readonly)}</p>
      <p class="muted">${esc(t.wrongClock)}</p>
    </article>
    ${state.user?.role === "OWNER" ? `
      <article class="card clock-preview-card">
        <h2>${esc(t.previewTitle)}</h2>
        <p class="muted">${esc(t.previewHelp)}</p>
        <div class="clock-preview-buttons">
          <button id="clockPreviewPhone" class="button primary" type="button">${esc(t.previewPhone)}</button>
          <label for="clockManualBaghdad">${esc(t.manualLabel)}
            <input id="clockManualBaghdad" type="datetime-local" step="60">
          </label>
          <button id="clockPreviewManual" class="button" type="button">${esc(t.previewManual)}</button>
        </div>
        <pre id="clockPreviewResult" class="clock-preview-output" aria-live="polite"></pre>
      </article>` : ""}
  `;
  el("clockRefresh")?.addEventListener("click", () => { void refreshClockStatus(); });
  el("clockPreviewPhone")?.addEventListener("click", () => { void clockPreviewProposal("PHONE", Date.now()); });
  el("clockPreviewManual")?.addEventListener("click", () => {
    const manual = clockBaghdadManualMillis(el("clockManualBaghdad")?.value);
    if (manual === null) {
      if (el("clockPreviewResult")) el("clockPreviewResult").textContent = t.invalidManual;
      return;
    }
    void clockPreviewProposal("MANUAL", manual);
  });
  clockPaintValues();
  void refreshClockStatus();
}

// Global Hub navigation: unlike Project pages it works with no project open.
// Use the existing same-origin authenticated browser API; no cloud service.
el("clockNav")?.addEventListener("click", () => {
  setPage("clock");
  el("clockNav").classList.add("active");
  setMessage(globalMessage, "");
  renderClockSettings();
});
setInterval(() => {
  if (state.page === "clock" && state.user) {
    clockPaintValues();
    const age = clockUI.lastSample ? Date.now() - clockUI.lastSample.capturedAt : Infinity;
    if (age > 20000) void refreshClockStatus();
  }
}, 1000);
