# Venn Dashboard

A real-time monitoring dashboard for Venn that displays configured blockchain chains and their current head blocks.

## Features

- Real-time chain status monitoring
- Head block tracking for each configured chain
- Health status indicators
- Auto-refresh every 5 seconds using HTMX
- Dark theme with Tailwind CSS
- Responsive grid layout

## Prerequisites

- Go 1.21+
- [templ](https://github.com/a-h/templ) - Install with: `go install github.com/a-h/templ/cmd/templ@latest`
- [Tailwind CSS](https://tailwindcss.com/) - Install with: `npm install -D tailwindcss`

## Building

1. Generate templ files:
```bash
make templ
```

2. Build Tailwind CSS:
```bash
make tailwind
```

3. Or build everything:
```bash
make build
```

## Development

For development with hot-reloading:

1. Watch templ files:
```bash
make watch-templ
```

2. Watch Tailwind CSS (in another terminal):
```bash
make watch-tailwind
```

## Usage

The dashboard is automatically mounted at `/dashboard` when running the Venn node. 

Access it at: `http://localhost:8080/dashboard`

## Architecture

- **Templates**: Uses [a-h/templ](https://github.com/a-h/templ) for type-safe Go templates
- **Styling**: Tailwind CSS with custom dark theme
- **Interactivity**: HTMX for real-time updates and hyperscript for UI enhancements
- **Static Files**: Embedded using Go's `embed` package for single binary deployment