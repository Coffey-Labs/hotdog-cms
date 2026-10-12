# Menu and prices pack

Two sections for a HotDog CMS site:

- **Price list**: a menu, or a list of services, in groups, with
  descriptions, prices and small tags such as "vegan" or "new".
- **Plans**: plans or packages side by side, each with a price, what it
  includes and a button, and one of them marked as the one most people pick.

    hotdog-cms pack add menu-and-prices

Prices are written as text, so any currency and format works ("$8",
"from €40", "Ask us"). Either section can read its list from a file in
`data/`, so a menu can be changed in one place and shown on several pages.
