(async () => {
  async function report(locale) {
    const language = el("languageSelect");
    language.value = locale;
    document.documentElement.lang = locale;
    document.documentElement.dir = locale === "en" ? "ltr" : "rtl";
    await renderScopedProjectFiles();
    const refresh = el("refreshScopedFiles");
    refresh?.focus();
    const rows = content.querySelectorAll("tbody tr");
    const result = {
      status: rows.length === 2 ? "PASS" : "FAIL",
      locale,
      rows: rows.length,
      text: content.innerText,
      html: content.innerHTML,
      rtl: document.documentElement.dir === "rtl",
      page_overflow: document.documentElement.scrollWidth > window.innerWidth,
      keyboard_focusable: !!refresh && document.activeElement === refresh,
      upload_form: !!el("projectFileUploadForm"),
      download_link: !!content.querySelector("a[download]"),
    };
    await fetch("/project-files-browser-report", {
      method: "POST",
      headers: {"Content-Type": "application/json"},
      body: JSON.stringify(result),
    });
  }
  await report("ar-IQ");
  await report("en");
})();
