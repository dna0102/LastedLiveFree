// Tools panel, and going LIVE / ending.
"use strict";

const TOOLS = [
  {
    group: "Engage",
    items: [
      { id: "cohost", label: "Co-host", icon: "users", paid: true },
      { id: "multiguest", label: "Multi-guest", icon: "userplus", paid: true },
      { id: "goal", label: "LIVE Goal", icon: "target" },
      { id: "games", label: "Interactive Games", icon: "gamepad", paid: true },
      { id: "wishes", label: "Viewer Wishes", icon: "wish" },
      { id: "together", label: "Play Together", icon: "joystick", paid: true },
      { id: "poll", label: "Poll & Gift vote", icon: "poll" },
      { id: "treasure", label: "Treasure Box", icon: "chest", paid: true },
      { id: "rewards", label: "Game Rewards", icon: "trophy", paid: true },
    ],
  },
  {
    group: "Growth hub",
    items: [
      { id: "fanclub", label: "Fan Club", icon: "heart", paid: true },
      { id: "playbook", label: "Playbook", icon: "book", paid: true },
      { id: "gifts", label: "Gift Gallery", icon: "gift" },
    ],
  },
  {
    group: "Manage",
    items: [
      { id: "moderation", label: "Moderation", icon: "shield", paid: true },
      { id: "standing", label: "LIVE standing", icon: "alert", paid: true },
      { id: "wallet", label: "Wallet", icon: "wallet" },
      { id: "customise", label: "Encoder", icon: "settings" },
    ],
  },
];

function renderTools() {
  const box = $("#st-tools");
  if (!box) return;
  box.innerHTML =
    '<div class="panel-title">Tools</div>' +
    TOOLS.map(
      (g) =>
        '<div class="tools-sub">' +
        g.group +
        '</div><div class="tool-grid">' +
        g.items
          .map(
            (t) =>
              '<button class="tool' +
              (t.paid ? " off" : "") +
              '" data-tool="' +
              t.id +
              '">' +
              icon(t.icon) +
              "<span>" +
              t.label +
              "</span>" +
              (t.paid ? PAID_TAG : "") +
              (t.id === "poll" && S.polls[S.acctId] ? '<i class="ind"></i>' : "") +
              "</button>",
          )
          .join("") +
        "</div>",
    ).join("");
  $$(".tool", box).forEach((b) => (b.onclick = () => openTool(b.dataset.tool)));
}

function openTool(id) {
  const t = TOOLS.flatMap((g) => g.items).find((x) => x.id === id);
  if (t && t.paid) return paidOnly(t.label);
  (
    ({
      goal: openGoal,
      wishes: openWishes,
      poll: openPoll,
      gifts: openGiftGallery,
      wallet: openWallet,
      customise: () => go("customise"),
    })[id] || (() => {})
  )();
}

function needLive(what) {
  if (isLive()) return true;
  toast("Go LIVE first to use " + what, "err");
  return false;
}
const acctPath = (p, id = S.acctId) => "/api/accounts/" + encodeURIComponent(id) + p;

function editTitle() {
  const id = S.acctId,
    live = isLive(id);
  const cur = live ? S.live[id].title : LS.get("title." + id, "");
  let about = null; // set once loaded
  const m = openModal({
    title: "LIVE info",
    sub: live
      ? "Title changes show on TikTok right away."
      : "The title is used the next time you go LIVE on this account.",
    body:
      '<div class="field"><label>Title</label><input class="input" id="tt-title" maxlength="80" value="' +
      esc(cur || "") +
      '" placeholder="What are you streaming?"></div>' +
      '<div class="field"><label>About me <span class="hint" id="tt-about-n"></span></label>' +
      '<textarea class="textarea prose" id="tt-about" maxlength="240" placeholder="Who you are, what your LIVEs are about and your perks" disabled></textarea>' +
      '<div class="hint">Shown to viewers on your LIVE profile. TikTok reviews changes before they appear.</div></div>',
    foot: [
      {
        label: "Save",
        cls: "primary",
        fn: async (btn) => {
          const title = $("#tt-title").value.trim();
          const text = $("#tt-about").value.trim();
          await withSpinner(btn, "", async () => {
            try {
              if (live && title !== (S.live[id].title || "")) {
                await api("POST", acctPath("/title", id), { title });
                S.live[id].title = title;
              }
              LS.set("title." + id, title);
              if (about && text !== (about.text || "").trim())
                await api("POST", acctPath("/about-me", id), { text, template_id: about.template_id });
              m.close();
              toast("LIVE info saved", "ok");
            } catch (e) {
              toast(e.message, "err");
            }
          });
        },
      },
    ],
  });
  const ta = $("#tt-about");
  const count = () => {
    $("#tt-about-n").textContent = ta.value.length + " / " + ta.maxLength;
  };
  ta.oninput = count;
  api("GET", acctPath("/about-me", id))
    .then((d) => {
      if (!$("#tt-about")) return;
      about = d;
      ta.maxLength = d.max || 240;
      ta.value = d.text || "";
      ta.disabled = false;
      count();
    })
    .catch(() => {
      if ($("#tt-about")) ta.placeholder = "Couldn't load About me";
    });
  setTimeout(() => $("#tt-title") && $("#tt-title").focus(), 30);
}
function shareLive() {
  const s = S.live[S.acctId];
  if (!s || !s.share_url) return toast("Go LIVE to get a share link", "err");
  Native.copy(s.share_url);
  toast("LIVE link copied", "ok");
}

