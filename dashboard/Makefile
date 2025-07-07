.PHONY: all templ tailwind build clean

all: build

TMPL_COMMAND := go run github.com/a-h/templ/cmd/templ@latest

templ:
	@echo "Generating templ files..."
	@$(TMPL_COMMAND) generate

tailwind:
	@echo "Building Tailwind CSS..."
	@tailwindcss -i ./app.css -o ./static/css/output.css --minify

build: templ tailwind
	@echo "Dashboard build complete!"

clean:
	@echo "Cleaning generated files..."
	@rm -f templates/*_templ.go
	@rm -f static/css/output.css

watch-templ:
	@$(TMPL_COMMAND) generate --watch

watch-tailwind:
	@tailwindcss -i ./app.css -o ./static/css/output.css --watch
