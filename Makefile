module = gitlab.com/oppy-finance/oppy-bridge

.PHONY: clear tools install test test-watch lint-pre lint lint-verbose protob build docker-gitlab-login docker-gitlab-push docker-gitlab-build


VERSION := $(shell echo $(shell git describe --tags --first-parent) | sed 's/^v//')
COMMIT := $(shell git log -1 --format='%H')

all: lint build

clear:
	clear


BUILD_FLAGS := -ldflags '$(ldflags)'

testnet: go.sum
	go install   -ldflags "-X gitlab.com/oppy-finance/oppy-bridge/pubchain.OppyContractAddressBSC=0x94277968dff216265313657425d9d7577ad32dd1 \
      -X gitlab.com/oppy-finance/oppy-bridge/pubchain.OppyContractAddressETH=0x395931E1f64f1DC1889cbC3208dD746667C31126\
    	-X  gitlab.com/oppy-finance/oppy-bridge/version.VERSION=$(VERSION) \
    	-X gitlab.com/oppy-finance/oppy-bridge/version.COMMIT=$(COMMIT)" ./cmd/bridge_service.go

install: go.sum
	go install   -ldflags "-X gitlab.com/oppy-finance/oppy-bridge/pubchain.OppyContractAddressBSC=0x66fff09f83bfce2ed9240fa6a1f7e96ba166ddf7 \
      -X gitlab.com/oppy-finance/oppy-bridge/pubchain.OppyContractAddressETH=0x77406A7678338abb5eA7a78b766F7F1125782C61 \
	-X  gitlab.com/oppy-finance/oppy-bridge/version.VERSION=$(VERSION) \
    	-X gitlab.com/oppy-finance/oppy-bridge/version.COMMIT=$(COMMIT)" ./cmd/bridge_service.go

go.sum: go.mod
	@echo "--> Ensure dependencies have not been modified"
	go mod verify

test:
	@go test --race ./...

test-watch: clear
	@gow -c test -tags testnet -mod=readonly ./...

lint-pre:
	@gofumpt -l  cmd config oppybridge monitor storage validators bridge common misc pubchain tssclient
	#@test -z "$(shell gofumpt -l  cmd config cosbridge monitor storage validators bridge common misc pubchain tssclient)" # cause error
	@go mod verify

lint: lint-pre
		@golangci-lint run --out-format=tab  -v --timeout 3600s -c ./.golangci.yml

lint-verbose: lint-pre
	@golangci-lint run -v

#protob:
#	protoc --go_out=module=$(module):. ./messages/*.proto



build: go.sum
	go build -ldflags "-X gitlab.com/oppy-finance/oppy-bridge/pubchain.OppyContractAddressBSC=0x66fff09f83bfce2ed9240fa6a1f7e96ba166ddf7 \
      -X gitlab.com/oppy-finance/oppy-bridge/pubchain.OppyContractAddressETH=0x77406A7678338abb5eA7a78b766F7F1125782C61 \
	-X  gitlab.com/oppy-finance/oppy-bridge/version.VERSION=$(VERSION) \
    	-X gitlab.com/oppy-finance/oppy-bridge/version.COMMIT=$(COMMIT)" ./cmd/bridge_service.go



format:
	@gofumpt -l -w .

# ------------------------------- GitLab ------------------------------- #
docker-gitlab-login:
	docker login -u ${CI_REGISTRY_USER} -p ${CI_REGISTRY_PASSWORD} ${CI_REGISTRY}

docker-gitlab-push:
	docker push registry.gitlab.com/oppy/tss/go-tss

docker-gitlab-build:
	docker build -t registry.gitlab.com/oppy/tss/go-tss .
	docker tag registry.gitlab.com/oppy/tss/go-tss $$(git rev-parse --short HEAD)
