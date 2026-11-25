
  /* ---------------------------------------------------------------- the page */
  // Paints the strip for r (from pick) into box, w px wide; returns a stop().
  function paint(box, r, w) {
    while (box.firstChild) box.removeChild(box.firstChild);
    var svg = el('svg', { width: w, height: H, viewBox: '0 0 ' + w + ' ' + H, focusable: 'false' }, box);
    var pad = 10, gap = 8, sw = 0, i;
    for (i = 0; i < r.scenes.length; i++) sw += SCENE_ART[r.scenes[i].s.id][0] + (i ? gap : 0);
    var x0 = r.scenes.length ? w - pad - sw : w;
    var strip = r.hol && r.hol.strip, tint = null;
    for (i = 0; i < r.scenes.length; i++) if (r.scenes[i].s.tint) tint = r.scenes[i].s.tint;
    for (i = 0; i < r.scenes.length; i++) if (tint && r.scenes[i].s.id === 'christmas') tint = 'shared';
    if (!strip && r.scenes.length) strip = 'lights';
    if (strip && STRIPS[strip]) {
      var sg = el('g', {}, svg);
      if (r.scenes.length) {
        var clip = el('clipPath', { id: 'season-clip' }, el('defs', {}, svg));
        el('rect', { x: 0, y: -10, width: Math.max(0, x0 - gap), height: H + 20 }, clip);
        sg.setAttribute('clip-path', 'url(#season-clip)');
      }
      STRIPS[strip](sg, r.scenes.length ? x0 - gap : w, { tint: tint });
    }
    for (var x = x0, j = 0; j < r.scenes.length; j++) {
      var art = SCENE_ART[r.scenes[j].s.id];
      art[1](g(svg, x, 0), r.scenes[j].v);
      x += art[0] + gap;
    }
    if (!r.greet.length) return function () {};
    var pill = document.createElement('span');
    pill.className = 'season-greet';
    pill.style.right = (r.scenes.length ? sw + pad + gap : 12) + 'px';
    box.appendChild(pill);
    var k = 0, timer;
    function show() {
      var gr = r.greet[k % r.greet.length];
      pill.textContent = gr[0];
      pill.setAttribute('lang', gr[1]);
      pill.setAttribute('dir', 'auto');
      pill.style.opacity = 1;
    }
    show();
    if (r.greet.length > 1) timer = setInterval(function () {
      pill.style.opacity = 0;
      setTimeout(function () { k++; show(); }, 400);
    }, 3600);
    return function () { clearInterval(timer); };
  }

  function withVersion(u, v) { return v ? u + (u.indexOf('?') === -1 ? '?' : '&') + 'v=' + v : u; }
  function resolve(tpl, prop) { return typeof tpl === 'function' ? tpl(prop) : tpl.split('{p}').join(prop); }

  // cfg: swap [[part of the mascot's URL, template with {p} for the prop]],
  // anchor (" | "-separated selectors; the strip hangs under the first found),
  // skip (selector: mascots inside keep their usual look), overlay + overlaySrc
  // (lay the prop over an element that isn't an image), version, only/except
  // (holiday and scene ids to show, or to leave out), greet (false: no
  // greetings), css (false: the page links season.css itself).
  function run(cfg) {
    var q = null;
    try {
      q = new URLSearchParams(location.search).get('season');
      if (q === 'auto') sessionStorage.removeItem('season');
      else if (q) sessionStorage.setItem('season', q);
      q = sessionStorage.getItem('season');
    } catch (e) { q = null; }
    if (q === 'off') return;
    var t = Date.now(), forced = null;
    if (q && /^\d{4}-\d\d-\d\d$/.test(q)) t = new Date(q + 'T12:00:00').getTime();
    else if (q === 'on') forced = 'advent';
    else if (q) forced = q;
    var only = cfg.only && cfg.only.length ? cfg.only : null, except = cfg.except || [];
    var r = pick(t, forced, function (id) { return (!only || only.indexOf(id) !== -1) && except.indexOf(id) === -1; });
    if (cfg.greet === false) r.greet = [];
    if (!r.hol && !r.scenes.length) return;

    var prop = r.hol && r.hol.prop, swaps = cfg.swap || [], anchor = cfg.anchor, skip = cfg.skip;
    function hat(img) {
      if (!prop || !img || img.getAttribute('data-season') || (skip && img.closest(skip))) return;
      var src = img.getAttribute('src') || '';
      for (var i = 0; i < swaps.length; i++) if (src.indexOf(swaps[i][0]) !== -1) {
        img.setAttribute('data-season', src);
        img.removeAttribute('srcset');
        img.setAttribute('src', withVersion(resolve(swaps[i][1], prop), cfg.version));
        return;
      }
    }
    function source(s) {  // a <picture>'s <source> wins over its <img>
      if (!prop || s.getAttribute('data-season') || (skip && s.closest(skip))) return;
      var set = s.getAttribute('srcset') || '';
      for (var i = 0; i < swaps.length; i++) if (set.indexOf(swaps[i][0]) !== -1) {
        s.setAttribute('data-season', set);
        s.setAttribute('srcset', withVersion(resolve(swaps[i][1], prop), cfg.version));
        return;
      }
    }
    // Where each kind of prop sits on a picture, in parts of its height.
    var SPOT = { head: [0, -0.31, 1.1], eyes: [0.05, 0.165, 0.95], ear: [0.62, -0.13, 0.475],
      pin: [0.6, 0.62, 0.475], side: [-0.55, -0.075, 0.475] };
    function overlay(host) {
      if (!prop || !cfg.overlay || host.getAttribute('data-season-prop')) return;
      var src = resolve(cfg.overlaySrc, prop);
      if (!src) return;
      host.setAttribute('data-season-prop', prop);
      // A picture can't hold the prop, so it goes in the picture's parent,
      // placed over the picture (and placed again when the page resizes).
      var pic = /^(img|svg|picture)$/i.test(host.tagName), box = pic ? host.parentElement : host;
      if (!box) return;
      if (getComputedStyle(box).position === 'static') box.style.position = 'relative';
      var im = document.createElement('img');
      im.className = 'season-prop season-prop-' + PROP_KIND[prop] + ' season-prop-is-' + prop;
      im.alt = '';
      im.setAttribute('aria-hidden', 'true');
      im.src = withVersion(src, cfg.version);
      box.appendChild(im);
      if (!pic) return;
      function put() {
        var a = host.getBoundingClientRect(), b = box.getBoundingClientRect(), s = SPOT[PROP_KIND[prop]], u = a.height;
        im.style.left = (a.left - b.left + s[0] * u) + 'px';
        im.style.top = (a.top - b.top + s[1] * u) + 'px';
        im.style.width = (s[2] * u) + 'px';
      }
      put();
      if (host.tagName === 'IMG' && !host.complete) host.addEventListener('load', put);
      window.addEventListener('resize', put);
    }
    function dress(root) {
      var scope = root.querySelectorAll ? root : document, i, l;
      if (swaps.length) {
        l = scope.querySelectorAll('picture source'); for (i = 0; i < l.length; i++) source(l[i]);
        l = scope.querySelectorAll('img'); for (i = 0; i < l.length; i++) hat(l[i]);
        if (root.tagName === 'IMG') hat(root);
      }
      if (cfg.overlay) {
        l = scope.querySelectorAll(cfg.overlay); for (i = 0; i < l.length; i++) overlay(l[i]);
        if (root.matches && root.matches(cfg.overlay)) overlay(root);
      }
    }

    var box;
    function place() {  // under the anchor when it's there, else along the window's top
      var host = null, list = anchor ? anchor.split(' | ') : [];
      for (var i = 0; i < list.length && !host; i++) host = document.querySelector(list[i]);
      if (host) {
        if (box.parentNode === host) return;
        box.classList.add('season-anchored');
        if (getComputedStyle(host).position === 'static') host.style.position = 'relative';
        host.appendChild(box);
      } else if (!box.isConnected || box.classList.contains('season-anchored')) {
        box.classList.remove('season-anchored');
        document.body.appendChild(box);
      }
    }

    function start() {
      var html = document.documentElement;
      html.classList.add('season-on');
      if (r.hol) html.classList.add('season-' + r.hol.id);
      if (cfg.css !== false) {
        var st = document.createElement('style');
        st.textContent = CSS;
        document.head.appendChild(st);
      }
      box = document.createElement('div');
      box.className = 'season-lights';
      box.setAttribute('aria-hidden', 'true');
      var stop = function () {}, lastW = 0;
      function draw() {
        var w = Math.max(html.clientWidth, 320);
        if (w === lastW) return;
        lastW = w; stop(); stop = paint(box, r, w);
      }
      place();
      draw();
      var tm;
      window.addEventListener('resize', function () { clearTimeout(tm); tm = setTimeout(draw, 150); });
      dress(document);
      // Pages that build their header after load (or rebuild it) still get
      // dressed, and the strip moves under its anchor once that exists.
      new MutationObserver(function (ms) {
        if (anchor) place();
        for (var i = 0; i < ms.length; i++)
          for (var j = 0; j < ms[i].addedNodes.length; j++)
            if (ms[i].addedNodes[j].nodeType === 1) dress(ms[i].addedNodes[j]);
      }).observe(document.body, { childList: true, subtree: true });
    }
    if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', start);
    else start();
  }
