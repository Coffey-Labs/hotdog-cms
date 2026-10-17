package build

import (
	_ "embed"
	"html/template"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/site"
)

// The cart (site.yaml cart: true): a script that keeps what a visitor picks
// in their own browser and opens the shop's checkout with all of it. The
// shop pack's product forms work without it; with it they gain Add to cart.

var (
	//go:embed static/hotdog-cart.js
	cartScript []byte
	//go:embed static/hotdog-cart.css
	cartStyle []byte
)

// cartTags is the cart's stylesheet and script, for a theme's head: {{ cart }}.
func cartTags(cfg *site.Config, a *Assets) template.HTML {
	if !cfg.Cart.On() {
		return ""
	}
	js, _ := a.URL("hotdog-cart.js")
	css, _ := a.URL("hotdog-cart.css")
	return template.HTML(`<link rel="stylesheet" href="` + template.HTMLEscapeString(css) + `">` +
		`<script src="` + template.HTMLEscapeString(js) + `" defer data-page="` + template.HTMLEscapeString(cfg.Cart.PagePath()) + `"></script>`)
}

// cartLink is a link to the cart with how many things are in it, shown once
// there's something: {{ cartLink }} in a theme's header.
func cartLink(cfg *site.Config) template.HTML {
	if !cfg.Cart.On() {
		return ""
	}
	return template.HTML(`<a class="cart-link" href="` + template.HTMLEscapeString(cfg.Cart.PagePath()) + `" data-cart-link hidden>Cart <span data-cart-count>0</span></a>`)
}
