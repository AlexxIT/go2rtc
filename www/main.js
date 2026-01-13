document.head.innerHTML += `
<style>
    body {
        background-color: white;  /* fix Hass black theme */
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
        border: 1px solid #ccc;
        border-radius: 4px;
        font-size: 16px;
    }

    button {
        padding: 10px 20px;
        border: 1px solid #ccc;
        border-radius: 4px;
        cursor: pointer;
        font-size: 16px;
    }

    /* table */
    table {
        width: 100%;
        background-color: white;
        border-collapse: collapse;
        margin: 0 auto;
        overflow: hidden;
    }

    th, td {
        padding: 12px 15px;
        text-align: left;
        border-bottom: 1px solid #e0e0e0;
    }

    th {
        background-color: #444;
        color: white;
    }

    tr:nth-child(even) {
        background-color: #fafafa;
    }

    tr:hover {
        background-color: #edf7ff;
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
        <button id="theme-toggle" style="float:right;margin:8px 16px 8px 0;">🌙</button>
    </nav>
</header>
` + document.body.innerHTML;

// Theme toggle logic
const themeButton = document.getElementById('theme-toggle');
const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
let darkMode = localStorage.getItem('theme') === 'dark' || (localStorage.getItem('theme') === null && prefersDark);

function setTheme(dark) {
    darkMode = dark;
    if (dark) {
        document.body.classList.add('dark-mode');
        themeButton.textContent = '☀️';
        localStorage.setItem('theme', 'dark');
    } else {
        document.body.classList.remove('dark-mode');
        themeButton.textContent = '🌙';
        localStorage.setItem('theme', 'light');
    }
}

themeButton.onclick = () => setTheme(!darkMode);
setTheme(darkMode);

// Add dark mode styles
document.head.innerHTML += `
<style>
body.dark-mode {
    background-color: #181a1b;
    color: #e0e0e0;
}
body.dark-mode nav {
    background-color: #222;
}
body.dark-mode nav a {
    color: #e0e0e0;
}
body.dark-mode nav a:hover {
    background-color: #444;
    color: #fff;
}
body.dark-mode main {
    background-color: #23272a;
}
body.dark-mode table {
    background-color: #23272a;
    color: #e0e0e0;
}
body.dark-mode th {
    background-color: #333;
    color: #fff;
}
body.dark-mode tr:nth-child(even) {
    background-color: #222;
}
body.dark-mode tr:hover {
    background-color: #333;
}
</style>
`;
