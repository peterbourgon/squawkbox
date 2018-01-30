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
<a href="/recordings">Recordings</a>
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

const recordingsTemplate = `<ul>
{{ if .Recordings }}{{ range .Recordings }}
<li><a href="/recordings/{{ . }}">{{ . }}</a></li>
{{ end }}{{ else }}
<li>(No recordings!)</li>
{{ end }}
</ul>`

const footerTemplate = `</body>
</html>`
