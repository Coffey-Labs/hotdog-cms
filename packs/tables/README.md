# Tables pack

Tables for a HotDog CMS site, the way people keep them: in a spreadsheet.

    hotdog-cms pack add tables

Save the sheet as CSV into `data/` (`data/prices.csv`), headings in the first
row, and add a **Table** section with `from: prices`. Update the table by
saving the sheet again.

It also reads YAML or JSON from `data/`: a list of rows, each a list of
cells, or a list of records with `columns:` naming the keys to show, in
order. A short table can be typed into the section, one row per line with
cells split by `|`.

- A caption, for everyone and for screen readers.
- Row headings (the first column names each row), numbers lined up on the
  right, compact rows, and a note below.
- On a phone, a wide table scrolls sideways inside its frame instead of
  stretching the page; it can be scrolled with the keyboard too.
- Cells can hold Markdown links and emphasis, and follow the site's Markdown
  rules: HTML in a cell is left out unless the site allows HTML in pages.

No sorting or filtering: those need a script, and packs carry none.
