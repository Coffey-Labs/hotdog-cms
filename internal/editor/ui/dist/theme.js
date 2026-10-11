// Applies the theme someone picked before the page draws, so it doesn't
// flash the other one first. A file, not inline: the editor's CSP allows no
// inline scripts. "system" (or nothing stored) follows the device.
try {
  var t = localStorage.getItem('hotdog-theme')
  if (t === 'light' || t === 'dark') document.documentElement.dataset.theme = t
} catch (e) {}