async function startGoLive() {
  const a = acct();
  if (!a) return;
  if (isLive()) return;
  if (!visibleSources().length) {
    toast("Add a source to your scene first", "err");
    return openAddSource();
  }
  if (S.devices.loaded && !S.devices.ffmpeg) return openFFmpegSetup();
  if (!S.tags.length) {
    try {
      S.tags = (await api("GET", "/api/tags")).hashtags || [];
    } catch {}
  }
  let topic = String(LS.get("topic." + a.id, "5"));
  let game = LS.get("game." + a.id, null); // {id, name}
  const [W, H] = canvasSize();
  const m = openModal({
    title: "Go LIVE",
    size: "wide",
    sub: "on <b>" + esc(nameOf(a)) + "</b>",
    body:
      '<div class="field"><label>Title</label><input class="input" id="gl-title" maxlength="80" placeholder="What are you streaming?" value="' +
      esc(LS.get("title." + a.id, "")) +
      '"></div>' +
      '<div class="field"><label>Topic</label><div class="chips topic-chips" id="gl-topics"></div></div>' +
      '<div class="field" id="gl-game-f"><label>Game</label><div class="game-pick"><div class="search">' +
      icon("search") +
      '<input id="gl-game-q" placeholder="Search games" spellcheck="false"></div><div class="game-sel" id="gl-game-sel"></div></div>' +
      '<div class="game-results" id="gl-games"></div></div>' +
      '<div class="golive-sum">' +
      "<div><span>Resolution</span><b>" +
      W +
      "×" +
      H +
      " · 60 FPS</b></div>" +
      "<div><span>Bitrate</span><b>" +
      E.bitrate +
      " kbps</b></div>" +
      "<div><span>Encoder</span><b>" +
      esc(codecLabel(activeCodec())) +
      "</b></div>" +
      "<div><span>Microphone</span><b>" +
      esc(E.mic || "Off") +
      "</b></div>" +
      "</div>" +
      '<div class="steps" id="gl-steps" hidden></div>',
    foot: [
      {
        label: "Encoder settings",
        cls: "ghost",
        fn: () => {
          m.close();
          go("customise");
        },
      },
      { label: "Go LIVE", cls: "primary", fn: (btn) => doGoLive(btn) },
    ],
  });

  const drawTopics = () => {
    $("#gl-topics").innerHTML = S.tags
      .map(
        (t) =>
          '<button class="chip' +
          (String(t.id) === topic ? " on" : "") +
          '" data-id="' +
          esc(t.id) +
          '">' +
          esc(t.title) +
          "</button>",
      )
      .join("");
    $$("#gl-topics .chip").forEach(
      (c) =>
        (c.onclick = () => {
          topic = c.dataset.id;
          drawTopics();
        }),
    );
    $("#gl-game-f").hidden = topic !== "5";
  };
  const drawGame = () => {
    $("#gl-game-sel").innerHTML = game
      ? '<span class="chip on">' +
        icon("gamepad", "ico xs") +
        esc(game.name) +
        '<i data-x title="Clear">' +
        icon("close", "ico xs") +
        "</i></span>"
      : '<span class="hint">No game selected</span>';
    const x = $("#gl-game-sel [data-x]");
    if (x)
      x.onclick = () => {
        game = null;
        drawGame();
      };
  };
  const searchGames = debounce(async (q) => {
    const box = $("#gl-games");
    if (!box) return;
    if (!q) {
      box.innerHTML = "";
      return;
    }
    let list = [];
    try {
      list = (await api("GET", "/api/games?q=" + encodeURIComponent(q))).games || [];
    } catch {}
    if (!$("#gl-games")) return;
    box.innerHTML = list.length
      ? list
          .slice(0, 12)
          .map((g) => '<button class="chip" data-id="' + esc(g.id) + '">' + esc(g.name) + "</button>")
          .join("")
      : '<span class="hint">No games found</span>';
    $$(".chip", box).forEach(
      (c, i) =>
        (c.onclick = () => {
          game = { id: String(list[i].id), name: list[i].name };
          $("#gl-game-q").value = "";
          box.innerHTML = "";
          drawGame();
        }),
    );
  }, 200);
  $("#gl-game-q").oninput = (e) => searchGames(e.target.value.trim());
  drawTopics();
  drawGame();
  setTimeout(() => $("#gl-title") && $("#gl-title").focus(), 30);

  async function doGoLive(btn) {
    const id = a.id;
    const title = $("#gl-title").value.trim();
    LS.set("title." + id, title);
    LS.set("topic." + id, topic);
    LS.set("game." + id, game);
    const steps = $("#gl-steps");
    steps.hidden = false;
    const step = (text, state) => {
      const row = node(
        '<div class="step ' +
          state +
          '">' +
          (state === "run" ? '<span class="spin"></span>' : icon(state === "ok" ? "checkc" : "alert")) +
          "<span>" +
          esc(text) +
          "</span></div>",
      );
      steps.appendChild(row);
      return row;
    };
    const finish = (row, state, text) => {
      row.className = "step " + state;
      row.innerHTML = icon(state === "ok" ? "checkc" : "alert") + "<span>" + esc(text) + "</span>";
    };
    steps.innerHTML = "";
    $$(".modal-foot .btn", m).forEach((b) => (b.disabled = true));
    btn.innerHTML = '<span class="spin"></span> Going LIVE…';

    let s1 = step("Creating your LIVE room", "run");
    let sess;
    try {
      sess = await api("POST", acctPath("/go-live", id), {
        title,
        hashtag_id: topic,
        game_tag_id: topic === "5" && game ? game.id : "",
      });
    } catch (e) {
      finish(s1, "err", "Couldn't create the room: " + e.message);
      $$(".modal-foot .btn", m).forEach((b) => (b.disabled = false));
      btn.textContent = "Try again";
      return;
    }
    finish(s1, "ok", "Room created");
    S.live[id] = sess;
    renderShell();
    if (S.view === "studio" && S.acctId === id) drawLiveBits();

    const s2 = step("Starting the stream (" + codecLabel(activeCodec()) + ")", "run");
    try {
      await api("POST", acctPath("/encoder/start", id), { config: sceneConfig() });
    } catch (e) {
      finish(s2, "err", "Stream failed: " + e.message);
      // a room with no stream is pointless and TikTok doesn't like it; close it
      const s3 = step("Ending the empty room", "run");
      S.ending[id] = true;
      try {
        await api("POST", acctPath("/end-live", id));
      } catch {}
      delete S.ending[id];
      delete S.live[id];
      finish(s3, "ok", "Room closed. Fix the scene or encoder settings and try again.");
      renderShell();
      if (S.view === "studio" && S.acctId === id) {
        drawLiveBits();
        refreshPreview();
      }
      $$(".modal-foot .btn", m).forEach((b) => (b.disabled = false));
      btn.textContent = "Try again";
      logActivity(nameOf(a) + ": stream failed to start", "err");
      return;
    }
    finish(s2, "ok", "Streaming");
    logActivity(nameOf(a) + " went LIVE", "live");
    toast(nameOf(a) + " is LIVE", "ok");
    m.close();
    if (S.view === "studio" && S.acctId === id) {
      St.dirtyLive = false;
      drawLiveBits();
      renderTools();
      perfTick();
      statusTick();
    } else if (VIEWS[S.view] && VIEWS[S.view].tick) VIEWS[S.view].tick();
  }
}

