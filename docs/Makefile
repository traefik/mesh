#######
# This Makefile contains all targets related to the documentation
#######

DOCS_VERIFY_SKIP ?= false
DOCS_LINT_SKIP ?= false

MAESH_DOCS_BUILD_IMAGE ?= maesh-docs
MAESH_DOCS_CHECK_IMAGE ?= $(MAESH_DOCS_BUILD_IMAGE)-check

SITE_DIR := $(CURDIR)/site

DOCKER_RUN_DOC_PORT := 8000
DOCKER_RUN_DOC_MOUNTS := -v $(CURDIR):/mkdocs 
DOCKER_RUN_DOC_OPTS := --rm $(DOCKER_RUN_DOC_MOUNTS) -p $(DOCKER_RUN_DOC_PORT):8000

# Default: generates the documentation into $(SITE_DIR)
docs: clean image lint build verify

# Writer Mode: build and serve docs on http://localhost:8000 with livereload
serve: image
	docker run  $(DOCKER_RUN_DOC_OPTS) $(MAESH_DOCS_BUILD_IMAGE) mkdocs serve

# Utilities Targets for each step
image:
	docker build -t $(MAESH_DOCS_BUILD_IMAGE) -f docs.Dockerfile ./

build: image
	docker run $(DOCKER_RUN_DOC_OPTS) $(MAESH_DOCS_BUILD_IMAGE) sh -c "mkdocs build \
		&& chown -R $(shell id -u):$(shell id -g) ./site"

verify: build
	@if [ "$(DOCS_VERIFY_SKIP)" != "true" ]; then \
		docker build -t $(MAESH_DOCS_CHECK_IMAGE) -f check.Dockerfile ./; \
		docker run --rm -v $(CURDIR):/app $(MAESH_DOCS_CHECK_IMAGE) /verify.sh; \
	else \
		echo "DOCS_VERIFY_SKIP is true: no verification done."; \
	fi

lint:
	@if [ "$(DOCS_LINT_SKIP)" != "true" ]; then \
		docker build -t $(MAESH_DOCS_CHECK_IMAGE) -f check.Dockerfile ./ && \
		docker run --rm -v $(CURDIR):/app $(MAESH_DOCS_CHECK_IMAGE) /lint.sh; \
	else \
		echo "DOCS_LINT_SKIP is true: no linting done."; \
	fi

clean:
	rm -rf $(SITE_DIR)

.PHONY: all verify docs clean build lint
