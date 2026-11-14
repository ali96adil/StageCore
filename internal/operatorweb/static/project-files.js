"use strict";

// The Project Files UI is progressive enhancement. Its navigation is hidden
// unless the separately opted-in, browser-authenticated, Project-scoped
// availability endpoint succeeds. No old global /projects or raw-hash Vault
// API is used as a fallback.
const scFileCopy = {
  nav: { en: "Files", "ar-IQ": "الملفات" },
  title: { en: "Project Files", "ar-IQ": "ملفات المشروع" },
  eyebrow: { en: "PROJECT MEDIA", "ar-IQ": "وسائط المشروع" },
  description: { en: "Files stay inside this Project's Vault-backed workspace.", "ar-IQ": "الملفات تبقى ضمن مساحة هذا المشروع المرتبطة بالـVault." },
  refresh: { en: "Refresh", "ar-IQ": "تحديث" },
  next: { en: "Next page", "ar-IQ": "الصفحة التالية" },
  download: { en: "Download", "ar-IQ": "تنزيل" },
  name: { en: "Name", "ar-IQ": "الاسم" },
  type: { en: "Type", "ar-IQ": "النوع" },
  size: { en: "Size", "ar-IQ": "الحجم" },
  policy: { en: "Storage policy", "ar-IQ": "سياسة التخزين" },
  usage: { en: "File-version size total", "ar-IQ": "إجمالي أحجام نسخ الملفات" },
  usageHint: {
    en: "Logical bytes across file versions; shared Vault content may be physically stored once.",
    "ar-IQ": "الحجم المنطقي لنسخ الملفات؛ قد يُخزّن محتوى Vault المشترك فعلياً مرة واحدة.",
  },
  showing: { en: "Showing", "ar-IQ": "المعروض" },
  entries: { en: "entries", "ar-IQ": "عنصر" },
  pageCategories: { en: "Types on this page:", "ar-IQ": "الأنواع بهذه الصفحة:" },
  empty: { en: "No media is registered for this Project yet.", "ar-IQ": "ماكو ملفات وسائط مسجّلة لهذا المشروع لحد الآن." },
  unavailable: { en: "This Project Files preview is unavailable or access was revoked. No cross-Project fallback is allowed.", "ar-IQ": "عرض ملفات المشروع غير متاح أو تم سحب الصلاحية. ماكو وصول بديل إلى ملفات المشاريع الأخرى." },
  pagination: { en: "This Project has more than 500 files. Cursor pagination must be enabled before listing them.", "ar-IQ": "هذا المشروع بيه أكثر من 500 ملف. لازم تفعيل عرض الصفحات حتى تنعرض القائمة بأمان." },
  other: { en: "Other", "ar-IQ": "أخرى" },
  upload: { en: "Upload file", "ar-IQ": "رفع ملف" },
  chooseFile: { en: "Choose a file", "ar-IQ": "اختار ملف" },
  assetName: { en: "Workspace name (optional)", "ar-IQ": "اسم الملف داخل المشروع (اختياري)" },
  uploadLimit: { en: "Up to 1 GiB per file. Uploads stream into managed Vault storage.", "ar-IQ": "لحد 1 غيغابايت للملف. الرفع ينحفظ تدريجياً بخزن Vault المُدار." },
  uploadFailed: { en: "The file could not be uploaded. Check access and available storage, then retry.", "ar-IQ": "تعذر رفع الملف. تحقق من الصلاحية ومساحة الخزن وجرب مرة ثانية." },
};
function scFileT(key) {
  const locale = el("languageSelect")?.value === "en" ? "en" : "ar-IQ";
  return scFileCopy[key]?.[locale] || scFileCopy[key]?.en || key;
}

const scFilesNav = document.createElement("button");
scFilesNav.type = "button";
scFilesNav.id = "projectFilesNav";
scFilesNav.className = "nav-button hidden";
scFilesNav.dataset.page = "files";
scFilesNav.textContent = scFileT("nav");
document.querySelector('#workspaceNav [data-page="cues"]')?.insertAdjacentElement("afterend", scFilesNav);

let scFilesProbeGeneration = 0;
let scFilesRenderGeneration = 0;

async function scFilesUpdateForProject(projectID) {
  const generation = ++scFilesProbeGeneration;
  scFilesNav.classList.add("hidden");
  if (!projectID || !state.user) return;
  try {
    // The endpoint returns 204 only for this live account's media.read grant.
    await api(`/api/v1/projects/${encodeURIComponent(projectID)}/files/availability`);
    if (generation !== scFilesProbeGeneration || state.project?.project_id !== projectID || !state.user) return;
    scFilesNav.classList.remove("hidden");
  } catch (_) {
    // Includes 404 (feature disabled, Project absent or denied), 401 revoked,
    // and transport failure: fail closed, without exposing Project metadata.
  }
}

