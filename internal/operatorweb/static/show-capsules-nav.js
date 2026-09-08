"use strict";

(() => {
  let button = document.querySelector('[data-page="capsules"]');
  if (!button) {
    button = document.createElement("button");
    button.id = "capsulesNav";
    button.type = "button";
    button.className = "nav-button";
    button.dataset.page = "capsules";
    const anchor = el("templatesNav") || el("projectsNav");
    anchor?.insertAdjacentElement("afterend", button);
  }

  async function renderGlobalShowCapsuleLibrary(selectedPlan = null) {
    setPage("capsules");
    f019UpdateNav();
    setMessage(globalMessage, "");
    const capsules = await f019List();
    const plan = selectedPlan;

    content.innerHTML = `
      <div class="page-head">
        <div>
          <p class="eyebrow">${esc(f019T("capsule.eyebrow"))}</p>
          <h1>${esc(f019T("capsule.title"))}</h1>
          <p>${esc(f019T("capsule.subtitle"))}</p>
        </div>
        <button id="capsuleRefresh" class="button" type="button">${esc(f019T("capsule.refresh"))}</button>
      </div>
      <section class="card" style="margin-bottom:14px">
        <div class="section-title-row">
          <div>
            <h2>${esc(f019T("capsule.library"))}</h2>
            <p class="muted">${esc(f019T("capsule.library_detail"))}</p>
          </div>
        </div>
        <div style="margin-top:14px">${f019LibraryTable(capsules)}</div>
      </section>
      ${plan ? f019PlanCard(plan) : ""}
      <section class="card">
        <div class="grid cards">
          <article class="action-editor"><strong>${esc(f019T("capsule.show_lock"))}</strong></article>
          <article class="action-editor"><strong>${esc(f019T("capsule.owner_restore"))}</strong></article>
          <article class="action-editor"><strong>${esc(f019T("capsule.extensions_review"))}</strong></article>
          <article class="action-editor"><strong>${esc(f019T("capsule.presentation_local"))}</strong></article>
        </div>
      </section>`;

    el("capsuleRefresh")?.addEventListener("click", () => renderGlobalShowCapsuleLibrary().catch(f019ShowError));
    content.querySelectorAll(".capsule-plan").forEach((planButton) => planButton.addEventListener("click", async () => {
      try {
        const payload = await api(`/api/v1/show-capsules/imports/${encodeURIComponent(planButton.dataset.id)}/plan`);
        await renderGlobalShowCapsuleLibrary(payload.plan);
      } catch (error) { f019ShowError(error); }
    }));
    el("capsuleMaterialize")?.addEventListener("click", async () => {
      if (!plan?.capsule_id) return;
      try {
        const payload = await api(`/api/v1/show-capsules/imports/${encodeURIComponent(plan.capsule_id)}/materialize`, { method: "POST" });
        setMessage(globalMessage, f019T("capsule.restored"), "success");
        await loadProjects();
        await renderGlobalShowCapsuleLibrary(payload.result?.plan || plan);
      } catch (error) { f019ShowError(error); }
    });
  }

  button?.addEventListener("click", (event) => {
    event.preventDefault();
    const render = state.project ? renderShowCapsuleWorkspace : renderGlobalShowCapsuleLibrary;
    render().catch(f019ShowError);
  });

  el("languageSelect")?.addEventListener("change", () => {
    if (state.page === "capsules" && !state.project) {
      renderGlobalShowCapsuleLibrary().catch(f019ShowError);
    }
  });

  f019UpdateNav();
})();
