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
    font-family: monospace;
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
  <strong>{{ .PageTitle }}</strong>
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
</body>
</html>
`
