(() => {
    const THEME_KEY = 'go2rtc_theme';
    const root = document.documentElement;

    function setTheme(theme) {
        if (theme === 'light' || theme === 'dark') {
            root.dataset.theme = theme;
        } else {
            delete root.dataset.theme;
        }
    }

    function getEffectiveTheme() {
        if (root.dataset.theme === 'light' || root.dataset.theme === 'dark') return root.dataset.theme;
        return window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
    }

    function themeIcon(theme) {
        if (theme === 'dark') {
            // moon
            return `<svg width="16" height="16" viewBox="0 0 24 24" fill="none" aria-hidden="true">
  <path d="M21 14.8A8.5 8.5 0 0 1 9.2 3a7 7 0 1 0 11.8 11.8Z" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
</svg>`;
        }
        // sun
        return `<svg width="16" height="16" viewBox="0 0 24 24" fill="none" aria-hidden="true">
  <path d="M12 18a6 6 0 1 0 0-12 6 6 0 0 0 0 12Z" stroke="currentColor" stroke-width="2"/>
  <path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.41-1.41M17.66 6.34l1.41-1.41" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
</svg>`;
    }

    function renderThemeToggle(btn) {
        const theme = getEffectiveTheme();
        const label = theme === 'dark' ? 'Dark theme' : 'Light theme';
        btn.setAttribute('aria-label', `Toggle theme (currently: ${label})`);
        btn.innerHTML = themeIcon(theme) + '<span class="hint">Theme</span>';
    }

    try {
        const saved = localStorage.getItem(THEME_KEY);
        if (saved === 'light' || saved === 'dark') setTheme(saved);
    } catch (e) {
    }

    const header = document.createElement('header');
    header.className = 'app-header';
    header.innerHTML = `
  <div class="app-header__inner">
    <a class="app-brand" href="index.html" aria-label="go2rtc home">
      <span class="app-dot" aria-hidden="true"></span>
      <span>go2rtc</span>
    </a>
    <nav class="app-nav" aria-label="Primary">
      <a href="index.html">streams</a>
      <a href="add.html">add</a>
      <a href="config.html">config</a>
      <a href="log.html">log</a>
      <a href="net.html">net</a>
    </nav>
    <div class="app-actions">
      <button class="btn btn-sm btn-ghost" type="button" data-theme-toggle></button>
    </div>
  </div>
`;

    if (!document.querySelector('header.app-header')) {
        document.body.prepend(header);
    }

    // Active nav item.
    try {
        const current = (location.pathname.split('/').pop() || 'index.html').toLowerCase();
        document.querySelectorAll('nav.app-nav a').forEach(a => {
            const href = (a.getAttribute('href') || '').toLowerCase();
            if (href === current) a.setAttribute('aria-current', 'page');
        });
    } catch (e) {
    }

    // Theme toggle.
    const toggle = document.querySelector('[data-theme-toggle]');
    if (toggle) {
        renderThemeToggle(toggle);
        toggle.addEventListener('click', () => {
            const current = getEffectiveTheme();
            const next = current === 'dark' ? 'light' : 'dark';
            setTheme(next);
            try {
                localStorage.setItem(THEME_KEY, next);
            } catch (e) {
            }
            renderThemeToggle(toggle);
        });
    }
})();
