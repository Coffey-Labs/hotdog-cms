// Theme and phone menu. Loaded in <head> so the saved theme applies before the
// page paints; the rest waits for the document.
(() => {
  const root = document.documentElement;
  const read = () => { try { return localStorage.getItem("theme") || "auto"; } catch { return "auto"; } };
  const apply = (t) => {
    if (t === "auto") root.removeAttribute("data-theme"); else root.setAttribute("data-theme", t);
    document.querySelectorAll("[data-theme-set]").forEach((b) => b.setAttribute("aria-pressed", String(b.dataset.themeSet === t)));
  };
  apply(read());
  document.addEventListener("DOMContentLoaded", () => {
    apply(read());
    document.querySelectorAll("[data-theme-set]").forEach((b) => b.addEventListener("click", () => {
      const t = b.dataset.themeSet;
      try { localStorage.setItem("theme", t); } catch {}
      apply(t);
    }));
    const btn = document.querySelector(".menu-btn");
    const nav = document.getElementById("site-nav");
    if (btn && nav) btn.addEventListener("click", () => {
      const open = nav.classList.toggle("open");
      btn.setAttribute("aria-expanded", String(open));
    });
  });
})();