async function endLiveFlow(id, { confirm = true } = {}) {
  const a = acct(id);
  if (!a || !isLive(id)) return;
  if (
    confirm &&
    !(await confirmBox(
      "End LIVE?",
      "End the LIVE on <b>" + esc(nameOf(a)) + "</b>. Viewers will see that the LIVE has ended.",
      "End LIVE",
    ))
  )
    return;
  const sess = S.live[id];
  const enc = encodeURIComponent(id);
  // get the final numbers before the room closes
  const [stats, rewards] = await Promise.all([
    api("GET", "/api/accounts/" + enc + "/stats").catch(() => S.perf[id] || {}),
    api("GET", "/api/accounts/" + enc + "/rewards").catch(() => null),
  ]);
  S.ending[id] = true;
  try {
    await api("POST", "/api/accounts/" + enc + "/end-live");
  } catch (e) {
    delete S.ending[id];
    return toast(e.message, "err");
  }
  delete S.ending[id];
  delete S.live[id];
  delete S.polls[id];
  logActivity(nameOf(a) + "'s LIVE ended");
  renderShell();
  if (S.view === "studio") {
    if (S.acctId === id) {
      drawLiveBits();
      renderTools();
      perfTick();
      refreshPreview();
    }
  } else if (VIEWS[S.view] && VIEWS[S.view].tick) VIEWS[S.view].tick();

  const p = stats || {};
  const dur = sess && sess.started_at ? hms(Date.now() / 1000 - sess.started_at) : "—";
  const tile = (v, l) => '<div class="stat-tile"><b>' + v + "</b><span>" + l + "</span></div>";
  openModal({
    title: "LIVE ended",
    sub: esc(nameOf(a)) + " · " + dur,
    body:
      '<div class="stat-tiles">' +
      tile(fmt(p.views != null ? p.views : p.watch), "Total views") +
      tile(fmt(p.likes), "Likes") +
      tile(fmt(p.new_fans), "New followers") +
      tile(fmt(p.comments), "Comments") +
      tile(fmt(p.gifters), "Gifters") +
      tile(fmt((rewards && rewards.diamonds) || p.diamonds), "Diamonds") +
      "</div>" +
      (rewards && rewards.amount && rewards.amount !== "0.00"
        ? '<p class="hint" style="margin-top:14px">Estimated LIVE rewards: <b>' +
          esc(rewards.amount) +
          " " +
          esc(rewards.currency || "") +
          "</b></p>"
        : "") +
      '<div id="le-top"></div>',
    foot: [{ label: "Done", cls: "primary", fn: (b, bg) => bg.close() }],
  });
  // top viewers; TikTok needs a moment after the room closes
  setTimeout(async () => {
    try {
      const d = await api("GET", "/api/accounts/" + enc + "/top-gifters");
      const box = $("#le-top");
      if (!box || !d.users || !d.users.length || (sess && d.room_id !== sess.room_id)) return;
      box.innerHTML =
        '<div class="section-title">Top viewers</div>' +
        d.users
          .slice(0, 5)
          .map(
            (u, i) =>
              '<div class="rank"><span class="no ' +
              (i < 3 ? "top" : "") +
              '">' +
              (i + 1) +
              '</span><img src="' +
              esc(cdn(u.avatar)) +
              '" onerror="this.style.visibility=\'hidden\'"><span class="nm">' +
              esc(u.nickname || u.handle) +
              '</span><span class="sc">' +
              fmt(u.score) +
              "</span></div>",
          )
          .join("");
    } catch {}
  }, 2500);
}

