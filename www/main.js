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

body.dark-mode input, 
body.dark-mode select, 
body.dark-mode textarea {
    background-color: #333;
    color: #e0e0e0;
    border: 1px solid #444;
}

body.dark-mode input::placeholder,
body.dark-mode textarea::placeholder {
    color: #bbb;
}

body.dark-mode hr {
    border-top: 1px solid #444;
}
</style>
<nav>
    <ul>
        <li><a href="index.html">Streams</a></li>
        <li><a href="add.html">Add</a></li>
        <li><a href="editor.html">Config</a></li>
        <li><a href="log.html">Log</a></li>
        <li><a href="network.html">Net</a></li>
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

    const isDarkModeEnabled = () => document.body.classList.contains('dark-mode');

    // Update the toggle button based on the dark mode state
    const updateToggleButton = () => {
        if (isDarkModeEnabled()) {
            darkModeToggle.innerHTML = sunIcon;
            darkModeToggle.setAttribute('aria-label', 'Enable light mode');
        } else {
            darkModeToggle.innerHTML = moonIcon;
            darkModeToggle.setAttribute('aria-label', 'Enable dark mode');
        }
    };

    const updateDarkMode = () => {
        if (localStorage.getItem('darkMode') === 'enabled' || prefersDarkScheme.matches && localStorage.getItem('darkMode') !== 'disabled') {
            document.body.classList.add('dark-mode');
        } else {
            document.body.classList.remove('dark-mode');
        }
        updateEditorTheme();
        updateToggleButton();
    };

    // Update the editor theme based on the dark mode state
    const updateEditorTheme = () => {
        if (typeof editor !== 'undefined') {
            editor.setTheme(isDarkModeEnabled() ? 'ace/theme/tomorrow_night_eighties' : 'ace/theme/github');
        }
    };

    // Initial update for dark mode and toggle button
    updateDarkMode();

    // Listen for changes in the system's color scheme preference
    prefersDarkScheme.addEventListener('change', updateDarkMode); // Modern approach

    // Toggle dark mode and update local storage on button click
    darkModeToggle.addEventListener('click', () => {
        const enabled = document.body.classList.toggle('dark-mode');
        localStorage.setItem('darkMode', enabled ? 'enabled' : 'disabled');
        updateToggleButton(); // Update the button after toggling
        updateEditorTheme();
    });
});
