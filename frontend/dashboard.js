"use strict";

const Dash = {};

VIEWS.dashboard = {
  render(app) {
    app.innerHTML =
      '<div class="page"><div class="page-inner fade-in">' +
      '<div class="page-head"><div><h1>Dashboard</h1><p>Your account and LIVE at a glance.</p></div>' +
      '<button class="btn" id="db-refresh">' +
      icon("refresh") +
      "Refresh</button></div>" +
      '<div class="sum-grid" id="db-sum"></div>' +
      '<div class="card table-card"><div class="acct-table" id="db-table"></div></div>' +
      "</div></div>";
    $("#db-refresh").onclick = async (e) => {
      await withSpinner(e.currentTarget, "", backgroundTick);
    };
    drawDashboard();
    Dash.clock = setInterval(updateDurations, 1000);
  },
  leave() {
    clearInterval(Dash.clock);
  },
  tick() {
    drawDashboard();
  },
};

function drawDashboard() {
  const a = acct();
  const live = !!a && isLive(a.id);
  const viewers = live ? viewersNow(S.perf[a.id]) : null;
  const box = $("#db-sum");
  if (box)
    box.innerHTML =
      sumCard("radio", "Status", live ? "LIVE" : "Offline", live ? "live" : "") +
      sumCard("users", "Viewers now", viewers != null ? fmt(viewers) : "—", "") +
      sumCard("userplus", "Followers", fmt(a ? a.follower_count : 0), "") +
      sumCard("heart", "Likes", fmt(a ? a.total_likes : 0), "");
  drawDashTable();
}

function sumCard(ic, label, value, cls) {
  return (
    '<div class="sum ' +
    cls +
    '"><div class="sum-ic">' +
    icon(ic) +
    "</div><div><b>" +
    value +
    "</b><span>" +
    label +
    "</span></div></div>"
  );
}

function drawDashTable() {
  const t = $("#db-table");
  if (!t) return;
  const a = acct();
  if (!a) {
    t.innerHTML =
      '<div class="empty big-empty">' +
      icon("userplus", "ico big") +
      "<b>You're not logged in</b><p>Log in with your TikTok account to get started.</p>" +
      '<button class="btn primary" onclick="go(\'login\')">Log in</button></div>';
    return;
  }
  t.innerHTML =
    '<div class="tr th"><div>Account</div><div>Status</div><div>Viewers</div><div>Followers</div><div>Likes</div><div></div></div>' +
    rowHTML(a);
  const row = $(".tr[data-id]", t);
  row.onclick = (e) => {
    if (e.target.closest("[data-act]")) return;
    openStudio(a.id);
  };
  $$("[data-act]", row).forEach(
    (b) =>
      (b.onclick = (e) => {
        e.stopPropagation();
        rowAction(a.id, b.dataset.act, b);
      }),
  );
}

function rowHTML(a) {
  const live = isLive(a.id);
  const p = S.perf[a.id] || {};
  const sess = S.live[a.id];
  const status = live
    ? '<span class="tag live"><span class="dot"></span>LIVE</span><span class="dur" data-start="' +
      (sess.started_at || 0) +
      '"></span>'
    : a.has_cookies
      ? '<span class="st-off">Offline</span>'
      : '<span class="tag warn">Session missing</span>';
  const main = live
    ? '<button class="btn sm danger" data-act="end">' + icon("close", "ico xs") + "End</button>"
    : '<button class="btn sm primary" data-act="golive">Go LIVE</button>';
  return (
    '<div class="tr' +
    (live ? " is-live" : "") +
    '" data-id="' +
    esc(a.id) +
    '">' +
    '<div class="acct-cell"><div class="av-wrap"><img src="' +
    esc(avatarOf(a)) +
    '" onerror="this.style.visibility=\'hidden\'">' +
    (live ? '<i class="ring"></i>' : "") +
    "</div>" +
    '<div class="nm"><b>' +
    esc(nameOf(a)) +
    (a.verified ? " " + icon("checkc", "ico xs verified") : "") +
    "</b><span>@" +
    esc(handleOf(a)) +
    "</span></div></div>" +
    '<div class="status-cell">' +
    status +
    "</div>" +
    '<div class="num">' +
    (live && viewersNow(p) != null
      ? fmt(viewersNow(p)) + '<span class="sub-num">' + fmt(p.views) + " views</span>"
      : '<span class="muted">—</span>') +
    "</div>" +
    '<div class="num">' +
    fmt(a.follower_count) +
    "</div>" +
    '<div class="num">' +
    fmt(a.total_likes) +
    "</div>" +
    '<div class="acts">' +
    main +
    '<button class="btn sm" data-act="studio">Studio</button>' +
    '<button class="icon-btn sm" data-act="more" title="More">' +
    icon("more") +
    "</button></div></div>"
  );
}

function updateDurations() {
  const now = Date.now() / 1000;
  $$(".dur[data-start]").forEach((d) => {
    const s = +d.dataset.start;
    d.textContent = s ? hms(now - s) : "";
  });
}

async function rowAction(id, act, btn) {
  const a = acct(id);
  if (!a) return;
  if (act === "studio") return openStudio(id);
  if (act === "golive") {
    openStudio(id);
    return setTimeout(() => startGoLive(), 60);
  }
  if (act === "end") return endLiveFlow(id);
  if (act === "more") {
    openMenu(
      btn,
      [
        { header: nameOf(a) },
        { icon: "refresh", label: "Refresh profile", onClick: () => refreshProfile(a) },
        "-",
        { icon: "logout", label: "Log out", danger: true, onClick: () => logOut(a) },
      ],
      "right",
    );
  }
}

async function refreshProfile(a) {
  try {
    await api("POST", "/api/accounts/" + encodeURIComponent(a.id) + "/validate");
    toast("Profile refreshed", "ok");
    await backgroundTick();
  } catch (e) {
    toast("Session check failed: " + e.message, "err");
  }
}

async function logOut(a) {
  const ok = await confirmBox(
    "Log out of " + esc(nameOf(a)) + "?",
    "This removes the saved session from Lasted Live. You can log in again, or with a different account, afterwards." +
      (isLive(a.id) ? " Your LIVE will end." : ""),
    "Log out",
  );
  if (!ok) return;
  try {
    await api("DELETE", "/api/accounts/" + encodeURIComponent(a.id));
    toast("Logged out", "ok");
    await refreshAccounts();
    go("login");
  } catch (e) {
    toast(e.message, "err");
  }
}