async function loadGifts() {
  if (S.gifts) return S.gifts;
  try {
    S.gifts = (await api("GET", "/api/gifts")).filter((g) => g.diamonds > 0);
  } catch {
    S.gifts = [];
  }
  // the catalog has regional duplicates; keep one per name and price
  const seen = new Set();
  S.gifts = S.gifts.filter((g) => {
    const k = g.name + "|" + g.diamonds;
    if (seen.has(k)) return false;
    seen.add(k);
    return true;
  });
  S.gifts.sort((x, y) => x.diamonds - y.diamonds || x.name.localeCompare(y.name));
  return S.gifts;
}
// used when a gift icon can't be loaded
const GIFT_FALLBACK =
  "data:image/svg+xml," +
  encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="#8a8a90" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">' +
      ICON_PATHS.gift +
      "</svg>",
  );
const giftImg = (src, cls = "") =>
  "<img" +
  (cls ? ' class="' + cls + '"' : "") +
  ' src="' +
  esc(src ? cdn(src) : GIFT_FALLBACK) +
  '" loading="lazy" onerror="this.onerror=null;this.src=GIFT_FALLBACK">';
const giftById = (id) => (S.gifts || []).find((g) => g.id === +id);
function giftHTML(g, on) {
  return (
    '<button class="gift' +
    (on ? " on" : "") +
    '" data-id="' +
    g.id +
    '">' +
    giftImg(g.icon) +
    '<span class="gn">' +
    esc(g.name) +
    '</span><span class="gc"><i class="coin"></i>' +
    fmt(g.diamonds) +
    "</span></button>"
  );
}
function giftGrid(container, { selected = null, onPick, q = "" } = {}) {
  const list = (S.gifts || []).filter((g) => !q || g.name.toLowerCase().includes(q));
  container.innerHTML = list.length
    ? list
        .slice(0, 400)
        .map((g) => giftHTML(g, g.id === selected))
        .join("")
    : '<div class="empty" style="grid-column:1/-1">No gifts match.</div>';
  $$(".gift", container).forEach((b) => (b.onclick = () => onPick && onPick(giftById(b.dataset.id))));
}
async function openGiftPicker(title, selected) {
  await loadGifts();
  return new Promise((resolve) => {
    let picked = null;
    const m = openModal({
      title,
      size: "wide",
      onClose: () => resolve(picked),
      body:
        '<div class="search wide">' +
        icon("search") +
        '<input id="gp-q" placeholder="Search gifts"></div><div class="gift-grid" id="gp-grid" style="margin-top:12px"></div>',
    });
    const draw = (q = "") =>
      giftGrid($("#gp-grid"), {
        selected,
        q,
        onPick: (g) => {
          picked = g;
          m.close();
        },
      });
    $("#gp-q").oninput = (e) => draw(e.target.value.trim().toLowerCase());
    draw();
    setTimeout(() => $("#gp-q") && $("#gp-q").focus(), 30);
  });
}

function openGiftGallery(tab = LS.get("ggTab", "week")) {
  const id = S.acctId;
  openModal({
    title: "Gift Gallery",
    size: "xwide",
    body: '<div class="seg mod-tabs" id="gg-tabs"><button data-t="week">This week</button><button data-t="all">All gifts</button></div><div id="gg-pane"></div>',
  });
  const setTab = (t) => {
    LS.set("ggTab", t);
    $$("#gg-tabs button").forEach((b) => b.classList.toggle("on", b.dataset.t === t));
    (t === "week" ? galleryWeek : galleryAll)($("#gg-pane"), id);
  };
  $$("#gg-tabs button").forEach((b) => (b.onclick = () => setTab(b.dataset.t)));
  setTab(tab);
}

// This week's gallery. A gift lights up once one viewer has sent enough of it.
async function galleryWeek(pane, id) {
  pane.innerHTML = '<div class="empty"><span class="spin" style="margin:auto"></span></div>';
  let d;
  try {
    d = await api("GET", acctPath("/gift-gallery", id));
  } catch (e) {
    pane.innerHTML = '<div class="empty">' + esc(e.message) + "</div>";
    return;
  }
  if (!pane.isConnected || !$("#gg-tabs .on") || $("#gg-tabs .on").dataset.t !== "week") return;
  const gifts = d.gifts || [];
  const lit = gifts.filter((g) => g.lit).length;
  const left = d.ends_at ? Math.max(0, d.ends_at - Date.now() / 1000) : 0;
  const days = Math.floor(left / 86400),
    hrs = Math.floor((left % 86400) / 3600);
  pane.innerHTML =
    '<div class="gal-head"><div><b>' +
    lit +
    " of " +
    gifts.length +
    " lit up</b><span>Resets in " +
    (days ? days + "d " : "") +
    hrs +
    "h" +
    (d.league ? " · League " + esc(d.league) : "") +
    "</span></div>" +
    '<div class="gb-track" style="width:220px"><i style="width:' +
    (gifts.length ? Math.round((lit / gifts.length) * 100) : 0) +
    '%"></i></div></div>' +
    '<p class="hint" style="margin:8px 0 12px">A gift lights up when one viewer sends enough of it this week. That viewer becomes its sponsor.</p>' +
    '<div class="gift-grid tall">' +
    gifts
      .map(
        (g) =>
          '<div class="gift gal' +
          (g.lit ? " lit" : "") +
          '">' +
          giftImg(g.lit ? g.image : g.unlit_image || g.image) +
          '<span class="gn">' +
          esc(g.name) +
          '</span><span class="gc"><i class="coin"></i>' +
          fmt(g.coins) +
          " · " +
          (g.goal || 0) +
          " needed</span>" +
          (g.lit
            ? '<span class="gs">' + (g.sponsor && g.sponsor !== "<nil>" ? "by " + esc(g.sponsor) : "Lit") + "</span>"
            : '<span class="gs off">Not lit</span>') +
          "</div>",
      )
      .join("") +
    "</div>";
}

