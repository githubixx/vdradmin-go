(() => {
  const STORAGE_KEY = 'vdradmin.theme';

  function normalizeTheme(value) {
    if (value === 'light' || value === 'dark' || value === 'system') return value;
    return 'system';
  }

  function getCookie(name) {
    const match = document.cookie.match(new RegExp('(?:^|; )' + name.replace(/[.$?*|{}()[\]\\/+^]/g, '\\$&') + '=([^;]*)'));
    return match ? decodeURIComponent(match[1]) : null;
  }

  function setCookie(name, value) {
    const maxAge = 60 * 60 * 24 * 365; // 1 year
    document.cookie = `${encodeURIComponent(name)}=${encodeURIComponent(value)}; Path=/; Max-Age=${maxAge}; SameSite=Lax`;
  }

  function getDefaultTheme() {
    return normalizeTheme(document.documentElement.dataset.themeDefault);
  }

  function getSavedTheme() {
    const ls = normalizeTheme(localStorage.getItem(STORAGE_KEY));
    if (ls !== 'system') return ls;
    const ck = normalizeTheme(getCookie('theme'));
    if (ck !== 'system') return ck;
    return 'system';
  }

  function applyTheme(mode) {
    const m = normalizeTheme(mode);
    if (m === 'system') {
      delete document.documentElement.dataset.theme;
    } else {
      document.documentElement.dataset.theme = m;
    }

    // Persist the explicit mode (including system)
    localStorage.setItem(STORAGE_KEY, m);
    setCookie('theme', m);
    updateToggleLabel();
  }

  function getEffectiveSystemTheme() {
    return window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
  }

  function toggleTheme() {
    const current = normalizeTheme(getCurrentMode());
    if (current === 'system') {
      // If following system, switch to the opposite of current system theme.
      const sys = getEffectiveSystemTheme();
      applyTheme(sys === 'dark' ? 'light' : 'dark');
      return;
    }
    applyTheme(current === 'dark' ? 'light' : 'dark');
  }

  function getCurrentMode() {
    const explicit = normalizeTheme(document.documentElement.dataset.theme);
    if (explicit === 'light' || explicit === 'dark') return explicit;

    // If no explicit, infer from saved theme or default.
    const saved = normalizeTheme(getSavedTheme());
    if (saved === 'light' || saved === 'dark') return saved;

    const def = normalizeTheme(getDefaultTheme());
    if (def === 'light' || def === 'dark') return def;

    return 'system';
  }

  function updateToggleLabel() {
    const btn = document.getElementById('theme-toggle');
    if (!btn) return;
    const mode = normalizeTheme(getCurrentMode());
    btn.textContent = mode === 'system' ? 'Theme: System' : `Theme: ${mode[0].toUpperCase()}${mode.slice(1)}`;
  }

  function initConfigRadios() {
    const radios = document.querySelectorAll('input[name="theme"]');
    if (!radios || radios.length === 0) return;

    const mode = normalizeTheme(getCurrentMode());
    radios.forEach((r) => {
      if (r instanceof HTMLInputElement) {
        r.checked = r.value === mode;
        r.addEventListener('change', () => {
          if (r.checked) applyTheme(r.value);
        });
      }
    });
  }

  // Initial apply:
  // 1) explicit saved theme (cookie/localStorage)
  // 2) server-provided default
  const saved = normalizeTheme(getSavedTheme());
  const def = normalizeTheme(getDefaultTheme());
  applyTheme(saved !== 'system' ? saved : def);

  // React to OS theme changes when in system mode
  if (window.matchMedia) {
    const mq = window.matchMedia('(prefers-color-scheme: dark)');
    const onChange = () => {
      const current = normalizeTheme(getCurrentMode());
      const savedNow = normalizeTheme(getSavedTheme());
      if (current === 'system' || savedNow === 'system') {
        // Re-apply to drop any explicit override.
        applyTheme('system');
      }
    };
    if (typeof mq.addEventListener === 'function') mq.addEventListener('change', onChange);
    else if (typeof mq.addListener === 'function') mq.addListener(onChange);
  }

  // Hook up toggle button
  document.addEventListener('click', (e) => {
    const target = e.target;
    if (!(target instanceof HTMLElement)) return;
    if (target.id !== 'theme-toggle') return;
    e.preventDefault();
    toggleTheme();
  });

  initConfigRadios();
  updateToggleLabel();

  // Expose for debugging/pages if needed
  window.vdradminTheme = { applyTheme };
})();
