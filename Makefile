export ROOT_DIR:=$(strip $(shell dirname $(realpath $(lastword $(MAKEFILE_LIST)))))
export TF_CLI_CONFIG_FILE:=${ROOT_DIR}/test/test.tfrc
export GOBIN=${ROOT_DIR}/bin

NPROCS = $(shell grep -c 'processor' /proc/cpuinfo)
MAKEFLAGS += -j$(NPROCS)


ROOT_DIR:=$(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))

# todo: this does not seems to rebuild when a file changes and should be fixed
${GOBIN}/terraform-provider-setup: **.go go.*
	go install

internal/provider/clients/test_server.tar: internal/provider/clients/test_server/*
	cd internal/provider/clients/test_server && tar -cvf ../test_server.tar .

build-assets: internal/provider/clients/test_server.tar

build: ${GOBIN}/terraform-provider-setup build-assets

tests: build-assets
	TF_ACC=True go test -v ./...

test-terraform: build
	cd test && rm -rf .terraform || true
	cd test && rm terraform.tfstate || true
	cd test && terraform init
	cd test && TF_LOG=DEBUG terraform apply -auto-approve

test-env: ssh-key test
	cd test && docker compose up --force-recreate --build

ssh-test-env:
	ssh -i .ssh/id_rsa -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -p 1234 test@localhost

ssh-key: .ssh/id_rsa .ssh/id_rsa.pub test/image/authorized_keys

test/image/authorized_keys: .ssh/id_rsa.pub
	cat .ssh/id_rsa.pub > test/image/authorized_keys

.ssh/id_rsa .ssh/id_rsa.pub:
	mkdir -p .ssh
	ssh-keygen -t rsa -b 4096 -f .ssh/id_rsa -N ""


clean:
	rm -rf bin

${GOBIN}/tools:
	mkdir -p ${GOBIN}/tools

${GOBIN}/tools/golangci-lint: ${GOBIN}/tools
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b ${GOBIN}/tools/golangci-lint v1.64.5
	chmod +x ${GOBIN}/tools/golangci-lint

lint:${GOBIN}/tools/golangci-lint
	${GOBIN}/tools/golangci-lint//golangci-lint run --config ${ROOT_DIR}/.golangci.yml

ci: build tests lint

bin/goreleaser:
	go install github.com/goreleaser/goreleaser/v2@latest
	
release: bin/goreleaser build-assets
	bin/goreleaser release --clean

create-release-v%:
	$(eval VERSION := $(subst create-release-,,$@))
	@echo "Releasing version ${VERSION}"
	git tag -a ${VERSION} -m "Release ${VERSION}"
	git push origin ${VERSION}