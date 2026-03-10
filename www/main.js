// Shared navigation component - loaded automatically by other pages
if (!document.querySelector('.logo')) {
    const head = document.head;
    if (!head.querySelector('link[href*="fonts.googleapis.com"]')) {
        head.insertAdjacentHTML(
            'beforeend',
            `
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;500;700&family=Orbitron:wght@700;900&display=swap" rel="stylesheet">
`.trim(),
        );
    }

    if (!head.querySelector('link[href="styles.css"]')) {
        head.insertAdjacentHTML('beforeend', '<link rel="stylesheet" href="styles.css">');
    }

    // Common UI refresh intervals (ms)
    window.SYSTEM_INFO_UPDATE_INTERVAL_MS = 2000;

    document.body.innerHTML = `
<header>
    <div class="container">
        <nav>
            <span class="logo">GO2RTC</span>
            <div class="nav-links">
                <a href="index.html" class="nav-link">Streams</a>
                <a href="add.html" class="nav-link">Add Stream</a>
                <a href="config.html" class="nav-link">Config</a>
                <a href="log.html" class="nav-link">Logs</a>
                <a href="net.html" class="nav-link">Network</a>
            </div>
            <a href="https://github.com/AlexxIT/go2rtc" target="_blank" class="nav-link docs-link">docs</a>
            <button class="theme-toggle" id="theme-toggle" aria-label="Toggle theme">
                <span class="theme-icon">🌙</span>
            </button>
        </nav>
    </div>
</header>
` + document.body.innerHTML;

    // Mark active nav link
    const currentPage = location.pathname.split('/').pop() || 'index.html';
    document.querySelectorAll('.nav-links .nav-link').forEach(link => {
        if (link.getAttribute('href') === currentPage) {
            link.classList.add('active');
        }
    });

    // Theme management functions
    function initTheme() {
        const savedTheme = localStorage.getItem('theme');
        const systemPrefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
        const theme = savedTheme || (systemPrefersDark ? 'dark' : 'light');

        setTheme(theme);

        // Listen for system theme changes
        window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', (e) => {
            if (!localStorage.getItem('theme')) {
                setTheme(e.matches ? 'dark' : 'light');
            }
        });
    }

    function setTheme(theme) {
        const html = document.documentElement;
        const themeIcon = document.querySelector('.theme-icon');

        if (theme === 'light') {
            html.setAttribute('data-theme', 'light');
            if (themeIcon) themeIcon.textContent = '☀️';
        } else {
            html.removeAttribute('data-theme');
            if (themeIcon) themeIcon.textContent = '🌙';
        }
    }

    function toggleTheme() {
        const html = document.documentElement;
        const currentTheme = html.getAttribute('data-theme') === 'light' ? 'light' : 'dark';
        const newTheme = currentTheme === 'light' ? 'dark' : 'light';

        setTheme(newTheme);
        localStorage.setItem('theme', newTheme);
        window.dispatchEvent(new Event('themeChanged'));
    }

    // Initialize theme
    initTheme();

    // Theme toggle button handler
    document.getElementById('theme-toggle')?.addEventListener('click', toggleTheme);
}

window.go2rtcReady = (async () => {
    try {
        const url = new URL('api', location.href);
        const r = await fetch(url, {cache: 'no-cache'});
        if (!r.ok) return null;
        const data = await r.json();
        window.go2rtcInfo = data;
        return data;
    } catch (e) {
        return null;
    }
})();

window.go2rtcReady.then(data => {
    if (!data || !data.read_only) return;
    const links = document.querySelectorAll('nav a[href="add.html"], nav a[href="config.html"]');
    links.forEach(link => {
        link.style.display = 'none';
    });
});
