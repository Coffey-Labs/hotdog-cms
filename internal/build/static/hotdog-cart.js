// HotDog CMS cart: what a visitor picks is kept in their own browser
// (localStorage) and sent nowhere until they choose Check out, which opens
// the shop's checkout with everything in it. Product forms (the shop pack's,
// marked data-cart) still work without this script: Buy now checks out the
// one thing. With it, Add to cart appears beside Buy now.
(() => {
  const me = document.currentScript
  if (!me) return
  const page = me.dataset.page || '/cart/'
  const KEY = 'hotdog-cart'
  const empty = () => ({ v: 1, checkout: null, items: [] })
  const load = () => {
    try {
      const c = JSON.parse(localStorage.getItem(KEY))
      if (c && c.v === 1 && Array.isArray(c.items)) return c
    } catch (e) {}
    return empty()
  }
  const save = (c) => {
    if (!c.items.length) c = empty()
    try { localStorage.setItem(KEY, JSON.stringify(c)) } catch (e) {}
    update(c)
  }
  const count = (c) => c.items.reduce((n, i) => n + i.qty, 0)
  const money = (n, cur) => {
    try { return new Intl.NumberFormat(document.documentElement.lang || undefined, { style: 'currency', currency: cur || 'USD' }).format(n) } catch (e) { return n.toFixed(2) }
  }
  const el = (tag, attrs, ...kids) => {
    const e = document.createElement(tag)
    for (const k in attrs || {}) {
      if (k === 'text') e.textContent = attrs[k]
      else if (k.startsWith('on')) e.addEventListener(k.slice(2), attrs[k])
      else e.setAttribute(k, attrs[k])
    }
    for (const kid of kids) if (kid) e.append(kid)
    return e
  }

  // The address that checks out the whole cart: one id:quantity per item,
  // joined, in the field the checkout reads.
  const checkoutURL = (c) => {
    const co = c.checkout
    const enc = (s) => encodeURIComponent(s).replace(/%3A/g, ':').replace(/%2C/g, ',')
    const items = c.items.map((i) => co.item.replace('{id}', i.id).replace('{qty}', i.qty)).join(co.join)
    const q = [enc(co.field) + '=' + enc(items)]
    for (const k in co.fields || {}) q.push(enc(k) + '=' + enc(co.fields[k]))
    return co.action + (co.action.includes('?') ? '&' : '?') + q.join('&')
  }

  // The count in the header, wherever the theme put {{ cartLink }}.
  const update = (c) => {
    const n = count(c)
    document.querySelectorAll('[data-cart-count]').forEach((e) => { e.textContent = n })
    document.querySelectorAll('[data-cart-link]').forEach((e) => { e.hidden = n === 0 })
    const view = document.querySelector('[data-cart-view]')
    if (view) render(view, c)
  }

  // Product forms: Add to cart beside Buy now.
  document.querySelectorAll('form[data-cart]').forEach((form) => {
    const add = form.querySelector('[data-cart-add]')
    if (!add) return
    add.hidden = false
    add.addEventListener('click', () => {
      if (!form.reportValidity()) return
      const pick = form.querySelector('[data-cart-variant]:checked') || form.querySelector('input[type=hidden][data-cart-variant]')
      if (!pick) return
      const d = form.dataset
      const fields = {}
      form.querySelectorAll('input[type=hidden]').forEach((h) => { if (h.name && h.name !== d.field) fields[h.name] = h.value })
      const co = { action: form.action, field: d.field, item: d.item || '{id}:{qty}', join: d.join || ',', fields }
      const c = load()
      if (c.checkout && c.checkout.action !== co.action) {
        status(form, 'This comes from a different shop than what’s in your cart. Check out your cart first, then add it.')
        return
      }
      c.checkout = co
      const id = pick.dataset.id
      const have = c.items.find((i) => i.id === id)
      if (have) have.qty = Math.min(have.qty + 1, 99)
      else c.items.push({ id, qty: 1, title: d.title, variant: pick.dataset.name || '', price: parseFloat(pick.dataset.price) || 0, currency: d.currency || 'USD', image: d.image || '', href: d.href || location.pathname })
      save(c)
      status(form, 'Added to your cart.', true)
    })
  })

  const status = (form, text, link) => {
    let s = form.querySelector('.cart-added')
    if (!s) {
      s = el('p', { class: 'cart-added', role: 'status' })
      form.append(s)
    }
    s.replaceChildren(text)
    if (link) s.append(' ', el('a', { href: page, text: 'View your cart (' + count(load()) + ')' }))
  }

  // The cart page.
  const render = (view, c) => {
    const d = view.dataset
    const list = el('ul', { class: 'cart-items' })
    let total = 0
    let cur = 'USD'
    for (const i of c.items) {
      total += i.price * i.qty
      cur = i.currency || cur
      const set = (q) => {
        const now = load()
        const it = now.items.find((x) => x.id === i.id)
        if (!it) return
        it.qty = Math.max(0, Math.min(q, 99))
        now.items = now.items.filter((x) => x.qty > 0)
        save(now)
      }
      list.append(el('li', { class: 'cart-item' },
        i.image ? el('img', { src: i.image, alt: '', width: '72', height: '96', loading: 'lazy' }) : null,
        el('div', { class: 'cart-what' },
          el('a', { href: i.href, text: i.title }),
          i.variant ? el('span', { class: 'cart-variant', text: i.variant }) : null),
        el('div', { class: 'cart-qty', role: 'group', 'aria-label': 'How many ' + i.title + (i.variant ? ', ' + i.variant : '') },
          el('button', { type: 'button', 'aria-label': 'One fewer', text: '−', onclick: () => set(i.qty - 1) }),
          el('span', { 'aria-live': 'polite', text: String(i.qty) }),
          el('button', { type: 'button', 'aria-label': 'One more', text: '+', onclick: () => set(i.qty + 1) })),
        el('span', { class: 'cart-line', text: money(i.price * i.qty, i.currency) }),
        el('button', { type: 'button', class: 'cart-remove', text: 'Remove', onclick: () => set(0) })))
    }
    if (!c.items.length) {
      view.replaceChildren(el('p', { class: 'cart-empty', text: 'Your cart is empty.' }),
        d.shop ? el('p', {}, el('a', { class: 'btn', href: d.shop, text: 'Browse the shop' })) : null)
    } else {
      view.replaceChildren(list,
        el('div', { class: 'cart-total' },
          el('p', {}, 'Subtotal ', el('strong', { text: money(total, cur) })),
          el('p', { class: 'cart-note', text: 'Shipping and any tax are added at checkout, where the prices are confirmed.' }),
          el('a', { class: 'btn primary cart-checkout', href: checkoutURL(c), rel: 'noopener', text: 'Check out' })))
    }
    suggest(c)
  }

  // Suggestions: products not in the cart, those sharing a tag with
  // something in it first, in the shop's own order.
  const suggest = (c) => {
    const box = document.querySelector('[data-cart-suggest]')
    if (!box) return
    const want = parseInt(box.dataset.count, 10) || 4
    const inCart = new Set(c.items.map((i) => i.href))
    const cards = [...box.querySelectorAll('[data-product]')]
    const tagsOf = (e) => (e.dataset.tags || '').split(',').filter(Boolean)
    const cartTags = new Set(cards.filter((e) => inCart.has(e.dataset.product)).flatMap(tagsOf))
    const pick = cards.filter((e) => !inCart.has(e.dataset.product))
      .map((e, n) => ({ e, n, score: tagsOf(e).some((t) => cartTags.has(t)) ? 0 : 1 }))
      .sort((a, b) => a.score - b.score || a.n - b.n)
      .slice(0, want).map((x) => x.e)
    cards.forEach((e) => { e.hidden = !pick.includes(e) })
    box.hidden = pick.length === 0
  }

  window.addEventListener('storage', (e) => { if (e.key === KEY) update(load()) })
  update(load())
})()
