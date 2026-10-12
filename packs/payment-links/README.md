# Payment links pack

Buy, book and donate buttons for a HotDog CMS site that open your payment
provider's own checkout page:

- Stripe Payment Links
- PayPal (PayPal.me, or a hosted button's link)
- Square online checkout links
- Ko-fi, Liberapay, Open Collective, Buy Me a Coffee, Gumroad, Lemon Squeezy
  and anything else with a link to pay

    hotdog-cms pack add payment-links

Two sections: **Buy buttons** (cards with a picture, a price and a button)
and **Donate** (one call to give).

People pay on the provider's page, so card details never touch your site,
and nothing from the provider loads until someone chooses a button: no
script, nothing to ask consent for. Checkout embedded in your own page needs
the provider's script running on your site, which a pack can't carry.
Mention your provider in your privacy notice (`privacy: extra:` in
`site.yaml`), since buyers' details go to them.
