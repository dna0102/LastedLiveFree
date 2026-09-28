"use strict";

VIEWS.splash = {
  render(app) {
    app.innerHTML =
      '<div class="splash"><div class="deco-ring"></div><div class="deco-arc"></div>' +
      '<div class="wordmark">' +
      LOGO_MARK +
      'Lasted <span class="live-badge">LIVE</span></div>' +
      '<div class="sub">Preparing your studio</div><div class="bar"><i></i></div></div>';
  },
};

const Login = { tab: "qr", label: "", qr: null, pollTimer: null, gen: 0 };

VIEWS.login = {
  render(app) {
    Login.qr = null;
    app.innerHTML =
      '<div class="login"><div class="deco-ring"></div>' +
      '<div class="login-card fade-in">' +
      '<div class="login-main">' +
      "<h1>Log in to Lasted Live</h1>" +
      '<p class="lead">Log in with your TikTok account to start streaming.</p>' +
      '<div class="login-tabs"><button data-tab="qr">' +
      'Scan QR code</button><button data-tab="session">Paste session</button></div>' +
      '<div id="login-pane"></div></div>' +
      '<aside class="login-side">' +
      '<div class="side-title">Free edition</div>' +
      '<div class="edition-box">' +
      "<p>Lasted Live Free runs one TikTok account on your own internet connection.</p>" +
      "<p>Running several accounts at once, each on its own proxy, is part of the full version.</p>" +
      "</div>" +
      '<div class="field"><label>Account name <span style="color:var(--muted);font-weight:500">(optional)</span></label>' +
      '<input class="input" id="lg-label" placeholder="e.g. Main account"></div>' +
      "</aside></div></div>";

    $("#lg-label").value = Login.label;
    $("#lg-label").oninput = (e) => {
      Login.label = e.target.value;
    };
    $$(".login-tabs button").forEach((b) => (b.onclick = () => setLoginTab(b.dataset.tab)));
    if (S.accounts.length) return showLoggedIn(S.accounts[0]);
    setLoginTab(Login.tab);
  },
  leave() {
    stopQRPoll();
  },
};

function setLoginTab(tab) {
  Login.tab = tab;
  $$(".login-tabs button").forEach((b) => b.classList.toggle("on", b.dataset.tab === tab));
  const pane = $("#login-pane");
  stopQRPoll();
  if (tab === "qr") {
    pane.innerHTML =
      '<div class="qr-wrap"><div class="qr-box" id="qr-box"><div class="spin" style="border-color:rgba(0,0,0,.15);border-top-color:#111"></div></div>' +
      '<div class="qr-steps"><h3>Scan to log in</h3><ol>' +
      "<li>Open the TikTok app on your phone</li>" +
      "<li>Tap the search icon, then the scan icon</li>" +
      "<li>Scan this code and tap Confirm</li></ol>" +
      '<div class="qr-status" id="qr-status"><span class="spin"></span>Generating code…</div></div></div>';
    startQR();
  } else {
    pane.innerHTML =
      '<div class="field"><label>Session cookies</label>' +
      '<textarea class="textarea" id="lg-cookies" spellcheck="false" placeholder="Paste cookies in any format: a Cookie header, JSON, a Cookie-Editor export or cookies.txt. It must include sessionid."></textarea></div>' +
      '<div class="field"><label>Device ID <span style="color:var(--muted);font-weight:500">(optional)</span></label>' +
      '<input class="input" id="lg-device" placeholder="Generated automatically if blank" spellcheck="false"></div>' +
      '<button class="btn primary lg" id="lg-session">Log in</button>';
    $("#lg-session").onclick = (e) => sessionLogin(e.currentTarget);
  }
}

function qrOverlay(text, btnLabel) {
  const box = $("#qr-box");
  if (!box) return;
  const old = $(".qr-over", box);
  if (old) old.remove();
  const o = node(
    '<div class="qr-over"><span>' +
      esc(text) +
      '</span><button class="btn sm">' +
      icon("refresh") +
      esc(btnLabel) +
      "</button></div>",
  );
  $("button", o).onclick = startQR;
  box.appendChild(o);
}

