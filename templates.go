package main

const headerTemplate = `<html>
<head>
<title>Squawkbox</title>
<style>
td, th {
	border: 1px solid #999;
	padding: 0.5rem;
	text-align: left;
	vertical-align: top;
}
</style>
</head>
<body>
<div class="header">
<strong>Squawkbox</strong> •
<a href="/events">Audit log</a> ·
<a href="/codes">Bypass codes</a>
</div>
<br/>`

const eventsTemplate = `
<table>
<tr>
	<th>Event ID</th>
	<th>Kind</th>
	<th>Details</th>
</tr>
{{ if .Events }}{{ range .Events }}
<tr style="background-color: {{ .Color }};">
	<td class="id"><a href="/events/{{ .ULID }}">{{ .ULID }}</a><br/>{{ .Time }}</td>
	<td class="kind">{{ .Kind }}</td>
	<td class="details">
		{{ range .Details }}{{ . }}<br/>{{ end }}
	</td>
</tr>
{{ end }}{{ else }}
<tr>
	<td>(No events!)</td>
	<td></td>
	<td></td>
</tr>
{{ end }}
</table>
{{ if .NextPage }}<a href="/events?from={{ .NextPage }}">Next page</a>{{ end }}
`

const eventTemplate = `
<ul>
	<li><strong>Event ID</strong>: {{ .ULID }}</li>
	<li><strong>Time</strong>: {{ .Time }}</li>
	<li><strong>UTC</strong>: {{ .UTC }}</li>
	<li><strong>Kind</strong>: <span style="background-color: {{ .Color }};">{{ .Kind }}</span></li>
	<li><strong>Details</strong>
		<ul>
			{{ if .Details }}{{ range .Details }}<li>{{ . }}</li>{{ end }}
			{{ else }}<li>(none)</li>
			{{ end }}
		</ul>
	</li>
	<li><strong>HTTP request information</strong>
		<ul>
			{{ if .HTTP }}{{ range .HTTP }}<li>{{ . }}</li>{{ end }}
			{{ else }}<li>(none)</li>
			{{ end }}
		</ul>
  	</li>
</ul>
`

const codesTemplate = `
<form action="/codes" method="POST">
	Code <input type="text" name="code" size=10 />
	Use count <input type="text" name="use_count" size=5 value="1" />
	Expires in <select name="expires_in">
		<option value="5m">5 minutes</option>
		<option value="1h" selected>1 hour</option>
		<option value="4h">4 hours</option>
		<option value="12h">12 hours</option>
		<option value="24h">1 day</option>
		<option value="48h">2 days</option>
		<option value="72h">3 days</option>
		<option value="168h">1 week</option>
	</select>
	<input type="submit" value="Create"/>
</form>

<table>
<tr>
	<th>Bypass code</th>
	<th>Use count</th>
	<th>Expires at</th>
	<th>Revoke</th>
</tr>
{{ if .Codes }}{{ range .Codes }}
<tr>
	<td>{{ .Code }}</td>
	<td>{{ .UseCount }}</td>
	<td>{{ .ExpiresAt }}</td>
	<td><form action="/codes/{{ .Code }}" method="POST"><input type="hidden" name="delete" value="true" /><input type="submit" value="Revoke"/></form></td>
</tr>
{{ end }}{{ else }}
<tr>
	<td>(No bypass codes!)</td>
	<td></td>
	<td></td>
	<td></td>
</tr>
{{ end }}
</table>`

const footerTemplate = `</body>
</html>`
