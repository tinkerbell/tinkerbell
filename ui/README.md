# Tinkerbell UI

This is the Tinkerbell user interface built with [templ](https://github.com/a-h/templ), a Go templating library that compiles to Go code, and [Tailwind CSS](https://tailwindcss.com/).

## Building

The UI is built as part of the main Tinkerbell binary. Use the targets from the top-level Makefile:

```bash
# Generate all UI files (templ templates and tailwind CSS)
make ui-generate

# Watch and rebuild CSS on changes (for development)
make ui-css-watch

# Clean UI build artifacts
make ui-clean
```

## Project Structure

```bash
ui/
├── assets/              # Static assets
│   ├── artwork/         # Logos and images
│   ├── css/             # Tailwind CSS input and output
│   └── assets.go        # Go embed for static files
├── internal/http/       # HTTP handlers
│   ├── auth.go          # Authentication middleware
│   ├── bmc.go           # BMC resource handlers
│   ├── hardware.go      # Hardware resource handlers
│   ├── kube.go          # Kubernetes client helpers
│   ├── pagination.go    # Pagination utilities
│   ├── search.go        # Global search handler
│   ├── templates.go     # Template resource handlers
│   └── workflows.go     # Workflow resource handlers
├── templates/           # Templ template files
│   ├── components.templ # Reusable UI components
│   ├── details.templ    # Resource detail pages
│   ├── icons.go         # SVG icon functions
│   ├── layout.templ     # Base layout template
│   ├── pages.templ      # Page templates
│   ├── scripts.templ    # JavaScript functionality
│   ├── tables.templ     # Table components
│   └── types.go         # Template data types
├── Makefile             # UI-specific build targets (included by top-level)
├── package.json         # Bun dependencies (Tailwind CSS)
└── ui.go               # Main UI entry point
```

## Features

- 🌙 Dark/Light mode toggle with localStorage persistence
- 📱 Responsive design with mobile navigation menu
- 🔍 Global search across all resources
- 🗂️ Collapsible navigation with BMC dropdown
- ⚡ Server-side rendering with templ
- 🎨 Tailwind CSS styling with custom Tinkerbell theme

## Development

### Modifying Templates

1. Edit `.templ` files in the `templates/` directory
2. Run `make ui-generate` to compile templates and CSS
3. Restart Tinkerbell to see changes

### Template Syntax

templ uses Go-like syntax for templates:

- `{ variable }` - Output variables
- `@ComponentName()` - Render components
- `if condition { }` - Conditional rendering
- `for item := range items { }` - Loops

Example:

```templ
templ MyComponent(title string, items []string) {
    <h1>{ title }</h1>
    <ul>
        for _, item := range items {
            <li>{ item }</li>
        }
    </ul>
}
```

### Modifying Styles

1. Edit `assets/css/input.css` for Tailwind configuration or custom CSS
2. Run `make ui-css` to rebuild, or `make ui-css-watch` for auto-rebuild
