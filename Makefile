export ROOT_DIR:=$(strip $(shell dirname $(realpath $(lastword $(MAKEFILE_LIST)))))
export TF_CLI_CONFIG_FILE:=${ROOT_DIR}/test/test.tfrc
export GOBIN=${ROOT_DIR}/bin

NPROCS = $(shell grep -c 'processor' /proc/cpuinfo)
MAKEFLAGS += -j$(NPROCS)

${GOBIN}/terraform-provider-setup: *.go go.*
	go install

build: ${GOBIN}/terraform-provider-setup


tests: build ssh-key
	go test -v ./...

test-terraform: build
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


build-%:
	GOOS=$(shell echo "$*" | cut -d "-" -f 1) GOARCH=$(shell echo "$*" | cut -d "-" -f 2) go build -o bin/$*/terraform-provider-setup .
	zip -j bin/terraform-provider-setup-$*.zip bin/$*/terraform-provider-setup

crossplatform-build: build-linux-amd64 build-linux-arm64 build-darwin-amd64 build-darwin-arm64

bin/tools:
	mkdir -p bin/tools

bin/tools/gh:bin/tools
	curl -sL https://github.com/cli/cli/releases/download/v2.65.0/gh_2.65.0_linux_arm64.tar.gz | tar -xz -C bin/tools gh_2.65.0_linux_arm64/bin/gh 
	mv bin/tools/gh_2.65.0_linux_arm64/bin/gh bin/tools/gh 
	rm -rf bin/tools/gh_2.65.0_linux_arm64

draft-release: bin/tools/gh crossplatform-build
	$(eval VERSION="v0.$(shell date +"%Y%m%d%H%M")")
	bin/tools/gh release create ${VERSION} --title ${VERSION} --draft --prerelease --generate-notes bin/terraform-provider-setup-*.zip 

clean:
	rm -rf bin