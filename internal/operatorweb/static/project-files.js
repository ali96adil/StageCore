"use strict";

// The Project Files UI is progressive enhancement. Its navigation is hidden
// unless the separately opted-in, browser-authenticated, Project-scoped
// availability endpoint succeeds. No old global /projects or raw-hash Vault
// API is used as a fallback.
const scFileCopy = {
  nav: { en: "Files", "ar-IQ": "الملفات" },
  title: { en: "Project Files", "ar-IQ": "ملفات المشروع" },
  eyebrow: { en: "PROJECT MEDIA", "ar-IQ": "وسائط المشروع" },
  description: { en: "Files referenced by this Project. Read-only: uploads, downloads and deletions are not enabled yet.", "ar-IQ": "الملفات المرتبطة بهذا المشروع. العرض للقراءة فقط؛ الرفع والتنزيل والحذف غير مفعّلة حالياً." },
  refresh: { en: "Refresh", "ar-IQ": "تحديث" },
  name: { en: "Name", "ar-IQ": "الاسم" },
  type: { en: "Type", "ar-IQ": "النوع" },
  size: { en: "Size", "ar-IQ": "الحجم" },
  policy: { en: "Storage policy", "ar-IQ": "سياسة التخزين" },
  empty: { en: "No media is registered for this Project yet.", "ar-IQ": "ماكو ملفات وسائط مسجّلة لهذا المشروع لحد الآن." },
  unavailable: { en: "This Project Files preview is unavailable or access was revoked. No cross-Project fallback is allowed.", "ar-IQ": "عرض ملفات المشروع غير متاح أو تم سحب الصلاحية. ماكو وصول بديل إلى ملفات المشاريع الأخرى." },
  pagination: { en: "This Project has more than 500 files. Cursor pagination must be enabled before listing them.", "ar-IQ": "هذا المشروع بيه أكثر من 500 ملف. لازم تفعيل عرض الصفحات حتى تنعرض القائمة بأمان." },
  other: { en: "Other", "ar-IQ": "أخرى" },
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

async function renderScopedProjectFiles() {
  const projectID = state.project?.project_id;
  if (!projectID) return;
  try {
    const payload = await api(`/api/v1/projects/${encodeURIComponent(projectID)}/files`);
    if (!state.project || state.project.project_id !== projectID || state.page !== "files") return;
    const workspace = payload?.workspace;
    if (!payload?.read_only || !payload?.scoped || workspace?.project_id !== projectID ||
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
      return `${(bytes / 1048576).toFixed(1)} MiB`;
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
            <td>${esc(item.asset_policy)}</td>
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
          ${pill(String(rows.length) + " files", "neutral")}
        </div>
        <div class="toolbar" style="margin-top:12px">
          ${["AUDIO","VIDEO","IMAGE","OTHER"].map((kind) => pill(`${kind}: ${counts[kind]}`)).join("")}
        </div>
      </section>
      <section class="card">${table}</section>`;
    el("refreshScopedFiles")?.addEventListener("click", () => {
      renderScopedProjectFiles().catch((err) => setMessage(globalMessage, errorMessage(err), "error"));
    });
  } catch (error) {
    if (state.project?.project_id !== projectID || state.page !== "files") return;
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
