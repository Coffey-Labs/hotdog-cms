
  /* --------------------------------------------------------------- drawing
   * Everything is drawn here, in SVG, 34px tall, sized to the window. */
  var NS = 'http://www.w3.org/2000/svg', H = 34;
  function el(name, attrs, parent) {
    var e = document.createElementNS(NS, name);
    for (var k in attrs) e.setAttribute(k, attrs[k]);
    if (parent) parent.appendChild(e);
    return e;
  }
  function g(parent, x, y, extra) { return el('g', Object.assign({ transform: 'translate(' + x.toFixed(1) + ' ' + y.toFixed(1) + ')' + (extra || '') }, {}), parent); }
  function anim(parent, x, y, cls, extra) { var o = g(parent, x, y, extra); return el('g', { class: cls }, o); }
  // A wire that swoops between hooks `span` apart, ending at `w`; calls hang(x, y, i)
  // at every hook and every dip.
  function wire(s, w, span, color, hang) {
    var n = Math.ceil(w / span), d = 'M0 3', i;
    for (i = 0; i < n; i++) d += ' Q' + (i * span + span / 2) + ' 19 ' + (i + 1) * span + ' 3';
    el('path', { d: d, fill: 'none', stroke: color || '#3f6b4b', 'stroke-width': 1.8, 'stroke-linecap': 'round' }, s);
    for (i = 0; i < n * 2; i++) {
      var x = i * span / 2 + span / 4;
      if (x > w) break;
      var y = 3 + 16 * (1 - Math.pow(2 * ((x % span) / span) - 1, 2)) * 0.5;
      hang(x, y, i);
    }
  }
  var BULB = 'M0 3.2 C3.6 3.6 4.4 8 3 11 C2.2 12.8 1 14 0 14.4 C-1 14 -2.2 12.8 -3 11 C-4.4 8 -3.6 3.6 0 3.2 Z';
  var PALETTES = {
    classic: ['#ff4b4b', '#ffcc3d', '#35d36a', '#4b9bff', '#ff8a2a'],
    hanukkah: ['#4b9bff', '#f4f8ff', '#2f6fe0', '#cfe3ff', '#7fb6ff'],
    // Hanukkah and Christmas on the same day: both strings, bulb by bulb.
    shared: ['#ff4b4b', '#4b9bff', '#ffcc3d', '#f4f8ff', '#35d36a', '#2f6fe0', '#ff8a2a', '#cfe3ff', '#ff4b4b', '#7fb6ff']
  };
  var STRIPS = {
    lights: function (s, w, o) {
      var pal = PALETTES[o.tint] || PALETTES.classic, nb = pal.length;
      wire(s, w, 88, '#3f6b4b', function (x, y, i) {
        var b = g(s, x, y, ' rotate(' + ((i % 2 ? 1 : -1) * (8 + (i * 7) % 9)) + ') scale(1.3)');
        el('rect', { x: -2.2, y: 0, width: 4.4, height: 4, rx: 1, fill: '#3f6b4b' }, b);
        el('path', { d: BULB, fill: pal[i % nb], style: 'filter:drop-shadow(0 0 3px ' + pal[i % nb] + ')', class: 'tw d' + (i % 4) }, b);
      });
    },
    // Pride Month
    pride: function (s, w) {  // a ribbon in all eleven Progress Pride colours
      var cols = ['#000000', '#613915', '#5BCEFA', '#F5A9B8', '#FFFFFF', '#E40303', '#FF8C00', '#FFED00', '#008026', '#24408E', '#732982'];
      for (var k = 0; k < cols.length; k++) {
        var y0 = 3 + k * 1.9, d = 'M0 ' + y0;
        for (var x = 0; x <= w + 120; x += 120) d += ' q30 8 60 0 t60 0';
        el('path', { d: d, fill: 'none', stroke: cols[k], 'stroke-width': 2.1, 'stroke-linecap': 'butt' }, s);
      }
    },
    fireworks: function (s, w) {
      var cols = ['#ffd23f', '#ff5fa2', '#7fe3ff', '#9b7bff', '#5ee08a'];
      for (var x = 24, i = 0; x < w; x += 58 + (i * 23) % 31, i++) {
        var b = anim(s, x, 8 + (i * 13) % 16, 'burst d' + (i % 4)), c = cols[i % 5], r = 6 + (i * 5) % 5;
        for (var a = 0; a < 12; a++) {
          var t = a * Math.PI / 6;
          el('line', { x1: Math.cos(t) * r * 0.35, y1: Math.sin(t) * r * 0.35, x2: Math.cos(t) * r, y2: Math.sin(t) * r,
            stroke: c, 'stroke-width': 1.6, 'stroke-linecap': 'round' }, b);
        }
        el('circle', { r: 1.6, fill: '#fff' }, b);
      }
    },
    lanterns: function (s, w) {
      wire(s, w, 96, '#8a2b1a', function (x, y, i) {
        if (i % 2) return;
        var b = anim(s, x, y, 'sway d' + (i % 4));
        el('line', { x1: 0, y1: 0, x2: 0, y2: 3, stroke: '#8a2b1a', 'stroke-width': 1 }, b);
        el('rect', { x: -4, y: 3, width: 8, height: 2.4, rx: 1, fill: '#f2c230' }, b);
        el('ellipse', { cx: 0, cy: 12, rx: 8, ry: 7.4, fill: '#d62839', style: 'filter:drop-shadow(0 0 3px #ff6a3d)' }, b);
        el('ellipse', { cx: 0, cy: 12, rx: 3.6, ry: 7.4, fill: 'none', stroke: '#a51c2c', 'stroke-width': 0.8 }, b);
        el('rect', { x: -4, y: 18.6, width: 8, height: 2.4, rx: 1, fill: '#f2c230' }, b);
        el('path', { d: 'M-1.2 21 L-1.6 28 L1.6 28 L1.2 21 Z', fill: '#f2c230' }, b);
      });
    },
    moonlanterns: function (s, w) {
      el('circle', { cx: Math.min(w - 40, w * 0.12 + 20), cy: 14, r: 10, fill: '#fff3c4', style: 'filter:drop-shadow(0 0 6px #ffe08a)' }, s);
      wire(s, w, 96, '#7a4a1f', function (x, y, i) {
        if (i % 2) return;
        var b = anim(s, x, y, 'sway d' + (i % 4));
        el('line', { x1: 0, y1: 0, x2: 0, y2: 3, stroke: '#7a4a1f', 'stroke-width': 1 }, b);
        el('circle', { cx: 0, cy: 11, r: 7.5, fill: ['#ffc94a', '#ff9a3c', '#ffde7a'][i / 2 % 3], style: 'filter:drop-shadow(0 0 4px #ffb340)' }, b);
        el('path', { d: 'M-1 18 L-1.4 25 L1.4 25 L1 18 Z', fill: '#e5412b' }, b);
      });
    },
    streamers: function (s, w) {
      var cols = ['#ff4fa3', '#ffd23f', '#2bb3a8', '#7b3fb0', '#3b8cff'];
      for (var k = 0; k < 3; k++) {
        var d = 'M0 ' + (6 + k * 6);
        for (var x = 0; x <= w + 40; x += 40) d += ' Q' + (x + 10) + ' ' + (k % 2 ? 0 : 12 + k * 6) + ' ' + (x + 20) + ' ' + (6 + k * 6) + ' T' + (x + 40) + ' ' + (6 + k * 6);
        el('path', { d: d, fill: 'none', stroke: cols[k], 'stroke-width': 2.4, 'stroke-linecap': 'round', opacity: 0.9 }, s);
      }
      for (var i = 0, xx = 9; xx < w; xx += 17 + (i * 7) % 11, i++)
        el('rect', { x: xx, y: 4 + (i * 11) % 26, width: 3.4, height: 5.4, rx: 1, fill: cols[(i + 2) % 5],
          transform: 'rotate(' + ((i * 37) % 90 - 45) + ' ' + xx + ' ' + (6 + (i * 11) % 26) + ')', class: 'tw d' + (i % 4) }, s);
    },
    bunting: function (s, w) {
      var cols = ['#f7a8c4', '#bde3a3', '#a8d8f7', '#ffe08a', '#d6c1f5'];
      wire(s, w, 44, '#c9a27a', function (x, y, i) {
        el('path', { d: 'M' + (x - 9) + ' ' + (y - 1) + ' L' + (x + 9) + ' ' + (y - 1) + ' L' + x + ' ' + (y + 16) + ' Z', fill: cols[i % 5] }, s);
      });
    },
    water: function (s, w) {
      for (var x = 10, i = 0; x < w; x += 22 + (i * 9) % 13, i++) {
        var b = anim(s, x, 2 + (i * 7) % 10, 'drip d' + (i % 4));
        el('path', { d: 'M0 0 C3 5 5 8 5 11 A5 5 0 0 1 -5 11 C-5 8 -3 5 0 0 Z', fill: ['#5cc8ff', '#2d9cff', '#9be3ff'][i % 3], opacity: 0.9 }, b);
        el('path', { d: 'M-2 10 a2.4 2.4 0 0 0 1.6 2.6', stroke: '#fff', 'stroke-width': 1, fill: 'none' }, b);
      }
    },
    crescents: function (s, w) {
      wire(s, w, 80, '#b08a2e', function (x, y, i) {
        var b = anim(s, x, y, 'tw d' + (i % 4));
        el('line', { x1: 0, y1: 0, x2: 0, y2: 4, stroke: '#b08a2e', 'stroke-width': 1 }, b);
        if (i % 2) el('path', { d: 'M0 5 L1.8 9.6 L6.6 9.8 L2.8 12.8 L4.2 17.4 L0 14.8 L-4.2 17.4 L-2.8 12.8 L-6.6 9.8 L-1.8 9.6 Z', fill: '#f2c230', style: 'filter:drop-shadow(0 0 3px #ffd86a)' }, b);
        else el('path', { d: 'M3 5 A8 8 0 1 0 3 21 A6.2 6.2 0 1 1 3 5 Z', fill: '#f2c230', style: 'filter:drop-shadow(0 0 3px #ffd86a)' }, b);
      });
    },
    leaves: function (s, w) {
      wire(s, w, 70, '#3b7d2c', function (x, y, i) {
        if (i % 3 === 2) {
          var f = g(s, x, y + 5);
          for (var a = 0; a < 6; a++) el('circle', { cx: Math.cos(a * 1.047) * 3.2, cy: Math.sin(a * 1.047) * 3.2, r: 2.4, fill: i % 2 ? '#ffffff' : '#6c8ef5' }, f);
          el('circle', { r: 1.8, fill: '#ffc83d' }, f);
        } else el('path', { d: 'M' + x + ' ' + y + ' q6 4 2 13 q-7 -5 -2 -13 Z', fill: ['#5aa83c', '#7cc35a', '#3f8f2e'][i % 3],
          transform: 'rotate(' + (i % 2 ? 25 : -25) + ' ' + x + ' ' + y + ')' }, s);
      });
    },
    hex: function (s, w) {
      var t = el('text', { x: 6, y: 21, fill: '#3bd16f', 'font-family': 'ui-monospace, Menlo, Consolas, monospace', 'font-size': 12, opacity: 0.85 }, s);
      var unit = '0x100 · 256 · 0b100000000 · 0o400 · ', str = '';
      while (str.length * 7.3 < w) str += unit;
      t.textContent = str;
    },
    bats: function (s, w) {
      wire(s, w, 90, '#4a3a5a', function (x, y, i) {
        if (i % 2) {
          var p = g(s, x, y);
          el('line', { x1: 0, y1: 0, x2: 0, y2: 3, stroke: '#4a3a5a', 'stroke-width': 1 }, p);
          el('ellipse', { cx: 0, cy: 10, rx: 7.5, ry: 6.5, fill: '#ff8a1f', style: 'filter:drop-shadow(0 0 3px #ffb347)' }, p);
          el('path', { d: 'M-3 9 l2 -2 l0 3 Z M3 9 l-2 -2 l0 3 Z M-3 12.5 q3 2 6 0', fill: '#3a2410', stroke: '#3a2410', 'stroke-width': 0.6 }, p);
          el('rect', { x: -1, y: 2.4, width: 2, height: 2.6, fill: '#3f6b2a' }, p);
        } else {
          var b = anim(s, x, y + 10, 'flap d' + (i % 4));
          el('path', { d: 'M0 0 C-3 -4 -7 -4 -11 -1 C-9 0 -8 2 -7 3 C-5 1 -3 2 -1.5 3 Z M0 0 C3 -4 7 -4 11 -1 C9 0 8 2 7 3 C5 1 3 2 1.5 3 Z M-1.6 -2 L-1 -4 L0 -2.4 L1 -4 L1.6 -2 Z', fill: '#3a2d4a', stroke: '#9c86c4', 'stroke-width': 0.6 }, b);
        }
      });
    },
    papel: function (s, w) {
      var cols = ['#ff4fa3', '#ff8a1f', '#ffd23f', '#3bd16f', '#3b8cff', '#9b5cff'];
      el('line', { x1: 0, y1: 2, x2: w, y2: 2, stroke: '#6b4a2a', 'stroke-width': 1.2 }, s);
      for (var x = 2, i = 0; x < w; x += 26, i++) {
        var f = g(s, x, 2);
        var d = 'M0 0 H22 V18 l-2.75 3 l-2.75 -3 l-2.75 3 l-2.75 -3 l-2.75 3 l-2.75 -3 l-2.75 3 l-2.75 -3 Z';
        d += ' M11 5 l2 3 l-2 3 l-2 -3 Z M5 9 a1.6 1.6 0 1 0 0.01 0 Z M17 9 a1.6 1.6 0 1 0 0.01 0 Z M7 14 h8 v1.6 h-8 Z';
        el('path', { d: d, fill: cols[i % 6], 'fill-rule': 'evenodd', opacity: 0.95 }, f);
      }
    },
    diyas: function (s, w) {
      wire(s, w, 76, '#e8891c', function (x, y, i) {
        if (i % 2) {
          var b = g(s, x, y + 2);
          el('path', { d: 'M-8 6 Q0 15 8 6 Z', fill: '#c8541a' }, b);
          el('path', { d: 'M-8 6 Q0 9 8 6', stroke: '#f2c230', 'stroke-width': 1, fill: 'none' }, b);
          el('path', { d: 'M0 -4 C2.6 0 2.6 3 0 6 C-2.6 3 -2.6 0 0 -4 Z', fill: '#ffcf3d', class: 'flick d' + (i % 4), style: 'filter:drop-shadow(0 0 4px #ffb020)' }, b);
        } else for (var k = 0; k < 3; k++)
          el('circle', { cx: x + (k - 1) * 6, cy: y + 1 + Math.abs(k - 1) * -1.2, r: 3, fill: k === 1 ? '#ffb627' : '#f08a1c' }, s);
      });
    },
    colour: function (s, w) {
      var cols = ['#ff4fa3', '#ffc21a', '#3bd16f', '#3b8cff', '#9b5cff', '#ff7a1f'];
      for (var x = 6, i = 0; x < w; x += 18 + (i * 13) % 19, i++) {
        var c = cols[i % 6], y = 6 + (i * 17) % 20, r = 4 + (i * 3) % 5;
        el('circle', { cx: x, cy: y, r: r, fill: c, opacity: 0.75, style: 'filter:blur(1.2px)' }, s);
        el('circle', { cx: x + r, cy: y - r * 0.6, r: r * 0.4, fill: c, opacity: 0.7 }, s);
        el('circle', { cx: x - r * 0.9, cy: y + r * 0.8, r: r * 0.3, fill: c, opacity: 0.7 }, s);
      }
    },
    blossom: function (s, w) {
      var d = 'M0 6', x;
      for (x = 0; x <= w; x += 60) d += ' Q' + (x + 30) + ' ' + (x / 60 % 2 ? 2 : 14) + ' ' + (x + 60) + ' 8';
      el('path', { d: d, fill: 'none', stroke: '#7a5236', 'stroke-width': 2, 'stroke-linecap': 'round' }, s);
      for (var i = 0, xx = 8; xx < w; xx += 21 + (i * 7) % 9, i++) {
        var f = g(s, xx, 7 + (i * 5) % 10);
        if (i % 4 === 3) { el('path', { d: 'M0 0 q4 2 3 8 q-5 -3 -3 -8 Z', fill: '#6cbf4b' }, f); continue; }
        for (var a = 0; a < 5; a++) el('circle', { cx: Math.cos(a * 1.2566 - 1.57) * 3, cy: Math.sin(a * 1.2566 - 1.57) * 3, r: 2.6, fill: i % 2 ? '#ffd1e3' : '#ffffff', stroke: '#f2a6c4', 'stroke-width': 0.5 }, f);
        el('circle', { r: 1.3, fill: '#e8608f' }, f);
      }
    }
  };

  // December's scenes: [width, draw(g, value)].
  function flame(p, x, y, i) {
    el('path', { d: 'M' + x + ' ' + (y - 5.5) + ' c2 3 2 4.6 0 5.5 c-2 -0.9 -2 -2.5 0 -5.5 Z', fill: '#ffcf3d', class: 'flick d' + (i % 4), style: 'filter:drop-shadow(0 0 3px #ffb020)' }, p);
  }
  var SCENE_ART = {
    hanukkah: [62, function (p, nights) {
      // A hanukkiah: eight candles and the shamash, raised in the middle.
      // Candles are set from the right; tonight's newest is lit first.
      el('path', { d: 'M31 32 V22 M18 32 h26 M4 18 Q4 24 31 24 Q58 24 58 18', fill: 'none', stroke: '#c9a227', 'stroke-width': 2, 'stroke-linecap': 'round' }, p);
      for (var j = 0; j < 9; j++) {
        var x = j < 4 ? 4 + j * 6.4 : (j === 4 ? 31 : 58 - (8 - j) * 6.4), sham = j === 4, top = sham ? 6 : 10;
        el('rect', { x: x - 1.6, y: top, width: 3.2, height: sham ? 12 : 8, rx: 1, fill: '#eaf3ff', stroke: '#7fb6ff', 'stroke-width': 0.6 }, p);
        var slot = j < 4 ? j : j - 1;  // 0..7 left to right, without the shamash
        if (sham || slot >= 8 - nights) flame(p, x, top, j);
      }
    }],
    sinterklaas: [40, function (p) {
      el('path', { d: 'M3 26 C3 20 10 18 16 18 L26 17 C30 16 32 13 36 14 C39 15 38 22 35 26 C30 31 8 31 3 26 Z', fill: '#f1c26b', stroke: '#9a6a2a', 'stroke-width': 1 }, p);
      el('path', { d: 'M17 17 L28 4 L31 6 L21 18 Z', fill: '#ff8a1f', stroke: '#c25a00', 'stroke-width': 0.8 }, p);
      el('path', { d: 'M28 4 l-1 -3 M28 4 l3 -2 M28 4 l2 -4', stroke: '#4e9a3a', 'stroke-width': 1.4, 'stroke-linecap': 'round' }, p);
    }],
    bodhi: [30, function (p) {
      el('path', { d: 'M15 3 C19 8 27 9 27 17 C27 24 20 26 15 33 C10 26 3 24 3 17 C3 9 11 8 15 3 Z', fill: '#5aa83c', stroke: '#2f6f22', 'stroke-width': 1 }, p);
      el('path', { d: 'M15 6 V31 M15 14 l-6 -3 M15 14 l6 -3 M15 20 l-7 -3 M15 20 l7 -3 M15 26 l-5 -3 M15 26 l5 -3', stroke: '#d8efc6', 'stroke-width': 0.8, fill: 'none' }, p);
    }],
    lucia: [44, function (p) {
      el('path', { d: 'M2 30 Q22 22 42 30', stroke: '#3e8e3a', 'stroke-width': 3, fill: 'none', 'stroke-linecap': 'round' }, p);
      for (var j = 0; j < 4; j++) {
        var x = 7 + j * 10;
        el('rect', { x: x - 2, y: 12, width: 4, height: 15, rx: 1, fill: '#fbfbf2', stroke: '#c9c3a6', 'stroke-width': 0.6 }, p);
        flame(p, x, 12, j);
      }
      el('circle', { cx: 13, cy: 28, r: 1.6, fill: '#d62839' }, p); el('circle', { cx: 31, cy: 28, r: 1.6, fill: '#d62839' }, p);
    }],
    posadas: [56, function (p) {
      for (var j = 0; j < 2; j++) {
        var x = 2 + j * 14;
        el('path', { d: 'M' + x + ' 18 h11 v14 h-11 Z', fill: '#f4d79a', stroke: '#b38a3e', 'stroke-width': 0.8, style: 'filter:drop-shadow(0 0 4px #ffcf6a)' }, p);
        flame(p, x + 5.5, 28, j);
      }
      var st = g(p, 44, 14), d = '';
      for (var a = 0; a < 14; a++) { var r = a % 2 ? 4 : 11, t = a * Math.PI / 7 - Math.PI / 2; d += (a ? 'L' : 'M') + (Math.cos(t) * r).toFixed(1) + ' ' + (Math.sin(t) * r).toFixed(1); }
      el('path', { d: d + 'Z', fill: '#ff4fa3', stroke: '#b81e6e', 'stroke-width': 0.8 }, st);
      el('circle', { r: 4, fill: '#ffd23f' }, st);
    }],
    yalda: [40, function (p) {
      el('circle', { cx: 16, cy: 21, r: 11, fill: '#b5162b', stroke: '#7a0d1c', 'stroke-width': 1 }, p);
      el('path', { d: 'M12 11 l2 -5 l2 3 l2 -3 l2 5 Z', fill: '#b5162b', stroke: '#7a0d1c', 'stroke-width': 0.8 }, p);
      el('path', { d: 'M10 18 a6 6 0 0 0 12 6 Z', fill: '#f7d6c8' }, p);
      for (var j = 0; j < 6; j++) el('circle', { cx: 12.5 + (j % 3) * 3, cy: 21 + Math.floor(j / 3) * 2.6, r: 1.2, fill: '#e01f3d' }, p);
      el('path', { d: 'M34 6 l1 2.4 l2.4 0.2 l-1.8 1.6 l0.6 2.4 l-2.2 -1.3 l-2.2 1.3 l0.6 -2.4 l-1.8 -1.6 l2.4 -0.2 Z', fill: '#ffe08a' }, p);
      el('circle', { cx: 30, cy: 22, r: 1.2, fill: '#ffe08a' }, p);
    }],
    dongzhi: [42, function (p) {
      el('circle', { cx: 34, cy: 9, r: 5, fill: '#ffd86a', style: 'filter:drop-shadow(0 0 4px #ffcf3d)' }, p);
      el('path', { d: 'M2 20 H32 C32 28 26 32 17 32 C8 32 2 28 2 20 Z', fill: '#4b9bff', stroke: '#2b5fb0', 'stroke-width': 1 }, p);
      el('path', { d: 'M5 24 H29', stroke: '#ffffff', 'stroke-width': 1.2, opacity: 0.8 }, p);
      el('circle', { cx: 11, cy: 18, r: 4, fill: '#ffffff', stroke: '#d8d8d8', 'stroke-width': 0.6 }, p);
      el('circle', { cx: 18, cy: 17, r: 4, fill: '#ffd1e3', stroke: '#e3a6bf', 'stroke-width': 0.6 }, p);
      el('circle', { cx: 25, cy: 18, r: 4, fill: '#ffffff', stroke: '#d8d8d8', 'stroke-width': 0.6 }, p);
    }],
    christmas: [32, function (p) {
      var d = '';
      for (var a = 0; a < 10; a++) { var r = a % 2 ? 5.5 : 13, t = a * Math.PI / 5 - Math.PI / 2; d += (a ? 'L' : 'M') + (16 + Math.cos(t) * r).toFixed(1) + ' ' + (18 + Math.sin(t) * r).toFixed(1); }
      el('path', { d: d + 'Z', fill: '#ffd23f', stroke: '#c99a12', 'stroke-width': 1, style: 'filter:drop-shadow(0 0 5px #ffe28a)', class: 'tw d1' }, p);
    }],
    kwanzaa: [62, function (p, days) {
      // A kinara: three red, the black in the middle, three green. The black
      // is lit first, then the outermost red and green, working inwards.
      el('rect', { x: 2, y: 26, width: 58, height: 6, rx: 2, fill: '#8a5a2b', stroke: '#5a3a1a', 'stroke-width': 0.8 }, p);
      var order = [3, 0, 6, 1, 5, 2, 4], lit = order.slice(0, days);
      var cols = ['#d62839', '#d62839', '#d62839', '#1d1d1d', '#2ba84a', '#2ba84a', '#2ba84a'];
      for (var j = 0; j < 7; j++) {
        var x = 7 + j * 8;
        el('rect', { x: x - 2, y: j === 3 ? 10 : 13, width: 4, height: j === 3 ? 16 : 13, rx: 1, fill: cols[j], stroke: '#00000040', 'stroke-width': 0.5 }, p);
        if (lit.indexOf(j) !== -1) flame(p, x, j === 3 ? 10 : 13, j);
      }
    }],
    reyes: [34, function (p) {
      el('path', { d: 'M3 30 L5 12 L11 20 L17 8 L23 20 L29 12 L31 30 Z', fill: '#f2c230', stroke: '#b38a12', 'stroke-width': 1 }, p);
      el('circle', { cx: 17, cy: 24, r: 2.4, fill: '#d62839' }, p); el('circle', { cx: 9, cy: 25, r: 1.8, fill: '#2d7dd2' }, p); el('circle', { cx: 25, cy: 25, r: 1.8, fill: '#2ba84a' }, p);
    }],
    genna: [32, function (p) { SCENE_ART.christmas[1](p); }]
  };

  // CSS (the strip's look) comes from season.css, put in by the build.

  var PROP_KIND = { santa: 'head', tophat: 'head', crown: 'head', bunny: 'head', flowers: 'head', birthday: 'head',
    witch: 'head', lucia: 'head', mask: 'eyes', shades: 'eyes', goggles: 'eyes', holi: 'eyes', marigold: 'ear',
    poppy: 'pin', lantern: 'side', mlantern: 'side', pride: 'pin' };
