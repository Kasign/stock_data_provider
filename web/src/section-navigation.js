export function setupSectionNavigation(options) {
  const { document, window, IntersectionObserver, linkSelector, dataKey } = options || {};
  if (!document?.querySelectorAll || !document?.getElementById || !window) return;
  const links = Array.from(document.querySelectorAll(linkSelector));
  if (!links.length) return;
  const setActive = (targetId) => {
    links.forEach((link) => {
      const active = link.dataset[dataKey] === targetId;
      link.classList.toggle("is-active", active);
      if (active) link.setAttribute("aria-current", "location");
      else link.removeAttribute("aria-current");
    });
  };
  const targets = links.map((link) => document.getElementById(link.dataset[dataKey] || "")).filter(Boolean);
  links.forEach((link) => {
    link.addEventListener("click", () => {
      const targetId = link.dataset[dataKey] || "";
      const target = targetId ? document.getElementById(targetId) : null;
      if (!target) return;
      setActive(targetId);
      const reduceMotion = Boolean(window.matchMedia?.("(prefers-reduced-motion: reduce)").matches);
      target.scrollIntoView({ behavior: reduceMotion ? "auto" : "smooth", block: "start" });
      const focusTarget = target.matches("h2, h3, h4") ? target : target.querySelector("h2, h3, h4") || target;
      if (!focusTarget.hasAttribute("tabindex")) focusTarget.setAttribute("tabindex", "-1");
      focusTarget.focus({ preventScroll: true });
    });
  });
  if (typeof IntersectionObserver !== "function" || !targets.length) return;
  const observer = new IntersectionObserver(() => {
    const referenceLine = Math.min(window.innerHeight * 0.28, 220);
    const current = targets
      .map((target) => ({ target, rect: target.getBoundingClientRect() }))
      .filter(({ target, rect }) => target.getClientRects().length > 0 && rect.bottom >= 120 && rect.top <= window.innerHeight - 120)
      .sort((a, b) => Math.abs(a.rect.top - referenceLine) - Math.abs(b.rect.top - referenceLine))[0];
    if (current) setActive(current.target.id);
  }, { rootMargin: "-120px 0px -120px 0px", threshold: 0 });
  targets.forEach((target) => observer.observe(target));
}
