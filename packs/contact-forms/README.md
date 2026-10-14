# Contact forms pack

Ready-made forms for a HotDog CMS site, and a section to put them on a page.

    hotdog-cms pack add contact-forms

- `forms/quote.yaml`: a quote request (what, by when, budget, details).
- `forms/booking.yaml`: a booking request (day, time of day, how many). It
  asks; you confirm by email.
- `forms/signup.yaml`: a mailing list sign-up, with a required "yes, email
  me" box.
- The **Contact form** section shows any form in `forms/` beside your email,
  phone, address and a note, such as your opening hours or how soon you
  answer. It works with the contact form every site starts with, too.

The forms are copied into the site once and are the site's own from then on:
upgrading or removing the pack leaves them alone. They send to
`you@example.org` until you set `to:` and `mailto:` to your own address, and
`hotdog-cms check` warns until you do. Forms are sent by the endpoint
(`hotdog-cms endpoint`), with its spam checks; see HotDog's `docs/forms.md`.

A pack's forms always send to a placeholder address like that one, never to
an address the pack's author chose, and can't load another site's script, so
adding a pack can't send your visitors' messages anywhere you didn't pick.
