// main menu
document.body.innerHTML = `
<style>
ul {
    list-style: none;
    margin: 0 auto;
}

a {
    text-decoration: none;
    font-family: 'Lora', serif;
    transition: .5s linear;
}

i {
    margin-right: 10px;
}

nav {
    display: block;
    /*width: 660px;*/
    margin: 0 auto 10px;
}

nav ul {
    padding: 1em 0;
    background: #ECDAD6;
}

nav a {
    padding: 1em;
    background: rgba(177, 152, 145, .3);
    border-right: 1px solid #b19891;
    color: #695753;
}

nav a:hover {
    background: #b19891;
}

nav li {
    display: inline;
}

body {
    font-family: Arial, Helvetica, sans-serif;
    background-color: white;
}
table {
    background-color: white;
    text-align: left;
    border-collapse: collapse;
}
table thead {
    background: #CFCFCF;
    background: linear-gradient(to bottom, #dbdbdb 0%, #d3d3d3 66%, #CFCFCF 100%);
    border-bottom: 3px solid black;
}
table thead th {
    font-size: 15px;
    font-weight: bold;
    color: black;
    text-align: center;
}
table td, table th {
    border: 1px solid black;
    padding: 5px 5px;
}

/* Dark mode styles */
body.dark-mode {
    background-color: #121212;
    color: #e0e0e0;
}

body.dark-mode nav ul {
    background: #333;
}

body.dark-mode a {
    background: rgba(45, 45, 45, .8);
    border-right: 1px solid #2c2c2c;
    color: #c7c7c7;
}

body.dark-mode a:hover {
    background: #555;
}

body.dark-mode a:visited {
    color: #999;
}

body.dark-mode table {
    background-color: #222;
    color: #ddd;
}

body.dark-mode table thead {
    background: linear-gradient(to bottom, #444 0%, #3d3d3d 66%, #333 100%);
    border-bottom: 3px solid #888;
}
body.dark-mode table thead th {
    font-size: 15px;
    font-weight: bold;
    color: #ddd;
    text-align: center;
}
body.dark-mode table td, body.dark-mode table th {
    border: 1px solid #444;
}

body.dark-mode button {
    background: rgba(255, 255, 255, .1);
    border: 1px solid #444;
    color: #ccc;
}

</style>
<nav>
    <ul>
        <li><a href="index.html">Streams</a></li>
        <li><a href="add.html">Add</a></li>
        <li><a href="editor.html">Config</a></li>
        <li><a href="log.html">Log</a></li>
       <li><a href="#" id="darkModeToggle">
       &#127769;
        </a>
        </li>
    </ul>
</nav>
` + document.body.innerHTML;

const sunIcon = '&#9728;&#65039;';
const moonIcon = '&#127765;';

document.addEventListener('DOMContentLoaded', () => {
    const darkModeToggle = document.getElementById('darkModeToggle');
    const prefersDarkScheme = window.matchMedia('(prefers-color-scheme: dark)');

    const updateToggleButton = () => {
        if (isDarkModeEnabled()) {
            darkModeToggle.innerHTML = sunIcon;
            darkModeToggle.setAttribute('aria-label', 'Enable light mode');
        } else {
            darkModeToggle.innerHTML = moonIcon;
            darkModeToggle.setAttribute('aria-label', 'Enable dark mode');
        }
    };

    const isDarkModeEnabled = () => document.body.classList.contains('dark-mode');

    const updateDarkMode = () => {
        if (prefersDarkScheme.matches) {
            document.body.classList.add('dark-mode');
        } else {
            document.body.classList.remove('dark-mode');
        }
    };

    updateDarkMode();
    updateToggleButton();

    prefersDarkScheme.addListener(updateDarkMode);

    darkModeToggle.addEventListener('click', () => {
        document.body.classList.toggle('dark-mode');
        if (document.body.classList.contains('dark-mode')) {
            localStorage.setItem('darkMode', 'enabled');
            darkModeToggle.innerHTML = sunIcon;
        } else {
            localStorage.removeItem('darkMode');
            darkModeToggle.innerHTML = moonIcon;
        }
    });

    if (localStorage.getItem('darkMode') === 'enabled' || prefersDarkScheme.matches) {
        document.body.classList.add('dark-mode');
    }
});