async function renderScopedProjectFiles(cursor = "") {
  const generation = ++scFilesRenderGeneration;
  const projectID = state.project?.project_id;
  if (!projectID) return;
  try {
    const base = `/api/v1/projects/${encodeURIComponent(projectID)}/files`;
    const query = new URLSearchParams({page_size: "100", cursor});
    const payload = await api(`${base}?${query}`);
    if (generation !== scFilesRenderGeneration || !state.project || state.project.project_id !== projectID || state.page !== "files") return;
    const workspace = payload?.workspace;
    if (!payload?.scoped || workspace?.project_id !== projectID ||
        !Array.isArray(workspace.files)) throw new Error("Invalid scoped Project Files response");

    const rows = workspace.files;
    const counts = { AUDIO: 0, VIDEO: 0, IMAGE: 0, OTHER: 0 };
    for (const item of rows) {
      counts[Object.hasOwn(counts, item.category) ? item.category : "OTHER"]++;
    }
    const humanSize = (value) => {
      const bytes = Number(value);
      if (!Number.isFinite(bytes) || bytes < 0) return "—";
      if (bytes < 1024) return `${bytes} B`;
      if (bytes < 1048576) return `${(bytes / 1024).toFixed(1)} KiB`;
      if (bytes < 1073741824) return `${(bytes / 1048576).toFixed(1)} MiB`;
      return `${(bytes / 1073741824).toFixed(2)} GiB`;
    };
    const table = rows.length
      ? `<div class="table-wrap"><table>
          <thead><tr><th>${esc(scFileT("name"))}</th><th>${esc(scFileT("type"))}</th>
            <th>${esc(scFileT("size"))}</th><th>${esc(scFileT("policy"))}</th></tr></thead>
          <tbody>${rows.map((item) => `<tr>
            <td><strong>${esc(item.media_asset_name)}</strong><br>
              <small class="muted">${esc(item.original_filename || "—")}</small></td>
            <td>${pill(item.category === "OTHER" ? scFileT("other") : item.category)}</td>
            <td>${esc(humanSize(item.size_bytes))}</td>
            <td>${esc(item.asset_policy)}${payload.download_available === true && item.content_version_id && item.asset_policy !== "REFERENCE_ONLY"
              ? `<br><a download href="${esc(`${base}/${encodeURIComponent(item.content_version_id)}/download`)}">${esc(scFileT("download"))}</a>` : ""}</td>
          </tr>`).join("")}</tbody></table></div>`
      : `<div class="empty">${esc(scFileT("empty"))}</div>`;

    content.innerHTML = `
      <div class="page-head">
        <div><p class="eyebrow">${esc(scFileT("eyebrow"))}</p>
          <h1>${esc(scFileT("title"))}</h1><p>${esc(scFileT("description"))}</p></div>
        <button id="refreshScopedFiles" class="button" type="button">${esc(scFileT("refresh"))}</button>
      </div>
      <section class="card" style="margin-bottom:14px">
        <div class="section-title-row">
          <h2>${esc(workspace.project_name)}</h2>
          ${pill(`${scFileT("showing")} ${rows.length} / ${esc(String(workspace.entry_count ?? rows.length))} ${scFileT("entries")}`, "neutral")}
        </div>
        <div class="toolbar" style="margin-top:12px">
          ${pill(`${scFileT("usage")}: ${humanSize(workspace.total_bytes)}`, "neutral")}
          <span class="muted">${esc(scFileT("pageCategories"))}</span>
          ${["AUDIO","VIDEO","IMAGE","OTHER"].map((kind) => pill(`${kind}: ${counts[kind]}`)).join("")}
        </div>
        <small class="muted">${esc(scFileT("usageHint"))}</small>
      </section>
      <section class="card">
        ${payload.upload_available === true ? `<form id="projectFileUploadForm" class="project-file-upload-form">
          <label>${esc(scFileT("chooseFile"))}<input id="projectFileUploadInput" type="file" required></label>
          <label>${esc(scFileT("assetName"))}<input id="projectFileAssetName" type="text" maxlength="160"></label>
          <p class="muted">${esc(scFileT("uploadLimit"))}</p>
          <button id="projectFileUploadButton" class="button primary" type="submit">${esc(scFileT("upload"))}</button>
        </form>` : ""}
        ${table}
        ${workspace.next_cursor ? `<button id="nextScopedFiles" class="button" type="button">${esc(scFileT("next"))}</button>` : ""}
      </section>`;
    el("projectFileUploadForm")?.addEventListener("submit", async (event) => {
      event.preventDefault();
      const file = el("projectFileUploadInput")?.files?.[0];
      const button = el("projectFileUploadButton");
      if (!file || !button) return;
      button.disabled = true;
      try {
        await api(base, {
          method: "POST",
          headers: {
            "Content-Type": file.type || "application/octet-stream",
            "X-Project-File-Name": encodeURIComponent(file.name),
            "X-Project-Asset-Name": encodeURIComponent(el("projectFileAssetName")?.value || ""),
          },
          body: file,
        });
        await renderScopedProjectFiles();
      } catch (error) {
        setMessage(globalMessage, errorMessage(error) || scFileT("uploadFailed"), "error");
        button.disabled = false;
      }
    });
    el("nextScopedFiles")?.addEventListener("click", (event) => {
      event.currentTarget.disabled = true;
      renderScopedProjectFiles(workspace.next_cursor);
    });
    el("refreshScopedFiles")?.addEventListener("click", () => {
      renderScopedProjectFiles().catch((err) => setMessage(globalMessage, errorMessage(err), "error"));
    });
  } catch (error) {
    if (generation !== scFilesRenderGeneration || state.project?.project_id !== projectID || state.page !== "files") return;
    if (error.status === 404 || error.status === 403) {
      scFilesNav.classList.add("hidden");
    }
    content.innerHTML = `<section class="card"><p class="muted">${esc(error.payload?.error_code === "PROJECT_FILES_PAGINATION_REQUIRED" ? scFileT("pagination") : scFileT("unavailable"))}</p></section>`;
  }
}

scFilesNav.addEventListener("click", () => {
  if (scFilesNav.classList.contains("hidden")) return;
  navigate("files");
});
el("languageSelect")?.addEventListener("change", () => {
  scFilesNav.textContent = scFileT("nav");
  if (state.page === "files" && state.project) renderScopedProjectFiles();
});
