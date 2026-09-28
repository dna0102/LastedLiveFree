// App shell: title bar, view switching, notifications and background polling.
"use strict";

const VIEWS = {}; // name -> { render(), leave() }

function go(view, opts = {}) {
  closeMenus();
  const prev = S.view;
  if (prev !== view && VIEWS[prev] && VIEWS[prev].leave) VIEWS[prev].leave();
  S.view = view;
  const app = $("#app");
  app.innerHTML = "";
  if (VIEWS[view]) VIEWS[view].render(app, opts);
  renderShell();
}

function renderShell() {
  $("#brand").innerHTML = LOGO_MARK + '<span>Lasted Live</span><span class="free-badge">FREE</span>';
  const inApp = ["dashboard", "studio", "customise"].includes(S.view);
  const nav = $("#nav-tabs");
  nav.hidden = !inApp;
  $$("button", nav).forEach((b) => {
    b.classList.toggle("on", b.dataset.view === S.view);
    b.onclick = () => {
      if (b.dataset.view === "studio" && !S.accounts.length) return toast("Log in first", "err");
      go(b.dataset.view);
    };
  });
  const acts = $("#tb-actions");
  if (!inApp) {
    acts.innerHTML = "";
    return;
  }
  acts.innerHTML =
    (liveCount() ? '<span class="live-counter"><i></i>LIVE</span>' : "") +
    '<button class="tb-btn" id="tb-bell" title="Notifications">' +
    icon("bell") +
    (S.unread ? '<span class="badge">' + Math.min(S.unread, 99) + "</span>" : "") +
    "</button>" +
    '<button class="tb-btn" id="tb-help" title="Help">' +
    icon("help") +
    "</button>" +
    '<span class="tb-sep"></span>';
  $("#tb-bell").onclick = (e) => openActivity(e.currentTarget);
  $("#tb-help").onclick = openHelp;
}

function openActivity(anchor) {
  S.unread = 0;
  renderShell();
  const items = S.activity.length
    ? S.activity.slice(0, 12).map((a) => ({
        icon: a.kind === "live" ? "radio" : a.kind === "err" ? "alert" : "info",
        html:
          esc(a.text) +
          ' <span style="color:var(--muted);font-weight:500">· ' +
          new Date(a.t).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }) +
          "</span>",
      }))
    : [{ label: "No activity yet", disabled: true }];
  openMenu($("#tb-bell"), [{ header: "Activity" }, ...items], "right");
}

function openHelp() {
  openModal({
    title: "Lasted Live Free",
    sub: "TikTok LIVE studio, free edition",
    body:
      '<div class="kv"><span>Version</span><b>1.0.0</b></div>' +
      '<div class="kv"><span>ffmpeg</span><b>' +
      (S.devices.ffmpeg ? "Installed" : "Not installed") +
      "</b></div>" +
      '<div class="kv"><span>Encoder</span><b>' +
      esc(codecLabel(activeCodec())) +
      "</b></div>" +
      '<div class="kv"><span>Account</span><b>' +
      (acct() ? esc(nameOf(acct())) : "Not logged in") +
      "</b></div>" +
      '<p class="hint" style="margin-top:14px">Everything runs on your own internet connection, and the video goes straight to TikTok\'s ingest, just like TikTok LIVE Studio. Host chat is not available because TikTok requires a signature this app does not generate.</p>' +
      '<p class="hint" style="margin-top:10px">The free edition runs one account. Streaming several accounts at once, each on its own proxy, is part of the full version.</p>',
  });
}

// Window buttons; double-clicking the title bar maximises.
function wireWindow() {
  $("#wc-min").innerHTML = icon("min");
  $("#wc-max").innerHTML = icon("max");
  $("#wc-close").innerHTML = icon("close");
  $("#wc-min").onclick = () => Native.minimise();
  $("#wc-max").onclick = () => Native.toggleMax();
  $("#wc-close").onclick = () => Native.quit();
  $("#titlebar").addEventListener("dblclick", (e) => {
    if (!e.target.closest("button")) Native.toggleMax();
  });
}

// Background refresh: the account, its LIVE state and stats.
async function backgroundTick() {
  await refreshAccounts();
  const ids = Object.keys(S.live);
  await Promise.all(
    ids.map(async (id) => {
      try {
        S.perf[id] = Object.assign(
          S.perf[id] || {},
          await api("GET", "/api/accounts/" + encodeURIComponent(id) + "/stats"),
        );
      } catch {}
    }),
  );
  renderShell();
  const v = VIEWS[S.view];
  if (v && v.tick) v.tick();
}
function startBackground() {
  backgroundTick();
  setInterval(backgroundTick, 6000);
}
