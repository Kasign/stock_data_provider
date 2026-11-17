export function setupSettingsNavigation(document) {
  if (!document?.querySelector) return;
  const nav = document.querySelector(".settings-nav");
  if (!nav || nav.dataset.enhanced === "true") return;
  nav.dataset.enhanced = "true";

  const tools = document.createElement("div");
  tools.className = "settings-nav-tools";
  const searchLabel = document.createElement("label");
  searchLabel.className = "settings-nav-search";
  const searchHint = document.createElement("span");
  searchHint.className = "sr-only";
  searchHint.textContent = "搜索设置分组";
  const searchInput = document.createElement("input");
  searchInput.type = "search";
  searchInput.id = "settings-section-search";
  searchInput.placeholder = "搜索设置分组";
  searchInput.autocomplete = "off";
  searchLabel.append(searchHint, searchInput);

  const actions = document.createElement("div");
  actions.className = "settings-nav-actions";
  const expandAll = document.createElement("button");
  expandAll.type = "button";
  expandAll.dataset.settingsExpandAll = "";
  expandAll.textContent = "全部展开";
  const collapseAll = document.createElement("button");
  collapseAll.type = "button";
  collapseAll.dataset.settingsCollapseAll = "";
  collapseAll.textContent = "全部收起";
  actions.append(expandAll, collapseAll);

  const resultStatus = document.createElement("p");
  resultStatus.className = "settings-nav-result";
  resultStatus.id = "settings-nav-result";
  resultStatus.setAttribute("aria-live", "polite");
  tools.append(searchLabel, actions, resultStatus);
  nav.prepend(tools);

  const links = Array.from(nav.querySelectorAll(".settings-nav-link"));
  const sections = links.map((link) => document.getElementById(link.dataset.settingsTarget)).filter(Boolean);
  sections.forEach((section) => {
    const heading = section.querySelector(":scope > .settings-section-heading");
    if (!heading || heading.querySelector("[data-settings-section-toggle]")) return;
    const toggle = document.createElement("button");
    toggle.type = "button";
    toggle.className = "settings-section-toggle";
    toggle.dataset.settingsSectionToggle = "true";
    toggle.setAttribute("aria-expanded", "true");
    toggle.textContent = "收起内容";
    toggle.setAttribute("aria-label", "收起" + (heading.querySelector("h2")?.textContent || "设置") + "内容");
    heading.append(toggle);
  });

  const setCollapsed = (section, collapsed) => {
    const heading = section.querySelector(":scope > .settings-section-heading");
    const toggle = heading?.querySelector("[data-settings-section-toggle]");
    if (!heading || !toggle) return;
    Array.from(section.children).filter((child) => child !== heading).forEach((child) => {
      child.hidden = collapsed;
    });
    toggle.setAttribute("aria-expanded", String(!collapsed));
    toggle.textContent = collapsed ? "展开内容" : "收起内容";
    toggle.setAttribute("aria-label", (collapsed ? "展开" : "收起") +
      (heading.querySelector("h2")?.textContent || "设置") + "内容");
    section.classList.toggle("is-collapsed", collapsed);
  };

  nav.addEventListener("click", (event) => {
    const toggle = event.target.closest("[data-settings-section-toggle]");
    if (toggle) {
      const section = toggle.closest(".settings-section");
      if (section) setCollapsed(section, toggle.getAttribute("aria-expanded") === "true");
      return;
    }
    if (event.target.closest("[data-settings-expand-all]")) {
      sections.forEach((section) => setCollapsed(section, false));
      return;
    }
    if (event.target.closest("[data-settings-collapse-all]")) {
      sections.forEach((section) => setCollapsed(section, true));
    }
  });

  const search = tools.querySelector("#settings-section-search");
  const result = tools.querySelector("#settings-nav-result");
  search.addEventListener("input", () => {
    const query = search.value.trim().toLocaleLowerCase();
    let visible = 0;
    links.forEach((link) => {
      const section = document.getElementById(link.dataset.settingsTarget);
      const matched = !query || (link.textContent + " " + (section?.textContent || ""))
        .toLocaleLowerCase().includes(query);
      link.hidden = !matched;
      if (matched) visible += 1;
    });
    result.textContent = query ? "找到 " + visible + " 个设置分组" : "共 " + links.length + " 个设置分组";
  });
  result.textContent = "共 " + links.length + " 个设置分组";
}

