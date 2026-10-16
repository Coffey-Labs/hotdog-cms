# Shop pack

A small online shop for a HotDog CMS site: a page for each product with its
photos, price, description and options, a shop page at /products/, and a
"Products" section for any page. Each Buy button opens your payment
provider's own checkout (a Stripe Payment Link, a PayPal or Square checkout
link), which takes the payment, the address, any tax and sends the receipt.

    hotdog-cms pack add shop

Each product (or option) has its own checkout, which suits a shop with a
handful of things to sell; a cart is optional (below). Card details never
touch your site,
and nothing from the provider loads until someone chooses Buy. Products are
also written as schema.org Product data, with the price and whether they're
in stock, for search engines.

Sizes and colors are variants, each with its own price and checkout. A
provider whose checkout reads the choice from its address, as Fourthwall's
does, gets a size picker that goes straight to checkout, still without a
script. `hotdog-cms import fourthwall` writes the whole catalog from a
Fourthwall shop (docs/importing.md).

When the checkout takes several things at once, the shop can have a cart:
`cart: true` in site.yaml, `{{ cart }}` in the theme's head, `{{ cartLink }}`
in its header, and a page with `layout: cart`. Product pages then have Add
to cart beside Buy now. The cart is kept in the visitor's own browser and
reaches the shop only at checkout; its page suggests other products, those
sharing a tag with what's in the cart first. Without JavaScript, Buy now
still works.

Tags make shelves. A page at `content/tags/<tag>.md` with `layout: shelf`
gives `/tags/<tag>/` its own title and words above that tag's products, in
the shop's order, and any other front matter the theme wants to read. The
"Products" section's tag field shows one tag's products on any page.
