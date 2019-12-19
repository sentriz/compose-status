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
      * {
        margin: 0;
        padding: 0;
      }
      body {
        background-color: #fffce0;
        max-width: 500px;
        margin: 0 auto;
        font-family: monospace;
      }
      body > * {
        margin: 10px;
      }
      a {
        color: unset;
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
        content: '\00a0';
      }
      section {
        display: flex;
        background-color: white;
        opacity: 0.8;
        padding: 10px;
        box-shadow: 0 4px 4px 0 rgba(0, 0, 0, 0.05);
        transition: 0.3s;
        border-radius: 4px;
      }
      p {
        font-style: italic;
      }
    </style>
  </head>
  <body>
    <section>
      {{ if not (eq .PageTitle "") }}
        <strong>{{ .PageTitle }}</strong>
      {{ end }}
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
    {{ range $project, $containers := .Projects }}
    <section>
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
    </section>
    {{ end }}
    {{ end }}
    {{ if .ShowCredit }}
    <div class="right light">
      <i><a target="_blank" href="https://github.com/sentriz/compose-status">compose status</a></i>
    </div>
    {{ end }}
    </div>
  </body>
  </html>
`
