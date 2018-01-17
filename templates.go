package main

const headerTemplate = `<html>
<head>
<title>Squawkbox</title>
<style>

</style>
</head>
<body>
<div class="header">
<strong>Squawkbox</strong> •
<a href="/events">Audit log</a> ·
<a href="/codes">Bypass codes</a>
</div>`

const eventsTemplate = `
<table>
<th>
	<td>Event ID</td>
	<td>Kind</td>
	<td>Details</td>
	<td>HTTP Request</td>
</th>
{{ range .Events }}
<tr style="background-color: {{ .Color }};">
	<td>{{ .ULID }}</td>
	<td>{{ .Kind }}</td>
	<td>{{ .Details }}</td>
	<td>{{ .HTTP }}</td>
</tr>
{{ end }}
</table>
{{ if .NextPage }}<a href="/events?from={{ .NextPage }}">Next page</a>{{ end }}
`

const codesTemplate = `
`

const footerTemplate = `</body>
</html>`
