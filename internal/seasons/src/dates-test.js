// Lists every holiday and scene day for some years (run with TZ=UTC).
const fs = require('fs');
const api = new Function(fs.readFileSync(__dirname + '/core.js', 'utf8') + '; return {pick, holi, diwali, solstice, easter, easterOrthodox, dn, ymd};')();
const years = process.argv.slice(2).map(Number);
for (const y of years) {
  const runs = {};
  for (let n = api.dn(y, 1, 1); n <= api.dn(y, 12, 31); n++) {
    const r = api.pick(n * 864e5 + 12 * 36e5);
    const tag = (r.hol ? r.hol.id : '') ;
    const add = (k, v) => { (runs[k] = runs[k] || []).push(v); };
    const iso = new Date(n * 864e5).toISOString().slice(5, 10);
    if (tag) add(tag, iso);
    for (const s of r.scenes) add('·' + s.s.id, iso + (s.v === true ? '' : '(' + s.v + ')'));
  }
  console.log('== ' + y);
  for (const [k, v] of Object.entries(runs)) console.log(k.padEnd(14), v.length > 8 ? v[0] + ' … ' + v[v.length - 1] + ' (' + v.length + ')' : v.join(' '));
}