function galleryAll(pane) {
  pane.innerHTML =
    '<div class="row"><div class="search wide grow">' +
    icon("search") +
    '<input id="gg-q" placeholder="Search gifts"></div>' +
    '<div class="seg" id="gg-sort" style="width:240px"><button data-s="asc" class="on">Cheapest</button><button data-s="desc">Priciest</button></div></div>' +
    '<div class="hint" id="gg-count" style="margin:10px 0 8px"></div><div class="gift-grid tall" id="gg-grid"><div class="empty" style="grid-column:1/-1"><span class="spin" style="margin:auto"></span></div></div>';
  let sort = "asc",
    q = "";
  const draw = () => {
    const grid = $("#gg-grid");
    if (!grid) return;
    const list = (S.gifts || []).slice();
    if (sort === "desc") list.reverse();
    const shown = list.filter((g) => !q || g.name.toLowerCase().includes(q));
    $("#gg-count").textContent = shown.length + " gifts";
    grid.innerHTML =
      shown.map((g) => giftHTML(g, false)).join("") ||
      '<div class="empty" style="grid-column:1/-1">No gifts match.</div>';
    $$(".gift", grid).forEach(
      (b) =>
        (b.onclick = () => {
          Native.copy(giftById(b.dataset.id).name);
          toast("Copied gift name");
        }),
    );
  };
  $("#gg-q").oninput = (e) => {
    q = e.target.value.trim().toLowerCase();
    draw();
  };
  $$("#gg-sort button").forEach(
    (b) =>
      (b.onclick = () => {
        sort = b.dataset.s;
        $$("#gg-sort button").forEach((x) => x.classList.toggle("on", x === b));
        draw();
      }),
  );
  loadGifts().then(draw);
}

async function openWishes() {
  const id = S.acctId,
    a = acct();
  await loadGifts();
  const saved = LS.get("wishes." + id, null);
  const st = Object.assign(
    {
      items: [],
      display_mode: 2,
      round_duration_sec: 3600,
      has_score: true,
      has_duration: true,
      enable_auto_restart: true,
    },
    saved || {},
  );
  const m = openModal({
    title: "Viewer Wishes",
    size: "wide",
    sub: "Show up to 6 gifts you'd like to receive. TikTok draws the list on your LIVE.",
    body:
      '<div id="vw-rows"></div><button class="btn sm" id="vw-add">' +
      icon("plus") +
      "Add gift</button>" +
      '<div class="grid2" style="margin-top:18px">' +
      '<div class="field"><label>Layout</label><div class="seg" id="vw-mode"><button data-v="2">Vertical</button><button data-v="1">Horizontal</button></div></div>' +
      '<div class="field"><label>Round length</label><select class="select" id="vw-dur">' +
      [
        [300, "5 minutes"],
        [900, "15 minutes"],
        [1800, "30 minutes"],
        [3600, "60 minutes"],
      ]
        .map(([v, l]) => '<option value="' + v + '">' + l + "</option>")
        .join("") +
      "</select></div></div>" +
      toggleRow("vw-score", "Show points", "Viewers see how close each wish is", st.has_score) +
      toggleRow("vw-timer", "Show timer", "Count down the round on screen", st.has_duration) +
      toggleRow("vw-auto", "Auto-restart", "Start a new round when one ends", st.enable_auto_restart),
    foot: [
      {
        label: "Save",
        cls: "ghost",
        fn: async (btn) => {
          collect();
          LS.set("wishes." + id, st);
          await withSpinner(btn, "", async () => {
            try {
              await api("POST", acctPath("/wishes/save", id), st);
              toast("Wishes saved", "ok");
            } catch (e) {
              toast("Couldn't save to TikTok: " + e.message, "err");
            }
          });
        },
      },
      {
        label: isLive(id) ? "Start on LIVE" : "Go LIVE to start",
        cls: "primary",
        fn: async (btn) => {
          collect();
          LS.set("wishes." + id, st);
          if (!isLive(id)) return toast("Go LIVE first, then start your wishes", "err");
          if (!st.items.length) return toast("Add at least one gift", "err");
          await withSpinner(btn, "", async () => {
            try {
              await api("POST", acctPath("/wishes/start", id), st);
              toast("Viewer Wishes are on your LIVE", "ok");
              logActivity(nameOf(a) + " started Viewer Wishes");
              m.close();
            } catch (e) {
              toast(e.message, "err");
            }
          });
        },
      },
    ],
  });
  // nothing saved locally yet: start from TikTok's saved list
  if (!saved) {
    api("GET", acctPath("/wishes", id))
      .then((d) => {
        if (!$("#vw-rows")) return;
        st.items = (d.items || []).map((x) => ({
          gift_id: +x.gift_id,
          label: x.label && x.label !== "<nil>" ? x.label : "",
        }));
        if (d.display_mode) st.display_mode = +d.display_mode;
        if (d.round_duration_sec) st.round_duration_sec = +d.round_duration_sec;
        drawOpts();
        drawRows();
      })
      .catch(() => {});
  }
  function drawOpts() {
    $$("#vw-mode button").forEach((b) => b.classList.toggle("on", +b.dataset.v === st.display_mode));
    $("#vw-dur").value = String(st.round_duration_sec);
  }
  function collect() {
    $$("#vw-rows .wish-row").forEach((r, i) => {
      if (st.items[i]) st.items[i].label = $("input", r).value.trim();
    });
    st.round_duration_sec = +$("#vw-dur").value;
    st.has_score = $("#vw-score").checked;
    st.has_duration = $("#vw-timer").checked;
    st.enable_auto_restart = $("#vw-auto").checked;
  }
  function drawRows() {
    const box = $("#vw-rows");
    if (!st.items.length) {
      box.innerHTML =
        '<div class="empty" style="padding:18px">No wishes yet. Add the gifts you want viewers to send.</div>';
    } else
      box.innerHTML = st.items
        .map((it, i) => {
          const g = giftById(it.gift_id) || { name: "Gift " + it.gift_id, icon: "", diamonds: 0 };
          return (
            '<div class="wish-row"><button class="wish-gift" data-i="' +
            i +
            '" title="Change gift">' +
            giftImg(g.icon) +
            "</button>" +
            '<div class="wn"><b>' +
            esc(g.name) +
            '</b><span><i class="coin"></i>' +
            fmt(g.diamonds) +
            "</span></div>" +
            '<input class="input grow" maxlength="30" placeholder="What it unlocks (optional)" value="' +
            esc(it.label || "") +
            '">' +
            '<button class="icon-btn sm" data-up="' +
            i +
            '" title="Move up"' +
            (i ? "" : " disabled") +
            ">" +
            icon("up") +
            "</button>" +
            '<button class="icon-btn sm" data-del="' +
            i +
            '" title="Remove">' +
            icon("trash") +
            "</button></div>"
          );
        })
        .join("");
    $$(".wish-gift", box).forEach(
      (b) =>
        (b.onclick = async () => {
          collect();
          const i = +b.dataset.i;
          const g = await openGiftPicker("Choose a gift", st.items[i].gift_id);
          if (g) {
            st.items[i].gift_id = g.id;
            drawRows();
          }
        }),
    );
    $$("[data-del]", box).forEach(
      (b) =>
        (b.onclick = () => {
          collect();
          st.items.splice(+b.dataset.del, 1);
          drawRows();
        }),
    );
    $$("[data-up]", box).forEach(
      (b) =>
        (b.onclick = () => {
          collect();
          const i = +b.dataset.up;
          [st.items[i - 1], st.items[i]] = [st.items[i], st.items[i - 1]];
          drawRows();
        }),
    );
    $("#vw-add").disabled = st.items.length >= 6;
  }
  $$("#vw-mode button").forEach(
    (b) =>
      (b.onclick = () => {
        st.display_mode = +b.dataset.v;
        drawOpts();
      }),
  );
  $("#vw-add").onclick = async () => {
    collect();
    const g = await openGiftPicker("Add a wish");
    if (g && st.items.length < 6) {
      st.items.push({ gift_id: g.id, label: "" });
      drawRows();
    }
  };
  drawOpts();
  drawRows();
}
function toggleRow(id, title, sub, on) {
  return (
    '<div class="setting-row"><div class="sl"><b>' +
    title +
    "</b><span>" +
    sub +
    '</span></div><label class="toggle"><input type="checkbox" id="' +
    id +
    '"' +
    (on ? " checked" : "") +
    "><i></i></label></div>"
  );
}

