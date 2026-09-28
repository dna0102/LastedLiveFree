// Studio chat panel: comments, gifts, likes and joins from the LIVE's feed.
"use strict";

const Chat = {
  seq: 0,
  acct: null,
  timer: 0,
  items: [],
  stick: true,
  filter: LS.get("chatFilter", { chat: true, gift: true, like: false, join: true, follow: true, share: true }),
};
const CHAT_KINDS = [
  ["chat", "Comments"],
  ["gift", "Gifts"],
  ["like", "Likes"],
  ["join", "Joins"],
  ["follow", "Follows"],
  ["share", "Shares"],
];

function chatReset() {
  Chat.seq = 0;
  Chat.items = [];
  Chat.acct = S.acctId;
  Chat.stick = true;
  Chat.err = "";
}
function chatStart() {
  chatStop();
  chatReset();
  drawChat();
  Chat.timer = setInterval(chatPoll, 1500);
  chatPoll();
}
function chatStop() {
  clearInterval(Chat.timer);
  Chat.timer = 0;
}

async function chatPoll() {
  if (S.view !== "studio" || document.hidden) return;
  const id = S.acctId,
    live = isLive(id);
  if (id !== Chat.acct || live !== Chat.wasLive) {
    chatReset();
    Chat.wasLive = live;
    drawChat();
  }
  if (!live) return;
  let d;
  try {
    d = await api("GET", acctPath("/chat?since=" + Chat.seq, id));
  } catch {
    return;
  }
  if (id !== S.acctId) return;
  Chat.err = d.error || "";
  if (d.items && d.items.length) {
    Chat.seq = d.seq;
    Chat.items.push(...d.items);
    if (Chat.items.length > 400) Chat.items = Chat.items.slice(-400);
    appendChat(d.items);
  } else if (d.seq < Chat.seq) {
    chatReset();
    drawChat();
  } // new LIVE, the feed started over
  const e = $("#ch-err");
  if (e) {
    e.hidden = !Chat.err;
    e.textContent = Chat.err ? "Chat feed paused: " + Chat.err : "";
  }
}

function chatLine(it) {
  const u = it.user || {};
  const name = '<b class="cn">' + esc(u.nickname || u.handle || "Viewer") + "</b>";
  let body;
  switch (it.kind) {
    case "chat":
      body = '<span class="ct">' + esc(it.text) + "</span>";
      break;
    case "gift":
      body =
        '<span class="cg">sent ' +
        (it.icon ? '<img src="' + esc(cdn(it.icon)) + '">' : "") +
        esc(it.gift || "a gift") +
        (it.count > 1 ? " ×" + it.count : "") +
        "</span>";
      break;
    case "like":
      body = '<span class="cs">liked the LIVE' + (it.count > 1 ? " ×" + it.count : "") + "</span>";
      break;
    case "join":
      body = '<span class="cs">joined</span>';
      break;
    case "follow":
      body = '<span class="cs">followed you</span>';
      break;
    case "share":
      body = '<span class="cs">shared the LIVE</span>';
      break;
    default:
      return "";
  }
  return (
    '<div class="cm k-' +
    it.kind +
    '" data-seq="' +
    it.seq +
    '"><img class="cav" src="' +
    esc(cdn(u.avatar)) +
    '" onerror="this.style.visibility=\'hidden\'"><div class="cb">' +
    name +
    " " +
    body +
    "</div></div>"
  );
}
const chatShown = (it) => it.kind !== "viewers" && Chat.filter[it.kind] !== false;

function drawChat() {
  const box = $("#ch-list");
  if (!box) return;
  if (!isLive()) {
    box.innerHTML = '<div class="empty">Go LIVE to see comments, gifts and joins here.</div>';
    return;
  }
  const shown = Chat.items.filter(chatShown);
  box.innerHTML = shown.length
    ? shown.slice(-200).map(chatLine).join("")
    : '<div class="empty ch-wait">Waiting for viewers…</div>';
  box.scrollTop = box.scrollHeight;
}
function appendChat(items) {
  const box = $("#ch-list");
  if (!box) return;
  const add = items.filter(chatShown);
  if (!add.length) return;
  const wait = $(".ch-wait", box);
  if (wait) wait.remove();
  const atBottom = box.scrollHeight - box.scrollTop - box.clientHeight < 40;
  box.insertAdjacentHTML("beforeend", add.map(chatLine).join(""));
  while (box.children.length > 250) box.firstElementChild.remove();
  if (atBottom) box.scrollTop = box.scrollHeight;
  else {
    const n = $("#ch-new");
    if (n) n.hidden = false;
  }
}

function wireChatPanel() {
  const box = $("#ch-list");
  if (!box) return;
  box.onscroll = () => {
    if (box.scrollHeight - box.scrollTop - box.clientHeight < 40) {
      const n = $("#ch-new");
      if (n) n.hidden = true;
    }
  };
  $("#ch-new").onclick = () => {
    box.scrollTop = box.scrollHeight;
    $("#ch-new").hidden = true;
  };
  $("#ch-filter").onclick = (e) =>
    openMenu(
      e.currentTarget,
      [
        { header: "Show in chat" },
        ...CHAT_KINDS.map(([k, label]) => ({
          icon: Chat.filter[k] !== false ? "check" : "",
          label,
          onClick: () => {
            Chat.filter[k] = Chat.filter[k] === false;
            LS.set("chatFilter", Chat.filter);
            drawChat();
          },
        })),
      ],
      "right",
    );
  box.onclick = (e) => {
    const row = e.target.closest(".cm");
    if (!row) return;
    const it = Chat.items.find((x) => String(x.seq) === row.dataset.seq);
    if (it) chatMenu(row, it);
  };
}

function chatMenu(anchor, it) {
  const u = it.user || {};
  const items = [{ header: (u.nickname || "Viewer") + (u.handle ? " · @" + u.handle : "") }];
  if (it.kind === "chat" && it.text) {
    items.push({
      icon: "copy",
      label: "Copy comment",
      onClick: () => {
        Native.copy(it.text);
        toast("Copied");
      },
    });
    items.push({ icon: "shield", label: "Mute a word from this…", paid: "Moderation" });
  }
  if (u.id) items.push({ icon: "userplus", label: "Make moderator", paid: "Moderation" });
  if (u.handle)
    items.push({
      icon: "link",
      label: "Open profile",
      onClick: () => openExternal("https://www.tiktok.com/@" + encodeURIComponent(u.handle)),
    });
  openMenu(anchor, items, "right");
}

function openExternal(url) {
  if (window.runtime && window.runtime.BrowserOpenURL) window.runtime.BrowserOpenURL(url);
  else window.open(url, "_blank");
}
