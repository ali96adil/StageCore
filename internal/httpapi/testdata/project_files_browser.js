(async () => {
  async function report(locale) {
    const language = el("languageSelect");
    language.value = locale;
    document.documentElement.lang = locale;
    document.documentElement.dir = locale === "en" ? "ltr" : "rtl";
    await renderScopedProjectFiles("", "");
    const refresh = el("refreshScopedFiles");
    refresh?.focus();
    const rows = content.querySelectorAll("tbody tr");
    const searchInput = el("projectFileSearchInput");
    searchInput.value = "opening";
    el("projectFileSearchForm").dispatchEvent(new Event("submit", {cancelable: true}));
    const searchFinished = await waitFor(() => {
      const searchRows = content.querySelectorAll("tbody tr");
      return searchRows.length === 1 && content.innerText.includes(locale === "en" ? "Showing 1 / 1 entries" : "المعروض 1 / 1 عنصر");
    });
    const searchRows = content.querySelectorAll("tbody tr");
    const searchPassed = searchFinished && searchRows.length === 1 && searchRows[0].innerText.includes("opening.wav");
    el("clearProjectFileSearch")?.click();
    const clearFinished = await waitFor(() => content.querySelectorAll("tbody tr").length === 2);
    const restoredRefresh = el("refreshScopedFiles");
    restoredRefresh?.focus();
    const result = {
      status: rows.length === 2 && searchPassed && clearFinished ? "PASS" : "FAIL",
      locale,
      rows: rows.length,
      search_rows: searchRows.length,
      search_match: searchPassed,
      search_cleared: clearFinished,
      text: content.innerText,
      html: content.innerHTML,
      rtl: document.documentElement.dir === "rtl",
      page_overflow: document.documentElement.scrollWidth > window.innerWidth,
      zoom_percent: Math.round(window.devicePixelRatio * 100),
      viewport_width: window.innerWidth,
      document_width: document.documentElement.scrollWidth,
      keyboard_focusable: !!restoredRefresh && document.activeElement === restoredRefresh,
      upload_form: !!el("projectFileUploadForm"),
      download_link: !!content.querySelector("a[download]"),
    };
    const screenshotResponse = await fetch("/project-files-browser-screenshot-ready", {
      method: "POST",
      headers: {"Content-Type": "application/json"},
      body: JSON.stringify({locale}),
    });
    if (!screenshotResponse.ok) throw new Error(`Project Files screenshot capture failed for ${locale}: ${screenshotResponse.status}`);
    await fetch("/project-files-browser-report", {
      method: "POST",
      headers: {"Content-Type": "application/json"},
      body: JSON.stringify(result),
    });
  }
  async function waitFor(predicate) {
    for (let attempt = 0; attempt < 100; attempt++) {
      if (predicate()) return true;
      await new Promise((resolve) => setTimeout(resolve, 50));
    }
    return predicate();
  }
  await report("ar-IQ");
  await report("en");
})();
