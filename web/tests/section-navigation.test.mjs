import test from "node:test";
import assert from "node:assert/strict";
import { setupSectionNavigation } from "../src/section-navigation.js";

function classList() {
  const values = new Set();
  return {
    toggle(name, force) { if (force) values.add(name); else values.delete(name); },
    contains(name) { return values.has(name); }
  };
}
function link(id) {
  const handlers = {}, attributes = {};
  return {
    dataset: { sectionTarget: id }, classList: classList(), handlers, attributes,
    addEventListener(name, callback) { handlers[name] = callback; },
    setAttribute(name, value) { attributes[name] = value; },
    removeAttribute(name) { delete attributes[name]; }
  };
}
function target(id, top = 180) {
  const attributes = {}, calls = { scroll: [], focus: [] };
  const heading = {
    hasAttribute(name) { return Object.hasOwn(attributes, name); },
    setAttribute(name, value) { attributes[name] = value; },
    focus(options) { calls.focus.push(options); }
  };
  return {
    id, calls, attributes,
    matches() { return false; },
    querySelector() { return heading; },
    scrollIntoView(options) { calls.scroll.push(options); },
    getClientRects() { return [{}]; },
    getBoundingClientRect() { return { top, bottom: top + 500 }; }
  };
}

test("click scrolls and focuses target heading while updating active link", () => {
  const first = link("overview"), second = link("details");
  const overview = target("overview"), details = target("details");
  const document = {
    querySelectorAll() { return [first, second]; },
    getElementById(id) { return ({ overview, details })[id] || null; }
  };
  setupSectionNavigation({
    document, window: { innerHeight: 800, matchMedia: () => ({ matches: false }) },
    IntersectionObserver: null, linkSelector: "[data-section-target]", dataKey: "sectionTarget"
  });
  second.handlers.click();
  assert.deepEqual(details.calls.scroll, [{ behavior: "smooth", block: "start" }]);
  assert.deepEqual(details.calls.focus, [{ preventScroll: true }]);
  assert.equal(details.attributes.tabindex, "-1");
  assert.equal(second.classList.contains("is-active"), true);
  assert.equal(second.attributes["aria-current"], "location");
  assert.equal(first.classList.contains("is-active"), false);
});

test("honors reduced motion and safely skips missing targets", () => {
  const item = link("missing");
  const document = { querySelectorAll() { return [item]; }, getElementById() { return null; } };
  setupSectionNavigation({
    document, window: { innerHeight: 600, matchMedia: () => ({ matches: true }) },
    IntersectionObserver: null, linkSelector: "[data-section-target]", dataKey: "sectionTarget"
  });
  assert.doesNotThrow(() => item.handlers.click());
  assert.equal(item.attributes["aria-current"], undefined);
});

test("observer highlights the nearest visible target", () => {
  const first = link("overview"), second = link("details");
  const overview = target("overview", 500), details = target("details", 210);
  let callback = null;
  class Observer {
    constructor(handler) { callback = handler; }
    observe() {}
  }
  const document = {
    querySelectorAll() { return [first, second]; },
    getElementById(id) { return ({ overview, details })[id] || null; }
  };
  setupSectionNavigation({
    document, window: { innerHeight: 800 }, IntersectionObserver: Observer,
    linkSelector: "[data-section-target]", dataKey: "sectionTarget"
  });
  callback([]);
  assert.equal(second.classList.contains("is-active"), true);
  assert.equal(second.attributes["aria-current"], "location");
  assert.equal(first.attributes["aria-current"], undefined);
});
