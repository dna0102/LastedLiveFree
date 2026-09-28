// Shared helpers: DOM utilities, the API client, the native bridge, app state,
// modals and menus.
"use strict";

const $ = (s, root = document) => root.querySelector(s);
const $$ = (s, root = document) => [...root.querySelectorAll(s)];
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const esc = (s) =>
  String(s ?? "").replace(
    /[&<>"']/g,
    (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c],
  );
const debounce = (fn, ms) => {
  let t;
  return (...a) => {
    clearTimeout(t);
    t = setTimeout(() => fn(...a), ms);
  };
};
const uid = () => Math.random().toString(36).slice(2, 9);
function node(html) {
  const t = document.createElement("template");
  t.innerHTML = html.trim();
  return t.content.firstElementChild;
}
function fmt(n) {
  n = +n || 0;
  if (n >= 1e6) return (n / 1e6).toFixed(1).replace(/\.0$/, "") + "M";
  if (n >= 1e4) return (n / 1e3).toFixed(1).replace(/\.0$/, "") + "K";
  return n.toLocaleString();
}
function hms(s) {
  s = Math.max(0, s | 0);
  const h = (s / 3600) | 0,
    m = ((s % 3600) / 60) | 0,
    x = s % 60;
  return (h ? h + ":" + String(m).padStart(2, "0") : m) + ":" + String(x).padStart(2, "0");
}

const LS = {
  get(k, d) {
    try {
      const v = localStorage.getItem("ll." + k);
      return v == null ? d : JSON.parse(v);
    } catch {
      return d;
    }
  },
  set(k, v) {
    try {
      localStorage.setItem("ll." + k, JSON.stringify(v));
    } catch {}
  },
};

async function api(method, path, body) {
  const opt = { method, headers: {} };
  if (body !== undefined) {
    opt.headers["Content-Type"] = "application/json";
    opt.body = JSON.stringify(body);
  }
  const r = await fetch(path, opt);
  const data = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error(data.error || "Request failed (" + r.status + ")");
  return data;
}

function toast(msg, kind = "") {
  const t = $("#toast");
  t.className = kind;
  t.innerHTML = icon(kind === "ok" ? "checkc" : kind === "err" ? "alert" : "info") + "<span>" + esc(msg) + "</span>";
  requestAnimationFrame(() => t.classList.add("show"));
  clearTimeout(toast.timer);
  toast.timer = setTimeout(() => t.classList.remove("show"), kind === "err" ? 4500 : 2600);
}

// Features that are only in the paid version are greyed out with this tag.
const PAID_TAG = '<span class="tag paid">Paid</span>';
function paidOnly(what) {
  toast(what + " is available in the paid version of Lasted Live");
}

// Wails bridge. Falls back to browser behaviour when run outside the app.
const Native = {
  get app() {
    return window.go && window.go.main && window.go.main.App;
  },
  minimise() {
    if (window.runtime) window.runtime.WindowMinimise();
  },
  toggleMax() {
    if (window.runtime) window.runtime.WindowToggleMaximise();
  },
  quit() {
    if (window.runtime) window.runtime.Quit();
  },
  async pickFile(kind) {
    try {
      return (await api("POST", "/api/pick-file", { kind })).path || "";
    } catch (e) {
      if (this.app && this.app.PickFile) {
        try {
          return await this.app.PickFile(kind);
        } catch {
          return "";
        }
      }
      toast("Couldn't open the file dialog: " + e.message, "err");
      return "";
    }
  },
  async platform() {
    try {
      return this.app ? await this.app.Platform() : "web";
    } catch {
      return "web";
    }
  },
  copy(text) {
    if (window.runtime && window.runtime.ClipboardSetText) return window.runtime.ClipboardSetText(text);
    if (navigator.clipboard) return navigator.clipboard.writeText(text);
  },
};

const S = {
  view: "splash",
  accounts: [],
  acctId: LS.get("lastAcct", null),
  devices: { encoders: { available: ["libx264"], default: "libx264" }, mics: [], ffmpeg: null },
  tags: [],
  gifts: null,
  scenes: [],
  live: {}, // accountId -> session
  perf: {}, // accountId -> stats snapshot
  polls: {}, // accountId -> active poll
  ending: {}, // accountId -> true while we're ending it
  activity: [],
  unread: 0,
};
// Encoder settings. Output is always 60 FPS.
const E = Object.assign({ bitrate: 6000, codec: "", mic: "", micVol: 1, vidVol: 1 }, LS.get("enc", {}));
function saveEnc() {
  LS.set("enc", E);
}

// Current viewers. Not the same as views, which only ever go up.
const viewersNow = (p) => (p && p.viewers != null ? +p.viewers : null);
const acct = (id = S.acctId) => S.accounts.find((a) => a.id === id);
const isLive = (id = S.acctId) => !!S.live[id];
const liveCount = () => Object.keys(S.live).length;
// TikTok images are loaded through the backend, which caches them on disk.
function cdn(url) {
  if (!url || !/^https:\/\//.test(url)) return url || "";
  return "/api/img?u=" + encodeURIComponent(url);
}
const avatarOf = (a) => (a && cdn(a.avatar_url)) || "";
const nameOf = (a) => (a && (a.nickname || a.label || a.username || a.id)) || "";
const handleOf = (a) => (a && (a.username || a.display_id || a.id)) || "";
function codecLabel(c) {
  return (
    {
      h264_nvenc: "NVIDIA NVENC (GPU)",
      h264_amf: "AMD AMF (GPU)",
      h264_qsv: "Intel Quick Sync (GPU)",
      h264_videotoolbox: "Apple VideoToolbox (GPU)",
      libx264: "x264 (CPU)",
    }[c] || c
  );
}
function activeCodec() {
  const av = S.devices.encoders.available || ["libx264"];
  return av.includes(E.codec) ? E.codec : S.devices.encoders.default || "libx264";
}

function logActivity(text, kind = "") {
  S.activity.unshift({ t: Date.now(), text, kind });
  S.activity = S.activity.slice(0, 50);
  S.unread++;
  if (typeof renderShell === "function") renderShell();
}

async function refreshAccounts() {
  try {
    const d = await api("GET", "/api/accounts");
    S.accounts = d.accounts || [];
    const live = {};
    for (const a of S.accounts) if (a.live && a.session) live[a.id] = Object.assign({}, S.live[a.id] || {}, a.session);
    // LIVEs that ended some other way, e.g. by logging out
    for (const id of Object.keys(S.live)) {
      if (!live[id] && !S.ending?.[id]) {
        const a = acct(id);
        logActivity((a ? nameOf(a) : id) + "'s LIVE ended");
        delete S.polls[id];
      }
    }
    S.live = live;
    if (S.acctId && !acct()) S.acctId = S.accounts.length ? S.accounts[0].id : null;
    if (!S.acctId && S.accounts.length) S.acctId = S.accounts[0].id;
  } catch {}
}

const modals = [];
function openModal({ title, sub = "", body = "", foot = [], size = "", onClose }) {
  const bg = node(
    '<div class="modal-bg"><div class="modal ' +
      size +
      '">' +
      '<div class="modal-head"><h3>' +
      title +
      '</h3><button class="icon-btn" data-x>' +
      icon("close") +
      "</button></div>" +
      (sub ? '<div class="modal-sub">' + sub + "</div>" : "") +
      '<div class="modal-body"></div><div class="modal-foot"></div></div></div>',
  );
  const b = $(".modal-body", bg);
  if (typeof body === "string") b.innerHTML = body;
  else if (body) b.appendChild(body);
  const f = $(".modal-foot", bg);
  if (!foot.length) f.remove();
  foot.forEach((bt) => {
    const btn = node('<button class="btn ' + (bt.cls || "") + '">' + bt.label + "</button>");
    btn.onclick = () => bt.fn(btn, bg);
    f.appendChild(btn);
  });
  bg.close = () => {
    bg.remove();
    const i = modals.indexOf(bg);
    if (i >= 0) modals.splice(i, 1);
    if (onClose) onClose();
  };
  $("[data-x]", bg).onclick = bg.close;
  bg.addEventListener("mousedown", (e) => {
    if (e.target === bg) bg.close();
  });
  document.body.appendChild(bg);
  modals.push(bg);
  return bg;
}
async function withSpinner(btn, label, fn) {
  const old = btn.innerHTML;
  btn.disabled = true;
  btn.innerHTML = '<span class="spin"></span>' + (label ? " " + label : "");
  try {
    return await fn();
  } finally {
    if (btn.isConnected) {
      btn.disabled = false;
      btn.innerHTML = old;
    }
  }
}
function confirmBox(title, text, okLabel = "Confirm") {
  return new Promise((resolve) => {
    let done = false;
    const m = openModal({
      title,
      body: '<p style="font-size:14px;color:var(--text-2);line-height:1.5">' + text + "</p>",
      onClose: () => {
        if (!done) resolve(false);
      },
      foot: [
        { label: "Cancel", cls: "ghost", fn: () => m.close() },
        {
          label: okLabel,
          cls: "primary",
          fn: () => {
            done = true;
            resolve(true);
            m.close();
          },
        },
      ],
    });
  });
}

function openMenu(anchor, items, align = "left") {
  closeMenus();
  const m = node('<div class="menu"></div>');
  items.forEach((it) => {
    if (it === "-") return m.appendChild(node("<hr>"));
    if (it.header) return m.appendChild(node('<div class="mh">' + esc(it.header) + "</div>"));
    const b = node(
      '<button class="' +
        (it.danger ? "danger" : it.paid ? "paid" : "") +
        '"' +
        (it.disabled ? " disabled" : "") +
        ">" +
        (it.icon ? icon(it.icon) : "") +
        "<span>" +
        (it.html || esc(it.label)) +
        "</span>" +
        (it.paid ? PAID_TAG : "") +
        "</button>",
    );
    b.onclick = () => {
      closeMenus();
      if (it.paid) return paidOnly(it.paid);
      if (it.onClick) it.onClick();
    };
    m.appendChild(b);
  });
  document.body.appendChild(m);
  const r = anchor.getBoundingClientRect();
  let x = align === "right" ? r.right - m.offsetWidth : r.left;
  let y = r.bottom + 6;
  if (y + m.offsetHeight > innerHeight - 8) y = r.top - m.offsetHeight - 6;
  m.style.left = Math.max(8, Math.min(x, innerWidth - m.offsetWidth - 8)) + "px";
  m.style.top = Math.max(8, y) + "px";
  const off = (e) => {
    if (!m.contains(e.target)) closeMenus();
  };
  setTimeout(() => document.addEventListener("mousedown", off), 0);
  m.off = () => document.removeEventListener("mousedown", off);
  return m;
}
function closeMenus() {
  $$(".menu").forEach((m) => {
    if (m.off) m.off();
    m.remove();
  });
}
