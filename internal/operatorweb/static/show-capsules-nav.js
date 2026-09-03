"use strict";

(() => {
  if (document.querySelector('[data-page="capsules"]')) return;
  const workspaceNav = document.getElementById("workspaceNav");
  if (!workspaceNav) return;
  const button = document.createElement("button");
  button.type = "button";
  button.className = "nav-button";
  button.dataset.page = "capsules";
  button.textContent = "Capsules / حزم العرض";
  const preflight = workspaceNav.querySelector('[data-page="preflight"]');
  workspaceNav.insertBefore(button, preflight || null);
})();