function qrStatus(text, cls = "", spin = false) {
  const s = $("#qr-status");
  if (s) {
    s.className = "qr-status " + cls;
    s.innerHTML = (spin ? '<span class="spin"></span>' : "") + esc(text);
  }
}

async function startQR() {
  const gen = ++Login.gen;
  stopQRPoll();
  const box = $("#qr-box");
  if (!box) return;
  box.innerHTML = '<div class="spin" style="border-color:rgba(0,0,0,.15);border-top-color:#111"></div>';
  qrStatus("Generating code…", "", true);
  try {
    const d = await api("POST", "/api/login/qr/start", { label: Login.label });
    if (gen !== Login.gen || !$("#qr-box")) return;
    Login.qr = d;
    $("#qr-box").innerHTML = '<img alt="QR code" src="data:image/png;base64,' + d.qrcode_png_b64 + '">';
    qrStatus("Waiting for scan…", "", true);
    Login.pollTimer = setInterval(() => pollQR(gen), 2000);
  } catch (e) {
    if (gen !== Login.gen) return;
    qrOverlay(e.message.length > 90 ? "Couldn't get a QR code" : e.message, "Try again");
    qrStatus("Check your connection and try again", "");
  }
}

function stopQRPoll() {
  if (Login.pollTimer) clearInterval(Login.pollTimer);
  Login.pollTimer = null;
}

async function pollQR(gen) {
  if (!Login.qr || gen !== Login.gen) return;
  let d;
  try {
    d = await api("GET", "/api/login/qr/poll?id=" + encodeURIComponent(Login.qr.login_id));
  } catch {
    return;
  }
  if (gen !== Login.gen) return;
  if (d.status === "scanned") qrStatus("Scanned — tap Confirm on your phone", "scan", true);
  else if (d.status === "pending") qrStatus("Waiting for scan…", "", true);
  else if (d.status === "expired") {
    stopQRPoll();
    Login.qr = null;
    qrOverlay("QR code expired", "Refresh");
    qrStatus("Code expired");
  } else if (d.status === "ok") {
    stopQRPoll();
    Login.qr = null;
    loginSucceeded(d.account);
  } else if (d.status === "error") {
    stopQRPoll();
    Login.qr = null;
    qrOverlay(d.detail || "Login failed", "Try again");
    qrStatus("Login failed");
  }
}

async function sessionLogin(btn) {
  const cookies = $("#lg-cookies").value.trim();
  if (!cookies) return toast("Paste your session cookies first", "err");
  await withSpinner(btn, "Logging in…", async () => {
    try {
      const d = await api("POST", "/api/login/session", {
        cookies,
        label: Login.label,
        device_id: $("#lg-device").value.trim(),
      });
      loginSucceeded(d.account);
    } catch (e) {
      toast(e.message, "err");
    }
  });
}

async function loginSucceeded(a) {
  logActivity("Logged in as " + nameOf(a), "ok");
  toast("Logged in as " + nameOf(a), "ok");
  await refreshAccounts();
  S.acctId = a.id;
  LS.set("lastAcct", a.id);
  Login.label = "";
  showLoggedIn(a);
}

function showLoggedIn(a) {
  const pane = $("#login-pane");
  if (!pane) return;
  $(".login-tabs").hidden = true;
  pane.innerHTML =
    '<div class="login-success fade-in"><img src="' +
    esc(avatarOf(a)) +
    '">' +
    "<h3>" +
    esc(nameOf(a)) +
    "</h3><p>@" +
    esc(handleOf(a)) +
    "</p>" +
    '<div class="row" style="justify-content:center;margin-top:18px">' +
    '<button class="btn" id="lg-studio">' +
    icon("video") +
    "Open Studio</button>" +
    '<button class="btn primary" id="lg-open">Open dashboard ' +
    icon("arrow") +
    "</button></div></div>";
  $("#lg-studio").onclick = () => openStudio();
  $("#lg-open").onclick = () => go("dashboard");
}
