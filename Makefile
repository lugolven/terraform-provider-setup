export ROOT_DIR:=$(strip $(shell dirname $(realpath $(lastword $(MAKEFILE_LIST)))))
export TF_CLI_CONFIG_FILE:=${ROOT_DIR}/test/test.tfrc
export GOBIN=${ROOT_DIR}/bin

NPROCS = $(shell grep -c 'processor' /proc/cpuinfo)
MAKEFLAGS += -j$(NPROCS)

# todo: this does not seems to rebuild when a file changes and should be fixed
${GOBIN}/terraform-provider-setup: **.go go.*
	go install

build: ${GOBIN}/terraform-provider-setup


tests: build ssh-key
	go test -v ./...

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