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
.button-container {
    display: flex;
    flex-direction: row;
    flex-wrap: wrap;
    justify-content: center;
    gap: 20px;
    margin: 20px 0;
}

button {
    font-family: 'Poppins', Arial, Helvetica, sans-serif;
    background-color: rgba(177, 152, 145, .3);
    color: #695753;
    font-weight: 600;
    font-size: 14px;
    padding: 10px 20px;
    border: none;
    border-radius: 4px;
    cursor: pointer;
    transition: background-color 0.3s ease;
}

button:hover {
    background-color: #b19891;
}
form input[type="submit"] {
    font-family: 'Poppins', Arial, Helvetica, sans-serif;
    background-color: rgba(177, 152, 145, .3);
    color: #695753;
    font-weight: 600;
    font-size: 14px;
    padding: 10px 20px;
    border: none;
    border-radius: 4px;
    cursor: pointer;
    transition: background-color 0.3s ease;
}

form input[type="submit"]:hover {
    background-color: #b19891;
}
</style>
<nav>
    <ul>
        <li><a href="index.html">Streams</a></li>
        <li><a href="add.html">Add</a></li>
        <li><a href="editor.html">Config</a></li>
    </ul>
</nav>
` + document.body.innerHTML;
