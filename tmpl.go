// vi: ft=html

package status

const homeTmpl = `
  <!doctype html>
  <html>
  <head>
    <link href="data:image/x-icon;base64,AAABAAEAEBAAAAEAIABoBAAAFgAAACgAAAAQAAAAIAAAAAEAIAAAAAAAAAQAAAAAAAAAAAAAAAAAAAAAAAD/hACb/4QAm/+EAJv/hACb+sCCm/rAgpv6wIKb+sCCm/rAgpv6wIKb/966m//eupv/3rqb/966m//eupv/3rqb/4QAm/+EAJv/hACb/4QAm/rAgpv6wIKb+sCCm/rAgpv6wIKb+sCCm//eupv/3rqb/966m//eupv/3rqb/966m/+EAJv/hACb/4QAm/+EAJv6wIKb+sCCm/rAgpv6wIKb+sCCm/rAgpv/3rqb/966m//eupv/3rqb/966m//eupv/hACb/4QAm/+EAJv/hACb+sCCm/rAgpv6wIKb+sCCm/rAgpv6wIKb/966m//eupv/3rqb/966m//eupv/3rqb/4QAm/+EAJv/hACb/4QAm/rAgpv6wIKb+sCCm/rAgpv6wIKb+sCCm//eupv/3rqb/966m//eupv/3rqb/966m/+EAJv/hACb/4QAm/+EAJv6wIKb+sCCm/rAgpv6wIKb+sCCm/rAgpv/3rqb/966m//eupv/3rqb/966m//eupv/hACb/4QAm/+EAJv/hACb+sCCm/rAgpv6wIKb+sCCm/rAgpv6wIKb/966m//eupv/3rqb/966m//eupv/3rqbAAAAAAAAAAAAAAAAAAAAAPrAgpv6wIKb+sCCm/rAgpv6wIKb+sCCm//eupv/3rqb/966m//eupv/3rqb/966mwAAAAAAAAAAAAAAAAAAAAD6wIKb+sCCm/rAgpv6wIKb+sCCm/rAgpv/3rqb/966m//eupv/3rqb/966m//eupsAAAAAAAAAAAAAAAAAAAAA+sCCm/rAgpv6wIKb+sCCm/rAgpv6wIKb/966m//eupv/3rqb/966m//eupv/3rqbAAAAAAAAAAAAAAAAAAAAAPrAgpv6wIKb+sCCm/rAgpv6wIKb+sCCm//eupv/3rqb/966m//eupv/3rqb/966mwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAD/3rqb/966m//eupv/3rqb/966m//eupsAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA/966m//eupv/3rqb/966m//eupv/3rqbAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAP/eupv/3rqb/966m//eupv/3rqb/966mwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAD/3rqb/966m//eupv/3rqb/966m//eupsAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA/966m//eupv/3rqb/966m//eupv/3rqbAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAPAAAADwAAAA8AAAAPAAAAD/wAAA/8AAAP/AAAD/wAAA/8AAAA==" rel="icon" type="image/x-icon" />
    <meta name="viewport" content="width=device-width, initial-scale=1, user-scalable=no">
    <title>{{ .PageTitle }}</title>
    <style>
    :root {
      --main-pad-size: 1rem;
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
      left: calc((100vw - var(--main-width) - var(--main-pad-size)) / 2);
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
    .stat-table {
      margin-left: auto;
      text-align: right;
    }
    .stat-table tr td:last-child {
      font-weight: bold;
    }
    .stat-table tr td:last-child::before {
      content: '\00a0'
    }
    .aligned-stat-table tr td:last-child {
	  min-width: calc(var(--main-width) / 7);
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
      <section class="right">
        <table class="stat-table">
          <tr>
            <td>cpu</td>
            <td>{{ printf "%.2f" .Stats.CPU }}% {{ printf "%.0f" .Stats.CPUTemp }}&deg;C</td>
          </tr>
          <tr>
            <td>memory</td>
            <td>{{ .Stats.MemUsed | humanBytes }} / {{ .Stats.MemTotal | humanBytes }}</td>
          </tr>
          <tr>
            <td>load</td>
            <td>{{ .Stats.Load1 }} {{ .Stats.Load5 }} {{ .Stats.Load15 }}</td>
          </tr>
        </table>
      </section>
      {{ if eq (len .Projects) 0 }}
        <section class="right">
          <i>no projects up</i>
        </section>
      {{ else }}
        <section>
          {{ range $project, $containers := .Projects }}
          <p><strong>{{ $project }}</strong></p>
          <table class="stat-table aligned-stat-table">
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
