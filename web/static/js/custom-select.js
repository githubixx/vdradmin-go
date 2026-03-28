function initCustomSelects(root = document) {
    const selects = root.querySelectorAll("select:not([multiple]):not(.custom-select-initialized)");
    
    selects.forEach(select => {
        if(select.style.display === "none" || select.classList.contains("hidden")) return;
        
        select.classList.add("custom-select-initialized");

        const wrapper = document.createElement("div");
        wrapper.className = "custom-select-container";
        
        select.parentNode.insertBefore(wrapper, select);
        wrapper.appendChild(select);
        select.style.display = "none";
        
        const trigger = document.createElement("div");
        trigger.className = "custom-select-trigger";
        trigger.className = select.className + " custom-select-trigger";
        
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
