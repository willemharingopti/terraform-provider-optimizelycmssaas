PROVIDER   := optimizelycmssaas
NAMESPACE  ?= willemharingopti
VERSION    ?= 0.1.0
OS_ARCH    := $(shell go env GOOS)_$(shell go env GOARCH)
MIRROR     := $(HOME)/.terraform.d/plugins/registry.terraform.io/$(NAMESPACE)/$(PROVIDER)/$(VERSION)/$(OS_ARCH)

.PHONY: build test fmt vet docs install snapshot check

build:
	go build -o terraform-provider-$(PROVIDER) .

fmt:
	gofmt -w .

vet:
	go vet ./...

test:
	go test ./... -count=1

# Regenerates docs/ from the schema, examples/ and templates/ (needs terraform on PATH).
docs:
	go generate ./...

# Installs a local build into Terraform's implied local mirror for testing without the registry.
# After rebuilding, delete .terraform.lock.hcl and .terraform/ in the configuration and run `terraform init`.
install:
	mkdir -p $(MIRROR)
	go build -ldflags "-X main.version=$(VERSION)" -o $(MIRROR)/terraform-provider-$(PROVIDER)_v$(VERSION) .

# Validates the release configuration and builds every platform without signing or publishing.
check:
	go run github.com/goreleaser/goreleaser/v2@latest check

snapshot:
	go run github.com/goreleaser/goreleaser/v2@latest release --snapshot --clean --skip=sign,publish
