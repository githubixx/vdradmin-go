(() => {
  function normalizeTheme(theme) {
    // Allow any theme name - validation happens server-side
    // Empty string defaults to system
    const t = (theme || '').toLowerCase();
    return t === '' ? 'system' : t;
  }

  function applyTheme(mode) {
    const m = normalizeTheme(mode);
    if (m === 'system') {
      delete document.documentElement.dataset.theme;
      return;
    }
    document.documentElement.dataset.theme = m;
  }

  // The server already computes the effective theme mode and renders it into
  // `data-theme` / `data-theme-default`. This script only fills in the gap for
  // "system" mode (OS preference) without persisting anything.
  const serverMode = normalizeTheme(document.documentElement.dataset.theme);
  const serverDefault = normalizeTheme(document.documentElement.dataset.themeDefault);

  // If the server explicitly chose a theme (including custom ones), do not override it.
  if (serverMode !== 'system') {
    return;
  }

  // System mode: if the server provided an explicit fallback theme, apply it.
  if (serverDefault !== 'system') {
    applyTheme(serverDefault);
    return;
  }

  // System mode: keep `data-theme` unset and let base.css handle light/dark.
  delete document.documentElement.dataset.theme;

})();
function initCustomSelects(root = document) {
    const selects = root.querySelectorAll("select:not([multiple]):not([size]):not(.custom-select-initialized)");
    
    selects.forEach(select => {
        if(select.disabled || select.style.display === "none" || select.classList.contains("hidden")) return;
        
        select.classList.add("custom-select-initialized");

        const wrapper = document.createElement("div");
        wrapper.className = "custom-select-container";
        
        select.parentNode.insertBefore(wrapper, select);
        wrapper.appendChild(select);
        select.style.display = "none";
        
        const trigger = document.createElement("div");
        trigger.className = select.className + " custom-select-trigger";
        
        let longestText = "";
        Array.from(select.options).forEach(opt => {
            if ((opt.text || "").length > longestText.length) {
                longestText = opt.text || "";
            }
        });
        const phantom = document.createElement("div");
        phantom.className = select.className + " custom-select-phantom";
        phantom.innerText = longestText;
        
        const updateTriggerText = () => {
            const selectedOption = select.options[select.selectedIndex];
            trigger.innerText = selectedOption ? selectedOption.text : "";
        };
        updateTriggerText();
        
        const dropdown = document.createElement("ul");
        dropdown.className = "custom-select-dropdown";
        
        const renderOptions = () => {
            dropdown.innerHTML = "";
            Array.from(select.options).forEach((option, index) => {
                const li = document.createElement("li");
                li.className = "custom-select-option";
                if (option.selected) li.classList.add("selected");
                li.innerText = option.text;
                li.dataset.value = option.value;
                
                li.addEventListener("click", (e) => {
                    e.stopPropagation();
                    select.selectedIndex = index;
                    updateTriggerText();
                    
                    dropdown.querySelectorAll(".custom-select-option").forEach(n => n.classList.remove("selected"));
                    li.classList.add("selected");
                    
                    dropdown.classList.remove("open");
                    trigger.classList.remove("open");
                    
                    // Fire native events
                    select.dispatchEvent(new Event("change", { bubbles: true }));
                    // Handle HTMX submit if it relies on this
                    if (select.getAttribute("onchange") && select.getAttribute("onchange").includes("submit")) {
                        if (select.form) select.form.submit();
                    }
                });
                dropdown.appendChild(li);
            });
        };
        renderOptions();
        
        wrapper.appendChild(phantom);
        wrapper.appendChild(trigger);
        wrapper.appendChild(dropdown);
        
        trigger.addEventListener("click", (e) => {
            e.stopPropagation();
            document.querySelectorAll(".custom-select-dropdown.open").forEach(d => {
                if(d !== dropdown) {
                    d.classList.remove("open");
                    d.previousElementSibling.classList.remove("open");
                }
            });
            dropdown.classList.toggle("open");
            trigger.classList.toggle("open");
        });
        
        // Listen to native select change (e.g. reset button)
        select.addEventListener("change", () => {
            updateTriggerText();
            renderOptions();
        });
    });
}

document.addEventListener("DOMContentLoaded", () => initCustomSelects(document));
document.body.addEventListener("htmx:load", (e) => initCustomSelects(e.detail.elt));

document.addEventListener("click", () => {
    document.querySelectorAll(".custom-select-dropdown.open").forEach(d => {
        d.classList.remove("open");
        d.previousElementSibling.classList.remove("open");
    });
});
