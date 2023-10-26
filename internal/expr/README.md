## Comparsion

| expr                     | python                   | js                             |
|--------------------------|--------------------------|--------------------------------|
| let x = 1;               | x = 1                    | let x = 1                      |
| {a: 1, b: 2}             | {"a": 1, "b": 2}         | {a: 1, b: 2}                   |
| r = fetch(url, {method}) | r = request(method, url) | r = await fetch(url, {method}) |
| r.ok                     | r.ok                     | r.ok                           |
| r.status                 | r.status_code            | r.status                       |
| r.text                   | r.text                   | await r.text()                 |
| r.json()                 | r.json()                 | await r.json()                 |
| r.headers                | r.headers                | r.headers                      |
