# Location and hours pack

Where to find you and when you're open, for a HotDog CMS site: the address
with a link to it on OpenStreetMap, your phone and email, opening hours as a
table, and a note for holidays and changes.

    hotdog-cms pack add location-and-hours

Keep the details in `data/location.yaml` and show them with the "Location and
hours" section on any page, or with the `location-hours` partial anywhere in
a layout, such as the footer. Change the file once, and every place follows.

The map is a link, not an embedded map: an embed would load another
company's script on every page and need a mention in your privacy notice.
