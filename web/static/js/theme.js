(() => {
  function normalizeTheme(theme) {
    switch ((theme || '').toLowerCase()) {
      case 'dark':
      case 'light':
      case 'system':
        return theme.toLowerCase();
      default:
        return 'system';
    }
  }

  function applyTheme(mode) {
    const m = normalizeTheme(mode);
    if (m === 'system') {
      delete document.documentElement.dataset.theme;
      return;
    }
    document.documentElement.dataset.theme = m;
  }

  function applySystemTheme() {
    const prefersDark = window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches;
    if (prefersDark) document.documentElement.dataset.theme = 'dark';
    else delete document.documentElement.dataset.theme;
  }

  // The server already computes the effective theme mode and renders it into
  // `data-theme` / `data-theme-default`. This script only fills in the gap for
  // "system" mode (OS preference) without persisting anything.
  const serverMode = normalizeTheme(document.documentElement.dataset.theme);
  const serverDefault = normalizeTheme(document.documentElement.dataset.themeDefault);

  // If the server explicitly chose light/dark, do not override it.
  if (serverMode === 'dark' || serverMode === 'light') {
    return;
  }

  // If the server default is an explicit mode, apply it.
  if (serverDefault === 'dark' || serverDefault === 'light') {
    applyTheme(serverDefault);
    return;
  }

  // System mode: apply OS preference and react to changes.
  applySystemTheme();
  if (window.matchMedia) {
    const mq = window.matchMedia('(prefers-color-scheme: dark)');
    const handler = () => applySystemTheme();
    if (mq.addEventListener) mq.addEventListener('change', handler);
    else if (mq.addListener) mq.addListener(handler);
  }

})();
