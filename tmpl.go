package status

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
    a {
      color: unset;
    }
    section ~ section {
      margin-top: var(--main-pad-size);
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
      left: calc((100vw - var(--main-width)) / 2);
      position: fixed;
      z-index: -1;
    }
    .right {
      text-align: right;
    }
    .light {
      opacity: 0.3;
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
      {{ if not (eq .PageTitle "") }}
        <section>
          <strong>{{ .PageTitle }}</strong>
        </section>
      {{ end }}
      {{ if eq (len .Projects) 0 }}
        <section class="right">
          <i>no projects up</i>
        </section>
      {{ else }}
        <section>
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
        </section>
      {{ end }}
      {{ if .ShowCredit }}
        <section class="right light">
          <i><a target="_blank" href="https://github.com/sentriz/compose-status">compose status</a></i>
        </section>
      {{ end }}
    </div>
  </body>
  </html>
`
