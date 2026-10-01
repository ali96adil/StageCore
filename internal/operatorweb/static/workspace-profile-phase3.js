"use strict";

// Feature-workspace integration for F-017. Registration is presentation-only:
// it never changes Project, Runtime Snapshot, Session, Cue, or SHOW authority.
(() => {
  if (typeof f017RegisterFeaturePage !== "function") return;

  const registrations = [
    {
      page: "timecode",
      label: { en: "Timecode", "ar-IQ": "التايم كود" },
      after: "runtime",
      visible_presets: ["stage-manager"],
    },
    {
      page: "timing",
      label: { en: "Timing", "ar-IQ": "التوقيت" },
      after: "timecode",
      visible_presets: ["stage-manager", "rehearsal", "monitoring"],
    },
    {
      page: "capsules",
      label: { en: "Show Capsules", "ar-IQ": "حزم العرض" },
      after: "notes",
      visible_presets: ["stage-manager", "rehearsal"],
    },
    {
      page: "devices",
      label: { en: "Stage Devices", "ar-IQ": "أجهزة المسرح" },
      after: "configuration",
      visible_presets: ["stage-manager", "video", "lighting", "sound", "rehearsal", "monitoring"],
    },
    {
      page: "tablet-controller",
      label: { en: "Tablet Controller", "ar-IQ": "تحكم التابلت" },
      after: "devices",
      visible_presets: ["stage-manager", "video", "rehearsal"],
    },
    {
      page: "tablet-scenes",
      label: { en: "Tablet Scenes", "ar-IQ": "مشاهد التابلت" },
      after: "tablet-controller",
      visible_presets: ["stage-manager", "video", "rehearsal"],
    },
    {
      page: "lighting-setup",
      label: { en: "Lighting Setup", "ar-IQ": "إعداد الإضاءة" },
      after: "tablet-scenes",
      visible_presets: ["stage-manager", "lighting", "rehearsal"],
    },
    {
      page: "lighting-cues",
      label: { en: "Lighting Cues", "ar-IQ": "كيوهات الإضاءة" },
      after: "lighting-setup",
      visible_presets: ["stage-manager", "lighting", "rehearsal"],
    },
    {
      page: "video",
      label: { en: "Live Video", "ar-IQ": "الفيديو الحي" },
      after: "lighting-cues",
      visible_presets: ["stage-manager", "video", "rehearsal", "monitoring"],
    },
    {
      page: "visual-engine",
      label: { en: "Visual Engine", "ar-IQ": "المحرك المرئي" },
      after: "video",
      visible_presets: ["stage-manager", "video", "rehearsal"],
    },
    {
      page: "callboard",
      label: { en: "Callboard", "ar-IQ": "شاشة الكواليس" },
      after: "visual-engine",
      visible_presets: ["stage-manager", "rehearsal", "monitoring"],
    },
    {
      page: "network",
      label: { en: "Network Cockpit", "ar-IQ": "شبكة المسرح" },
      after: "preflight",
      visible_presets: ["stage-manager", "rehearsal", "monitoring"],
    },
    {
      page: "simulation",
      label: { en: "Simulation", "ar-IQ": "المحاكاة" },
      after: "runtime",
      visible_presets: ["stage-manager", "rehearsal"],
    },
  ];

  for (const registration of registrations) {
    f017RegisterFeaturePage(registration.page, registration);
  }

  // Re-apply presentation only so already-present static navigation (for
  // example Timecode/Capsules) follows the active profile immediately.
  f017FeatureNavigationChanged();
})();
