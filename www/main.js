document.head.innerHTML += `
<style>
    :root {
        --bg-color: white;
        --text-color: black;
        --input-bg: white;
        --input-border: #ccc;
        --table-bg: white;
        --table-header-bg: #444;
        --table-border: #e0e0e0;
        --table-row-alt: #fafafa;
        --table-row-hover: #edf7ff;
        --link-color: #0066cc;
        --link-visited: #551a8b;
    }

    html.dark {
        --bg-color: #121212;
        --text-color: #e0e0e0;
        --input-bg: #2d2d2d;
        --input-border: #444;
        --table-bg: #1e1e1e;
        --table-header-bg: #333;
        --table-border: #333;
        --table-row-alt: #2a2a2a;
        --table-row-hover: #3a3a3a;
        --link-color: #8ab4f8;
        --link-visited: #c58af9;
    }

    body {
        background-color: var(--bg-color);
        color: var(--text-color);
        display: flex;
        flex-direction: column;
        font-family: Arial, sans-serif;
        margin: 0;
    }

    /* navigation block */
    nav {
        background-color: #333;
        overflow: hidden;
    }

    nav a {
        float: left;
        display: block;
        color: #f2f2f2;
        text-align: center;
        padding: 14px 16px;
        text-decoration: none;
        font-size: 17px;
    }

    nav a:hover {
        background-color: #ddd;
        color: black;
    }

    /* main block */
    main {
        padding: 10px;
        display: flex;
        flex-direction: column;
        gap: 10px;
    }

    main a {
        color: var(--link-color);
    }

    main a:visited {
        color: var(--link-visited);
    }

    /* checkbox */
    label {
        display: flex;
        gap: 5px;
        align-items: center;
        cursor: pointer;
    }

    input[type="checkbox"] {
        width: 18px;
        height: 18px;
        cursor: pointer;
    }

    /* form */
    form {
        display: flex;
        flex-wrap: wrap;
        gap: 10px;
    }

    input[type="text"], input[type="email"], input[type="password"], select {
        padding: 10px;
        background-color: var(--input-bg);
        color: var(--text-color);
        border: 1px solid var(--input-border);
        border-radius: 4px;
        font-size: 16px;
    }

    button {
        padding: 10px 20px;
        background-color: var(--input-bg);
        color: var(--text-color);
        border: 1px solid var(--input-border);
        border-radius: 4px;
        cursor: pointer;
        font-size: 16px;
    }

    /* table */
    table {
        width: 100%;
        background-color: var(--table-bg);
        border-collapse: collapse;
        margin: 0 auto;
        overflow: hidden;
    }

    th, td {
        padding: 12px 15px;
        text-align: left;
        border-bottom: 1px solid var(--table-border);
    }

    th {
        background-color: var(--table-header-bg);
        color: white;
    }

    tr:nth-child(even) {
        background-color: var(--table-row-alt);
    }

    tr:hover {
        background-color: var(--table-row-hover);
        transition: background-color 0.3s ease;
    }

    /* table on mobile */
    @media (max-width: 480px) {
        table, thead, tbody, th, td, tr {
            display: block;
        }

        th, td {
            box-sizing: border-box;
            width: 100% !important;
            border: none;
        }

        tr {
            margin-bottom: 10px;
            border-radius: 4px;
        }
    }
</style>
`;

document.body.innerHTML = `
<header>
    <nav>
        <a href="index.html"><b>go2rtc</b></a>
        <a href="add.html">add</a>
        <a href="config.html">config</a>
        <a href="log.html">log</a>
        <a href="net.html">net</a>
        <a href="#" id="theme-toggle" title="Toggle theme" style="float: right"></a>
    </nav>
</header>
` + document.body.innerHTML;

(function() {
    const modes = ['auto', 'light', 'dark'];
    let mode = localStorage.getItem('theme') || 'auto';
    if (!modes.includes(mode)) mode = 'auto';

    const toggle = document.getElementById('theme-toggle');
    const media = window.matchMedia('(prefers-color-scheme: dark)');

    function update() {
        const dark = mode === 'dark' || (mode === 'auto' && media.matches);
        document.documentElement.classList.toggle('dark', dark);
        toggle.innerText = mode.charAt(0).toUpperCase() + mode.slice(1);
        window.dispatchEvent(new Event('themeChanged'));
    }

    toggle.addEventListener('click', (e) => {
        e.preventDefault();
        const idx = modes.indexOf(mode);
        mode = modes[(idx + 1) % modes.length];
        localStorage.setItem('theme', mode);
        update();
    });

    media.addEventListener('change', update);

    update();
})();