async function openGoal() {
  const id = S.acctId,
    a = acct();
  await loadGifts();
  const draft = Object.assign({ description: "", reward: "", items: [] }, LS.get("goal." + id, {}));
  const m = openModal({
    title: "LIVE Goal",
    size: "wide",
    sub: "Set gift targets for this LIVE. Viewers see your progress on screen.",
    body:
      '<div id="lg-current"><div class="empty"><span class="spin" style="margin:auto"></span></div></div>' +
      '<div class="section-title">Set a goal</div>' +
      '<div id="lg-rows"></div><button class="btn sm" id="lg-add">' +
      icon("plus") +
      "Add gift</button>" +
      '<div class="field" style="margin-top:16px"><label>Goal message</label><input class="input" id="lg-desc" maxlength="60" placeholder="Let\'s reach this LIVE goal together!" value="' +
      esc(draft.description) +
      '"></div>' +
      '<div class="field"><label>Reward <span style="color:var(--muted);font-weight:500">(optional)</span></label><input class="input" id="lg-reward" maxlength="50" placeholder="e.g. Play a game with me" value="' +
      esc(draft.reward) +
      '"></div>',
    foot: [
      {
        label: isLive(id) ? "Set goal" : "Go LIVE to set a goal",
        cls: "primary",
        fn: async (btn) => {
          collect();
          LS.set("goal." + id, draft);
          if (!isLive(id)) return toast("Goals are set on a LIVE. Go LIVE first.", "err");
          if (!draft.items.length) return toast("Add at least one gift", "err");
          await withSpinner(btn, "", async () => {
            try {
              await api("POST", acctPath("/goals", id), draft);
              toast("LIVE Goal set", "ok");
              logActivity(nameOf(a) + " set a LIVE Goal");
              loadCurrent();
            } catch (e) {
              toast(e.message, "err");
            }
          });
        },
      },
    ],
  });
  function collect() {
    $$("#lg-rows .wish-row").forEach((r, i) => {
      if (draft.items[i]) draft.items[i].target = Math.max(1, +$("input", r).value || 1);
    });
    draft.description = $("#lg-desc").value.trim();
    draft.reward = $("#lg-reward").value.trim();
  }
  function drawRows() {
    const box = $("#lg-rows");
    box.innerHTML = draft.items.length
      ? draft.items
          .map((it, i) => {
            const g = giftById(it.gift_id) || { name: "Gift " + it.gift_id, icon: "", diamonds: 0 };
            return (
              '<div class="wish-row">' +
              giftImg(g.icon) +
              '<div class="wn"><b>' +
              esc(g.name) +
              '</b><span><i class="coin"></i>' +
              fmt(g.diamonds) +
              "</span></div>" +
              '<span class="hint">Target</span><input class="input" type="number" min="1" max="99999" style="width:110px" value="' +
              (it.target || 1) +
              '"><span class="grow"></span>' +
              '<button class="icon-btn sm" data-del="' +
              i +
              '" title="Remove">' +
              icon("trash") +
              "</button></div>"
            );
          })
          .join("")
      : '<div class="empty" style="padding:14px">Pick the gifts this goal counts, and how many of each.</div>';
    $$("[data-del]", box).forEach(
      (b) =>
        (b.onclick = () => {
          collect();
          draft.items.splice(+b.dataset.del, 1);
          drawRows();
        }),
    );
    $("#lg-add").disabled = draft.items.length >= 3;
  }
  async function loadCurrent() {
    const box = $("#lg-current");
    let d = null;
    try {
      d = await api("GET", acctPath("/goals", id));
    } catch (e) {
      if (box) box.innerHTML = '<div class="hint">' + esc(e.message) + "</div>";
      return;
    }
    if (!$("#lg-current")) return;
    const g = d && d.specified_goal;
    const subs = (g && g.sub_goals) || [];
    if (!g || !subs.length) {
      box.innerHTML =
        '<div class="goal-now empty-goal">' +
        icon("target") +
        "<div><b>No active goal</b><span>" +
        (isLive(id) ? "Set one below." : "Goals run during a LIVE.") +
        "</span></div></div>";
      return;
    }
    const btn = $$(".modal-foot .btn", m).at(-1);
    if (btn && isLive(id)) btn.textContent = "Update goal";
    box.innerHTML =
      '<div class="goal-now"><div class="gn-head"><b>' +
      esc(g.description || "Current goal") +
      '</b><span class="tag live">ACTIVE</span></div>' +
      subs
        .map((sg) => {
          const pct = Math.min(100, Math.round(((+sg.progress || 0) / Math.max(1, +sg.target || 1)) * 100));
          const ic = sg.gift && sg.gift.icon && sg.gift.icon.url_list ? sg.gift.icon.url_list[0] : "";
          return (
            '<div class="goal-bar">' +
            giftImg(ic) +
            '<div class="grow"><div class="gb-top"><span>' +
            esc((sg.gift && sg.gift.name) || "Gift") +
            "</span><b>" +
            fmt(sg.progress) +
            "/" +
            fmt(sg.target) +
            "</b></div>" +
            '<div class="gb-track"><i style="width:' +
            pct +
            '%"></i></div></div></div>'
          );
        })
        .join("") +
      "</div>";
  }
  $("#lg-add").onclick = async () => {
    collect();
    const g = await openGiftPicker("Add a gift to the goal");
    if (g && draft.items.length < 3) {
      draft.items.push({ gift_id: g.id, target: 1 });
      drawRows();
    }
  };
  drawRows();
  loadCurrent();
  void m;
}

