package main

var homeTmpl = `<!doctype html>
<html>
<head>
  <style>
  * {
    margin: 0;
    padding: 0;
  }
  body {
    max-width: 600px;
    margin: 0 auto;
    overflow-y: hidden;
    font-family: monospace;
  }
  #container {
    height: 100vh;
    position: relative;
    padding: 1.2rem;
  }
  div::after {
    content: "";
    background-image: url("https://www.toptal.com/designers/subtlepatterns/patterns/full-bloom.png");
    opacity: 0.6;
    top: 0;
    left: 0;
    bottom: 0;
    right: 0;
    position: absolute;
    z-index: -1;
  }
  hr {
    opacity: 0.5;
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
    <hr>
    <p><strong>{{ $project }}</strong></p>
    <table class="c-stats">
    {{ range $container := $containers }}
      {{ $isDown := eq $container.Status "" }}
      {{ if $isDown }}
        <tr class="red">
      {{ else }}
        <tr class="green">
      {{ end }}
      {{ if not (eq $container.Link "") }}
        <td><a href="//{{ $container.Link }}" target="_blank">{{ $container.ID }}</a></td>
      {{ else }}
        <td>{{ $container.ID }}</td>
      {{ end }}
      {{ if $isDown }}
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
