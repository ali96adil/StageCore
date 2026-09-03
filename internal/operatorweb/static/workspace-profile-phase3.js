"use strict";

// Phase 3 workspace integration. F-017 owns presentation-only workspace
// visibility; these pages were introduced after its original stable registry.
// Extend the registry without changing runtime, Session, Snapshot, or SHOW
// authority.
(() => {
  const phase3Pages = ["timecode", "timing"];

  for (const page of phase3Pages) {
    if (!F017_PAGES.includes(page)) F017_PAGES.push(page);
  }

  f017Strings["workspace.page.timecode"] = {
    en: "Timecode",
    "ar-IQ": "التايم كود",
  };
  f017Strings["workspace.page.timing"] = {
    en: "Timing",
    "ar-IQ": "التوقيت",
  };

  const stageManager = F017_PRESETS["stage-manager"];
  if (stageManager) {
    const insertAfterRuntime = (pages) => {
      const withoutPhase3 = pages.filter((page) => !phase3Pages.includes(page));
      const runtimeIndex = withoutPhase3.indexOf("runtime");
      const insertion = runtimeIndex >= 0 ? runtimeIndex + 1 : withoutPhase3.length;
      withoutPhase3.splice(insertion, 0, ...phase3Pages);
      return withoutPhase3;
    };

    stageManager.visible_pages = insertAfterRuntime(stageManager.visible_pages);
    stageManager.page_order = insertAfterRuntime(stageManager.page_order);
  }

  // Re-apply presentation only so an already-loaded default profile exposes
  // the Phase 3 workspaces immediately. This performs no Hub mutation.
  f017ApplyProfile({ navigateIfNeeded: false });
})();
