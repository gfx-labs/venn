#!/bin/bash

# Install dependencies if needed
if ! command -v templ &> /dev/null; then
    echo "Installing templ..."
    go install github.com/a-h/templ/cmd/templ@latest
fi

if ! command -v tailwindcss &> /dev/null; then
    echo "Installing tailwindcss..."
    curl -sLO https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-linux-x64
    chmod +x tailwindcss-linux-x64
    sudo mv tailwindcss-linux-x64 /usr/local/bin/tailwindcss
fi

# Generate templ files
echo "Generating templ files..."
templ generate

# Build Tailwind CSS
echo "Building Tailwind CSS..."
tailwindcss -i ./static/css/tailwind.css -o ./static/css/output.css --minify

echo "Dashboard build complete!"