module = gitlab.com/oppy-finance/oppy-bridge

.PHONY: clear tools install test test-watch lint-pre lint lint-verbose protob build docker-gitlab-login docker-gitlab-push docker-gitlab-build

all: lint build

clear:
	clear




BUILD_FLAGS := -ldflags '$(ldflags)'

testnet: go.sum
	go install   -ldflags "-X gitlab.com/oppy-finance/oppy-bridge/pubchain.OppyContractAddress=0x94277968dff216265313657425d9d7577ad32dd1"       ./cmd/oppyBridge.go

install: go.sum
	go install   -ldflags "-X gitlab.com/oppy-finance/oppy-bridge/pubchain.OppyContractAddress=0x66fff09f83bfce2ed9240fa6a1f7e96ba166ddf7"       ./cmd/oppyBridge.go

go.sum: go.mod
	@echo "--> Ensure dependencies have not been modified"
	go mod verify

test:
	@go test --race ./...

test-watch: clear
	@gow -c test -tags testnet -mod=readonly ./...

lint-pre:
	@gofumpt -l  cmd config oppybridge monitor storage validators bridge common misc pubchain tssclient
	#@test -z "$(shell gofumpt -l  cmd config oppybridge monitor storage validators bridge common misc pubchain tssclient)" # cause error
	@go mod verify

lint: lint-pre
	@golangci-lint run

lint-verbose: lint-pre
	@golangci-lint run -v

#protob:
#	protoc --go_out=module=$(module):. ./messages/*.proto



build: go.sum
	go build   -ldflags "-X gitlab.com/oppy-finance/oppy-bridge/pubchain.OppyContractAddress=0x66fff09f83bfce2ed9240fa6a1f7e96ba166ddf7"       ./cmd/oppyBridge.go

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