function openPoll() {
  const id = S.acctId;
  if (S.polls[id]) return pollResults(id);
  if (!needLive("polls")) return;
  const opts = LS.get("pollOpts", ["", ""]);
  const m = openModal({
    title: "Start a poll",
    sub: "Viewers vote on your LIVE. Results update as votes come in.",
    body:
      '<div id="pl-tpl"></div><div id="pl-opts"></div><button class="btn sm" id="pl-add">' +
      icon("plus") +
      "Add option</button>" +
      '<div class="field" style="margin-top:16px"><label>Duration</label><div class="seg" id="pl-dur">' +
      [
        [30000, "30s"],
        [60000, "1 min"],
        [120000, "2 min"],
        [300000, "5 min"],
      ]
        .map(([v, l]) => '<button data-v="' + v + '"' + (v === 60000 ? ' class="on"' : "") + ">" + l + "</button>")
        .join("") +
      "</div></div>",
    foot: [
      {
        label: "Start poll",
        cls: "primary",
        fn: async (btn) => {
          collect();
          const list = opts.map((o) => o.trim()).filter(Boolean);
          if (list.length < 2) return toast("Add at least 2 options", "err");
          LS.set("pollOpts", opts);
          const dur = +($("#pl-dur .on") || {}).dataset?.v || 60000;
          await withSpinner(btn, "", async () => {
            try {
              const d = await api("POST", acctPath("/poll/start", id), { options: list, duration_ms: dur });
              S.polls[id] = {
                poll_id: String(d.poll_id_str || d.poll_id),
                end: +d.end_time || Date.now() + dur,
                options: d.poll_option_list || list.map((t) => ({ display_content: t, votes: 0 })),
              };
              m.close();
              renderTools();
              toast("Poll started", "ok");
              pollResults(id);
            } catch (e) {
              toast(e.message, "err");
            }
          });
        },
      },
    ],
  });
  function collect() {
    $$("#pl-opts input").forEach((inp, i) => (opts[i] = inp.value));
  }
  function draw() {
    $("#pl-opts").innerHTML = opts
      .map(
        (o, i) =>
          '<div class="row" style="margin-bottom:8px"><input class="input grow" maxlength="30" placeholder="Option ' +
          (i + 1) +
          '" value="' +
          esc(o) +
          '">' +
          (opts.length > 2 ? '<button class="icon-btn sm" data-del="' + i + '">' + icon("trash") + "</button>" : "") +
          "</div>",
      )
      .join("");
    $$("#pl-opts [data-del]").forEach(
      (b) =>
        (b.onclick = () => {
          collect();
          opts.splice(+b.dataset.del, 1);
          draw();
        }),
    );
    $("#pl-add").disabled = opts.length >= 4;
  }
  $("#pl-add").onclick = () => {
    collect();
    if (opts.length < 4) {
      opts.push("");
      draw();
    }
  };
  $$("#pl-dur button").forEach(
    (b) => (b.onclick = () => $$("#pl-dur button").forEach((x) => x.classList.toggle("on", x === b))),
  );
  draw();
  setTimeout(() => {
    const i = $("#pl-opts input");
    if (i) i.focus();
  }, 30);
  // polls saved in TikTok LIVE Studio
  api("GET", acctPath("/poll/templates", id))
    .then((d) => {
      const box = $("#pl-tpl"),
        tpls = (d.templates || []).filter((t) => t.options && t.options.length >= 2);
      if (!box || !tpls.length) return;
      box.innerHTML =
        '<div class="hint" style="margin-bottom:6px">Saved polls</div><div class="chips" style="margin-bottom:14px">' +
        tpls
          .map(
            (t, i) =>
              '<button class="chip" data-i="' + i + '">' + esc(t.options.join(" / ").slice(0, 40)) + "</button>",
          )
          .join("") +
        "</div>";
      $$("[data-i]", box).forEach(
        (c) =>
          (c.onclick = () => {
            const t = tpls[+c.dataset.i];
            opts.length = 0;
            opts.push(...t.options.slice(0, 4));
            draw();
            const dur = String(t.duration_ms || 60000);
            $$("#pl-dur button").forEach((x) => x.classList.toggle("on", x.dataset.v === dur));
          }),
      );
    })
    .catch(() => {});
}
function pollResults(id) {
  const p = S.polls[id];
  if (!p) return;
  let timer;
  const m = openModal({
    title: "Poll results",
    sub: '<span id="pr-left"></span>',
    body: '<div id="pr-bars"></div><div class="hint" id="pr-total" style="margin-top:6px"></div>',
    onClose: () => clearInterval(timer),
    foot: [
      {
        label: "End poll",
        cls: "danger",
        fn: async (btn) => {
          await withSpinner(btn, "", async () => {
            try {
              await api("POST", acctPath("/poll/end", id), { poll_id: p.poll_id });
            } catch (e) {
              toast(e.message, "err");
            }
            p.end = Date.now();
            await refresh();
            finishPoll();
          });
        },
      },
    ],
  });
  const draw = () => {
    if (!$("#pr-bars")) return;
    const total = p.options.reduce((t, o) => t + (+o.votes || 0), 0);
    $("#pr-bars").innerHTML = p.options
      .map((o) => {
        const pct = total ? Math.round(((+o.votes || 0) / total) * 100) : 0;
        return (
          '<div class="poll-bar"><i style="width:' +
          pct +
          '%"></i><span>' +
          esc(o.display_content) +
          "</span><span>" +
          pct +
          "% · " +
          fmt(o.votes) +
          "</span></div>"
        );
      })
      .join("");
    $("#pr-total").textContent = fmt(total) + " vote" + (total === 1 ? "" : "s");
    const left = Math.max(0, (p.end - Date.now()) / 1000);
    $("#pr-left").textContent = left > 0 ? hms(left) + " left" : "Poll ended";
  };
  const refresh = async () => {
    try {
      const d = await api("GET", acctPath("/poll/query?poll_id=" + encodeURIComponent(p.poll_id), id));
      const pd = d.poll_info && d.poll_info.poll_data;
      if (pd && pd.poll_option_list) p.options = pd.poll_option_list;
      if (pd && pd.poll_status && +pd.poll_status !== 1) p.end = Math.min(p.end, Date.now());
    } catch {}
    draw();
  };
  const finishPoll = () => {
    delete S.polls[id];
    renderTools();
    const f = $(".modal-foot", m);
    if (f) {
      f.innerHTML = '<button class="btn primary">Done</button>';
      $("button", f).onclick = m.close;
    }
    draw();
  };
  draw();
  refresh();
  let n = 0;
  timer = setInterval(async () => {
    if (!m.isConnected || !S.polls[id]) return clearInterval(timer);
    if (Date.now() >= p.end + 1500) {
      clearInterval(timer);
      await refresh();
      return finishPoll();
    }
    if (++n % 3 === 0) refresh();
    else draw();
  }, 1000);
}

