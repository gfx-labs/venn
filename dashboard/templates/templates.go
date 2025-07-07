package templates

import (
	"fmt"
	"html/template"
	"io"
)

type ChainInfo struct {
	Name        string
	ChainID     int
	HeadBlock   uint64
	Timestamp   int64
	IsHealthy   bool
	LastUpdated string
	BlockTime   float64
}

const layoutHTML = `<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>{{.Title}}</title>
	<script src="https://unpkg.com/htmx.org@1.9.10"></script>
	<script src="https://unpkg.com/hyperscript.org@0.9.12"></script>
	<link href="/dashboard/static/css/output.css" rel="stylesheet">
</head>
<body class="bg-gray-900 text-gray-100">
	<div class="min-h-screen">
		{{.Content}}
	</div>
</body>
</html>`

const dashboardHTML = `
<div class="container mx-auto px-4 py-8">
	<header class="mb-8">
		<h1 class="text-4xl font-bold text-white mb-2">Venn Dashboard</h1>
		<p class="text-gray-400">Monitoring blockchain head blocks</p>
	</header>
	
	<div id="chains-container" class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
		{{range .Chains}}
		<div 
			data-chain-card
			data-chain-name="{{.Name}}"
			hx-get="/dashboard/api/chain/{{.Name}}"
			hx-trigger="load, every 5s"
			hx-swap="outerHTML"
			class="bg-gray-800 rounded-lg shadow-lg p-6 border-2 border-gray-600 hover:border-gray-500 transition-all hover:shadow-xl"
			_="on htmx:beforeRequest add .opacity-50 to me
			   on htmx:afterRequest remove .opacity-50 from me"
		>
			{{template "chainCard" .}}
		</div>
		{{end}}
	</div>
	
	<div class="mt-8 text-center text-gray-500 text-sm">
		<p>Auto-refreshing every 5 seconds</p>
	</div>
</div>`

const chainCardHTML = `
{{define "chainCard"}}
<a href="/dashboard/{{.Name}}" class="block h-full">
	<div class="h-full flex flex-col">
		<div class="flex justify-between items-start mb-4">
			<div class="min-w-0 flex-1 mr-2">
				<h2 class="text-xl font-semibold text-white truncate">{{.Name}}</h2>
				<p class="text-sm text-gray-400">Chain ID: {{.ChainID}}</p>
			</div>
			<div class="flex-shrink-0">
				{{if .IsHealthy}}
					<div class="w-3 h-3 rounded-full bg-green-500" title="Healthy"></div>
				{{else}}
					<div class="w-3 h-3 rounded-full bg-red-500" title="Unhealthy"></div>
				{{end}}
			</div>
		</div>

	<div class="flex-1 space-y-3">
		<div class="bg-gray-700/30 rounded p-3">
			<div class="flex justify-between items-center">
				<span class="text-sm text-gray-400">Head Block</span>
				<span class="font-mono text-lg text-white">{{.HeadBlock}}</span>
			</div>
		</div>
		
		<div class="bg-gray-700/30 rounded p-3">
			<div class="flex justify-between items-center">
				<span class="text-sm text-gray-400">Last Update</span>
				<span class="text-sm font-mono text-gray-300">{{.LastUpdated}}</span>
			</div>
		</div>
	</div>

	<div class="mt-4 pt-3 border-t border-gray-700">
		<div class="flex justify-between items-center text-xs text-gray-500">
			<span>Auto-refresh</span>
			<span class="flex items-center">
				<div class="w-2 h-2 bg-green-400 rounded-full mr-1 animate-pulse"></div>
				Active
			</span>
		</div>
	</div>
	</div>
</a>
{{end}}`

var (
	layoutTmpl    = template.Must(template.New("layout").Parse(layoutHTML))
	dashboardTmpl = template.Must(template.New("dashboard").Parse(dashboardHTML + chainCardHTML))
	chainCardTmpl = template.Must(template.New("chainCard").Parse(chainCardHTML))
)

func RenderDashboard(w io.Writer, chains []ChainInfo) error {
	dashboardContent := ""
	buf := &stringWriter{}
	err := dashboardTmpl.Execute(buf, struct{ Chains []ChainInfo }{Chains: chains})
	if err != nil {
		return err
	}
	dashboardContent = buf.String()

	return layoutTmpl.Execute(w, struct {
		Title   string
		Content template.HTML
	}{
		Title:   "Venn Dashboard",
		Content: template.HTML(dashboardContent),
	})
}

func RenderChainCard(w io.Writer, chain ChainInfo) error {
	return chainCardTmpl.Execute(w, chain)
}

func RenderChainCardWithHTMX(w io.Writer, chain ChainInfo) error {
	// Wrap the chain card with HTMX attributes
	wrapper := `<div 
		data-chain-card
		data-chain-name="{{.Name}}"
		hx-get="/dashboard/api/chain/{{.Name}}"
		hx-trigger="every 5s"
		hx-swap="outerHTML"
		class="bg-gray-800 rounded-lg shadow-lg p-6 border-2 border-gray-600 hover:border-gray-500 transition-all hover:shadow-xl"
		_="on htmx:beforeRequest add .opacity-50 to me
		   on htmx:afterRequest remove .opacity-50 from me"
	>
		{{template "chainCard" .}}
	</div>`
	
	tmpl := template.Must(template.New("wrapper").Parse(wrapper + chainCardHTML))
	return tmpl.Execute(w, chain)
}

