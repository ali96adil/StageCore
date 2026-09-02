"use strict";

(function registerF025WorkspacePage() {
  if (typeof F017_PAGES === "undefined" || typeof F017_PRESETS === "undefined") return;

  if (!F017_PAGES.includes("environments")) F017_PAGES.push("environments");
  if (typeof f017Strings === "object") {
    f017Strings["workspace.page.environments"] = f025Strings["f025.nav"];
  }

  const insertAfterConfiguration = (values) => {
    if (!Array.isArray(values) || values.includes("environments")) return;
    const index = values.indexOf("configuration");
    if (index >= 0) values.splice(index + 1, 0, "environments");
    else values.push("environments");
  };

  for (const preset of Object.values(F017_PRESETS)) {
    insertAfterConfiguration(preset.page_order);
    if (preset.visible_pages.includes("configuration")) insertAfterConfiguration(preset.visible_pages);
  }

  if (typeof f017ApplyProfile === "function") f017ApplyProfile();
})();
