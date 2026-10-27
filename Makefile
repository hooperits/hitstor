.PHONY: build build-arm64 agent agent-arm64 agent-windows agent-windows-arm64 agent-all \
	test test-integration bench bench-skip-restic lint clean \
	proto proto-backup proto-agent proto-plugin \
	plugin-memory notifier-log hook-log scanner-builtin plugins pluginkit plugin-packages \
	pkg-server-deb pkg-server-rpm pkg-agent-deb pkg-agent-rpm pkg-linux \
	pkg-server-windows pkg-agent-windows pkg-windows \
	installer-server installer-server-arm64 installer-agent installer-agent-arm64 installers \
	docker-build docker-save docker-quickstart-up docker-quickstart-down \
	build-all checksums release release-assets release-linux release-docker \
	docs-serve docs-build docs-deploy

BINARY=bin/hitstor
AGENT_BINARY=bin/hitstor-agent
VERSION ?= dev
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE    ?= $(shell date -u +%Y-%m-%d)
LDFLAGS = -X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.Date=$(DATE)

NFPM      ?= nfpm
MAKENSIS  ?= makensis
DIST_DIR   = dist

# --- Build targets ---

build:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o $(BINARY) ./cmd/hitstor/

build-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(BINARY)-arm64 ./cmd/hitstor/

agent:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o $(AGENT_BINARY) ./cmd/hitstor-agent/

agent-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(AGENT_BINARY)-arm64 ./cmd/hitstor-agent/

agent-windows:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(AGENT_BINARY).exe ./cmd/hitstor-agent/

agent-windows-arm64:
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(AGENT_BINARY)-arm64.exe ./cmd/hitstor-agent/

agent-all: agent agent-arm64 agent-windows agent-windows-arm64

build-all: build build-arm64 build-windows agent-all

# --- Test targets ---

test:
	go test ./...

test-integration:
	go test -tags integration ./...

# --- Benchmark targets ---

PROFILE ?= small

bench:
	./benchmarks/run.sh --profile $(PROFILE)

bench-skip-restic:
	./benchmarks/run.sh --profile $(PROFILE) --skip-restic


lint:
	go vet ./...

# --- Protobuf targets ---

proto: proto-backup proto-agent proto-plugin

proto-backup:
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		internal/proto/hitstor/v1/backup.proto

proto-agent:
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		internal/proto/hitstor/v1/agent.proto

proto-plugin:
	protoc -I api/proto --go_out=api/proto --go_opt=paths=source_relative \
		--go-grpc_out=api/proto --go-grpc_opt=paths=source_relative \
		hitstor/plugin/v1/storage.proto \
		hitstor/plugin/v1/notifier.proto \
		hitstor/plugin/v1/hook.proto \
		hitstor/plugin/v1/scanner.proto \
		hitstor/plugin/v1/workload.proto

# --- Plugin targets ---

plugin-memory:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o bin/hitstor-backend-memory ./cmd/hitstor-backend-memory/

notifier-log:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o bin/hitstor-notifier-log ./cmd/hitstor-notifier-log/

hook-log:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o bin/hitstor-hook-log ./cmd/hitstor-hook-log/

scanner-builtin:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o bin/hitstor-scanner-builtin ./cmd/hitstor-scanner-builtin/

plugins: plugin-memory notifier-log hook-log scanner-builtin

# --- Workload plugin packages (.hpk) ---
#
# Each plugin in plugins/<id>/ becomes one signed package per platform in
# $(DIST_DIR). The signing key comes from PLUGIN_KEY_FILE, or else from the
# HITSTOR_PLUGIN_SIGNING_KEY environment variable (release pipeline).
# PLUGIN_UNSIGNED=1 builds unsigned packages for local development.

WORKLOAD_PLUGINS ?= example
PLUGIN_PLATFORMS ?= linux/amd64 windows/amd64
PLUGINKIT = bin/hitstor-pluginkit

ifdef PLUGIN_KEY_FILE
PLUGIN_SIGN_FLAGS = --key-file $(PLUGIN_KEY_FILE)
else ifeq ($(PLUGIN_UNSIGNED),1)
PLUGIN_SIGN_FLAGS = --unsigned
endif

pluginkit:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o $(PLUGINKIT) ./cmd/hitstor-pluginkit/

plugin-packages: pluginkit
	@mkdir -p $(DIST_DIR) bin/plugins
	@set -e; for p in $(WORKLOAD_PLUGINS); do for plat in $(PLUGIN_PLATFORMS); do \
		os=$${plat%/*}; arch=$${plat#*/}; ext=; \
		if [ "$$os" = windows ]; then ext=.exe; fi; \
		out=bin/plugins/hitstor-plugin-$$p-$$os-$$arch$$ext; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -ldflags="$(LDFLAGS)" -o $$out ./plugins/$$p/; \
		$(PLUGINKIT) pack --source plugins/$$p/plugin.yaml --binary $$out \
			--os $$os --arch $$arch --out $(DIST_DIR) $(PLUGIN_SIGN_FLAGS); \
	done; done