type stringWriter struct {
	content string
}

func (s *stringWriter) Write(p []byte) (n int, err error) {
	s.content += string(p)
	return len(p), nil
}

func (s *stringWriter) String() string {
	return s.content
}

func formatInt(i int) string {
	return fmt.Sprintf("%d", i)
}

func formatUint64(u uint64) string {
	return fmt.Sprintf("%d", u)
}

const chainDetailHTML = `
<div class="container mx-auto px-4 py-8" hx-get="/dashboard/{{.Chain.Name}}" hx-trigger="every 5s" hx-swap="outerHTML">
	<header class="mb-6">
		<div class="flex items-center mb-2">
			<a href="/dashboard" class="text-gray-400 hover:text-white mr-4">
				← Back to Dashboard
			</a>
		</div>
		<h1 class="text-4xl font-bold text-white">{{.Chain.Name}}</h1>
	</header>
	
	<div class="space-y-6">
		<div class="bg-gray-800 rounded-lg p-6 border-2 border-gray-600">
			<div class="grid grid-cols-1 md:grid-cols-4 gap-4">
				<div class="bg-gray-700/50 rounded-lg p-4">
					<p class="text-sm text-gray-400 mb-1">Chain ID</p>
					<p class="text-2xl font-semibold text-white">{{.Chain.ChainID}}</p>
				</div>
				<div class="bg-gray-700/50 rounded-lg p-4">
					<p class="text-sm text-gray-400 mb-1">Head Block</p>
					<p class="text-2xl font-semibold text-white font-mono">{{.Chain.HeadBlock}}</p>
				</div>
				<div class="bg-gray-700/50 rounded-lg p-4">
					<p class="text-sm text-gray-400 mb-1">Block Time</p>
					<p class="text-2xl font-semibold text-white">{{.Chain.BlockTime}}s</p>
				</div>
				<div class="bg-gray-700/50 rounded-lg p-4">
					<p class="text-sm text-gray-400 mb-1">Active / Total</p>
					<p class="text-2xl font-semibold {{if .Chain.IsHealthy}}text-green-400{{else}}text-red-400{{end}}">
						{{.ActiveRemotes}} / {{.TotalRemotes}}
					</p>
				</div>
			</div>
		</div>
		
		<div class="bg-gray-800 rounded-lg p-6 border-2 border-gray-600">
			<h2 class="text-2xl font-semibold text-white mb-4">Remote Endpoints</h2>
			<div class="grid grid-cols-1 lg:grid-cols-2 xl:grid-cols-3 gap-3">
				{{range .Remotes}}
				<div class="bg-gray-700/50 rounded-lg p-4 border border-gray-600 hover:border-gray-500 transition-colors">
					<div class="flex items-center justify-between mb-2">
						<h3 class="text-base font-semibold text-white">{{.Name}}</h3>
						{{if .IsHealthy}}
							<div class="w-3 h-3 rounded-full bg-green-500" title="Healthy"></div>
						{{else}}
							<div class="w-3 h-3 rounded-full bg-red-500" title="Unhealthy"></div>
						{{end}}
					</div>
					
					<div class="grid grid-cols-2 gap-2 text-sm">
						<div>
							<span class="text-gray-400">Priority:</span>
							<span class="text-white ml-1">{{.Priority}}</span>
						</div>
						<div>
							<span class="text-gray-400">Rate:</span>
							<span class="text-white ml-1">{{.RateLimit}}/s</span>
						</div>
						<div class="col-span-2">
							<span class="text-gray-400">Block:</span>
							<span class="text-white font-mono ml-1">{{.LatestBlock}}</span>
						</div>
					</div>
					
					{{if .LastError}}
					<div class="mt-2 p-2 bg-red-900/20 rounded border border-red-800">
						<p class="text-xs text-red-400 truncate" title="{{.LastError}}">{{.LastError}}</p>
					</div>
					{{end}}
				</div>
				{{end}}
			</div>
		</div>
		
		<div class="mt-4 text-center text-gray-500 text-sm">
			<p>Auto-refreshing every 5 seconds</p>
		</div>
	</div>
</div>`

type RemoteInfo struct {
	Name        string
	URL         string
	Priority    int
	IsHealthy   bool
	LatestBlock uint64
	RateLimit   float64
	LastError   string
}

type ChainDetailData struct {
	Chain         ChainInfo
	Remotes       []RemoteInfo
	ActiveRemotes int
	TotalRemotes  int
}

var chainDetailTmpl = template.Must(template.New("chainDetail").Parse(chainDetailHTML))

func RenderChainDetail(w io.Writer, data ChainDetailData) error {
	content := &stringWriter{}
	err := chainDetailTmpl.Execute(content, data)
	if err != nil {
		return err
	}
	
	return layoutTmpl.Execute(w, struct {
		Title   string
		Content template.HTML
	}{
		Title:   data.Chain.Name + " - Venn Dashboard",
		Content: template.HTML(content.String()),
	})
}