"use strict";

(async function boot() {
  wireWindow();
  go("splash");
  const t0 = Date.now();
  if ((await Native.platform()) === "darwin") document.body.classList.add("is-mac");

  // Checking encoders runs test encodes, so don't wait for it.
  loadDevices().then(checkFFmpeg);

  await refreshAccounts();
  await sleep(Math.max(0, 900 - (Date.now() - t0)));
  go(S.accounts.length ? "dashboard" : "login");
  startBackground();

  document.addEventListener("keydown", (e) => {
    if (e.key !== "Escape") return;
    if ($(".menu")) return closeMenus();
    const top = modals[modals.length - 1];
    if (top) top.close();
  });
  // No browser context menu, except in text fields.
  document.addEventListener("contextmenu", (e) => {
    if (!e.target.closest("input, textarea, [contenteditable]")) e.preventDefault();
  });
})();