# --- Linux packaging targets (requires nfpm) ---

pkg-server-deb: build
	@mkdir -p $(DIST_DIR)
	cd packaging/nfpm && VERSION=$(VERSION) ARCH=amd64 $(NFPM) package \
		--config hitstor-server.yaml --packager deb --target ../../$(DIST_DIR)/

pkg-server-rpm: build
	@mkdir -p $(DIST_DIR)
	cd packaging/nfpm && VERSION=$(VERSION) ARCH=amd64 $(NFPM) package \
		--config hitstor-server.yaml --packager rpm --target ../../$(DIST_DIR)/

pkg-agent-deb: agent scanner-builtin
	@mkdir -p $(DIST_DIR)
	cd packaging/nfpm && VERSION=$(VERSION) ARCH=amd64 $(NFPM) package \
		--config hitstor-agent.yaml --packager deb --target ../../$(DIST_DIR)/

pkg-agent-rpm: agent scanner-builtin
	@mkdir -p $(DIST_DIR)
	cd packaging/nfpm && VERSION=$(VERSION) ARCH=amd64 $(NFPM) package \
		--config hitstor-agent.yaml --packager rpm --target ../../$(DIST_DIR)/

pkg-linux: pkg-server-deb pkg-server-rpm pkg-agent-deb pkg-agent-rpm

# --- Windows packaging targets (requires makensis) ---

build-windows:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o bin/hitstor.exe ./cmd/hitstor/

render-windows-icon:
	bash scripts/render-windows-icon.sh

pkg-server-windows: build-windows render-windows-icon
	@mkdir -p $(DIST_DIR)
	$(MAKENSIS) -DVERSION=$(VERSION) -DBIN_DIR=../../bin packaging/nsis/hitstor-server.nsi

pkg-agent-windows: agent-windows render-windows-icon
	@mkdir -p $(DIST_DIR)
	$(MAKENSIS) -DVERSION=$(VERSION) -DBIN_DIR=../../bin packaging/nsis/hitstor-agent.nsi

pkg-windows: pkg-server-windows pkg-agent-windows

# --- Self-extracting installer targets ---

installer-server: build
	@mkdir -p $(DIST_DIR)
	sh packaging/installer/build-installer.sh server $(VERSION) amd64

installer-server-arm64: build-arm64
	@mkdir -p $(DIST_DIR)
	sh packaging/installer/build-installer.sh server $(VERSION) arm64

installer-agent: agent
	@mkdir -p $(DIST_DIR)
	sh packaging/installer/build-installer.sh agent $(VERSION) amd64

installer-agent-arm64: agent-arm64
	@mkdir -p $(DIST_DIR)
	sh packaging/installer/build-installer.sh agent $(VERSION) arm64

installers: installer-server installer-agent

# --- Docker targets ---

docker-build:
	docker build -t hitstor:$(VERSION) \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg DATE=$(DATE) .

docker-save: docker-build
	@mkdir -p $(DIST_DIR)
	docker save hitstor:$(VERSION) | gzip > $(DIST_DIR)/hitstor-$(VERSION)-docker.tar.gz

docker-quickstart-up:
	@test -f .env || (echo "Copy .env.example to .env and set HITSTOR_PASSWORD" && exit 1)
	docker compose -f docker-compose.quickstart.yml up -d --build

docker-quickstart-down:
	docker compose -f docker-compose.quickstart.yml down

# --- Release targets ---

checksums:
	cd $(DIST_DIR) && rm -f SHA256SUMS && sha256sum * > SHA256SUMS

release-linux: pkg-linux
	@echo "Linux packages built in $(DIST_DIR)/"

release-docker: docker-save
	@echo "Docker image saved to $(DIST_DIR)/"

# Files attached to a GitHub release (.github/workflows/release.yml). The
# Docker image is published separately by docker-publish.yml.
release-assets: clean build-all pkg-linux pkg-windows installers installer-server-arm64 installer-agent-arm64 checksums
	@echo ""
	@echo "Release $(VERSION) assets:"
	@ls -lh $(DIST_DIR)/

release: clean build-all pkg-linux pkg-windows installers installer-server-arm64 installer-agent-arm64 docker-save checksums
	@echo ""
	@echo "Release $(VERSION) artifacts:"
	@ls -lh $(DIST_DIR)/

# --- Documentation targets (requires mkdocs-material) ---

docs-serve:
	mkdocs serve

docs-build:
	mkdocs build --strict

docs-deploy:
	mkdocs gh-deploy --force

# --- Clean ---

clean:
	rm -rf bin/ $(DIST_DIR)/ site/
