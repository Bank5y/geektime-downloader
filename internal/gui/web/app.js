(() => {
  'use strict';

  // ---------- storage (per-viewer conveniences only) ----------
  const store = {
    get(k, d) { try { const v = localStorage.getItem('gd.' + k); return v == null ? d : JSON.parse(v); } catch (_) { return d; } },
    set(k, v) { try { localStorage.setItem('gd.' + k, JSON.stringify(v)); } catch (_) { /* ignore */ } },
  };

  const PAGE_SIZE = { explore: 24, mine: 24, course: 50, dl: 30 };

  // ---------- state ----------
  const S = {
    token: '',
    fatal: '',
    loggedIn: false,
    settings: null,
    types: [],
    limits: { maxThreads: 32, maxConcurrentTasks: 8 },
    // 全部课程
    meta: null,
    metaErr: '',
    ex: Object.assign({ tag: 0, sub: 0, type: 0, form: 0, pvip: 0, sort: 8 }, store.get('ex', {}), { q: '', page: 1, data: null, loading: false, err: '', seq: 0 }),
    // 我的课程
    products: null,
    productsErr: '',
    productsLoading: false,
    lib: Object.assign({ cat: '全部', media: 'all', status: 'all', dl: 'all', sort: 'recent' }, store.get('lib', {}), { q: '', page: 1 }),
    // download marks: courseId -> {done, total, marked}
    marks: {},
    // courseId -> {first, last, count}: publish time of the first / newest article
    dates: new Map(),
    // batch selection on shelves
    pick: { on: false, sel: new Map() },
    batch: null,
    quick: store.get('quick', { type: 'a0', id: '' }),
    course: null, // {key, type, id, data, err, loading, sel:Set, q, page, from}
    tasks: [],
    tasksSig: '',
    dl: { tab: store.get('dlTab', 'all'), q: '', page: 1 },
    page: '',
  };
  if (S.ex.sort === 1) S.ex.sort = 101; // the official "最新" order was replaced by 最新上线

  const $view = document.getElementById('view');
  const $badge = document.getElementById('nav-badge');
  const $logout = document.getElementById('logout');

  // ---------- helpers ----------
  const esc = (s) => String(s == null ? '' : s).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));

  function highlight(text, q) {
    text = String(text || '');
    if (!q) return esc(text);
    const lower = text.toLowerCase();
    const ql = q.toLowerCase();
    let out = '';
    let i = 0;
    for (;;) {
      const j = lower.indexOf(ql, i);
      if (j < 0) break;
      out += esc(text.slice(i, j)) + '<mark>' + esc(text.slice(j, j + q.length)) + '</mark>';
      i = j + q.length;
    }
    return out + esc(text.slice(i));
  }

  function fmtBytes(n) {
    if (!n || n < 0) return '0 B';
    const u = ['B', 'KB', 'MB', 'GB'];
    let i = 0;
    while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
    return (i === 0 ? n : n.toFixed(n >= 100 ? 0 : 1)) + ' ' + u[i];
  }

  function fmtDuration(ms) {
    if (!ms || ms < 0) return '';
    const s = Math.round(ms / 1000);
    if (s < 60) return s + ' 秒';
    const m = Math.floor(s / 60);
    if (m < 60) return m + ' 分 ' + (s % 60) + ' 秒';
    return Math.floor(m / 60) + ' 小时 ' + (m % 60) + ' 分';
  }

  const pad2 = (n) => String(n).padStart(2, '0');

  function debounce(fn, ms) {
    let t;
    return (...a) => { clearTimeout(t); t = setTimeout(() => fn(...a), ms); };
  }

  // deterministic muted "book cloth" color for covers without an image
  const CLOTH = ['#7a3b2e', '#2f5446', '#34466b', '#6b5530', '#5a3f5e', '#2d5c6b', '#5c4a3a', '#3f5a3a'];
  function clothColor(s) {
    let h = 0;
    for (const ch of String(s || '')) h = (h * 31 + ch.codePointAt(0)) >>> 0;
    return CLOTH[h % CLOTH.length];
  }
  const plateHtml = (title) => `<div class="plate" style="background:${clothColor(title)}"><span class="p-letter">${esc((title || '?').slice(0, 1))}</span><span class="p-title">${esc(title)}</span></div>`;

  const icons = {
    search: '<svg viewBox="0 0 24 24"><circle cx="11" cy="11" r="6.5"/><path d="M20 20l-4.2-4.2"/></svg>',
    refresh: '<svg viewBox="0 0 24 24"><path d="M20 11a8 8 0 1 0-2.3 5.7M20 5v6h-6"/></svg>',
    download: '<svg viewBox="0 0 24 24"><path d="M12 4v11m0 0-4.5-4.5M12 15l4.5-4.5M5 20h14"/></svg>',
    folder: '<svg viewBox="0 0 24 24"><path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/></svg>',
    retry: '<svg viewBox="0 0 24 24"><path d="M4 12a8 8 0 1 0 2.3-5.7M4 5v6h6"/></svg>',
    trash: '<svg viewBox="0 0 24 24"><path d="M5 7h14M10 7V5h4v2M7 7l1 12h8l1-12"/></svg>',
    back: '<svg viewBox="0 0 24 24"><path d="M19 12H5m6-6-6 6 6 6"/></svg>',
    video: '<svg viewBox="0 0 24 24"><path d="M8 5.5v13l10-6.5z"/></svg>',
    book: '<svg viewBox="0 0 24 24"><path d="M5 5.5A1.5 1.5 0 0 1 6.5 4H19v14H6.5A1.5 1.5 0 0 0 5 19.5z"/><path d="M5 19.5A1.5 1.5 0 0 0 6.5 21H19"/></svg>',
    doc: '<svg viewBox="0 0 24 24"><path d="M7 4h7l4 4v12H7z"/><path d="M14 4v4h4M10 13h5M10 16.5h5"/></svg>',
    stop: '<svg viewBox="0 0 24 24"><rect x="7" y="7" width="10" height="10" rx="1"/></svg>',
    open: '<svg viewBox="0 0 24 24"><path d="M14 4h6v6M20 4l-9 9M18 14v5H5V6h5"/></svg>',
    lock: '<svg viewBox="0 0 24 24"><rect x="5.5" y="11" width="13" height="9" rx="1.5"/><path d="M8.5 11V8a3.5 3.5 0 0 1 7 0v3"/></svg>',
    prev: '<svg viewBox="0 0 24 24"><path d="M15 6l-6 6 6 6"/></svg>',
    check: '<svg viewBox="0 0 24 24"><path d="M5 12.5l4.5 4.5L19 7.5"/></svg>',
    key: '<svg viewBox="0 0 24 24"><circle cx="8" cy="15" r="4"/><path d="M11 12l8-8M16 7l2.5 2.5M14 9l2 2"/></svg>',
    pick: '<svg viewBox="0 0 24 24"><rect x="4" y="4" width="16" height="16" rx="2"/><path d="M8 12l3 3 5-6"/></svg>',
    next: '<svg viewBox="0 0 24 24"><path d="M9 6l6 6-6 6"/></svg>',
  };

  function toast(msg, kind) {
    const el = document.createElement('div');
    el.className = 'toast ' + (kind || '');
    el.textContent = msg;
    document.getElementById('toasts').appendChild(el);
    setTimeout(() => el.remove(), kind === 'error' ? 5000 : 2800);
  }

  // ---------- theme ----------
  function currentTheme() {
    const t = document.documentElement.dataset.theme;
    if (t) return t;
    return window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
  }
  document.getElementById('theme').addEventListener('click', () => {
    const next = currentTheme() === 'dark' ? 'light' : 'dark';
    document.documentElement.dataset.theme = next;
    store.set('theme', next);
  });

  // ---------- pagination ----------
  function pager(target, page, total, size, cls) {
    const pages = Math.max(1, Math.ceil(total / size));
    if (total <= size) return '';
    const nums = [];
    const add = (n) => { if (n >= 1 && n <= pages && !nums.includes(n)) nums.push(n); };
    add(1);
    for (let n = page - 2; n <= page + 2; n++) add(n);
    add(pages);
    nums.sort((a, b) => a - b);
    let html = `<button class="pg" data-action="page" data-target="${target}" data-page="${page - 1}" ${page <= 1 ? 'disabled' : ''} aria-label="上一页">${icons.prev}</button>`;
    let last = 0;
    for (const n of nums) {
      if (n - last > 1) html += '<span class="pg-gap">…</span>';
      html += `<button class="pg ${n === page ? 'active' : ''}" data-action="page" data-target="${target}" data-page="${n}">${pad2(n)}</button>`;
      last = n;
    }
    html += `<button class="pg" data-action="page" data-target="${target}" data-page="${page + 1}" ${page >= pages ? 'disabled' : ''} aria-label="下一页">${icons.next}</button>`;
    const from = (page - 1) * size + 1;
    const to = Math.min(total, page * size);
    return `<div class="pager ${cls || ''}"><span>${from}–${to} / ${total}</span><div class="pg-btns">${html}</div>
      <label class="pg-jump">跳至 <input class="input" type="number" min="1" max="${pages}" data-jump="${target}" data-pages="${pages}"> 页</label></div>`;
  }

  function clampPage(page, total, size) {
    const pages = Math.max(1, Math.ceil(total / size));
    return Math.min(Math.max(1, page), pages);
  }

  function gotoPage(target, n) {
    switch (target) {
      case 'explore': S.ex.page = n; loadCatalog(); break;
      case 'mine': S.lib.page = n; renderLibraryResults(); break;
      case 'course': if (S.course) { S.course.page = n; renderArticleList(); } break;
      case 'dl': S.dl.page = n; renderTaskList(true); break;
      default: return;
    }
    const anchor = document.getElementById('results-top');
    if (anchor && anchor.getBoundingClientRect().top < 64) anchor.scrollIntoView({ block: 'start' });
  }

  // ---------- api ----------
  async function api(method, path, body) {
    const res = await fetch(path, {
      method,
      headers: { 'X-GD-Token': S.token, 'Content-Type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    let data = {};
    try { data = await res.json(); } catch (_) { /* empty */ }
    if (!res.ok) {
      const err = new Error(data.error || ('请求失败 (' + res.status + ')'));
      err.status = res.status;
      if (res.status === 401) {
        if ((data.error || '').indexOf('访问令牌') >= 0) {
          S.fatal = data.error;
          render();
        } else if (S.loggedIn) {
          S.loggedIn = false;
          updateChrome();
          if (S.page !== 'login') location.hash = '#/login';
        }
      }
      throw err;
    }
    return data;
  }

  // ---------- product helpers ----------
  const CATEGORY_ORDER = ['专栏', '视频课', '训练营', '每日一课', '大厂案例', '公开课', '其他'];
  function category(p) {
    if ((p.code || '').charAt(0) === 'u' || p.typeName === '训练营') return '训练营';
    switch (p.code) {
      case 'c1': return '专栏';
      case 'c3': return '视频课';
      case 'd': return '每日一课';
      case 'q': return '大厂案例';
      case 'p35': case 'p29': case 'p30': return '公开课';
      default: return '其他';
    }
  }
  const typeInfo = (key) => S.types.find((t) => t.key === key);

  // product lookup across all loaded lists
  const productIndex = new Map();
  const courseIndex = new Map();
  function indexProducts(list) {
    for (const p of list || []) {
      productIndex.set(String(p.id), p);
      courseIndex.set(p.typeKey + ':' + p.courseId, p);
    }
  }

  function coverHtml(p, title) {
    if (p && p.cover) {
      return `<img src="${esc(p.cover)}" alt="" loading="lazy" referrerpolicy="no-referrer" data-plate="${esc(title)}">`;
    }
    return plateHtml(title);
  }

  // download state of a course: full / part / local / null
  function markInfo(p) {
    const r = S.marks[String(p.courseId)];
    if (r && (r.marked || (r.total > 0 && r.done >= r.total))) return { cls: 'full', text: '已下载' };
    if (r && r.done > 0) return { cls: 'part', text: r.total > 0 ? `${r.done}/${r.total}` : `已下载 ${r.done}` };
    if (p.local) return { cls: 'local', text: '本地已有' };
    return null;
  }
  const isFullyDownloaded = (p) => { const m = markInfo(p); return !!m && m.cls === 'full'; };
  function markHtml(p) {
    const m = markInfo(p);
    return m ? `<span class="book-mark ${m.cls}" title="${m.cls === 'part' ? '已下载 ' + m.text + ' 篇' : m.text}">${m.cls !== 'local' ? icons.check : ''}${esc(m.text)}</span>` : '';
  }
  const pickable = (p) => p.openable && p.purchased;

  async function loadMarks() {
    try {
      const r = await api('GET', '/api/marks');
      S.marks = r.marks || {};
      patchMarks();
    } catch (_) { /* ignore */ }
  }
  const loadMarksSoon = debounce(loadMarks, 800);

  function patchMarks() {
    document.querySelectorAll('.book[data-id]').forEach((el) => {
      const p = productIndex.get(el.dataset.id);
      const holder = el.querySelector('.mark-slot');
      if (!p || !holder) return;
      const html = markHtml(p);
      if (holder.innerHTML !== html) holder.innerHTML = html;
    });
    if (S.page === 'mine' && S.lib.dl !== 'all') renderLibraryResults();
    const st = document.querySelector('.dl-state');
    if (S.page === 'course' && st && S.course && S.course.data) {
      const tmp = document.createElement('div');
      tmp.innerHTML = courseStateHtml(S.course.data);
      st.replaceWith(tmp.firstElementChild);
    }
  }

  // a course rendered as a book on the shelf
  function bookCard(p, q) {
    let stamp;
    if (!p.openable) {
      stamp = `<span class="stamp locked" title="训练营请在「我的课程」中下载">${icons.lock}</span>`;
    } else if (!p.purchased && p.claimable) {
      stamp = `<button class="stamp unlock" data-action="unlock" data-id="${p.id}" title="免费领取 (解锁课程)" aria-label="免费领取《${esc(p.title)}》">${icons.key}</button>`;
    } else if (!p.purchased) {
      stamp = `<span class="stamp locked" title="尚未购买, 无法下载">${icons.lock}</span>`;
    } else {
      stamp = `<button class="stamp" data-action="quick-dl" data-id="${p.id}" title="${p.direct ? '下载' : '下载整套课程'}" aria-label="下载《${esc(p.title)}》">${icons.download}</button>`;
    }
    const unit = p.unit && !/\d/.test(p.unit) ? p.unit : '讲';
    const count = p.articleCount ? `${p.articleCount} ${esc(unit)}` : '';
    let flag = '';
    if (!p.purchased && p.claimable) flag = '<span class="book-flag free">免费领取</span>';
    else if (!p.purchased) flag = '<span class="book-flag">未购买</span>';
    else if (p.direct) flag = '<span class="book-flag">单个视频</span>';
    const picked = S.pick.sel.has(String(p.id));
    return `
      <article class="book ${p.openable ? '' : 'muted'} ${picked ? 'picked' : ''} ${pickable(p) ? '' : 'unpickable'}" data-action="open-product" data-id="${p.id}" tabindex="0" role="link" aria-label="${esc(p.title)}">
        <div class="book-cover">${coverHtml(p, p.title)}${stamp}${flag}<span class="mark-slot">${markHtml(p)}</span><span class="pick-box">${icons.check}</span></div>
        <div class="book-meta">
          <div class="book-kicker"><span>${esc(category(p))}</span>${count ? `<span class="dot"></span><span class="k-muted">${count}</span>` : ''}${p.isFinish ? '<span class="dot"></span><span class="k-muted">完结</span>' : ''}</div>
          <h3 class="book-title" title="${esc(p.subtitle || '')}">${highlight(p.title, q)}</h3>
          <div class="book-author">${highlight(p.author || p.subtitle, q)}</div>
          <div class="book-dates" data-dates="${p.id}">${datesHtml(p)}</div>
        </div>
      </article>`;
  }

  // ---------- hover preview of the chapter outline ----------
  const PEEK_DELAY = 420;
  const PEEK_WIDTH = 340;
  const peekCache = new Map(); // typeKey:courseId -> outline | Promise
  const canHover = window.matchMedia && window.matchMedia('(hover: hover) and (pointer: fine)').matches;
  let peekTimer = 0;
  let peekFor = null; // the .book element the preview belongs to

  function peekKey(p) { return p.typeKey + ':' + p.courseId; }

  function loadOutline(p) {
    const k = peekKey(p);
    if (peekCache.has(k)) return Promise.resolve(peekCache.get(k));
    const pr = api('GET', `/api/course/preview?type=${encodeURIComponent(p.typeKey)}&id=${p.courseId}`)
      .then((d) => {
        peekCache.set(k, d);
        if (d.last || d.first) {
          const old = S.dates.get(p.courseId) || {};
          S.dates.set(p.courseId, Object.assign({}, old, { first: d.first || old.first, last: d.last || old.last, count: d.count || old.count, done: true }));
          patchDates(new Set([p.courseId]));
        }
        return d;
      })
      .catch((e) => { peekCache.delete(k); throw e; });
    peekCache.set(k, pr);
    return pr;
  }

  function peekBodyHtml(p, d) {
    if (!p.openable) return '<div class="peek-note">训练营的目录请在「我的课程」中查看</div>';
    if (p.direct) return `<div class="peek-note">${esc(p.subtitle || '')}${p.subtitle ? '<br>' : ''}单个视频课程, 没有章节目录</div>`;
    if (!d) return '<div class="peek-list">' + Array.from({ length: 6 }).map((_, i) => `<div class="skeleton" style="height:11px;margin:10px 0;width:${90 - i * 9}%"></div>`).join('') + '</div>';
    if (d.error) return `<div class="peek-note err">目录加载失败: ${esc(d.error)}</div>`;
    if (!d.chapters.length) return '<div class="peek-note">暂无目录</div>';
    // budget of lines so the card stays compact: chapter rows + a few lessons each
    const MAX_LINES = 16;
    const chapters = d.chapters;
    const named = chapters.length > 1 || chapters[0].title;
    const perChapter = !named ? MAX_LINES : Math.max(0, Math.min(3, Math.floor((MAX_LINES - Math.min(chapters.length, 10)) / Math.max(1, Math.min(chapters.length, 10)))));
    let lines = 0;
    let html = '';
    let shown = 0;
    for (const c of chapters) {
      if (lines >= MAX_LINES) break;
      shown++;
      if (named) {
        html += `<div class="peek-ch"><span class="no">${pad(shown)}</span><span class="t">${esc(c.title || '未分章节')}</span><span class="n">${c.count}</span></div>`;
        lines++;
      }
      const take = Math.min(c.articles.length, perChapter, MAX_LINES - lines);
      html += c.articles.slice(0, take).map((t) => `<div class="peek-a">${esc(t)}</div>`).join('');
      lines += take;
      // named chapters show their size on the right, an extra "more" line would be noise
      if (!named && c.articles.length > take) html += `<div class="peek-a more">… 另有 ${c.articles.length - take} 讲</div>`;
    }
    const rest = chapters.length - shown;
    const planned = p.articleCount > d.total ? `已更新 ${d.total} / ${p.articleCount} 讲` : `共 ${d.total} 讲`;
    const foot = named ? `${chapters.length} 章 · ${planned}${rest > 0 ? ` · 另有 ${rest} 章` : ''}` : planned;
    return `<div class="peek-list">${html}</div><div class="peek-foot">${foot}<span>点击查看完整目录</span></div>`;
  }

  function peekHtml(p, d) {
    const dt = S.dates.get(p.courseId) || {};
    const launch = p.beginTime || (d && d.first) || dt.first;
    const last = (d && d.last) || dt.last;
    const meta = [category(p), p.articleCount ? `${p.articleCount} 讲` : '', p.isFinish ? '已完结' : (datable(p) ? '更新中' : '')]
      .filter(Boolean).map(esc).join(' · ');
    const dates = [launch ? `上线 ${fmtDay(launch)}` : '', last ? `最近更新 ${fmtDay(last)}` : ''].filter(Boolean).join('<span class="sep"></span>');
    return `
      <div class="peek-kicker">${meta}</div>
      <div class="peek-title">${esc(p.title)}</div>
      ${dates ? `<div class="peek-dates">${dates}</div>` : ''}
      ${peekBodyHtml(p, d)}`;
  }

  function placePeek(el, box) {
    const r = el.getBoundingClientRect();
    const gap = 14;
    let left = r.right + gap;
    let side = 'right';
    if (left + PEEK_WIDTH > window.innerWidth - 12) { left = r.left - gap - PEEK_WIDTH; side = 'left'; }
    if (left < 12) return false;
    box.style.left = left + 'px';
    const h = box.offsetHeight;
    box.style.top = Math.max(76, Math.min(r.top, window.innerHeight - h - 12)) + 'px';
    box.dataset.side = side;
    return true;
  }

  function showPeek(el, p) {
    let box = document.getElementById('peek');
    if (!box) {
      box = document.createElement('div');
      box.id = 'peek';
      box.className = 'peek';
      box.setAttribute('role', 'tooltip');
      document.body.appendChild(box);
    }
    peekFor = el;
    const cached = peekCache.get(peekKey(p));
    const ready = cached && !(cached instanceof Promise) ? cached : null;
    box.innerHTML = peekHtml(p, ready);
    box.classList.remove('show');
    if (!placePeek(el, box)) { hidePeek(); return; }
    requestAnimationFrame(() => box.classList.add('show'));
    if (ready || !p.openable || p.direct) return;
    loadOutline(p).then((d) => d, (e) => ({ error: e.message, chapters: [] })).then((d) => {
      if (peekFor !== el || !document.body.contains(el)) return;
      box.innerHTML = peekHtml(p, d);
      placePeek(el, box);
    });
  }

  function hidePeek() {
    clearTimeout(peekTimer);
    peekFor = null;
    const box = document.getElementById('peek');
    if (box) box.classList.remove('show');
  }

  if (canHover) {
    document.addEventListener('mouseover', (e) => {
      const el = e.target.closest && e.target.closest('.shelf .book[data-id]');
      if (!el || el === peekFor) return;
      clearTimeout(peekTimer);
      if (peekFor) hidePeek();
      const p = productIndex.get(el.dataset.id);
      if (!p) return;
      peekTimer = setTimeout(() => { if (el.matches(':hover')) showPeek(el, p); }, PEEK_DELAY);
    });
    document.addEventListener('mouseout', (e) => {
      const el = e.target.closest && e.target.closest('.shelf .book[data-id]');
      if (!el || (e.relatedTarget && el.contains(e.relatedTarget))) return;
      hidePeek();
    });
    window.addEventListener('scroll', hidePeek, { passive: true });
    window.addEventListener('blur', hidePeek);
    document.addEventListener('mousedown', hidePeek, true);
    window.addEventListener('hashchange', hidePeek);
  }

  // ---------- launch / last update dates ----------
  const pad = (n) => String(n).padStart(2, '0');
  function fmtDay(ts, withDay = true) {
    const d = new Date(ts * 1000);
    return d.getFullYear() + '.' + pad(d.getMonth() + 1) + (withDay ? '.' + pad(d.getDate()) : '');
  }
  // courses whose article list (and so the dates) can be looked up
  const datable = (p) => p && !p.direct && p.typeKey !== 'n4' && p.typeKey !== 'e0' && p.courseId > 0;

  function datesHtml(p) {
    const d = S.dates.get(p.courseId) || {};
    const launch = p.beginTime || d.first;
    const parts = [];
    if (launch) parts.push(`<span title="上线时间 ${fmtDay(launch)}">${fmtDay(launch, false)} 上线</span>`);
    if (d.last) {
      const recent = !p.isFinish && Date.now() / 1000 - d.last < 14 * 86400;
      parts.push(`<span class="${recent ? 'recent' : ''}" title="最近一讲发布于 ${fmtDay(d.last)}">${fmtDay(d.last)} 更新</span>`);
    } else if (datable(p) && !d.done) {
      parts.push('<span class="pending">更新时间…</span>');
    }
    return parts.join('');
  }

  function patchDates(ids) {
    document.querySelectorAll('[data-dates]').forEach((el) => {
      const p = productIndex.get(el.dataset.dates);
      if (!p || (ids && !ids.has(p.courseId))) return;
      const html = datesHtml(p);
      if (el.innerHTML !== html) el.innerHTML = html;
    });
  }

  const datesPending = new Set();
  // fetch missing dates for the courses currently on screen
  const loadVisibleDates = debounce(async () => {
    const need = [];
    document.querySelectorAll('.book[data-id]').forEach((el) => {
      const p = productIndex.get(el.dataset.id);
      if (datable(p) && !S.dates.has(p.courseId) && !datesPending.has(p.courseId)) need.push(p);
    });
    for (let i = 0; i < need.length; i += 40) {
      const chunk = need.slice(i, i + 40);
      chunk.forEach((p) => datesPending.add(p.courseId));
      const ids = chunk.map((p) => p.courseId);
      const first = chunk.filter((p) => !p.beginTime).map((p) => p.courseId);
      try {
        const r = await api('GET', `/api/course/dates?ids=${ids.join(',')}&first=${first.join(',')}`);
        for (const id of ids) S.dates.set(id, Object.assign({ done: true }, (r.dates || {})[id]));
      } catch (_) {
        for (const id of ids) S.dates.set(id, { done: true });
      } finally {
        ids.forEach((id) => datesPending.delete(id));
      }
      patchDates(new Set(ids));
    }
  }, 60);

  function skeletonShelf() {
    return '<div class="shelf">' + Array.from({ length: 12 }).map(() =>
      '<div class="book"><div class="book-cover skeleton"></div><div class="book-meta"><div class="skeleton" style="height:10px;width:40%"></div><div class="skeleton" style="height:16px;width:85%;margin-top:10px"></div><div class="skeleton" style="height:12px;width:50%;margin-top:8px"></div></div></div>').join('') + '</div>';
  }

  function openProduct(p) {
    if (!p.openable) { toast('该训练营无法从搜索结果打开, 请在「我的课程」中找到它', 'error'); return; }
    if (p.direct) { quickDownload(p); return; }
    location.hash = `#/course/${p.typeKey}/${p.courseId}`;
  }

  function quickDownload(p) {
    if (!p.openable || !p.purchased) return;
    enqueue(p.typeKey, p.courseId, [], p.title);
  }

  // facet row of text filters
  function facet(label, items, cur, action, opts) {
    opts = opts || {};
    const btns = items.map((it) => `<button class="fi ${String(it.value) === String(cur) ? 'active' : ''}" data-action="${action}" data-v="${esc(it.value)}" ${opts.disabled ? 'disabled' : ''}>${esc(it.title)}${it.count != null ? `<sup>${it.count}</sup>` : ''}</button>`).join('');
    return `<div class="facet ${opts.cls || ''}"><span class="facet-label">${label}</span><div class="facet-items">${btns}${opts.end || ''}</div></div>`;
  }

  // ---------- task helpers ----------
  const STATUS_TEXT = { queued: '等待中', running: '下载中', done: '已完成', failed: '失败', canceled: '已取消', skipped: '已跳过' };
  const isActive = (t) => t.status === 'queued' || t.status === 'running';

  function taskIndex() {
    const m = new Map();
    for (const t of S.tasks) m.set(t.typeKey + ':' + t.courseId + ':' + t.articleId, t);
    return m;
  }

  const pendingEnqueue = new Set();
  async function enqueue(type, courseId, articleIds, title) {
    const key = type + ':' + courseId + ':' + (articleIds || []).join(',');
    if (pendingEnqueue.has(key)) return false;
    pendingEnqueue.add(key);
    if (title && !(articleIds && articleIds.length)) toast(`正在获取《${title}》的目录…`);
    try {
      const r = await api('POST', '/api/tasks', { type, courseId: Number(courseId), articleIds: articleIds || [] });
      if (r.added > 0) {
        toast((title ? `《${title}》` : '') + `已加入 ${r.added} 个下载任务`, 'success');
      } else {
        toast('任务已在下载队列中');
      }
      pollTasks();
      return true;
    } catch (e) {
      toast(e.message, 'error');
      return false;
    } finally {
      pendingEnqueue.delete(key);
    }
  }

  // ---------- chrome ----------
  function updateChrome() {
    const nav = S.page === 'course' ? (S.course && S.course.from) || 'explore' : S.page;
    document.querySelectorAll('[data-nav]').forEach((a) => a.classList.toggle('active', a.dataset.nav === nav));
    $logout.hidden = !S.loggedIn;
    const active = S.tasks.filter(isActive).length;
    $badge.hidden = active === 0;
    $badge.textContent = active;
  }

  // ---------- router ----------
  function parseRoute() {
    const parts = (location.hash.replace(/^#\/?/, '') || 'explore').split('/').filter(Boolean);
    return parts.length ? parts : ['explore'];
  }

  let lastListPage = 'explore';
  function render() {
    if (S.fatal) {
      S.page = 'fatal';
      $view.innerHTML = `<div class="page"><div class="empty"><div class="glyph">断</div><div class="title">无法连接到下载器</div><div>${esc(S.fatal)}</div></div></div>`;
      updateChrome();
      return;
    }
    const parts = parseRoute();
    let page = parts[0];
    if (page === 'library') page = 'explore';
    if (!S.loggedIn && page !== 'settings') page = 'login';
    if (S.loggedIn && page === 'login') page = 'explore';
    const prev = S.page;
    S.page = page;
    if (page === 'explore' || page === 'mine') lastListPage = page;
    if (prev !== page && S.pick.on) { S.pick.on = false; S.pick.sel.clear(); }
    if (prev !== page && S.batch && !S.batch.running) closeDrawer();
    switch (page) {
      case 'login': renderLogin(); break;
      case 'mine': renderLibrary(); break;
      case 'course': renderCourse(parts[1], parts[2]); break;
      case 'downloads': renderDownloads(); break;
      case 'settings': renderSettings(); break;
      default: S.page = 'explore'; renderExplore(); break;
    }
    updateChrome();
    if (prev !== S.page) {
      window.scrollTo(0, 0);
      const pg = $view.firstElementChild;
      if (pg) pg.classList.add('enter');
    }
  }

  // ---------- login ----------
  function renderLogin() {
    $view.innerHTML = `
      <div class="page login-wrap">
        <form class="login" id="login-form" autocomplete="off">
          <span class="seal">极</span>
          <div class="kicker">Sign in</div>
          <h1>登录极客时间</h1>
          <p class="sub">使用浏览器中的 Cookie 登录。凭据只保存在本机内存中, 关闭程序即失效。</p>
          <div class="error-box" id="login-err" hidden></div>
          <div class="quick-login">
            <button class="btn btn-primary btn-block" id="quick-login" type="button" disabled>${icons.key}从浏览器快速登录</button>
            <div class="quick-src" id="quick-src">正在检测本机浏览器…</div>
          </div>
          <div class="or"><span>或手动填写 Cookie</span></div>
          <div class="field">
            <label for="gcid">GCID</label>
            <input class="input" id="gcid" name="gcid" spellcheck="false" required>
          </div>
          <div class="field">
            <label for="gcess">GCESS</label>
            <input class="input" id="gcess" name="gcess" spellcheck="false" required>
          </div>
          <button class="btn btn-block" type="submit" id="login-submit">登录</button>
          <div class="help">
            如何获取 Cookie
            <ol>
              <li>在浏览器中登录 <code>time.geekbang.org</code></li>
              <li>按 <code>F12</code> 打开开发者工具, 进入 Application → Cookies</li>
              <li>复制 <code>GCID</code> 与 <code>GCESS</code> 的值填入上方</li>
            </ol>
            <p>快速登录会读取本机 Chrome / Edge / Brave / Chromium 中 <code>time.geekbang.org</code> 的这两个 Cookie (复制浏览器的 Cookie 文件后以无界面方式由浏览器自身解密, 不会改动浏览器数据), 需要先在该浏览器中登录极客时间。暂不支持 Firefox 与 Safari。</p>
          </div>
        </form>
      </div>`;
    const form = document.getElementById('login-form');
    const loggedIn = (msg) => {
      S.loggedIn = true;
      S.products = null;
      S.meta = null;
      S.ex.data = null;
      loadMarks();
      toast(msg, 'success');
      location.hash = '#/explore';
      render();
    };
    setupQuickLogin(loggedIn);
    form.addEventListener('submit', async (e) => {
      e.preventDefault();
      const btn = form.querySelector('button[type=submit]');
      const err = document.getElementById('login-err');
      btn.disabled = true;
      btn.textContent = '正在校验…';
      err.hidden = true;
      try {
        await api('POST', '/api/login', { gcid: form.gcid.value, gcess: form.gcess.value });
        loggedIn('登录成功');
      } catch (ex) {
        err.textContent = ex.message;
        err.hidden = false;
      } finally {
        btn.disabled = false;
        btn.textContent = '登录';
      }
    });
  }

  // quick login: the server reads GCID / GCESS from the local browsers, values never reach the page
  async function setupQuickLogin(loggedIn) {
    const btn = document.getElementById('quick-login');
    const $src = document.getElementById('quick-src');
    const err = document.getElementById('login-err');
    let sources = [];
    try {
      sources = (await api('GET', '/api/login/browsers')).sources || [];
    } catch (_) { /* treated as none */ }
    if (!document.body.contains(btn)) return;
    const label = (x) => x.browser + (x.profile ? ' · ' + x.profile : '');
    if (!sources.length) {
      $src.textContent = '未检测到 Chrome / Edge / Brave / Chromium, 请手动填写 Cookie';
      document.getElementById('gcid').focus();
      return;
    }
    $src.innerHTML = sources.length === 1
      ? `将从 <b>${esc(label(sources[0]))}</b> 读取`
      : `<select class="select" id="quick-pick" aria-label="选择浏览器">
          <option value="">自动 (依次尝试 ${sources.length} 个浏览器配置)</option>
          ${sources.map((x) => `<option value="${esc(x.id)}">${esc(label(x))}</option>`).join('')}
        </select>`;
    btn.disabled = false;
    btn.focus();
    btn.addEventListener('click', async () => {
      const pick = document.getElementById('quick-pick');
      const html = btn.innerHTML;
      btn.disabled = true;
      btn.textContent = '正在读取浏览器登录信息…';
      err.hidden = true;
      try {
        const r = await api('POST', '/api/login/browser', { id: pick ? pick.value : '' });
        loggedIn('已使用 ' + r.source + ' 的登录信息登录');
      } catch (ex) {
        err.textContent = ex.message;
        err.hidden = false;
        btn.disabled = false;
        btn.innerHTML = html;
      }
    });
  }

  // ---------- 全部课程 (catalog) ----------
  function saveEx() {
    const { tag, sub, type, form, pvip, sort } = S.ex;
    store.set('ex', { tag, sub, type, form, pvip, sort });
  }

  async function loadMeta() {
    try {
      S.meta = await api('GET', '/api/catalog/meta');
      S.metaErr = '';
    } catch (e) {
      S.metaErr = e.message;
    }
    if (S.page === 'explore') renderExploreFilters();
  }

  async function loadCatalog() {
    const ex = S.ex;
    const seq = ++ex.seq;
    ex.loading = true;
    ex.err = '';
    renderExploreResults();
    const params = new URLSearchParams({ page: ex.page, size: PAGE_SIZE.explore });
    if (ex.q) {
      params.set('q', ex.q);
    } else {
      const tag = ex.sub || ex.tag;
      if (tag) params.set('tag', tag);
      if (ex.type) params.set('type', ex.type);
      if (ex.form) params.set('form', ex.form);
      if (ex.pvip) params.set('pvip', ex.pvip);
      params.set('sort', ex.sort);
    }
    try {
      const data = await api('GET', '/api/catalog?' + params);
      if (seq !== ex.seq) return;
      data.items = data.items || [];
      indexProducts(data.items);
      ex.data = data;
    } catch (e) {
      if (seq !== ex.seq) return;
      ex.err = e.message;
    } finally {
      if (seq === ex.seq) {
        ex.loading = false;
        if (S.page === 'explore') renderExploreResults();
      }
    }
  }

  function renderExplore() {
    $view.innerHTML = `
      <div class="page">
        <div class="page-head">
          <div>
            <div class="kicker">Catalog</div>
            <h1>全部课程</h1>
            <div class="sub">与极客时间官网同步的完整目录。已购买或会员可学的课程, 将鼠标移到封面上即可一键下载整套课程。</div>
          </div>
        </div>
        <div class="searchbar">${icons.search}<input id="ex-q" placeholder="搜索全站课程、讲师…" value="${esc(S.ex.q)}" aria-label="搜索课程"><span class="hint-key">按 <kbd>Enter</kbd> 搜索</span></div>
        <div id="ex-filters"></div>
        <div class="result-bar" id="results-top"><span class="count" id="ex-count"></span><span id="ex-note"></span><span class="pick-tools" id="pick-tools"></span></div>
        <div id="ex-results"></div>
        <div class="selbar" id="pick-bar" hidden></div>
      </div>`;
    const q = document.getElementById('ex-q');
    q.addEventListener('keydown', (e) => { if (e.key === 'Enter') doSearch(); });
    q.addEventListener('input', debounce(() => { if (!q.value.trim() && S.ex.q) doSearch(); }, 300));
    if (!S.meta) loadMeta();
    renderExploreFilters();
    if (!S.ex.data && !S.ex.loading) loadCatalog();
    else renderExploreResults();
  }

  function doSearch() {
    const q = document.getElementById('ex-q');
    S.ex.q = q ? q.value.trim() : '';
    S.ex.page = 1;
    renderExploreFilters();
    loadCatalog();
  }

  function renderExploreFilters() {
    const $f = document.getElementById('ex-filters');
    if (!$f) return;
    const ex = S.ex;
    if (S.metaErr && !S.meta) {
      $f.innerHTML = `<div class="facets"><div class="error-box" style="margin:12px 0 0">加载筛选条件失败: ${esc(S.metaErr)}</div></div>`;
      return;
    }
    if (!S.meta) {
      $f.innerHTML = '<div class="facets">' + Array.from({ length: 4 }).map(() => '<div class="facet"><div class="skeleton" style="height:16px;width:70%"></div></div>').join('') + '</div>';
      return;
    }
    const m = S.meta;
    const dis = !!ex.q;
    const labels = [{ title: '全部', value: 0 }].concat(m.labels.map((l) => ({ title: l.name, value: l.id, count: l.count })));
    let html = facet('方向', labels, ex.tag, 'ex-tag', { disabled: dis });
    const parent = m.labels.find((l) => l.id === ex.tag);
    if (parent && parent.children && parent.children.length) {
      const subs = [{ title: '全部', value: 0 }].concat(parent.children.map((l) => ({ title: l.name, value: l.id, count: l.count })));
      html += facet('', subs, ex.sub, 'ex-sub', { disabled: dis, cls: 'sub' });
    }
    const opt = (list) => (list || []).map((o) => ({ title: o.title, value: o.value, count: o.count }));
    html += facet('类型', opt(m.types), ex.type, 'ex-type', { disabled: dis });
    html += facet('形式', opt(m.forms), ex.form, 'ex-form', { disabled: dis });
    html += facet('会员', opt(m.pvips), ex.pvip, 'ex-pvip', { disabled: dis });
    // 101 / 102: launch time order, sorted by the gui server over the whole filtered list
    html += facet('排序', [{ title: '默认', value: 8 }, { title: '最热', value: 4 }, { title: '最新上线', value: 101 }, { title: '最早上线', value: 102 }], ex.sort, 'ex-sort', {
      disabled: dis,
      end: `<button class="text-link facet-end" data-action="ex-reset" ${dis ? 'disabled' : ''}>重置筛选</button>`,
    });
    $f.innerHTML = `<div class="facets ${dis ? 'disabled' : ''}">${html}</div>`;
  }

  function renderExploreResults() {
    const $res = document.getElementById('ex-results');
    if (!$res) return;
    const ex = S.ex;
    const $count = document.getElementById('ex-count');
    document.getElementById('ex-note').innerHTML = ex.q
      ? `搜索「${esc(ex.q)}」时不应用筛选 · <a class="link" data-action="ex-clear-search">清除搜索</a>` : '';
    if (ex.err) {
      $res.innerHTML = `<div class="error-box">加载课程失败: ${esc(ex.err)}</div>`;
      $count.textContent = '';
      return;
    }
    if (!ex.data || (ex.loading && !ex.data.items.length)) {
      $count.textContent = '正在翻阅目录…';
      $res.innerHTML = skeletonShelf();
      return;
    }
    const d = ex.data;
    $count.innerHTML = ex.loading ? '正在翻阅目录…' : `<b>${d.total}</b>门课程`;
    if (!d.items.length) {
      $res.innerHTML = '<div class="empty"><div class="glyph">无</div><div class="title">没有找到相关课程</div><div>换个关键词或调整筛选条件试试</div></div>';
      return;
    }
    renderPickUI();
    $res.innerHTML = `<div class="shelf ${ex.loading ? 'loading' : ''} ${S.pick.on ? 'picking' : ''}">${d.items.map((p) => bookCard(p, ex.q)).join('')}</div>` +
      pager('explore', d.page, Math.max(d.total, d.more ? d.page * d.size + 1 : 0), d.size);
    loadVisibleDates();
  }

  // ---------- batch selection ----------
  function renderPickUI() {
    const $t = document.getElementById('pick-tools');
    if ($t) {
      $t.innerHTML = S.pick.on
        ? `<button class="text-link" data-action="pick-page">全选本页</button><span class="sep"></span>
           <button class="text-link" data-action="pick-undl">选择本页未下载</button><span class="sep"></span>
           <button class="text-link" data-action="pick-clear">清空</button>
           <button class="btn btn-sm" data-action="pick-toggle">完成</button>`
        : `${S.page === 'explore' ? `<button class="btn btn-ghost btn-sm" data-action="batch-unlock">${icons.key}批量解锁</button>` : ''}
           <button class="btn btn-ghost btn-sm" data-action="pick-toggle">${icons.pick}手动选择</button>
           <button class="btn btn-sm" data-action="batch-all">${icons.download}下载全部结果</button>`;
    }
    const $bar = document.getElementById('pick-bar');
    if ($bar) {
      const n = S.pick.sel.size;
      $bar.hidden = !S.pick.on || n === 0;
      $bar.innerHTML = `<span>已选择 <b>${n}</b> 门课程</span>
        <div class="head-actions">
          <button class="btn btn-sm" data-action="pick-clear">清空</button>
          <button class="btn btn-sm btn-primary" data-action="pick-open">${icons.download}批量下载…</button>
        </div>`;
    }
  }

  function refreshPicks() {
    document.querySelectorAll('.shelf').forEach((sh) => sh.classList.toggle('picking', S.pick.on));
    document.querySelectorAll('.book[data-id]').forEach((el) => el.classList.toggle('picked', S.pick.sel.has(el.dataset.id)));
    renderPickUI();
  }

  function setPickMode(on) {
    S.pick.on = on;
    if (!on) S.pick.sel.clear();
    refreshPicks();
  }

  function togglePick(p) {
    if (!pickable(p)) { toast(p.openable ? '尚未购买的课程无法下载' : '该课程无法从这里下载', 'error'); return; }
    const k = String(p.id);
    if (S.pick.sel.has(k)) S.pick.sel.delete(k); else S.pick.sel.set(k, p);
    refreshPicks();
  }

  function pickPage(onlyUndownloaded) {
    document.querySelectorAll('.book[data-id]').forEach((el) => {
      const p = productIndex.get(el.dataset.id);
      if (!p || !pickable(p)) return;
      if (onlyUndownloaded && markInfo(p)) return;
      S.pick.sel.set(String(p.id), p);
    });
    refreshPicks();
  }

  // ---------- unlock (免费领取) ----------
  // returns the unlock result status or throws
  async function unlockRequest(p) {
    const r = await api('POST', '/api/unlock', { productId: Number(p.id) });
    if (r.status === 'claimed' || r.status === 'owned') {
      p.purchased = true;
      p.claimable = false;
      S.products = null; // 我的课程 changed
    } else if (r.status === 'unsupported') {
      p.claimable = false;
    }
    return r;
  }

  function rerenderBook(p) {
    document.querySelectorAll(`.book[data-id="${p.id}"]`).forEach((el) => {
      const tmp = document.createElement('div');
      tmp.innerHTML = bookCard(p, S.page === 'mine' ? S.lib.q : S.ex.q).trim();
      el.replaceWith(tmp.firstElementChild);
    });
  }

  const unlocking = new Set();
  async function unlockOne(p) {
    if (unlocking.has(p.id)) return;
    unlocking.add(p.id);
    try {
      const r = await unlockRequest(p);
      if (r.status === 'claimed') toast(`已领取《${p.title}》, 现在可以下载了`, 'success');
      else if (r.status === 'owned') toast(`《${p.title}》已在你的课程中`);
      else toast(r.message || '领取失败', 'error');
      rerenderBook(p);
      if (S.page === 'course') renderCourse(S.course.type, S.course.id);
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      unlocking.delete(p.id);
    }
  }

  // ---------- batch drawer ----------
  // the drawer collects either all results of the current filters (across every page) or manual picks
  const COLLECT_PAGE_SIZE = 30;
  const COLLECT_MAX_PAGES = 80;

  function filterSummary() {
    if (S.page === 'mine') {
      const l = S.lib;
      const parts = [];
      if (l.q) parts.push(`关键词「${l.q}」`);
      if (l.cat !== '全部') parts.push(l.cat);
      if (l.media !== 'all') parts.push(l.media === 'video' ? '视频' : '图文 / 音频');
      if (l.status !== 'all') parts.push(l.status === 'finish' ? '已完结' : '更新中');
      if (l.dl !== 'all') parts.push({ done: '已下载', part: '部分下载', none: '未下载' }[l.dl]);
      return '我的课程' + (parts.length ? ' · ' + parts.join(' · ') : ' · 全部');
    }
    const ex = S.ex;
    if (ex.q) return `全部课程 · 搜索「${ex.q}」`;
    const m = S.meta || { labels: [], types: [], forms: [], pvips: [] };
    const parts = [];
    const tag = m.labels.find((l) => l.id === ex.tag);
    if (tag) {
      parts.push(tag.name);
      const sub = (tag.children || []).find((l) => l.id === ex.sub);
      if (sub) parts.push(sub.name);
    }
    const pick = (list, v) => { const o = (list || []).find((x) => x.value === v); return o && v ? o.title : ''; };
    [pick(m.types, ex.type), pick(m.forms, ex.form), pick(m.pvips, ex.pvip)].forEach((x) => { if (x) parts.push(x); });
    return '全部课程' + (parts.length ? ' · ' + parts.join(' · ') : ' · 全部方向');
  }

  // whether an item can be checked in the drawer
  const selectableIn = (b, p) => (b.kind === 'unlock' ? p.claimable && !p.purchased : pickable(p));

  function newBatch(source, summary, kind) {
    return { kind: kind || 'download', source, summary, items: [], skipped: { owned: 0, other: 0 }, then: false, mode: 'skip', loading: false, loaded: 0, total: 0, err: '', running: false, stop: false, progress: '', q: '', seq: (S.batch ? S.batch.seq : 0) + 1 };
  }

  function batchAdd(list) {
    const b = S.batch;
    const seen = new Set(b.items.map((it) => String(it.p.id)));
    for (const p of list) {
      if (seen.has(String(p.id))) continue;
      seen.add(String(p.id));
      if (b.kind === 'unlock') {
        // only free open courses are listed when unlocking
        if (!selectableIn(b, p)) { if (p.purchased) b.skipped.owned++; else b.skipped.other++; continue; }
        b.items.push({ p, on: true });
      } else {
        b.items.push({ p, on: pickable(p) && !isFullyDownloaded(p) });
      }
    }
  }

  // open the drawer with every result of the current filters
  async function openDrawerAll(kind) {
    S.batch = newBatch('filter', filterSummary(), kind);
    const b = S.batch;
    renderDrawer(true);
    if (S.page === 'mine') {
      batchAdd(filteredProducts());
      b.total = b.loaded = b.items.length;
      renderDrawer(true);
      return;
    }
    // catalog: page through the server with the same filters
    b.loading = true;
    renderDrawer(true);
    const ex = S.ex;
    for (let page = 1; page <= COLLECT_MAX_PAGES; page++) {
      if (S.batch !== b || b.stop) break;
      const params = new URLSearchParams({ page, size: COLLECT_PAGE_SIZE });
      if (ex.q) params.set('q', ex.q);
      else {
        const tag = ex.sub || ex.tag;
        if (tag) params.set('tag', tag);
        if (ex.type) params.set('type', ex.type);
        if (ex.form) params.set('form', ex.form);
        if (ex.pvip) params.set('pvip', ex.pvip);
        params.set('sort', ex.sort);
      }
      try {
        const d = await api('GET', '/api/catalog?' + params);
        if (S.batch !== b) return;
        const items = d.items || [];
        indexProducts(items);
        batchAdd(items);
        b.scanned = (b.scanned || 0) + items.length;
        b.total = Math.max(d.total || 0, b.scanned);
        b.loaded = b.scanned;
        renderDrawer(true);
        if (!d.more || !items.length) break;
      } catch (e) {
        if (S.batch !== b) return;
        b.err = e.message;
        break;
      }
    }
    if (S.batch !== b) return;
    b.loading = false;
    b.stop = false;
    renderDrawer(true);
  }

  function openDrawerFromPicks() {
    const list = Array.from(S.pick.sel.values());
    if (!list.length) return;
    S.batch = newBatch('pick', `手动选择的 ${list.length} 门课程`);
    batchAdd(list);
    S.batch.total = S.batch.loaded = S.batch.items.length;
    renderDrawer(true);
  }

  function setBatchMode(mode) {
    const b = S.batch;
    b.mode = mode;
    for (const it of b.items) {
      if (pickable(it.p) && isFullyDownloaded(it.p)) it.on = mode === 'redo';
    }
    renderDrawer(true);
  }

  function batchSelect(kind) {
    for (const it of S.batch.items) {
      if (!selectableIn(S.batch, it.p)) continue;
      if (kind === 'none') it.on = false;
      else if (kind === 'undl') it.on = !markInfo(it.p);
      else it.on = S.batch.mode === 'redo' || !isFullyDownloaded(it.p);
    }
    renderDrawer(true);
  }

  function closeDrawer() {
    const b = S.batch;
    if (b && b.running) return;
    S.batch = null;
    const d = document.getElementById('drawer');
    if (d) {
      d.classList.add('closing');
      setTimeout(() => d.remove(), 180);
    }
  }

  function batchRowHtml(it, i, running) {
    const p = it.p;
    const mk = markInfo(p);
    let state;
    if (S.batch.kind === 'unlock') state = p.purchased ? '<span class="ok">已领取</span>' : '免费领取';
    else if (!p.openable) state = '<span class="no">暂不支持从这里下载</span>';
    else if (!p.purchased) state = '<span class="no">未购买</span>';
    else if (!mk) state = '未下载';
    else if (mk.cls === 'full') state = '<span class="ok">已下载</span>';
    else if (mk.cls === 'part') state = `<span class="ok">已下载 ${esc(mk.text)}</span> · 补齐缺失篇目`;
    else state = '本地已有文件 · 已存在的文件会跳过';
    const ok = selectableIn(S.batch, p);
    return `<label class="pick-row ${it.on ? '' : 'off'} ${ok ? '' : 'disabled'}" data-row="${i}" data-title="${esc((p.title + ' ' + (p.author || '')).toLowerCase())}">
      <input type="checkbox" data-bi="${i}" ${it.on ? 'checked' : ''} ${ok && !running ? '' : 'disabled'}>
      <span class="thumb">${coverHtml(p, p.title)}</span>
      <span class="info"><div class="t">${esc(p.title)}</div><div class="m">${esc(category(p))}${p.articleCount ? ' · ' + p.articleCount + ' 讲' : ''} · ${state}</div></span>
    </label>`;
  }

  // full=true rebuilds the list, otherwise only counters/footer are refreshed
  function renderDrawer(full) {
    const b = S.batch;
    if (!b) return;
    let d = document.getElementById('drawer');
    if (!d) {
      d = document.createElement('div');
      d.id = 'drawer';
      d.className = 'drawer-wrap';
      d.innerHTML = `<div class="drawer-scrim" data-action="batch-close"></div>
        <aside class="drawer" role="dialog" aria-modal="true" aria-labelledby="drawer-title">
          <div class="drawer-head"></div>
          <div class="drawer-tools"></div>
          <div class="drawer-body"></div>
          <div class="drawer-foot"></div>
        </aside>`;
      document.body.appendChild(d);
      d.querySelector('.drawer-tools').addEventListener('input', (e) => {
        if (e.target.id !== 'batch-q') return;
        b.q = e.target.value.trim().toLowerCase();
        filterDrawerRows();
      });
      full = true;
    }
    const items = b.items;
    const unlock = b.kind === 'unlock';
    const selectable = items.filter((it) => selectableIn(b, it.p));
    const full_ = selectable.filter((it) => isFullyDownloaded(it.p)).length;
    const unbought = items.length - selectable.length;
    const chosen = items.filter((it) => it.on && selectableIn(b, it.p)).length;
    const pct = b.total ? Math.min(100, (b.loaded / b.total) * 100) : 0;

    d.querySelector('.drawer-head').innerHTML = `
      <div class="kicker">${unlock ? 'Batch unlock' : 'Batch download'}</div>
      <div class="drawer-title-row"><h2 id="drawer-title">${unlock ? '批量解锁' : '批量下载'}</h2>
        <button class="icon-btn" data-action="batch-close" aria-label="关闭" ${b.running ? 'disabled' : ''}><svg viewBox="0 0 24 24"><path d="M6 6l12 12M18 6 6 18"/></svg></button></div>
      <div class="drawer-summary">${esc(b.summary)}</div>
      ${b.loading ? `<div class="collect"><span>正在汇总${b.source === 'filter' ? '全部筛选结果' : ''} <b>${b.loaded}</b> / ${b.total || '…'}</span><button class="text-link" data-action="batch-stop-collect">停止汇总</button></div><div class="progress"><i style="width:${pct}%"></i></div>` : ''}
      ${b.err ? `<div class="error-box" style="margin:12px 0 0">汇总中断: ${esc(b.err)} — 已获取的 ${b.items.length} 门仍可下载</div>` : ''}
      ${!unlock && !b.loading && !b.err && b.source === 'filter' && b.total > b.items.length ? `<div class="collect"><span>已汇总全部可列出的 <b>${b.items.length}</b> 门课程; 官网统计数为 ${b.total}, 差额为已下架或不公开展示的课程。</span></div>` : ''}
      ${unlock ? `<div class="drawer-stats">
        <div><b>${b.loaded}</b><span>已汇总</span></div>
        <div><b>${selectable.length}</b><span>可免费领取</span></div>
        <div><b>${b.skipped.owned + items.length - selectable.length}</b><span>已拥有</span></div>
        <div><b>${b.skipped.other}</b><span>不可领取</span></div>
      </div>` : `<div class="drawer-stats">
        <div><b>${items.length}</b><span>课程</span></div>
        <div><b>${selectable.length}</b><span>可下载</span></div>
        <div><b>${full_}</b><span>已下载</span></div>
        <div><b>${unbought}</b><span>不可下载</span></div>
      </div>`}`;

    if (full) {
      d.querySelector('.drawer-tools').innerHTML = `
        <div class="search-sm">${icons.search}<input class="input" id="batch-q" placeholder="在结果中查找" value="${esc(b.q)}"></div>
        <button class="text-link" data-action="batch-sel" data-v="all" ${b.running ? 'disabled' : ''}>全选</button>
        ${unlock ? '' : `<button class="text-link" data-action="batch-sel" data-v="undl" ${b.running ? 'disabled' : ''}>仅未下载</button>`}
        <button class="text-link" data-action="batch-sel" data-v="none" ${b.running ? 'disabled' : ''}>全不选</button>`;
      const body = d.querySelector('.drawer-body');
      const top = body.scrollTop;
      body.innerHTML = items.length
        ? items.map((it, i) => batchRowHtml(it, i, b.running)).join('')
        : (b.loading ? '<div class="empty" style="border:0">正在汇总…</div>' : `<div class="empty" style="border:0"><div class="title">${unlock ? '当前条件下没有可免费领取的课程' : '没有课程'}</div></div>`);
      body.scrollTop = top;
      filterDrawerRows();
    }

    d.querySelector('.drawer-foot').innerHTML = unlock ? `
      <label class="check"><input type="checkbox" id="batch-then" ${b.then ? 'checked' : ''} ${b.running ? 'disabled' : ''}>领取成功后加入下载队列</label>
      <div class="modal-note">等同于在官网课程页点击「免费领取」。只会领取官网标记为免费领取的公开课, 领取后课程会出现在「我的课程」中。</div>
      <div class="row">
        <span class="modal-note">${b.progress ? esc(b.progress) : `已选择 <b class="num">${chosen}</b> 门课程`}</span>
        <div class="head-actions">
          ${b.running ? '<button class="btn btn-ghost" data-action="batch-stop">停止</button>' : '<button class="btn btn-ghost" data-action="batch-close">取消</button>'}
          <button class="btn btn-primary" data-action="batch-start" ${b.running || b.loading || !chosen ? 'disabled' : ''}>${icons.key}${b.running ? '正在领取…' : (b.loading ? '正在汇总…' : `解锁 ${chosen} 门课程`)}</button>
        </div>
      </div>` : `
      <div class="radio-group">
        <span class="lbl">已下载的课程</span>
        <label class="radio"><input type="radio" name="batch-mode" value="skip" ${b.mode === 'skip' ? 'checked' : ''} ${b.running ? 'disabled' : ''}>跳过</label>
        <label class="radio"><input type="radio" name="batch-mode" value="redo" ${b.mode === 'redo' ? 'checked' : ''} ${b.running ? 'disabled' : ''}>重新下载 (覆盖已有文件)</label>
      </div>
      <div class="modal-note">${b.mode === 'skip' ? '跳过: 已下载的课程不加入队列; 部分下载的课程只补齐缺失的篇目。' : '重新下载: 所选课程的全部篇目都会重新下载并覆盖已有文件。'}
        ${chosen > 30 ? ' 课程较多时添加需要一些时间, 请留意限流。' : ''}</div>
      <div class="row">
        <span class="modal-note">${b.progress ? esc(b.progress) : `已选择 <b class="num">${chosen}</b> 门课程`}</span>
        <div class="head-actions">
          ${b.running
            ? '<button class="btn btn-ghost" data-action="batch-stop">停止添加</button>'
            : '<button class="btn btn-ghost" data-action="batch-close">取消</button>'}
          <button class="btn btn-primary" data-action="batch-start" ${b.running || b.loading || !chosen ? 'disabled' : ''}>${icons.download}${b.running ? '正在添加…' : (b.loading ? '正在汇总…' : `下载 ${chosen} 门课程`)}</button>
        </div>
      </div>`;
  }

  function filterDrawerRows() {
    const b = S.batch;
    if (!b) return;
    document.querySelectorAll('#drawer .pick-row').forEach((row) => {
      row.hidden = !!b.q && row.dataset.title.indexOf(b.q) < 0;
    });
  }

  async function startUnlockBatch() {
    const b = S.batch;
    const items = b.items.filter((it) => it.on && selectableIn(b, it.p));
    b.running = true;
    b.stop = false;
    renderDrawer(true);
    let claimed = 0;
    let queued = 0;
    const failed = [];
    let aborted = '';
    for (let i = 0; i < items.length; i++) {
      if (b.stop) { aborted = '已停止'; break; }
      const p = items[i].p;
      b.progress = `正在领取 ${i + 1} / ${items.length} · ${p.title}`;
      renderDrawer(false);
      try {
        const r = await unlockRequest(p);
        if (r.status === 'claimed' || r.status === 'owned') {
          if (r.status === 'claimed') claimed++;
          if (b.then) {
            try {
              const t = await api('POST', '/api/tasks', { type: p.typeKey, courseId: Number(p.courseId), articleIds: [] });
              queued += t.added || 0;
            } catch (e) { failed.push(`${p.title}: 加入下载失败 ${e.message}`); }
          }
        } else {
          failed.push(`${p.title}: ${r.message}`);
        }
      } catch (e) {
        failed.push(`${p.title}: ${e.message}`);
        if (e.status === 429 || e.status === 401) { aborted = e.message; break; }
      }
      if (i < items.length - 1) await new Promise((r) => setTimeout(r, 400));
    }
    b.running = false;
    b.progress = '';
    pollTasks();
    toast(`已领取 ${claimed} 门课程` + (b.then ? `, 加入 ${queued} 个下载任务` : '') + (failed.length ? `; ${failed.length} 门失败` : '') + (aborted ? ` (${aborted})` : ''), failed.length || aborted ? 'error' : 'success');
    failed.slice(0, 3).forEach((f) => toast(f, 'error'));
    closeDrawer();
    if (S.page === 'explore') renderExploreResults();
  }

  async function startBatch() {
    const b = S.batch;
    if (!b || b.running) return;
    if (b.kind === 'unlock') { startUnlockBatch(); return; }
    const items = b.items.filter((it) => it.on && pickable(it.p));
    b.running = true;
    b.stop = false;
    renderDrawer(true);
    let tasks = 0;
    let ok = 0;
    const failed = [];
    let aborted = '';
    for (let i = 0; i < items.length; i++) {
      if (b.stop) { aborted = '已停止'; break; }
      const p = items[i].p;
      b.progress = `正在添加 ${i + 1} / ${items.length} · ${p.title}`;
      renderDrawer(false);
      try {
        const overwrite = b.mode === 'redo' && !!markInfo(p);
        const r = await api('POST', '/api/tasks', { type: p.typeKey, courseId: Number(p.courseId), articleIds: [], overwrite });
        tasks += r.added || 0;
        ok++;
      } catch (e) {
        failed.push(`${p.title}: ${e.message}`);
        if (e.status === 429 || e.status === 401) { aborted = e.message; break; }
      }
      if (i < items.length - 1) await new Promise((r) => setTimeout(r, 250));
    }
    b.running = false;
    b.progress = '';
    pollTasks();
    const msg = `已加入 ${ok} 门课程, 共 ${tasks} 个任务` + (failed.length ? `; ${failed.length} 门失败` : '') + (aborted ? ` (${aborted})` : '');
    toast(msg, failed.length || aborted ? 'error' : 'success');
    failed.slice(0, 3).forEach((f) => toast(f, 'error'));
    if (b.source === 'pick') setPickMode(false);
    closeDrawer();
  }

  // ---------- 我的课程 ----------
  async function loadProducts(refresh) {
    S.productsLoading = true;
    S.productsErr = '';
    if (S.page === 'mine') renderLibraryResults();
    try {
      const r = await api('GET', '/api/products' + (refresh ? '?refresh=1' : ''));
      S.products = r.products || [];
      indexProducts(S.products);
    } catch (e) {
      S.productsErr = e.message;
    } finally {
      S.productsLoading = false;
      if (S.page === 'mine') renderLibraryResults();
    }
  }

  function renderLibrary() {
    $view.innerHTML = `
      <div class="page">
        <div class="page-head">
          <div>
            <div class="kicker">Library</div>
            <h1>我的课程</h1>
            <div class="sub">你已购买的专栏、视频课与训练营。</div>
          </div>
          <div class="head-actions">
            <button class="btn btn-ghost btn-sm" data-action="refresh-products">${icons.refresh}刷新</button>
          </div>
        </div>
        <div class="searchbar">${icons.search}<input id="lib-q" placeholder="在我的课程中查找…" value="${esc(S.lib.q)}" aria-label="搜索我的课程"></div>
        <div id="lib-filters"></div>
        <div class="result-bar" id="results-top"><span class="count" id="lib-count"></span><span class="pick-tools" id="pick-tools"></span></div>
        <div id="lib-results"></div>
        <div class="selbar" id="pick-bar" hidden></div>
      </div>`;
    const q = document.getElementById('lib-q');
    q.addEventListener('input', debounce(() => { S.lib.q = q.value.trim(); S.lib.page = 1; renderLibraryResults(); }, 120));
    if (S.products === null && !S.productsLoading) loadProducts(false);
    renderLibraryResults();
  }

  function renderLibraryFilters() {
    const $f = document.getElementById('lib-filters');
    if (!$f) return;
    const counts = { '全部': (S.products || []).length };
    for (const p of S.products || []) counts[category(p)] = (counts[category(p)] || 0) + 1;
    if (S.lib.cat !== '全部' && !counts[S.lib.cat] && S.products) S.lib.cat = '全部';
    const cats = ['全部'].concat(CATEGORY_ORDER.filter((c) => counts[c])).map((c) => ({ title: c, value: c, count: counts[c] || 0 }));
    const typeOpts = S.types.map((t) => `<option value="${esc(t.key)}" ${t.key === S.quick.type ? 'selected' : ''}>${esc(t.name)}</option>`).join('');
    const focused = document.activeElement && document.activeElement.id === 'quick-id';
    if (focused) return; // don't steal focus while typing an id
    $f.innerHTML = `<div class="facets">
      ${facet('分类', cats, S.lib.cat, 'lib-cat')}
      ${facet('形式', [{ title: '全部', value: 'all' }, { title: '图文 / 音频', value: 'text' }, { title: '视频', value: 'video' }], S.lib.media, 'lib-media')}
      ${facet('状态', [{ title: '全部', value: 'all' }, { title: '已完结', value: 'finish' }, { title: '更新中', value: 'updating' }], S.lib.status, 'lib-status')}
      ${facet('下载', [{ title: '全部', value: 'all' }, { title: '已下载', value: 'done' }, { title: '部分下载', value: 'part' }, { title: '未下载', value: 'none' }], S.lib.dl, 'lib-dl')}
      ${facet('排序', [{ title: '最近学习', value: 'recent' }, { title: '最新上线', value: 'launch' }, { title: '最早上线', value: 'launch-asc' }, { title: '名称', value: 'title' }, { title: '课时最多', value: 'count' }], S.lib.sort, 'lib-sort', {
        end: '<button class="text-link facet-end" data-action="reset-filters">重置筛选</button>',
      })}
      <div class="quick-open">
        <span class="facet-label">按 ID</span>
        <select class="select" id="quick-type" aria-label="课程类型">${typeOpts}</select>
        <input class="input" id="quick-id" inputmode="numeric" placeholder="课程 ID" value="${esc(S.quick.id)}" aria-label="课程 ID">
        <button class="btn btn-sm" data-action="quick-open">打开</button>
        <span class="hint">ID 见课程网址; 训练营填班级 ID; 每日一课 / 大厂案例直接下载</span>
      </div>
    </div>`;
    document.getElementById('quick-id').addEventListener('keydown', (e) => { if (e.key === 'Enter') quickOpen(); });
  }

  function filteredProducts() {
    const { q, cat, media, status, sort } = S.lib;
    const ql = q.toLowerCase();
    let list = (S.products || []).filter((p) => {
      if (cat !== '全部' && category(p) !== cat) return false;
      if (media === 'video' && !p.isVideo) return false;
      if (media === 'text' && p.isVideo) return false;
      if (status === 'finish' && !p.isFinish) return false;
      if (status === 'updating' && p.isFinish) return false;
      if (S.lib.dl !== 'all') {
        const m = markInfo(p);
        const st = !m ? 'none' : (m.cls === 'full' ? 'done' : 'part');
        if (st !== S.lib.dl) return false;
      }
      if (ql) {
        const hay = (p.title + '\n' + p.subtitle + '\n' + p.author + '\n' + p.id).toLowerCase();
        if (hay.indexOf(ql) < 0) return false;
      }
      return true;
    });
    if (sort === 'launch' || sort === 'launch-asc') {
      const t = (p) => p.beginTime || (S.dates.get(p.courseId) || {}).first || 0;
      const dir = sort === 'launch' ? -1 : 1;
      // unknown launch times last
      list = list.slice().sort((a, b) => (!t(a) - !t(b)) || dir * (t(a) - t(b)) || a.order - b.order);
    } else if (sort === 'title') list = list.slice().sort((a, b) => a.title.localeCompare(b.title, 'zh-CN'));
    else if (sort === 'count') list = list.slice().sort((a, b) => b.articleCount - a.articleCount);
    else list = list.slice().sort((a, b) => a.order - b.order);
    return list;
  }

  function renderLibraryResults() {
    const $res = document.getElementById('lib-results');
    const $count = document.getElementById('lib-count');
    if (!$res) return;
    renderLibraryFilters();

    const errHtml = S.productsErr ? `<div class="error-box">加载课程列表失败: ${esc(S.productsErr)} — 仍可通过上方「按 ID」打开课程。</div>` : '';
    if (S.productsLoading && !S.products) {
      $count.textContent = '正在整理书架…';
      $res.innerHTML = skeletonShelf();
      return;
    }
    if (!S.products) {
      $count.textContent = '';
      $res.innerHTML = errHtml;
      return;
    }

    const list = filteredProducts();
    $count.innerHTML = `<b>${list.length}</b>门课程` + (list.length !== S.products.length ? ` <span style="color:var(--muted);font-size:13px">/ 共 ${S.products.length}</span>` : '');
    if (!list.length) {
      $res.innerHTML = errHtml + `<div class="empty"><div class="glyph">空</div><div class="title">${S.products.length ? '没有符合条件的课程' : '书架还是空的'}</div><div>${S.products.length ? '调整关键词或筛选条件试试' : '去「全部课程」看看'}</div>${S.products.length ? '<button class="btn btn-sm" data-action="reset-filters">清除筛选</button>' : ''}</div>`;
      return;
    }
    const size = PAGE_SIZE.mine;
    S.lib.page = clampPage(S.lib.page, list.length, size);
    const pageItems = list.slice((S.lib.page - 1) * size, S.lib.page * size);
    renderPickUI();
    $res.innerHTML = errHtml + `<div class="shelf ${S.pick.on ? 'picking' : ''}">` + pageItems.map((p) => bookCard(p, S.lib.q)).join('') + '</div>' +
      pager('mine', S.lib.page, list.length, size);
    loadVisibleDates();
  }

  function quickOpen() {
    const type = document.getElementById('quick-type').value;
    const id = document.getElementById('quick-id').value.trim();
    if (!/^\d+$/.test(id)) { toast('课程 ID 格式不合法', 'error'); return; }
    S.quick = { type, id };
    store.set('quick', S.quick);
    const t = typeInfo(type);
    if (t && !t.needSelectArticle) enqueue(type, id);
    else location.hash = `#/course/${type}/${id}`;
  }

  // ---------- course ----------
  function renderCourse(type, id) {
    const key = type + ':' + id;
    if (!S.course || S.course.key !== key) {
      S.course = { key, type, id, data: null, err: '', loading: true, sel: new Set(), q: '', page: 1, from: lastListPage, tab: 'toc', intro: null, introErr: '' };
      loadCourse(false);
      loadIntro(S.course);
    }
    const c = S.course;
    const backText = c.from === 'mine' ? '我的课程' : '全部课程';
    const product = courseIndex.get(key);
    const intro = c.intro || {};
    const coverSrc = product && product.cover ? product : (intro.cover ? { cover: intro.cover } : null);
    const author = (product && product.author) || intro.authorName;
    const title = c.data ? c.data.title : (product ? product.title : '');
    const uni = type === 'n4' || type === 'e0';

    let colophon;
    if (c.err) {
      colophon = `<div class="book-cover">${plateHtml('?')}</div><div><h1>无法打开</h1></div>`;
    } else {
      const d = c.data;
      const specs = d ? [
        ['类型', esc(d.typeName) + (uni ? '' : (d.isVideo ? ' · 视频' : ' · 图文'))],
        ['篇目', `<span class="num">${d.articles.length}</span>`],
        ['编号', `<span class="num">${d.id}</span>`],
        author ? ['讲师', esc(author)] : null,
      ].filter(Boolean) : [];
      colophon = `
        <div class="book-cover">${title ? coverHtml(coverSrc, title) : '<div class="skeleton" style="width:100%;height:100%"></div>'}</div>
        <div>
          <h1>${title ? esc(title) : '<span class="skeleton" style="display:inline-block;width:80%;height:28px"></span>'}</h1>
          ${intro.subtitle ? `<div class="subtitle">${esc(intro.subtitle)}</div>` : ''}
          ${d ? courseStateHtml(d) : ''}
          ${d ? `<dl class="spec">${specs.map(([k, v]) => `<dt>${k}</dt><dd>${v}</dd>`).join('')}</dl>` : ''}
          ${product && product.claimable && !product.purchased ? `<button class="btn btn-primary" data-action="course-unlock">${icons.key}免费领取 · 解锁课程</button>` : ''}
          <button class="btn ${product && product.claimable && !product.purchased ? 'btn-ghost' : 'btn-primary'}" data-action="course-download-all" ${d ? '' : 'disabled'}>${icons.download}下载全部</button>
          <button class="btn btn-ghost" data-action="course-refresh">${icons.refresh}刷新目录</button>
          ${uni ? '<div class="note">训练营: 视频课保存为视频, 图文保存为 Markdown。</div>' : ''}
        </div>`;
    }

    $view.innerHTML = `
      <div class="page">
        <a class="back" data-action="back">${icons.back}${backText}</a>
        <div class="course">
          <aside class="colophon">${colophon}</aside>
          <section id="course-body"></section>
        </div>
      </div>`;
    renderCourseBody();
  }

  async function loadCourse(refresh) {
    const c = S.course;
    c.loading = true;
    c.err = '';
    try {
      const data = await api('GET', `/api/course?type=${encodeURIComponent(c.type)}&id=${encodeURIComponent(c.id)}${refresh ? '&refresh=1' : ''}`);
      if (S.course !== c) return;
      c.data = data;
    } catch (e) {
      if (S.course !== c) return;
      c.err = e.message;
    } finally {
      c.loading = false;
      if (S.course === c && S.page === 'course') renderCourse(c.type, c.id);
    }
  }

  async function loadIntro(c) {
    try {
      const data = await api('GET', `/api/course/intro?type=${encodeURIComponent(c.type)}&id=${encodeURIComponent(c.id)}`);
      if (S.course !== c) return;
      c.intro = data;
    } catch (e) {
      if (S.course !== c) return;
      c.introErr = e.message;
      c.intro = {};
    }
    if (S.course === c && S.page === 'course') renderCourse(c.type, c.id);
  }

  // downloaded state line in the colophon
  function courseStateHtml(d) {
    const r = S.marks[String(d.id)] || {};
    const done = Math.max((d.downloaded || []).length, r.done || 0);
    const marked = r.marked || d.marked;
    let text;
    if (marked) text = `${icons.check}已标记为已下载`;
    else if (done && done >= d.articles.length) text = `${icons.check}已全部下载`;
    else if (done) text = `已下载 ${done} / ${d.articles.length} 篇`;
    else if (d.local) text = '下载目录中已有本课程的文件';
    else text = '<span style="color:var(--muted)">尚未下载</span>';
    return `<div class="dl-state">${text.replace('<svg', '<svg style="width:13px;height:13px;fill:none;stroke:currentColor;stroke-width:2.4"')}
      <button class="text-link" data-action="mark-toggle">${marked ? '取消标记' : '标记为已下载'}</button></div>`;
  }

  async function toggleCourseMark() {
    const c = S.course;
    if (!c || !c.data) return;
    const r = S.marks[String(c.data.id)] || {};
    const marked = !(r.marked || c.data.marked);
    try {
      const res = await api('POST', '/api/marks', { courseId: c.data.id, title: c.data.title, marked });
      S.marks = res.marks || {};
      c.data.marked = marked;
      renderCourse(c.type, c.id);
      toast(marked ? '已标记为已下载' : '已取消标记', 'success');
    } catch (e) { toast(e.message, 'error'); }
  }

  function introHtml(c) {
    const i = c.intro;
    if (!i) return '<div class="prose">' + Array.from({ length: 6 }).map(() => '<div class="skeleton" style="height:14px;margin:14px 0"></div>').join('') + '</div>';
    if (c.introErr) return `<div class="error-box" style="margin-top:20px">加载课程介绍失败: ${esc(c.introErr)}</div>`;
    let html = '';
    if (i.introHtml) html += `<div class="prose">${i.introHtml}</div>`;
    if (i.authorHtml || (!i.introHtml && i.authorName)) {
      html += `<div class="author-card"><div class="kicker">Author</div><span class="name">${esc(i.authorName || '')}</span><span class="role">${esc(i.authorIntro || '')}</span>
        ${i.authorHtml ? `<div class="prose" style="padding-bottom:0">${i.authorHtml}</div>` : ''}</div>`;
    }
    if (!html) html = '<div class="empty" style="border-top:0"><div class="glyph">简</div><div class="title">暂无课程介绍</div><div>该课程在极客时间没有提供介绍内容</div></div>';
    return html;
  }

  function courseArticles() {
    const c = S.course;
    if (!c || !c.data) return [];
    const ql = c.q.toLowerCase();
    return c.data.articles.map((a, i) => Object.assign({ idx: i + 1 }, a))
      .filter((a) => !ql || a.title.toLowerCase().indexOf(ql) >= 0 || (a.section || '').toLowerCase().indexOf(ql) >= 0);
  }

  function renderCourseBody() {
    const c = S.course;
    const $body = document.getElementById('course-body');
    if (!$body) return;
    if (c.err) {
      $body.innerHTML = `<div class="empty"><div class="glyph">阙</div><div class="title">${esc(c.err)}</div><button class="btn btn-sm" data-action="back">返回</button></div>`;
      return;
    }
    if (!c.data) {
      $body.innerHTML = '<div class="toc-head"><div class="toc-tabs"><span class="toc-tab active">目录</span></div></div>' + Array.from({ length: 10 }).map(() => '<div class="entry"><div class="skeleton" style="height:14px;flex:1"></div></div>').join('');
      return;
    }
    // chapter numbering over the whole course
    c.chapters = new Map();
    for (const a of c.data.articles) if (a.section && !c.chapters.has(a.section)) c.chapters.set(a.section, c.chapters.size + 1);

    const tabs = `<div class="toc-tabs">
        <button class="toc-tab ${c.tab === 'toc' ? 'active' : ''}" data-action="course-tab" data-tab="toc">目录<sup>${c.data.articles.length}</sup></button>
        <button class="toc-tab ${c.tab === 'intro' ? 'active' : ''}" data-action="course-tab" data-tab="intro">课程介绍</button>
      </div>`;
    if (c.tab === 'intro') {
      $body.innerHTML = `<div class="toc-head" id="results-top">${tabs}</div>${introHtml(c)}`;
      return;
    }
    $body.innerHTML = `
      <div class="toc-head" id="results-top">
        ${tabs}
        <label class="check"><input type="checkbox" id="sel-all"><span id="sel-all-text">全选</span></label>
        <div class="search-sm">${icons.search}<input class="input" id="course-q" placeholder="查找篇目或章节" value="${esc(c.q)}"></div>
      </div>
      <div id="article-list"></div>
      <div id="article-pager"></div>
      <div class="selbar" id="sel-bar" hidden>
        <span id="sel-bar-text"></span>
        <div class="head-actions">
          <button class="btn btn-sm" data-action="sel-clear">取消选择</button>
          <button class="btn btn-sm btn-primary" data-action="course-download-selected">${icons.download}下载所选</button>
        </div>
      </div>`;
    const q = document.getElementById('course-q');
    q.addEventListener('input', debounce(() => { c.q = q.value.trim(); c.page = 1; renderArticleList(); }, 100));
    document.getElementById('sel-all').addEventListener('change', (e) => {
      for (const a of courseArticles()) { if (e.target.checked) c.sel.add(a.id); else c.sel.delete(a.id); }
      renderArticleList();
    });
    renderArticleList();
  }

  function statusHtml(t) {
    if (!t) return '';
    let txt = STATUS_TEXT[t.status] || t.status;
    if (t.status === 'running' && t.total > 0) txt += ' ' + Math.floor((t.done / t.total) * 100) + '%';
    return `<span class="st ${t.status}">${esc(txt)}</span>`;
  }

  function renderArticleList() {
    const c = S.course;
    const $list = document.getElementById('article-list');
    if (!$list || !c || !c.data) return;
    const all = courseArticles();
    const size = PAGE_SIZE.course;
    c.page = clampPage(c.page, all.length, size);
    const list = all.slice((c.page - 1) * size, c.page * size);
    const idx = taskIndex();
    c.done = new Set(c.data.downloaded || []);
    if (!list.length) {
      $list.innerHTML = '<div class="empty" style="border-top:0"><div class="title">没有匹配的篇目</div></div>';
    } else {
      let html = '';
      let section = null;
      for (const a of list) {
        if (a.section && a.section !== section) {
          section = a.section;
          html += `<div class="chapter"><span class="c-no">${pad2(c.chapters.get(section) || 0)}</span><span>${highlight(section, c.q)}</span></div>`;
        }
        const t = idx.get(c.type + ':' + c.data.id + ':' + a.id);
        html += `
          <div class="entry">
            <label class="check"><input type="checkbox" data-sel="${a.id}" ${c.sel.has(a.id) ? 'checked' : ''} aria-label="选择 ${esc(a.title)}"></label>
            <span class="e-no">${pad2(a.idx)}</span>
            <span class="e-title" title="${esc(a.title)}">${highlight(a.title, c.q)}</span>
            <span class="e-lead"></span>
            <span class="e-act">
              <span data-st="${a.id}">${statusHtml(t) || (c.done.has(a.id) ? '<span class="st done">已下载</span>' : '')}</span>
              <button class="ibtn accent" title="下载此篇" data-action="course-download-one" data-aid="${a.id}">${icons.download}</button>
            </span>
          </div>`;
      }
      $list.innerHTML = html;
    }
    document.getElementById('article-pager').innerHTML = pager('course', c.page, all.length, size, 'tight');
    updateSelection();
  }

  function updateSelection() {
    const c = S.course;
    const list = courseArticles();
    const all = document.getElementById('sel-all');
    if (!all) return;
    const selVisible = list.filter((a) => c.sel.has(a.id)).length;
    all.checked = list.length > 0 && selVisible === list.length;
    all.indeterminate = selVisible > 0 && selVisible < list.length;
    document.getElementById('sel-all-text').textContent = `全选${c.q ? '匹配' : ''} ${list.length}`;
    const bar = document.getElementById('sel-bar');
    bar.hidden = c.sel.size === 0;
    document.getElementById('sel-bar-text').innerHTML = `已选择 <b>${c.sel.size}</b> 篇`;
  }

  function patchCourseStatuses() {
    const c = S.course;
    if (!c || !c.data) return;
    const idx = taskIndex();
    document.querySelectorAll('[data-st]').forEach((el) => {
      const html = statusHtml(idx.get(c.type + ':' + c.data.id + ':' + el.dataset.st)) || (c.done && c.done.has(Number(el.dataset.st)) ? '<span class="st done">已下载</span>' : '');
      if (el.innerHTML !== html) el.innerHTML = html;
    });
  }

  // ---------- downloads ----------
  const TABS = [
    ['all', '全部', () => true],
    ['active', '进行中', (t) => t.status === 'running'],
    ['queued', '等待中', (t) => t.status === 'queued'],
    ['done', '已完成', (t) => t.status === 'done' || t.status === 'skipped'],
    ['failed', '失败', (t) => t.status === 'failed'],
    ['canceled', '已取消', (t) => t.status === 'canceled'],
  ];

  function renderDownloads() {
    $view.innerHTML = `
      <div class="page">
        <div class="page-head">
          <div>
            <div class="kicker">Downloads</div>
            <h1>下载管理</h1>
            <div class="sub" id="dl-sub"></div>
          </div>
          <div class="head-actions">
            <button class="btn btn-ghost btn-sm" data-action="open-root">${icons.folder}打开下载目录</button>
          </div>
        </div>
        <div class="ledger-stats" id="dl-stats"></div>
        <div class="ledger-tools" id="results-top">
          <div class="tabs" id="dl-tabs"></div>
          <div class="search-sm">${icons.search}<input class="input" id="dl-q" placeholder="查找任务" value="${esc(S.dl.q)}"></div>
        </div>
        <div class="ledger-actions">
          <button class="btn btn-ghost btn-sm" data-action="dl-retry-failed">${icons.retry}重试失败</button>
          <button class="btn btn-ghost btn-sm btn-danger" data-action="dl-cancel-all">${icons.stop}全部取消</button>
          <span class="spacer"></span>
          <button class="btn btn-ghost btn-sm" data-action="dl-clear">${icons.trash}清除已完成</button>
        </div>
        <div id="dl-list"></div>
        <div id="dl-pager"></div>
      </div>`;
    const q = document.getElementById('dl-q');
    q.addEventListener('input', debounce(() => { S.dl.q = q.value.trim(); S.dl.page = 1; renderTaskList(true); }, 100));
    renderTaskList(true);
  }

  function renderTaskList(full) {
    const $list = document.getElementById('dl-list');
    if (!$list) return;
    const tasks = S.tasks;
    const n = (f) => tasks.filter(f).length;
    const st = { running: n((t) => t.status === 'running'), queued: n((t) => t.status === 'queued'), done: n((t) => t.status === 'done' || t.status === 'skipped'), failed: n((t) => t.status === 'failed') };
    const speed = tasks.reduce((s, t) => s + (t.status === 'running' ? t.speed || 0 : 0), 0);

    document.getElementById('dl-stats').innerHTML = [
      ['下载中', st.running, 'var(--info)'], ['等待中', st.queued, 'var(--muted)'], ['已完成', st.done, 'var(--success)'], ['失败', st.failed, 'var(--danger)'],
    ].map(([k, v, c]) => `<div><div class="v">${v}</div><div class="k"><i style="background:${c}"></i>${k}</div></div>`).join('');
    const s = S.settings || {};
    document.getElementById('dl-sub').textContent =
      `同时 ${s.concurrentTasks || 1} 个任务 · 每个任务 ${s.threads || 1} 线程` + (speed > 0 ? ` · 当前 ${fmtBytes(speed)}/s` : '');

    document.getElementById('dl-tabs').innerHTML = TABS.map(([k, label, f]) =>
      `<button class="tab ${S.dl.tab === k ? 'active' : ''}" data-action="dl-tab" data-tab="${k}">${label}<span class="n">${n(f)}</span></button>`).join('');

    const filter = (TABS.find((t) => t[0] === S.dl.tab) || TABS[0])[2];
    const ql = S.dl.q.toLowerCase();
    const all = tasks.filter(filter).filter((t) => !ql || (t.title + '\n' + t.courseTitle).toLowerCase().indexOf(ql) >= 0);
    const size = PAGE_SIZE.dl;
    const prevPage = S.dl.page;
    S.dl.page = clampPage(S.dl.page, all.length, size);
    if (prevPage !== S.dl.page) full = true;
    const list = all.slice((S.dl.page - 1) * size, S.dl.page * size);

    const pagerHtml = pager('dl', S.dl.page, all.length, size);
    const $pager = document.getElementById('dl-pager');
    if ($pager.innerHTML !== pagerHtml && !$pager.contains(document.activeElement)) $pager.innerHTML = pagerHtml;

    if (!list.length) {
      $list.innerHTML = `<div class="empty" style="border-top:0"><div class="glyph">閒</div><div class="title">${tasks.length ? '没有符合条件的任务' : '暂无下载任务'}</div><div>${tasks.length ? '' : '在书架上点击封面右上角的印章, 即可下载整套课程'}</div>${tasks.length ? '' : '<a class="btn btn-sm" href="#/explore">浏览全部课程</a>'}</div>`;
      return;
    }

    // keyed patching keeps buttons stable between polls
    const existing = new Map();
    $list.querySelectorAll('.task[data-id]').forEach((el) => existing.set(el.dataset.id, el));
    const frag = document.createDocumentFragment();
    for (const t of list) {
      let el = existing.get(String(t.id));
      if (!el || full) {
        el = document.createElement('div');
        el.className = 'task';
        el.dataset.id = t.id;
        el.innerHTML = `
          <div class="task-glyph">${t.typeKey === 'n4' || t.typeKey === 'e0' ? icons.book : (t.isVideo ? icons.video : icons.doc)}</div>
          <div class="task-main">
            <div class="task-title"><span class="t"></span><span class="stw"></span></div>
            <div class="task-sub"></div>
            <div class="task-err" hidden></div>
            <div class="progress"><i></i></div>
            <div class="task-nums"><span class="l"></span><span class="r"></span></div>
          </div>
          <div class="task-actions"></div>`;
      }
      patchTask(el, t);
      frag.appendChild(el);
    }
    $list.innerHTML = '';
    $list.appendChild(frag);
  }

  function patchTask(el, t) {
    const set = (sel, prop, v) => { const n = el.querySelector(sel); if (n[prop] !== v) n[prop] = v; };
    set('.t', 'textContent', t.title);
    el.querySelector('.t').title = t.title;
    set('.stw', 'innerHTML', `<span class="st ${t.status}">${STATUS_TEXT[t.status] || t.status}</span>`);
    set('.task-sub', 'textContent', `${t.courseTitle} · ${t.typeName}` + (t.stage && t.status !== 'done' ? ` · ${t.stage}` : ''));
    const err = el.querySelector('.task-err');
    err.hidden = !t.error;
    if (t.error && err.textContent !== t.error) { err.textContent = t.error; err.title = t.error; }

    const bar = el.querySelector('.progress');
    const fill = bar.firstElementChild;
    let pct = 0;
    if (t.status === 'done' || t.status === 'skipped') pct = 100;
    else if (t.total > 0) pct = Math.min(100, (t.done / t.total) * 100);
    const indeterminate = t.status === 'running' && !(t.total > 0);
    bar.className = 'progress' + (indeterminate ? ' indeterminate' : '') + (t.status === 'done' ? ' done' : '');
    bar.hidden = t.status === 'queued' || t.status === 'failed' || t.status === 'canceled';
    fill.style.width = pct + '%';

    let left = '';
    let right = '';
    if (t.total > 0 && t.status !== 'done') left = `${fmtBytes(t.done)} / ${fmtBytes(t.total)} · ${pct.toFixed(0)}%`;
    if (t.status === 'running') {
      if (t.speed > 0) right = fmtBytes(t.speed) + '/s';
      if (t.speed > 0 && t.total > 0) right += ' · 剩余 ' + fmtDuration(((t.total - t.done) / t.speed) * 1000);
      if (!right && t.startedAt) right = '已用时 ' + fmtDuration(Date.now() - t.startedAt);
    } else if (t.status === 'done' && t.startedAt && t.finishedAt) {
      left = (t.total > 0 ? fmtBytes(t.total) + ' · ' : '') + '用时 ' + fmtDuration(t.finishedAt - t.startedAt);
    }
    set('.l', 'textContent', left);
    set('.r', 'textContent', right);

    const acts = el.querySelector('.task-actions');
    if (acts.dataset.status !== t.status) {
      acts.dataset.status = t.status;
      const b = (action, icon, title, cls) => `<button class="ibtn ${cls || ''}" data-action="${action}" data-id="${t.id}" title="${title}" aria-label="${title}">${icon}</button>`;
      let html = '';
      if (isActive(t)) html += b('task-cancel', icons.stop, '取消', 'danger');
      if (t.status === 'failed' || t.status === 'canceled' || t.status === 'skipped') html += b('task-retry', icons.retry, '重试', 'accent');
      html += b('task-open', icons.folder, '打开所在目录');
      html += b('task-remove', icons.trash, '移除任务', 'danger');
      acts.innerHTML = html;
    }
  }

  async function taskAction(action, ids) {
    try {
      const r = await api('POST', '/api/tasks/action', { action, ids: ids || [] });
      applyTasks(r.tasks || []);
    } catch (e) { toast(e.message, 'error'); }
  }

  // ---------- settings ----------
  function renderSettings() {
    if (!S.settings) { $view.innerHTML = ''; return; }
    const s = S.settings;
    const out = s.columnOutputType;
    const maxT = S.limits.maxThreads;
    const maxC = S.limits.maxConcurrentTasks;
    const section = (no, title, desc, body) => `
      <div class="section">
        <div class="section-head"><div class="no">${no}</div><h2>${title}</h2><p>${desc}</p></div>
        <div class="form-rows">${body}</div>
      </div>`;
    const range = (id, name, min, max, v) => `<div class="range"><input type="range" min="${min}" max="${max}" value="${v}" data-link="${id}" aria-label="${name}"><input class="input" id="${id}" name="${name}" type="number" min="${min}" max="${max}" value="${v}"></div>`;
    $view.innerHTML = `
      <div class="page">
        <div class="page-head"><div><div class="kicker">Preferences</div><h1>设置</h1><div class="sub">修改后对新开始的任务生效。</div></div></div>
        <form id="settings-form">
          <div class="sections">
          ${section('01', '存储', '文件保存的位置与视频画质。', `
            <div class="frow">
              <label class="fl" for="s-folder">下载目录</label>
              <div class="inline" style="gap:10px">
                <input class="input" id="s-folder" name="downloadFolder" value="${esc(s.downloadFolder)}" spellcheck="false" style="flex:1;min-width:220px;font-family:var(--mono);font-size:13px">
                <button class="btn btn-ghost" type="button" data-action="open-root">${icons.open}打开</button>
              </div>
              <div class="hint">每门课程保存在以课程名命名的子目录中。</div>
            </div>
            <div class="frow">
              <span class="fl">视频清晰度</span>
              <div class="seg" data-seg="quality">
                ${[['ld', '标清'], ['sd', '高清'], ['hd', '超清']].map(([v, l]) => `<button type="button" data-v="${v}" class="${s.quality === v ? 'active' : ''}">${l}</button>`).join('')}
              </div>
            </div>`)}
          ${section('02', '多线程下载', '并行越多越快, 但也越容易触发极客时间的限流。', `
            <div class="frow">
              <label class="fl" for="s-threads">下载线程数</label>
              ${range('s-threads', 'threads', 1, maxT, s.threads)}
              <div class="hint">每个任务内的并行连接数: 视频分片并行下载, 大文件分块并行下载。</div>
            </div>
            <div class="frow">
              <label class="fl" for="s-tasks">同时下载任务数</label>
              ${range('s-tasks', 'concurrentTasks', 1, maxC, s.concurrentTasks)}
              <div class="hint">同时进行的文章 / 视频数量。专栏 PDF 由 Chrome 生成, 建议不超过 3。</div>
            </div>
            <div class="frow">
              <label class="fl" for="s-interval">任务间隔 (秒)</label>
              ${range('s-interval', 'interval', 0, 10, s.interval)}
              <div class="hint">每个任务完成后额外等待的时间, 另加 0–2 秒随机抖动。</div>
            </div>`)}
          ${section('03', '专栏输出', '图文专栏的保存格式与附加内容。', `
            <div class="frow">
              <span class="fl">输出格式</span>
              <div class="inline">
                <label class="check"><input type="checkbox" name="out" value="1" ${out & 1 ? 'checked' : ''}>PDF</label>
                <label class="check"><input type="checkbox" name="out" value="2" ${out & 2 ? 'checked' : ''}>Markdown</label>
                <label class="check"><input type="checkbox" name="out" value="4" ${out & 4 ? 'checked' : ''}>音频</label>
              </div>
              <div class="hint">PDF 需要本机安装 Chrome; 训练营图文固定保存为 Markdown。</div>
            </div>
            <div class="frow">
              <label class="fl" for="s-comments">评论</label>
              <select class="select" id="s-comments" name="downloadComments" style="width:220px">
                <option value="0" ${s.downloadComments === 0 ? 'selected' : ''}>不下载</option>
                <option value="1" ${s.downloadComments === 1 ? 'selected' : ''}>下载首页评论</option>
                <option value="2" ${s.downloadComments === 2 ? 'selected' : ''}>下载所有评论</option>
              </select>
              <div class="hint">仅对 PDF 生效。</div>
            </div>
            <div class="frow">
              <label class="fl" for="s-wait">PDF 等待 / 超时 (秒)</label>
              <div class="inline" style="gap:10px">
                <input class="input" id="s-wait" name="printPDFWaitSeconds" type="number" min="0" max="60" value="${s.printPDFWaitSeconds}" style="width:90px;font-family:var(--mono)">
                <span style="color:var(--muted)">/</span>
                <input class="input" name="printPDFTimeoutSeconds" type="number" min="1" max="120" value="${s.printPDFTimeoutSeconds}" style="width:90px;font-family:var(--mono)" aria-label="PDF 超时">
              </div>
            </div>`)}
          ${section('04', '其他', '', `
            <div class="frow" style="display:flex;flex-direction:column;gap:12px">
              <label class="check"><input type="checkbox" name="skipEmbedVideo" ${s.skipEmbedVideo ? 'checked' : ''}>跳过图文课程中嵌入的视频</label>
              <label class="check"><input type="checkbox" name="overwrite" ${s.overwrite ? 'checked' : ''}>覆盖已存在的文件 (默认跳过已下载的内容)</label>
            </div>`)}
          </div>
          <div class="form-foot">
            <button class="btn btn-ghost" type="button" data-action="settings-reset">还原</button>
            <button class="btn btn-primary" type="submit">保存设置</button>
          </div>
        </form>
      </div>`;

    const form = document.getElementById('settings-form');
    form.querySelectorAll('[data-link]').forEach((r) => {
      const num = document.getElementById(r.dataset.link);
      r.addEventListener('input', () => { num.value = r.value; });
      num.addEventListener('input', () => { r.value = num.value; });
    });
    form.querySelectorAll('[data-seg]').forEach((seg) => seg.addEventListener('click', (e) => {
      const b = e.target.closest('button[data-v]');
      if (!b) return;
      seg.querySelectorAll('button').forEach((x) => x.classList.toggle('active', x === b));
    }));
    form.addEventListener('submit', async (e) => {
      e.preventDefault();
      let outType = 0;
      form.querySelectorAll('input[name=out]:checked').forEach((c) => { outType |= Number(c.value); });
      if (!outType) { toast('请至少选择一种专栏输出格式', 'error'); return; }
      const body = {
        downloadFolder: form.downloadFolder.value.trim(),
        quality: form.querySelector('[data-seg=quality] .active').dataset.v,
        columnOutputType: outType,
        downloadComments: Number(form.downloadComments.value),
        printPDFWaitSeconds: Number(form.printPDFWaitSeconds.value),
        printPDFTimeoutSeconds: Number(form.printPDFTimeoutSeconds.value),
        interval: Number(form.interval.value),
        skipEmbedVideo: form.skipEmbedVideo.checked,
        threads: Number(form.threads.value),
        concurrentTasks: Number(form.concurrentTasks.value),
        overwrite: form.overwrite.checked,
      };
      try {
        S.settings = await api('PUT', '/api/settings', body);
        toast('设置已保存', 'success');
      } catch (ex) { toast(ex.message, 'error'); }
    });
  }

  // ---------- polling ----------
  let lastFinished = -1;
  function applyTasks(tasks) {
    S.tasks = tasks;
    const finished = tasks.filter((t) => t.status === 'done' || t.status === 'skipped').length;
    if (finished !== lastFinished) {
      if (lastFinished >= 0) loadMarksSoon();
      lastFinished = finished;
    }
    const sig = JSON.stringify(tasks.map((t) => [t.id, t.status, t.done, t.total, t.stage, t.speed, t.error]));
    const changed = sig !== S.tasksSig;
    S.tasksSig = sig;
    updateChrome();
    if (!changed) return;
    if (S.page === 'downloads') renderTaskList(false);
    if (S.page === 'course') patchCourseStatuses();
  }

  let pollTimer = 0;
  let polling = false;
  async function pollTasks() {
    clearTimeout(pollTimer);
    if (!S.fatal && S.token && !polling) {
      polling = true;
      try {
        const r = await api('GET', '/api/tasks');
        applyTasks(r.tasks || []);
      } catch (_) { /* ignore */ }
      polling = false;
    }
    const active = S.tasks.some(isActive);
    const delay = document.hidden ? 5000 : (active ? 1000 : 3000);
    pollTimer = setTimeout(pollTasks, delay);
  }

  // ---------- events ----------
  function setEx(k, v) {
    S.ex[k] = v;
    if (k === 'tag') S.ex.sub = 0;
    S.ex.page = 1;
    saveEx();
    renderExploreFilters();
    loadCatalog();
  }
  function setLib(k, v) {
    S.lib[k] = v;
    S.lib.page = 1;
    saveLib();
    renderLibraryResults();
  }

  document.addEventListener('click', async (e) => {
    const el = e.target.closest('[data-action]');
    if (!el || el.disabled) return;
    const a = el.dataset.action;
    const id = el.dataset.id;
    switch (a) {
      // pagination
      case 'page': gotoPage(el.dataset.target, Number(el.dataset.page)); break;
      // 全部课程
      case 'ex-clear-search': {
        const q = document.getElementById('ex-q');
        if (q) q.value = '';
        doSearch();
        break;
      }
      case 'ex-tag': setEx('tag', Number(el.dataset.v)); break;
      case 'ex-sub': setEx('sub', Number(el.dataset.v)); break;
      case 'ex-type': setEx('type', Number(el.dataset.v)); break;
      case 'ex-form': setEx('form', Number(el.dataset.v)); break;
      case 'ex-pvip': setEx('pvip', Number(el.dataset.v)); break;
      case 'ex-sort': setEx('sort', Number(el.dataset.v)); break;
      case 'ex-reset': Object.assign(S.ex, { tag: 0, sub: 0, type: 0, form: 0, pvip: 0, sort: 8, page: 1 }); saveEx(); renderExploreFilters(); loadCatalog(); break;
      // 我的课程
      case 'refresh-products': loadProducts(true); break;
      case 'lib-cat': setLib('cat', el.dataset.v); break;
      case 'lib-media': setLib('media', el.dataset.v); break;
      case 'lib-status': setLib('status', el.dataset.v); break;
      case 'lib-sort': setLib('sort', el.dataset.v); break;
      case 'lib-dl': setLib('dl', el.dataset.v); break;
      case 'reset-filters': {
        Object.assign(S.lib, { q: '', cat: '全部', media: 'all', status: 'all', dl: 'all', sort: 'recent', page: 1 });
        saveLib();
        const q = document.getElementById('lib-q');
        if (q) q.value = '';
        renderLibraryResults();
        break;
      }
      case 'quick-open': quickOpen(); break;
      case 'open-product': {
        const p = productIndex.get(id);
        if (!p) break;
        if (S.pick.on) togglePick(p);
        else openProduct(p);
        break;
      }
      case 'pick-toggle': setPickMode(!S.pick.on); break;
      case 'pick-page': pickPage(false); break;
      case 'pick-undl': pickPage(true); break;
      case 'pick-clear': S.pick.sel.clear(); refreshPicks(); break;
      case 'pick-open': openDrawerFromPicks(); break;
      case 'batch-all': openDrawerAll('download'); break;
      case 'batch-unlock': openDrawerAll('unlock'); break;
      case 'unlock': {
        const p = productIndex.get(id);
        if (p) unlockOne(p);
        break;
      }
      case 'course-unlock': {
        const p = S.course && courseIndex.get(S.course.key);
        if (p) unlockOne(p);
        break;
      }
      case 'batch-close': closeDrawer(); break;
      case 'batch-start': startBatch(); break;
      case 'batch-stop': if (S.batch) { S.batch.stop = true; S.batch.progress = '正在停止…'; renderDrawer(false); } break;
      case 'batch-stop-collect': if (S.batch) S.batch.stop = true; break;
      case 'batch-sel': if (S.batch && !S.batch.running) batchSelect(el.dataset.v); break;
      case 'mark-toggle': toggleCourseMark(); break;
      case 'course-tab': if (S.course) { S.course.tab = el.dataset.tab; renderCourseBody(); } break;
      case 'quick-dl': {
        const p = productIndex.get(id);
        if (p) quickDownload(p);
        break;
      }
      // course
      case 'back': location.hash = '#/' + ((S.course && S.course.from) || lastListPage); break;
      case 'course-refresh':
        if (S.course) { S.course.data = null; loadCourse(true); renderCourse(S.course.type, S.course.id); }
        break;
      case 'course-download-all':
        if (S.course && S.course.data) enqueue(S.course.type, S.course.data.id, [], S.course.data.title);
        break;
      case 'course-download-selected':
        if (S.course && S.course.data && S.course.sel.size) {
          if (await enqueue(S.course.type, S.course.data.id, Array.from(S.course.sel))) {
            S.course.sel.clear();
            renderArticleList();
          }
        }
        break;
      case 'course-download-one':
        if (S.course && S.course.data) enqueue(S.course.type, S.course.data.id, [Number(el.dataset.aid)]);
        break;
      case 'sel-clear': if (S.course) { S.course.sel.clear(); renderArticleList(); } break;
      // downloads
      case 'dl-tab': S.dl.tab = el.dataset.tab; S.dl.page = 1; store.set('dlTab', S.dl.tab); renderTaskList(true); break;
      case 'dl-retry-failed': taskAction('retryFailed'); break;
      case 'dl-cancel-all':
        if (S.tasks.some(isActive) && confirm('确定取消所有进行中和等待中的任务吗?')) taskAction('cancelAll');
        break;
      case 'dl-clear': taskAction('clearFinished'); break;
      case 'task-cancel': taskAction('cancel', [Number(id)]); break;
      case 'task-retry': taskAction('retry', [Number(id)]); break;
      case 'task-remove': taskAction('remove', [Number(id)]); break;
      case 'task-open':
      case 'open-root':
        try { await api('POST', '/api/open', { taskId: a === 'task-open' ? Number(id) : 0 }); } catch (ex) { toast(ex.message, 'error'); }
        break;
      case 'settings-reset': renderSettings(); break;
      default: break;
    }
  });

  document.addEventListener('keydown', (e) => {
    const t = e.target;
    if (e.key === 'Escape' && S.batch && !S.batch.running) { closeDrawer(); return; }
    if (e.key === 'Enter' && t.classList && t.classList.contains('book')) t.click();
    if (e.key === 'Enter' && t.dataset && t.dataset.jump) {
      const n = Number(t.value);
      const pages = Number(t.dataset.pages);
      if (n >= 1 && n <= pages) gotoPage(t.dataset.jump, n);
    }
    // "/" focuses the page search
    if (e.key === '/' && !/INPUT|TEXTAREA|SELECT/.test(t.tagName)) {
      const q = document.getElementById('ex-q') || document.getElementById('lib-q') || document.getElementById('course-q') || document.getElementById('dl-q');
      if (q) { e.preventDefault(); q.focus(); }
    }
  });

  document.addEventListener('change', (e) => {
    const el = e.target;
    if (el.name === 'batch-mode' && S.batch) { setBatchMode(el.value); return; }
    if (el.id === 'batch-then' && S.batch) { S.batch.then = el.checked; return; }
    if (el.dataset && el.dataset.bi != null && S.batch) {
      S.batch.items[Number(el.dataset.bi)].on = el.checked;
      const row = el.closest('.pick-row');
      if (row) row.classList.toggle('off', !el.checked);
      renderDrawer(false);
      return;
    }
    if (el.dataset && el.dataset.sel && S.course) {
      const aid = Number(el.dataset.sel);
      if (el.checked) S.course.sel.add(aid); else S.course.sel.delete(aid);
      updateSelection();
    }
  });

  function saveLib() {
    const { cat, media, status, dl, sort } = S.lib;
    store.set('lib', { cat, media, status, dl, sort });
  }

  $logout.addEventListener('click', async () => {
    if (S.tasks.some(isActive) && !confirm('退出登录将取消所有进行中的任务, 确定吗?')) return;
    try { await api('POST', '/api/logout'); } catch (_) { /* ignore */ }
    S.loggedIn = false;
    S.products = null;
    S.meta = null;
    S.ex.data = null;
    S.course = null;
    location.hash = '#/login';
    render();
  });

  // broken cover images fall back to a cloth-bound plate
  document.addEventListener('error', (e) => {
    const img = e.target;
    if (img.tagName !== 'IMG' || img.dataset.plate == null) return;
    const wrap = document.createElement('div');
    wrap.innerHTML = plateHtml(img.dataset.plate);
    img.replaceWith(wrap.firstElementChild);
  }, true);

  window.addEventListener('hashchange', render);
  document.addEventListener('visibilitychange', () => { if (!document.hidden) pollTasks(); });

  // ---------- boot ----------
  async function boot() {
    const m = location.hash.match(/token=([0-9a-f]+)/i);
    if (m) {
      S.token = m[1];
      store.set('token', S.token);
      history.replaceState(null, '', location.pathname + '#/explore');
    } else {
      S.token = store.get('token', '');
    }
    if (!S.token) {
      S.fatal = '缺少访问令牌, 请使用终端中输出的地址打开页面';
      render();
      return;
    }
    try {
      const st = await api('GET', '/api/state');
      S.loggedIn = st.loggedIn;
      S.settings = st.settings;
      S.types = st.types || [];
      S.limits = st.limits || S.limits;
      if (!S.types.find((t) => t.key === S.quick.type)) S.quick.type = (S.types[0] || {}).key;
    } catch (e) {
      if (!S.fatal) S.fatal = e.message;
    }
    render();
    pollTasks();
    if (S.loggedIn) loadMarks();
  }

  boot();
})();
