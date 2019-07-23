package main

const homeTmpl = `
  <!doctype html>
  <html>
  <head>
    <meta name="viewport" content="width=device-width, initial-scale=1, user-scalable=no">
    <title>{{ .PageTitle }}</title>
    <style>
    :root {
      --main-pad-size: 1.2rem;
      --main-width: 600px;
    }
    * {
      margin: 0;
      padding: 0;
    }
    body {
      max-width: var(--main-width);
      margin: 0 auto;
      font-family: monospace;
    }
    #container {
      padding: var(--main-pad-size);
    }
    #container::after {
      content: "";
      background-image: url("https://www.toptal.com/designers/subtlepatterns/patterns/full-bloom.png");
      opacity: 0.55;
      top: 0;
      bottom: 0;
      width: var(--main-width);
      transform: translateX(calc(var(--main-pad-size) * -1));
      position: fixed;
      z-index: -1;
    }
    a {
      color: unset;
    }
    .red {
      color: red;
    }
    .green {
      color: green;
    }
    .c-stats {
      margin-left: auto;
      text-align: right;
    }
    .c-stats tr td:last-child {
      font-weight: bold;
    }
    .c-stats tr td:last-child::before {
      content: '⠀'
    }
    </style>
  </head>
  <body>
    <div id="container">
      <strong>{{ .PageTitle }}</strong>
      <br>
      <br>
      {{ range $project, $containers := .Projects }}
      <p><strong>{{ $project }}</strong></p>
      <table class="c-stats">
      {{ range $container := $containers }}
        {{ if $container.IsDown }}
          <tr class="red">
        {{ else }}
          <tr class="green">
        {{ end }}
        {{ if not (eq $container.Link "") }}
          <td><a href="//{{ $container.Link }}" target="_blank">{{ $container.Name }}</a></td>
        {{ else }}
          <td>{{ $container.Name }}</td>
        {{ end }}
        {{ if $container.IsDown }}
          <td>last seen {{ $container.LastSeen | humanDate }}</td>
        {{ else }}
          <td>{{ $container.Status }}</td>
        {{ end }}
        </tr>
      {{ end }}
      </table>
      {{ end }}
    </div>
  </body>
  </html>
`
