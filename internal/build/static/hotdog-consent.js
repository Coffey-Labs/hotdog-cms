// HotDog CMS consent: visit counting starts only after a visitor agrees.
// The banner is a region at the foot of the page, not a wall: the page works
// without answering it. Decline comes first and looks the same as Accept.
// Declining (now or later) removes the counter's cookies. A browser that
// sends Global Privacy Control has already said no. Any element with
// data-hotdog-consent-open reopens the choice ("Cookie settings").
(() => {
  const me = document.currentScript
  if (!me) return
  const c = me.dataset
  const KEY = 'hotdog-consent'
  const get = () => { try { return localStorage.getItem(KEY) } catch (e) { return null } }
  const put = (v) => { try { localStorage.setItem(KEY, v) } catch (e) {} }
  const names = { plausible: 'Plausible', matomo: 'Matomo', ga4: 'Google Analytics' }
  let started = false

  const script = (src, attrs) => {
    const s = document.createElement('script')
    s.async = true
    s.src = src
    for (const k in attrs) s.setAttribute(k, attrs[k])
    document.head.appendChild(s)
  }
  const start = () => {
    if (started) return
    started = true
    if (c.provider === 'plausible') script(c.src || 'https://plausible.io/js/script.js', { 'data-domain': c.domain })
    if (c.provider === 'matomo') {
      const u = c.url.replace(/\/?$/, '/')
      const q = (window._paq = window._paq || [])
      q.push(['setTrackerUrl', u + 'matomo.php'], ['setSiteId', c.site], ['trackPageView'], ['enableLinkTracking'])
      script(u + 'matomo.js', {})
    }
    if (c.provider === 'ga4') {
      window.dataLayer = window.dataLayer || []
      window.gtag = function () { window.dataLayer.push(arguments) }
      window.gtag('js', new Date())
      window.gtag('config', c.id)
      script('https://www.googletagmanager.com/gtag/js?id=' + encodeURIComponent(c.id), {})
    }
  }
  // The counters' cookies, on this host and the domains above it.
  const forget = () => {
    const parts = location.hostname.split('.')
    for (const raw of document.cookie.split(';')) {
      const n = raw.split('=')[0].trim()
      if (!/^(_ga|_gid|_gat|_pk_|MATOMO_|mtm_|_hj|_gcl)/.test(n)) continue
      document.cookie = n + '=; Max-Age=0; path=/'
      for (let i = 0; i < parts.length - 1; i++) document.cookie = n + '=; Max-Age=0; path=/; domain=.' + parts.slice(i).join('.')
    }
  }

  let box = null
  const hide = () => { if (box) { box.remove(); box = null } }
  const choose = (v) => {
    const wasOn = started
    put(v)
    hide()
    if (v === 'yes') start()
    else {
      forget()
      if (wasOn) location.reload() // a counter already running can't be unloaded
    }
  }
  const show = (focus) => {
    if (box) return
    box = document.createElement('div')
    box.className = 'hotdog-consent'
    box.setAttribute('role', 'region')
    box.setAttribute('aria-label', 'Visit statistics')
    const p = document.createElement('p')
    p.textContent = 'May this site count your visit with ' + (names[c.provider] || 'its statistics') + '? Nothing is counted unless you agree. '
    if (c.privacy) {
      const a = document.createElement('a')
      a.href = c.privacy
      a.textContent = 'Privacy notice'
      p.appendChild(a)
    }
    const row = document.createElement('div')
    row.className = 'hotdog-consent-row'
    for (const [v, label] of [['no', 'Decline'], ['yes', 'Accept']]) {
      const b = document.createElement('button')
      b.type = 'button'
      b.textContent = label
      b.addEventListener('click', () => choose(v))
      row.appendChild(b)
    }
    box.append(p, row)
    document.body.appendChild(box)
    if (focus) row.firstChild.focus()
  }

  document.addEventListener('click', (e) => {
    const t = e.target.closest && e.target.closest('[data-hotdog-consent-open]')
    if (!t || !c.provider) return
    e.preventDefault()
    show(true)
  })
  document.addEventListener('keydown', (e) => { if (e.key === 'Escape' && box && box.contains(document.activeElement)) hide() })

  if (!c.provider) return
  if (c.gated === 'false') return start() // the site chose to count without asking
  const v = get()
  if (v === 'yes') start()
  else if (v === null && navigator.globalPrivacyControl !== true) show(false)
})()
