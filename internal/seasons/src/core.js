  'use strict';
  var DAY = 864e5;

  /* ------------------------------------------------------------------ dates
   * Days are whole numbers counted from 1970-01-01. The visitor's own date
   * decides almost everything ("today" is where they are); Advent alone runs
   * on UTC, from Advent Sunday 00:00 UTC. */
  function dn(y, m, d) { return Math.floor(Date.UTC(y, m - 1, d) / DAY); }
  function ymd(n) { var t = new Date(n * DAY); return [t.getUTCFullYear(), t.getUTCMonth() + 1, t.getUTCDate()]; }
  function wday(n) { return new Date(n * DAY).getUTCDay(); }

  // The browser's own calendars (Chinese, Hebrew, Islamic, Persian). Where a
  // browser lacks one, the holidays on it just don't show.
  var fmts = {};
  function cal(c, n) {
    try {
      var f = fmts[c];
      if (f === undefined) {
        f = new Intl.DateTimeFormat('en-u-ca-' + c + '-nu-latn',
          { timeZone: 'UTC', month: c === 'hebrew' ? 'long' : 'numeric', day: 'numeric' });
        if (f.resolvedOptions().calendar !== c) f = null;
        fmts[c] = f;
      }
      if (!f) return null;
      var o = {}, p = f.formatToParts(new Date(n * DAY + DAY / 2));
      for (var i = 0; i < p.length; i++) o[p[i].type] = p[i].value;
      return { m: o.month, d: +o.day };
    } catch (e) { return null; }
  }
  function isCal(c, n, m, d) { var x = cal(c, n); return !!x && x.m === m && x.d === d; }
  // The day number in the last `span` days (today included) that is m/d on calendar c.
  function sinceCal(c, n, m, d, span) {
    for (var k = 0; k < span; k++) if (isCal(c, n - k, m, d)) return n - k;
    return null;
  }

  function easter(y) { // Gregorian (Anonymous Gregorian algorithm)
    var a = y % 19, b = Math.floor(y / 100), c = y % 100, d = Math.floor(b / 4), e = b % 4,
      f = Math.floor((b + 8) / 25), g = Math.floor((b - f + 1) / 3), h = (19 * a + b - d - g + 15) % 30,
      i = Math.floor(c / 4), k = c % 4, l = (32 + 2 * e + 2 * i - h - k) % 7, m = Math.floor((a + 11 * h + 22 * l) / 451),
      x = h + l - 7 * m + 114;
    return dn(y, Math.floor(x / 31), x % 31 + 1);
  }
  function easterOrthodox(y) { // Julian computus, moved to the Gregorian date (1900-2099: +13)
    var a = y % 4, b = y % 7, c = y % 19, d = (19 * c + 15) % 30, e = (2 * a + 4 * b - d + 34) % 7, x = d + e + 114;
    return dn(y, Math.floor(x / 31), x % 31 + 1) + 13;
  }

  // New and full moons (Meeus, Astronomical Algorithms ch. 49; good to a few
  // minutes). k whole: a new moon; k + 0.5: a full moon. Returns days since 1970.
  function moon(k) {
    var T = k / 1236.85, R = Math.PI / 180, E = 1 - 0.002516 * T;
    var M = (2.5534 + 29.1053567 * k) * R, P = (201.5643 + 385.81693528 * k) * R, F = (160.7108 + 390.67050284 * k) * R;
    var full = k % 1 !== 0, s = Math.sin;
    var c = (full ? -0.40614 : -0.40720) * s(P) + (full ? 0.17302 : 0.17241) * E * s(M) +
      (full ? 0.01614 : 0.01608) * s(2 * P) + (full ? 0.01043 : 0.01039) * s(2 * F) +
      (full ? 0.00734 : 0.00739) * E * s(P - M) - (full ? 0.00515 : 0.00514) * E * s(P + M) +
      (full ? 0.00209 : 0.00208) * E * E * s(2 * M) - 0.00111 * s(P - 2 * F) - 0.00057 * s(P + 2 * F) +
      0.00056 * E * s(2 * P + M) - 0.00042 * s(3 * P);
    return 2451550.09766 + 29.530588861 * k + 0.00015437 * T * T + c - 2440587.5;
  }
  function inZone(t, hours) { return Math.floor(t + hours / 24); }
  function newMoonFrom(n, hours) { // the first new moon on or after day n, in that zone
    var y = ymd(n)[0], k = Math.floor((y - 2000) * 12.3685 + (n - dn(y, 1, 1)) / 29.53) - 2;
    while (inZone(moon(k), hours) < n) k++;
    return k;
  }
  // Hindu lunisolar dates, as India keeps them (IST). Holi follows the full
  // moon of Phalguna, the last before the month that holds the sun's entry
  // into Aries (sidereal); Diwali is the new moon after the sun enters Libra.
  function holi(y) { return inZone(moon(newMoonFrom(dn(y, 3, 15), 5.5) - 0.5), 5.5); }
  function diwali(y) { return inZone(moon(newMoonFrom(dn(y, 10, 17), 5.5)), 5.5); }
  // The December solstice (Meeus ch. 27, mean value; within minutes), as a day in Beijing.
  function solstice(y) {
    var Y = (y - 2000) / 1000;
    return inZone(2451900.05952 + 365242.74049 * Y - 0.06223 * Y * Y - 0.00823 * Y * Y * Y + 0.00032 * Y * Y * Y * Y - 2440587.5, 8);
  }
  // The fourth Sunday before Christmas, 00:00 UTC, in ms.
  function adventStart(y) {
    var xmas = Date.UTC(y, 11, 25), w = new Date(xmas).getUTCDay();
    return xmas - ((w || 7) + 21) * DAY;
  }
  function lastWday(y, m, w) { var n = dn(y, m + 1, 1) - 1; return n - ((wday(n) - w + 7) % 7); }
  function between(n, a, b) { return n >= a && n <= b; }

  /* --------------------------------------------------------------- holidays
   * The first that matches today wins: a strip along the top of the page and
   * a prop for the mascot. Greetings are [text, language] pairs; several take
   * turns. */
  var EN = 'en';
  var HOLIDAYS = [
    { id: 'newyear', strip: 'fireworks', prop: 'tophat',
      on: function (n, y, m, d) { return (m === 12 && d === 31) || (m === 1 && d <= 2); },
      greet: [['Happy New Year', EN], ['Feliz Año Nuevo', 'es'], ['Bonne année', 'fr'], ['Frohes neues Jahr', 'de'],
        ['Feliz Ano Novo', 'pt'], ['Felice anno nuovo', 'it'], ['新年快乐', 'zh'], ['あけましておめでとう', 'ja'],
        ['새해 복 많이 받으세요', 'ko'], ['С Новым годом', 'ru'], ['Szczęśliwego Nowego Roku', 'pl'], ['Selamat Tahun Baru', 'id']] },
    { id: 'lucia', strip: 'lights', prop: 'lucia',
      on: function (n, y, m, d) { return m === 12 && d === 13; } },
    { id: 'advent', strip: 'lights', prop: 'santa',
      on: function (n, y, m, d, t) { var u = new Date(t).getUTCFullYear(); return t >= adventStart(u) && t < Date.UTC(u + 1, 0, 1); } },
    { id: 'kings', strip: 'lights', prop: 'crown',
      on: function (n, y, m, d) { return m === 1 && d >= 3 && d <= 7; } },
    { id: 'lunarnewyear', strip: 'lanterns', prop: 'lantern',
      on: function (n) { return isCal('chinese', n + 1, '1', 1) || sinceCal('chinese', n, '1', 1, 3) !== null; },
      greet: [['新年快乐', 'zh'], ['Chúc mừng năm mới', 'vi'], ['새해 복 많이 받으세요', 'ko'], ['Happy Lunar New Year', EN]] },
    { id: 'eidalfitr', strip: 'crescents',
      on: function (n) { return isCal('islamic-umalqura', n + 1, '10', 1) || sinceCal('islamic-umalqura', n, '10', 1, 3) !== null; },
      greet: [['عيد مبارك', 'ar'], ['Eid Mubarak', EN], ['Selamat Hari Raya', 'ms'], ['Bayramınız kutlu olsun', 'tr']] },
    { id: 'eidaladha', strip: 'crescents',
      on: function (n) { return isCal('islamic-umalqura', n + 1, '12', 10) || sinceCal('islamic-umalqura', n, '12', 10, 4) !== null; },
      greet: [['عيد أضحى مبارك', 'ar'], ['Eid al-Adha Mubarak', EN], ['Kurban Bayramınız kutlu olsun', 'tr']] },
    { id: 'holi', strip: 'colour', prop: 'holi',
      on: function (n, y) { return between(n, holi(y) - 1, holi(y) + 1); },
      greet: [['होली की शुभकामनाएँ', 'hi'], ['Happy Holi', EN]] },
    { id: 'nowruz', strip: 'blossom',
      on: function (n) { return isCal('persian', n + 1, '1', 1) || sinceCal('persian', n, '1', 1, 4) !== null; },
      greet: [['نوروز مبارک', 'fa'], ['Happy Nowruz', EN], ['Nowruz bayramıňyz gutly bolsun', 'tk']] },
    { id: 'carnival', strip: 'streamers', prop: 'mask',
      on: function (n, y) { var e = easter(y); return between(n, e - 50, e - 47); },
      greet: [['Feliz Carnaval', 'pt'], ['Happy Carnival', EN], ['Buon Carnevale', 'it']] },
    { id: 'easter', strip: 'bunting', prop: 'bunny',
      on: function (n, y) { var w = easter(y), o = easterOrthodox(y); return between(n, w, w + 1) || between(n, o, o + 1); },
      greet: function (n, y) {
        var o = easterOrthodox(y), w = easter(y);
        var west = [['Happy Easter', EN], ['Frohe Ostern', 'de'], ['Joyeuses Pâques', 'fr'], ['Felices Pascuas', 'es'], ['Buona Pasqua', 'it'], ['Wesołych Świąt', 'pl']];
        var east = [['Христос воскресе!', 'ru'], ['Χριστός ανέστη!', 'el'], ['Hristos a înviat!', 'ro']];
        return between(n, o, o + 1) ? (o === w ? west.concat(east) : east.concat([['Happy Orthodox Easter', EN]])) : west;
      } },
    { id: 'songkran', strip: 'water', prop: 'goggles',
      on: function (n, y, m, d) { return m === 4 && d >= 13 && d <= 15; },
      greet: [['สุขสันต์วันสงกรานต์', 'th'], ['Happy Songkran', EN], ['Happy Thingyan', EN], ['Suosdei Chnam Thmey', 'km']] },
    { id: 'midsummer', strip: 'leaves', prop: 'flowers',
      on: function (n, y) { var f = dn(y, 6, 19) + ((5 - wday(dn(y, 6, 19)) + 7) % 7); return between(n, f, f + 1); },
      greet: [['Glad midsommar', 'sv'], ['Hyvää juhannusta', 'fi'], ['Gode sankthans', 'no'], ['Līgo!', 'lv'], ['Happy Midsummer', EN]] },
    { id: 'sysadmin', prop: 'shades',
      on: function (n, y) { return n === lastWday(y, 7, 5); },
      greet: [['Thank your sysadmin today', EN]] },
    { id: 'linux', prop: 'birthday',
      on: function (n, y, m, d) { return m === 8 && d === 25; },
      greet: function (n, y) { return [['Happy birthday, Linux: ' + (y - 1991) + ' today', EN]]; } },
    { id: 'programmer', strip: 'hex',
      on: function (n, y) { return n === dn(y, 1, 1) + 255; },
      greet: [['Happy Day of the Programmer', EN], ['С Днём программиста', 'ru']] },
    { id: 'midautumn', strip: 'moonlanterns', prop: 'mlantern',
      on: function (n) { return sinceCal('chinese', n + 1, '8', 15, 3) !== null; },
      greet: [['中秋节快乐', 'zh'], ['즐거운 추석 보내세요', 'ko'], ['Tết Trung Thu vui vẻ', 'vi'], ['Happy Mid-Autumn', EN]] },
    { id: 'remembrance', prop: 'poppy',
      on: function (n, y, m, d) { return m === 11 && d === 11; },
      greet: [['Lest we forget', EN], ['Nous nous souviendrons', 'fr']] },
    { id: 'diwali', strip: 'diyas',
      on: function (n, y) { var x = diwali(y); return between(n, x - 3, x + 2); },
      greet: [['शुभ दीपावली', 'hi'], ['Happy Diwali', EN], ['தீபாவளி நல்வாழ்த்துகள்', 'ta'], ['Bandi Chhor Divas', 'pa-Latn']] },
    { id: 'halloween', strip: 'bats', prop: 'witch',
      on: function (n, y, m, d) { return m === 10 && d >= 29; },
      greet: [['Happy Halloween', EN]] },
    { id: 'muertos', strip: 'papel', prop: 'marigold',
      on: function (n, y, m, d) { return m === 11 && d <= 2; },
      greet: [['Día de Muertos', 'es']] },
    // Pride Month: all of June, under every single-day holiday (they keep
    // their day; everyone is still greeted).
    { id: 'pride', strip: 'pride', prop: 'pride',
      on: function (n, y, m) { return m === 6; },
      greet: [['Happy Pride', EN], ['Feliz Orgullo', 'es'], ['Bonne fierté', 'fr'], ['Feliz Orgulho', 'pt'], ['Fijne Pride', 'nl'], ['Buon Pride', 'it'], ['Happy Pride Month', EN]] }
  ];

  /* December's observances, each drawn the way its own tradition draws it, at
   * the end of the string of lights. Several can share a day. */
  var KWANZAA = [['Umoja', 'unity'], ['Kujichagulia', 'self-determination'], ['Ujima', 'collective work and responsibility'],
    ['Ujamaa', 'cooperative economics'], ['Nia', 'purpose'], ['Kuumba', 'creativity'], ['Imani', 'faith']];
  var SCENES = [
    { id: 'hanukkah', tint: 'hanukkah',
      // Candles are lit at nightfall: the day of 24 Kislev shows the first, and
      // the eighth stays up through the last day.
      on: function (n) { var s = sinceCal('hebrew', n, 'Kislev', 24, 9); return s === null ? false : Math.min(8, n - s + 1); },
      greet: [['חג חנוכה שמח', 'he'], ['Happy Hanukkah', EN]] },
    { id: 'sinterklaas', on: function (n, y, m, d) { return m === 12 && (d === 5 || d === 6); },
      greet: [['Fijne Sinterklaas', 'nl'], ['Happy Sinterklaas', EN]] },
    { id: 'bodhi', on: function (n, y, m, d) { return m === 12 && d === 8; },
      greet: [['Happy Bodhi Day', EN], ['成道会', 'ja']] },
    { id: 'lucia', on: function (n, y, m, d) { return m === 12 && d === 13; },
      greet: [['Glad Lucia', 'sv'], ['Happy St Lucia’s Day', EN]] },
    { id: 'posadas', on: function (n, y, m, d) { return m === 12 && d >= 16 && d <= 24; },
      greet: [['¡Felices Posadas!', 'es']] },
    { id: 'yalda', on: function (n) { return isCal('persian', n, '9', 30); },
      greet: [['شب یلدا مبارک', 'fa'], ['Happy Yalda', EN]] },
    { id: 'dongzhi', on: function (n, y) { return n === solstice(y); },
      greet: [['冬至快乐', 'zh'], ['Happy solstice', EN]] },
    { id: 'christmas', on: function (n, y, m, d) { return m === 12 && d >= 24 && d <= 26; },
      greet: [['Merry Christmas', EN], ['Feliz Navidad', 'es'], ['Joyeux Noël', 'fr'], ['Frohe Weihnachten', 'de'],
        ['Buon Natale', 'it'], ['Feliz Natal', 'pt'], ['God jul', 'sv'], ['Wesołych Świąt', 'pl'], ['Vrolijk kerstfeest', 'nl'],
        ['Nollaig Shona', 'ga'], ['Nadolig Llawen', 'cy'], ['Maligayang Pasko', 'fil'], ['Selamat Hari Natal', 'id'],
        ['メリークリスマス', 'ja'], ['圣诞快乐', 'zh'], ['메리 크리스마스', 'ko']] },
    { id: 'kwanzaa',
      on: function (n, y, m, d) { return m === 12 && d >= 26 ? d - 25 : (m === 1 && d === 1 ? 7 : false); },
      greet: function (n, y, k) { return [['Habari gani? ' + KWANZAA[k - 1][0], 'sw'], ['Kwanzaa: ' + KWANZAA[k - 1][1], EN]]; } },
    { id: 'reyes', on: function (n, y, m, d) { return m === 1 && d === 6; },
      greet: [['¡Feliz Día de Reyes!', 'es'], ['Happy Epiphany', EN]] },
    { id: 'genna', on: function (n, y, m, d) { return m === 1 && d === 7; },
      greet: [['С Рождеством Христовым', 'ru'], ['Srećan Božić', 'sr-Latn'], ['መልካም ገና', 'am'], ['Merry Christmas', EN]] }
  ];

  // What shows at time t (ms): the holiday (or null) and December's scenes.
  // A forced holiday id stands in for the date's own.
  // ok (optional) says which holiday and scene ids the site wants.
  function pick(t, forced, ok) {
    ok = ok || function () { return true; };
    var lt = new Date(t), n = dn(lt.getFullYear(), lt.getMonth() + 1, lt.getDate()), x = ymd(n);
    var hol = null, scenes = [], i, v;
    var also = [];  // every holiday today: the first dresses the page, all of them are greeted
    for (i = 0; i < HOLIDAYS.length; i++)
      if (forced ? HOLIDAYS[i].id === forced : ok(HOLIDAYS[i].id) && HOLIDAYS[i].on(n, x[0], x[1], x[2], t)) { if (!hol) hol = HOLIDAYS[i]; also.push(HOLIDAYS[i]); }
    for (i = 0; i < SCENES.length; i++) {
      v = ok(SCENES[i].id) && SCENES[i].on(n, x[0], x[1], x[2], t);
      if (v) scenes.push({ s: SCENES[i], v: v });
    }
    var greet = [];
    function add(g, k) { greet = greet.concat(typeof g === 'function' ? g(n, x[0], k) : g); }
    for (i = 0; i < scenes.length; i++) add(scenes[i].s.greet, scenes[i].v);
    for (i = 0; i < also.length; i++) if (also[i].greet) add(also[i].greet);
    return { n: n, hol: hol, scenes: scenes, greet: greet };
  }
