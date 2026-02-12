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
