.PHONY: build-image

# Build the container image using ko via go run (local only, no push)
build-image:
	@echo "Building container image locally with ko..."
	KO_DOCKER_REPO=ko.local go run github.com/google/ko@latest build ./cmd/venn
