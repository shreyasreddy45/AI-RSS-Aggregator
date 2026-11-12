/* Marginalia — single-page frontend for the RSS aggregator API.
 * Vanilla JS with hash routing. All dynamic text is inserted with textContent,
 * never innerHTML: feed content and model output are untrusted. The only
 * innerHTML use is for the static icon markup defined in this file. */
(function () {
  "use strict";

  const API = "/v1";
  const TOKEN_KEY = "marginalia.token";
  const USER_KEY = "marginalia.user";

  // ------------------------------------------------------------ icons
  const ICONS = {
    today: '<rect x="3" y="3" width="18" height="18" rx="2"/><path d="M3 9h18M9 21V9"/>',
    star: '<polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2"/>',
    search: '<circle cx="11" cy="11" r="7"/><path d="M21 21l-4.3-4.3"/>',
    chat: '<path d="M21 11.5a8.4 8.4 0 0 1-9 8.4 8.5 8.5 0 0 1-3.8-.9L3 21l1.9-5.2A8.4 8.4 0 1 1 21 11.5z"/>',
    compass: '<circle cx="12" cy="12" r="10"/><polygon points="16.24 7.76 14.12 14.12 7.76 16.24 9.88 9.88 16.24 7.76"/>',
    logout: '<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><polyline points="16 17 21 12 16 7"/><line x1="21" y1="12" x2="9" y2="12"/>',
    sparkle: '<path d="M12 3l1.9 5.1L19 10l-5.1 1.9L12 17l-1.9-5.1L5 10l5.1-1.9L12 3z"/><path d="M19 15l.8 2.2L22 18l-2.2.8L19 21l-.8-2.2L16 18l2.2-.8L19 15z"/>',
    external: '<path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/><path d="M15 3h6v6"/><path d="M10 14L21 3"/>',
    check: '<path d="M20 6L9 17l-5-5"/>',
    refresh: '<path d="M21 12a9 9 0 1 1-3-6.7"/><path d="M21 3v6h-6"/>',
    plus: '<path d="M12 5v14M5 12h14"/>',
  };

  function icon(name, size) {
    const px = size || 18;
    const t = document.createElement("template");
    t.innerHTML = `<svg class="icon" width="${px}" height="${px}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${ICONS[name] || ""}</svg>`;
    return t.content.firstChild;
  }

  function logo() {
    const t = document.createElement("template");
    t.innerHTML = '<svg class="logo" width="30" height="30" viewBox="0 0 32 32" aria-hidden="true"><rect width="32" height="32" rx="9" fill="currentColor"/><rect x="8" y="9" width="16" height="3" rx="1.5" fill="#fff"/><rect x="8" y="15" width="11" height="3" rx="1.5" fill="#fff" opacity=".85"/><rect x="8" y="21" width="16" height="3" rx="1.5" fill="#ffd43b"/></svg>';
    return t.content.firstChild;
  }

  function brand() {
    return h("div", { class: "brand" }, logo(), h("span", {}, "Marginalia"));
  }

  // ------------------------------------------------------------ helpers
  function h(tag, props, ...children) {
    const el = document.createElement(tag);
    for (const [key, value] of Object.entries(props || {})) {
      if (value === false || value == null) continue;
      if (key === "class") el.className = value;
      else if (key.startsWith("on")) el.addEventListener(key.slice(2), value);
      else if (key === "text") el.textContent = value;
      else el.setAttribute(key, value === true ? "" : value);
    }
    for (const child of children.flat()) {
      if (child == null || child === false) continue;
      el.append(child.nodeType ? child : document.createTextNode(String(child)));
    }
    return el;
  }

  // replaceChildren() would render a null child as the text "null"; drop them instead.
  function fill(el, ...kids) {
    el.replaceChildren(...kids.flat().filter((k) => k != null && k !== false));
  }

  function stripHtml(html) {
    if (!html) return "";
    const doc = new DOMParser().parseFromString(html, "text/html");
    doc.querySelectorAll("script,style").forEach((n) => n.remove());
    return (doc.body.textContent || "").replace(/\s+/g, " ").trim();
  }

  function clip(text, n) {
    return text.length > n ? text.slice(0, n - 1).trimEnd() + "…" : text;
  }

  // Dates: servers can send the zero time ("0001-01-01") for items whose date
  // could not be parsed. Treat anything before 1990 or far in the future as missing.
  function validDate(iso) {
    if (!iso) return null;
    const d = new Date(iso);
    if (isNaN(d.getTime()) || d.getFullYear() < 1990) return null;
    if (d.getTime() - Date.now() > 36 * 3600 * 1000) return null;
    return d;
  }
  const postDate = (p) => validDate(p.published_at) || validDate(p.created_at);

  function ago(d) {
    if (!d) return "";
    const s = Math.max(0, (Date.now() - d.getTime()) / 1000);
    if (s < 60) return "Just now";
    if (s < 3600) return Math.round(s / 60) + " min ago";
    if (s < 86400) return Math.round(s / 3600) + " h ago";
    if (s < 604800) return Math.round(s / 86400) + " d ago";
    const opts = { month: "short", day: "numeric" };
    if (d.getFullYear() !== new Date().getFullYear()) opts.year = "numeric";
    return d.toLocaleDateString(undefined, opts);
  }

  function safeUrl(url) {
    try {
      const u = new URL(url);
      return u.protocol === "http:" || u.protocol === "https:" ? u.href : "#";
    } catch { return "#"; }
  }

  function shortUrl(url) {
    try { const u = new URL(url); return u.host + (u.pathname === "/" ? "" : u.pathname); } catch { return url; }
  }

  function feedAvatar(name) {
    let hash = 0;
    for (const c of name || "?") hash = (hash * 31 + c.charCodeAt(0)) >>> 0;
    const el = h("span", { class: "favatar", "aria-hidden": "true" }, (name || "?").trim().charAt(0).toUpperCase());
    el.style.background = `hsl(${hash % 360} 52% 40%)`;
    return el;
  }

  // ------------------------------------------------------------ api
  class ApiError extends Error {
    constructor(status, message) { super(message); this.status = status; }
  }

  const session = {
    get token() { return localStorage.getItem(TOKEN_KEY); },
    get user() { try { return JSON.parse(localStorage.getItem(USER_KEY)); } catch { return null; } },
    set(token, user) { localStorage.setItem(TOKEN_KEY, token); localStorage.setItem(USER_KEY, JSON.stringify(user)); },
    clear() { localStorage.removeItem(TOKEN_KEY); localStorage.removeItem(USER_KEY); },
  };

  async function api(path, { method = "GET", body, query } = {}) {
    const url = new URL(API + path, location.origin);
    for (const [k, v] of Object.entries(query || {})) if (v !== "" && v != null) url.searchParams.set(k, v);
    const headers = { Accept: "application/json" };
    if (body !== undefined) headers["Content-Type"] = "application/json";
    if (session.token) headers.Authorization = "Bearer " + session.token;

    let res;
    try {
      res = await fetch(url, { method, headers, body: body !== undefined ? JSON.stringify(body) : undefined });
    } catch {
      throw new ApiError(0, "Can’t reach the server. Check your connection and try again.");
    }
    const raw = await res.text();
    let data = null;
    if (raw) { try { data = JSON.parse(raw); } catch { data = null; } }

    if (res.status === 401 && session.token) {
      session.clear();
      state.sessionExpired = true;
      route();
      throw new ApiError(401, "Your session expired. Please sign in again.");
    }
    if (!res.ok) {
      throw new ApiError(res.status, (data && (data.error || data.message)) || `Request failed (${res.status})`);
    }
    return data;
  }

  function friendly(err) {
    if (err.status === 503) return "The AI service isn’t available right now. Everything else still works.";
    if (err.status === 429) return "Too many requests — give it a few seconds.";
    return err.message || "Something went wrong.";
  }

  // ------------------------------------------------------------ state
  const state = { feeds: new Map(), follows: [], sessionExpired: false, loaded: false };

  async function loadFeeds() {
    const [feeds, follows] = await Promise.all([api("/feeds"), api("/feedfollows")]);
    state.feeds = new Map((feeds || []).map((f) => [f.id, f]));
    state.follows = follows || [];
    state.loaded = true;
  }
  const feedName = (id) => (state.feeds.get(id) || {}).name || "Unknown feed";

  // ------------------------------------------------------------ shared UI
  const root = document.getElementById("app");

  const notice = (text, kind) => h("p", { class: "notice" + (kind ? " " + kind : ""), role: kind === "error" ? "alert" : "status" }, text);

  function thinking(label) {
    return h("div", { class: "thinking", role: "status" }, h("span", { class: "dots", "aria-hidden": "true" }, h("span"), h("span"), h("span")), label);
  }

  function skeletons(n) {
    return h("div", { class: "stack", "aria-hidden": "true" }, Array.from({ length: n || 3 }, () =>
      h("div", { class: "card skel" }, h("div", { class: "skel-line w30" }), h("div", { class: "skel-line tall w80" }), h("div", { class: "skel-line w95" }), h("div", { class: "skel-line w60" }))));
  }

  const emptyState = (title, body) => h("div", { class: "empty" }, h("strong", {}, title), body);
  const pageHead = (title, lede) => h("header", { class: "page-head" }, h("h1", {}, title), lede ? h("p", { class: "lede" }, lede) : null);

  function postItem(post, nameOverride) {
    const name = nameOverride || feedName(post.feed_id);
    const date = postDate(post);
    const text = clip(stripHtml(post.description), 340);
    const panel = h("div");
    let summary = null;

    const showLoading = (label) => panel.replaceChildren(h("div", { class: "summary loading", role: "status" }, h("span", { class: "dots", "aria-hidden": "true" }, h("span"), h("span"), h("span")), label));
    const showSummary = (s) => panel.replaceChildren(h("div", { class: "summary" },
      h("div", { class: "summary-head" }, icon("sparkle", 15), h("span", {}, "AI summary"), s.model ? h("span", { class: "model" }, s.model) : null),
      h("p", { class: "summary-body" }, s.summary)));
    const showError = (msg) => panel.replaceChildren(h("div", { class: "summary error", role: "alert" },
      h("p", {}, msg), h("button", { class: "btn small", type: "button", onclick: run }, "Try again")));

    async function run() {
      showLoading("Reading the article…");
      try {
        try {
          summary = await api(`/posts/${post.id}/summary`);
        } catch (err) {
          if (err.status !== 404) throw err;
          // No stored summary yet: ask the AI service to write one now.
          showLoading("Writing a summary…");
          summary = await api(`/posts/${post.id}/summary`, { method: "POST" });
        }
        showSummary(summary);
      } catch (err) { showError(friendly(err)); }
    }

    const btn = h("button", { class: "chip-btn", type: "button", "aria-expanded": "false" }, icon("sparkle", 15), h("span", {}, "Summary"));
    btn.addEventListener("click", () => {
      if (btn.getAttribute("aria-expanded") === "true") { panel.replaceChildren(); btn.setAttribute("aria-expanded", "false"); return; }
      btn.setAttribute("aria-expanded", "true");
      if (summary) showSummary(summary); else run();
    });

    const href = safeUrl(post.url);
    return h("article", { class: "card post" },
      h("div", { class: "post-meta" }, feedAvatar(name), h("span", { class: "feed" }, name),
        date ? h("span", { "aria-hidden": "true" }, "·") : null,
        date ? h("time", { datetime: date.toISOString(), title: date.toLocaleString() }, ago(date)) : null),
      h("h3", {}, h("a", { href, target: "_blank", rel: "noopener noreferrer" }, post.title)),
      text ? h("p", { class: "excerpt" }, text) : null,
      h("div", { class: "post-actions" }, btn,
        h("a", { class: "link-btn", href, target: "_blank", rel: "noopener noreferrer" }, "Read article", icon("external", 14))),
      panel);
  }

  // ------------------------------------------------------------ views
  async function viewToday(main) {
    const PAGE = 20;
    let offset = 0, search = "";
    const list = h("div", { class: "stack" });
    const status = h("div");
    const moreWrap = h("div", { class: "more" });
    const input = h("input", { class: "input", type: "search", placeholder: "Filter by title…", "aria-label": "Filter posts by title" });
    const form = h("form", { class: "toolbar", role: "search" },
      h("div", { class: "input-wrap" }, icon("search", 16), input),
      h("button", { class: "btn", type: "submit" }, "Filter"),
      h("button", { class: "btn icon-only", type: "button", title: "Refresh", "aria-label": "Refresh", onclick: () => load(true) }, icon("refresh", 16)));

    async function load(reset) {
      if (reset) { offset = 0; list.replaceChildren(); status.replaceChildren(skeletons(3)); }
      moreWrap.replaceChildren();
      try {
        const posts = (await api("/posts", { query: { limit: PAGE, offset, search } })) || [];
        status.replaceChildren();
        posts.forEach((p) => list.append(postItem(p)));
        offset += posts.length;
        if (!list.children.length) {
          status.replaceChildren(state.follows.length
            ? emptyState(search ? "Nothing matches that" : "Nothing new yet", search ? "Try a different word." : "Feeds are checked every minute; new posts will appear here.")
            : emptyState("Your reading list is empty", h("span", {}, "Follow a feed in ", h("a", { href: "#/discover" }, "Discover"), " to get started.")));
        }
        if (posts.length === PAGE) moreWrap.append(h("button", { class: "btn", type: "button", onclick: () => load(false) }, "Load more"));
      } catch (err) { status.replaceChildren(notice(friendly(err), "error")); }
    }

    form.addEventListener("submit", (e) => { e.preventDefault(); search = input.value.trim(); load(true); });
    main.append(h("div", { class: "column" },
      pageHead("Today", "Everything new from the feeds you follow. Open a summary to get the gist before you commit to the read."),
      form, list, status, moreWrap));
    load(true);
  }

  async function viewDiscover(main) {
    const body = h("div");
    const msg = h("div");

    function feedRow(feed, follow, refresh) {
      const btn = follow
        ? h("button", { class: "btn follow on", type: "button", "aria-label": `Unfollow ${feed.name}` }, h("span", { class: "rest" }, icon("check", 14), "Following"), h("span", { class: "hover" }, "Unfollow"))
        : h("button", { class: "btn primary", type: "button" }, "Follow");
      btn.addEventListener("click", async () => {
        btn.disabled = true;
        try {
          if (follow) await api(`/feedfollows/${follow.id}`, { method: "DELETE" });
          else await api("/feedfollows", { method: "POST", body: { feed_id: feed.id } });
          await refresh();
        } catch (err) { msg.replaceChildren(notice(friendly(err), "error")); btn.disabled = false; }
      });
      return h("div", { class: "feedrow" }, feedAvatar(feed.name),
        h("div", { class: "info" }, h("div", { class: "name" }, feed.name), h("div", { class: "url", title: feed.url }, shortUrl(feed.url))), btn);
    }

    async function refresh() {
      await loadFeeds();
      const followBy = new Map(state.follows.map((f) => [f.feed_id, f]));
      const all = [...state.feeds.values()].sort((a, b) => a.name.localeCompare(b.name));
      const mine = all.filter((f) => followBy.has(f.id));
      const others = all.filter((f) => !followBy.has(f.id));
      const section = (title, feeds, emptyText) => h("section", {},
        h("h2", {}, title, h("span", { class: "count" }, feeds.length)),
        feeds.length ? h("div", { class: "card feedlist" }, feeds.map((f) => feedRow(f, followBy.get(f.id), refresh))) : h("p", { class: "hint" }, emptyText));
      fill(body, section("Following", mine, "You aren’t following any feeds yet."), others.length ? section("More feeds", others) : null);
    }

    const name = h("input", { class: "input", id: "feed-name", placeholder: "BBC Technology", required: true, maxlength: "100", autocomplete: "off" });
    const url = h("input", { class: "input", id: "feed-url", type: "url", placeholder: "https://feeds.bbci.co.uk/news/technology/rss.xml", required: true, autocomplete: "off" });
    const add = h("button", { class: "btn primary", type: "submit" }, icon("plus", 16), "Add & follow");
    const form = h("form", { class: "card addfeed" },
      h("h2", { class: "card-title" }, "Add a feed"),
      h("div", { class: "addfeed-grid" },
        h("div", { class: "field" }, h("label", { for: "feed-name" }, "Name"), name),
        h("div", { class: "field" }, h("label", { for: "feed-url" }, "RSS address"), url),
        add),
      h("p", { class: "hint" }, "Use the direct RSS address, e.g. https://feeds.bbci.co.uk/news/technology/rss.xml"),
      msg);

    form.addEventListener("submit", async (e) => {
      e.preventDefault(); add.disabled = true; msg.replaceChildren();
      try {
        const feed = await api("/feeds", { method: "POST", body: { name: name.value.trim(), url: url.value.trim() } });
        // Older API builds return the raw database struct, whose key is "ID" rather than "id".
        const feedId = feed && (feed.id || feed.ID);
        if (!feedId) throw new ApiError(500, "The feed was created, but the server did not return its id. Reload the page and press Follow.");
        await api("/feedfollows", { method: "POST", body: { feed_id: feedId } });
        form.reset();
        msg.replaceChildren(notice("Feed added. Its first posts appear within a minute.", "success"));
        await refresh();
      } catch (err) {
        msg.replaceChildren(notice(friendly(err), err.status === 409 ? "warn" : "error"));
        if (err.status === 409) { try { await refresh(); } catch { /* list stays as is */ } }
      }
      add.disabled = false;
    });

    main.append(h("div", { class: "column" }, pageHead("Discover", "Add any RSS feed, or follow one that’s already here."), form, body));
    try { await refresh(); } catch (err) { body.replaceChildren(notice(friendly(err), "error")); }
  }

  async function viewSearch(main) {
    let mode = "text";
    const modes = [["text", "Keywords"], ["semantic", "Meaning"], ["hybrid", "Both"]];
    const hints = { text: "Matches words in titles and descriptions.", semantic: "Finds articles about the same idea, even when the words differ.", hybrid: "Needs the words and the idea — usually the best of both." };
    const results = h("div");
    const hint = h("p", { class: "hint", style: "margin:-6px 0 16px" }, hints.text);
    const input = h("input", { class: "input", type: "search", placeholder: "e.g. vector databases", "aria-label": "Search query", required: true });
    const seg = h("div", { class: "segmented", role: "group", "aria-label": "Search mode" });
    const drawSeg = () => seg.replaceChildren(...modes.map(([id, label]) =>
      h("button", { type: "button", "aria-pressed": String(mode === id), onclick: () => { mode = id; hint.textContent = hints[id]; drawSeg(); } }, label)));
    drawSeg();

    const form = h("form", { class: "toolbar", role: "search" },
      h("div", { class: "input-wrap" }, icon("search", 16), input), h("button", { class: "btn primary", type: "submit" }, "Search"));
    form.addEventListener("submit", async (e) => {
      e.preventDefault();
      const q = input.value.trim(); if (!q) return;
      results.replaceChildren(skeletons(2));
      try {
        const r = await api("/search", { query: { q, mode, limit: 20 } });
        const items = r.results || [];
        results.replaceChildren(
          h("p", { class: "hint", style: "margin:0 0 12px" }, `${r.total} result${r.total === 1 ? "" : "s"}${r.total > items.length ? `, showing the top ${items.length}` : ""}`),
          items.length ? h("div", { class: "stack" }, items.map((p) => postItem(p)))
            : emptyState("No matches", mode === "text" ? "Try fewer words, or switch to Meaning." : "Articles are embedded in the background; very new ones may not be searchable yet."));
      } catch (err) { results.replaceChildren(notice(friendly(err), "error")); }
    });
    main.append(h("div", { class: "column" }, pageHead("Search", "Look across everything you follow."), seg, hint, form, results));
  }

  const STEP_LABELS = { search: "Search", semantic_search: "Semantic search", recommendation: "Recommendations", rag: "Answer from articles", summary: "Summary" };
  const STARTERS = ["What’s new today?", "Summarize the top story", "What’s happening in tech?"];

  async function viewAsk(main) {
    let agent = false;
    const out = h("div");
    const input = h("textarea", { class: "input", id: "q", placeholder: "Ask anything about the articles in your feeds…", maxlength: "500", required: true });
    const seg = h("div", { class: "segmented", role: "group", "aria-label": "Answer style" });
    const hint = h("p", { class: "hint", style: "margin:-6px 0 16px" });
    const hints = { quick: "Answers from your most relevant articles, with the sources it used.", agent: "Plans which tools to use — search, semantic lookup, summary — and combines the results." };
    const drawSeg = () => {
      hint.textContent = agent ? hints.agent : hints.quick;
      seg.replaceChildren(...[[false, "Quick answer"], [true, "Agent"]].map(([val, label]) =>
        h("button", { type: "button", "aria-pressed": String(agent === val), onclick: () => { agent = val; drawSeg(); } }, label)));
    };
    drawSeg();

    const submit = h("button", { class: "btn primary", type: "submit" }, icon("sparkle", 16), "Ask");
    async function ask() {
      const query = input.value.trim(); if (!query) return;
      submit.disabled = true;
      out.replaceChildren(thinking(agent ? "The agent is working…" : "Reading your articles…"));
      try {
        const r = await api(agent ? "/agent/chat" : "/ask", { method: "POST", body: { query } });
        out.replaceChildren(renderAnswer(r, agent, ask));
      } catch (err) { out.replaceChildren(notice(friendly(err), "error")); }
      submit.disabled = false;
    }

    const starters = h("div", { class: "starters" }, STARTERS.map((s) => h("button", { type: "button", onclick: () => { input.value = s; ask(); } }, s)));
    const form = h("form", {}, starters,
      h("div", { class: "field" }, h("label", { for: "q", class: "sr-only" }, "Your question"), input),
      h("div", { class: "ask-actions" }, submit, h("span", { class: "kbd" }, "Ctrl + Enter to send")));
    form.addEventListener("submit", (e) => { e.preventDefault(); ask(); });
    input.addEventListener("keydown", (e) => { if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) { e.preventDefault(); ask(); } });

    main.append(h("div", { class: "column" }, pageHead("Ask", "Questions answered from the articles you follow."), seg, hint, form, out));
  }

  function renderAnswer(r, isAgent, retry) {
    const degraded = !!r.llm_error || /answer generation failed/i.test(r.answer || "");
    const head = h("div", { class: "answer-head" }, h("span", { class: "answer-label" }, icon("sparkle", 16), isAgent ? "Agent answer" : "Answer"));
    if (isAgent) {
      (r.plan || []).forEach((p) => {
        const used = (r.tools_used || []).includes(p);
        head.append(h("span", { class: "chip" + (used ? " ok" : "") }, used ? icon("check", 12) : null, STEP_LABELS[p] || p));
      });
      if (r.execution_time_ms != null) head.append(h("span", { class: "chip" }, (r.execution_time_ms / 1000).toFixed(1) + " s"));
    } else if (r.model && !degraded) {
      head.append(h("span", { class: "chip" }, r.model));
    }

    const sources = r.sources || [];
    const detail = r.llm_error || "";
    return h("section", { class: "card answer", "aria-live": "polite" }, head,
      degraded
        ? h("div", {},
            notice("The AI couldn’t write an answer right now, but these articles match your question.", "warn"),
            detail ? h("details", {}, h("summary", {}, "Technical details"), h("p", { class: "mono" }, detail)) : null,
            h("div", { style: "margin-top:10px" }, h("button", { class: "btn small", type: "button", onclick: retry }, "Try again")))
        : h("div", { class: "answer-body" }, r.answer),
      (r.errors && r.errors.length) ? h("div", { style: "margin-top:12px" }, notice("Some steps didn’t finish: " + r.errors.join("; "), "warn")) : null,
      sources.length ? h("h2", {}, "Sources", h("span", { class: "count" }, sources.length)) : null,
      sources.length ? h("ul", { class: "sources" }, sources.map((s) => {
        const d = validDate(s.published_at);
        const fname = feedName(s.feed_id);
        return h("li", {}, feedAvatar(fname), h("div", {},
          h("a", { href: safeUrl(s.url), target: "_blank", rel: "noopener noreferrer" }, s.title),
          h("span", { class: "when" }, [fname, ago(d)].filter(Boolean).join(" · "))));
      })) : null);
  }

  async function viewForYou(main) {
    const body = h("div");
    main.append(h("div", { class: "column" }, pageHead("For you", "Ranked from what’s fresh, what’s trending across your feeds, and how well it matches your reading."), body));
    body.append(skeletons(3));
    try {
      const r = await api("/recommendations", { query: { limit: 20 } });
      const items = r.items || [];
      body.replaceChildren();
      if (r.message) body.append(notice(r.message));
      if (!items.length) { body.append(emptyState("Nothing to recommend yet", "Follow a few feeds and check back after the first posts arrive.")); return; }
      body.append(h("div", { class: "stack" }, items.map((it) => {
        const card = postItem(it, it.feed_name);
        const tags = [it.category, ...(it.tags || [])].filter(Boolean);
        if (tags.length) card.querySelector(".post-actions").before(h("div", { class: "tags", style: "margin:0 0 12px" }, tags.slice(0, 5).map((t) => h("span", { class: "tag" }, t))));
        return card;
      })));
    } catch (err) { body.replaceChildren(notice(friendly(err), "error")); }
  }

  // ------------------------------------------------------------ auth
  function viewAuth(initialMode) {
    let mode = initialMode || "login";
    const card = h("div", { class: "card auth-card" });

    function draw(message) {
      const isLogin = mode === "login";
      const name = h("input", { class: "input", id: "a-name", autocomplete: "name", required: true });
      const email = h("input", { class: "input", id: "a-email", type: "email", autocomplete: "email", required: true });
      const pass = h("input", { class: "input", id: "a-pass", type: "password", autocomplete: isLogin ? "current-password" : "new-password", required: true, minlength: isLogin ? null : "8" });
      const err = h("div");
      const go = h("button", { class: "btn primary", type: "submit" }, isLogin ? "Sign in" : "Create account");
      const form = h("form", {},
        !isLogin && h("div", { class: "field" }, h("label", { for: "a-name" }, "Name"), name),
        h("div", { class: "field" }, h("label", { for: "a-email" }, "Email"), email),
        h("div", { class: "field" }, h("label", { for: "a-pass" }, "Password"), pass), err, go);

      form.addEventListener("submit", async (e) => {
        e.preventDefault(); go.disabled = true; err.replaceChildren();
        try {
          if (!isLogin) await api("/users", { method: "POST", body: { name: name.value.trim(), email: email.value.trim(), password: pass.value } });
          const r = await api("/login", { method: "POST", body: { email: email.value.trim(), password: pass.value } });
          session.set(r.token, { email: email.value.trim() });
          state.sessionExpired = false;
          const alreadyThere = location.hash === "#/today";
          location.hash = "#/today";
          if (alreadyThere) route(); // otherwise the hashchange event renders it
        } catch (e2) { err.replaceChildren(notice(e2.status === 401 ? "Email or password is incorrect." : friendly(e2), "error")); go.disabled = false; }
      });

      fill(card, brand(),
        h("h1", {}, isLogin ? "Welcome back" : "Create your account"),
        h("p", { class: "lede" }, isLogin ? "Sign in to read your feeds." : "Start following feeds in under a minute."),
        message ? notice(message) : null, form,
        h("p", { class: "switch" }, isLogin ? "New here? " : "Already registered? ",
          h("a", { href: "#", onclick: (e) => { e.preventDefault(); mode = isLogin ? "register" : "login"; draw(); } }, isLogin ? "Create an account" : "Sign in")));
    }

    draw(state.sessionExpired ? "Your session expired. Please sign in again." : null);
    root.replaceChildren(h("div", { class: "auth" }, card));
  }

  // ------------------------------------------------------------ router
  const routes = {
    today: ["Today", viewToday, "today"],
    foryou: ["For you", viewForYou, "star"],
    search: ["Search", viewSearch, "search"],
    ask: ["Ask", viewAsk, "chat"],
    discover: ["Discover", viewDiscover, "compass"],
  };

  function signOut() { session.clear(); state.loaded = false; location.hash = ""; route(); }

  function userPanel() {
    const email = (session.user && session.user.email) || "";
    const local = email.split("@")[0] || "You";
    return h("div", { class: "user" },
      h("div", { class: "avatar", "aria-hidden": "true" }, (local.charAt(0) || "?").toUpperCase()),
      h("div", { class: "who" }, h("div", { class: "who-name", title: local }, local), h("div", { class: "who-mail", title: email }, email)),
      h("button", { class: "icon-btn", type: "button", title: "Sign out", "aria-label": "Sign out", onclick: signOut }, icon("logout", 18)));
  }

  async function route() {
    if (!session.token) { viewAuth(); return; }
    const key = (location.hash.replace(/^#\/?/, "") || "today").split("?")[0];
    const current = routes[key] ? key : "today";
    const [title, view] = routes[current];
    document.title = `${title} · Marginalia`;

    const nav = h("nav", { class: "nav", "aria-label": "Main" },
      Object.entries(routes).map(([k, [label, , ic]]) =>
        h("a", { href: "#/" + k, "aria-current": k === current ? "page" : null }, icon(ic, 18), h("span", {}, label))));
    const main = h("main", { id: "main", tabindex: "-1" },
      h("div", { class: "topbar" }, brand(),
        h("button", { class: "icon-btn", type: "button", title: "Sign out", "aria-label": "Sign out", onclick: signOut }, icon("logout", 18))));

    root.replaceChildren(h("div", { class: "shell" }, h("aside", { class: "rail" }, brand(), nav, userPanel()), main));

    try { if (!state.loaded) await loadFeeds(); } catch (err) { if (err.status === 401) return; }
    await view(main);
  }

  window.addEventListener("hashchange", route);
  route();
})();
