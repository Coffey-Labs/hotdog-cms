# Shop pack

A small online shop for a HotDog CMS site: a page for each product with its
photos, price, description and options, a shop page at /products/, and a
"Products" section for any page. Each Buy button opens your payment
provider's own checkout (a Stripe Payment Link, a PayPal or Square checkout
link), which takes the payment, the address, any tax and sends the receipt.

    hotdog-cms pack add shop

There's no cart: each product (or option) has its own checkout, which suits
a shop with a handful of things to sell. Card details never touch your site,
and nothing from the provider loads until someone chooses Buy. Products are
also written as schema.org Product data, with the price and whether they're
in stock, for search engines.
