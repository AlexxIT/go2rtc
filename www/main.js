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
    </nav>
</header>
` + document.body.innerHTML;

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
