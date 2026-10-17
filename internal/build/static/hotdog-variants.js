// A product with its sizes grouped by color ({{ variantPicker }}): a
// drop-down picks the color, and only that color's sizes and photos show.
// Without this script the page lists every color's sizes and photos, and
// works the same.
(() => {
  const select = document.querySelector("[data-group-select]");
  const form = select && select.closest("form");
  if (!form) return;
  const sets = [...form.querySelectorAll("fieldset.variants[data-group]")];
  const slides = [...document.querySelectorAll(".photo-slide")];
  const picks = [...document.querySelectorAll(".photo-pick")];

  const show = (group) => {
    // The size chosen in the color being left, to choose again in the new one.
    const was = form.querySelector("input[data-option]:checked");
    const size = was ? was.dataset.option : "";
    for (const set of sets) {
      const on = set.dataset.group === group;
      // A hidden color's sizes are disabled, so they're never sent or checked.
      set.disabled = !on;
      if (!on) {
        set.hidden = true;
        continue;
      }
      const sizes = [...set.querySelectorAll("input[data-option]")];
      const again = sizes.find((i) => i.dataset.option === size && !i.disabled);
      if (again) again.checked = true;
      else if (was) was.checked = false;
      // A color with one choice needs no row of its own: the drop-down is it.
      if (sizes.length === 1 && !sizes[0].disabled) sizes[0].checked = true;
      set.hidden = sizes.length === 1;
    }
    // Photos of this color, and those of no one color.
    let first = null;
    picks.forEach((pick, i) => {
      const g = pick.dataset.group;
      const on = !g || g === group;
      pick.hidden = !on;
      if (slides[i]) slides[i].hidden = !on;
      if (on && !first && g) first = pick;
    });
    first = first || picks.find((p) => !p.hidden);
    if (first) first.querySelector("input").checked = true;
  };

  for (const set of sets) {
    const legend = set.querySelector("legend");
    if (legend) legend.textContent = select.dataset.choose;
  }
  select.closest("[data-group-pick]").hidden = false;
  // Coming back to the page, the browser may have kept a size: keep its color.
  const kept = form.querySelector("input[data-option]:checked");
  const keptSet = kept && kept.closest("fieldset[data-group]");
  if (keptSet) select.value = keptSet.dataset.group;
  select.addEventListener("change", () => show(select.value));
  show(select.value);
})();