async function openWallet() {
  const id = S.acctId,
    a = acct();
  openModal({
    title: "Wallet",
    sub: esc(nameOf(a)),
    body: '<div id="wl-body"><div class="empty"><span class="spin" style="margin:auto"></span></div></div>',
  });
  const [w, r] = await Promise.all([
    api("GET", acctPath("/wallet", id)).catch((e) => ({ error: e.message })),
    isLive(id) ? api("GET", acctPath("/rewards", id)).catch(() => null) : Promise.resolve(null),
  ]);
  const box = $("#wl-body");
  if (!box) return;
  if (w.error) {
    box.innerHTML = '<div class="empty">' + esc(w.error) + "</div>";
    return;
  }
  const tile = (v, l, cls = "") => '<div class="stat-tile ' + cls + '"><b>' + v + "</b><span>" + l + "</span></div>";
  box.innerHTML =
    '<div class="stat-tiles">' +
    tile(fmt(w.diamonds), "Diamonds", "dia") +
    tile(fmt(w.coins), "Coins") +
    tile(fmt(w.frozen), "Pending diamonds") +
    "</div>" +
    (r
      ? '<div class="section-title">This LIVE</div><div class="stat-tiles">' +
        tile(fmt(r.diamonds), "Diamonds") +
        tile(esc(r.amount || "0.00") + " <small>" + esc(r.currency || "") + "</small>", "Estimated rewards") +
        "</div>"
      : "") +
    '<p class="hint" style="margin-top:16px">Withdraw and exchange diamonds in the TikTok app.</p>';
}
