GO_FILES := $(shell find . -type f -name '*.go')
OC_BINARY_DL_URL := 'https://github.com/opencloud-eu/opencloud/releases/download/v3.0.0/opencloud-3.0.0-linux-amd64'
E2E_TESTS_DOCKER_IMAGE := oc-sftp-e2e

opencloud-sftp: $(GO_FILES)
	go build

.PHONY: clean
clean: delete-test-containers
	rm -f ./opencloud && rm -f opencloud-sftp


#### End to End tests ####
# Run the end to end tests against already running OpenCloud instance and SFTP server.
.PHONY: test-e2e
test-e2e:
	go test -v ./test/e2e -tags=e2e


### Run this to execute the end to end tests in a Docker container.
.PHONY: test-e2e-docker
test-e2e-docker: build-e2e-test-image
	docker run --env-file ./test/e2e/config.env $(E2E_TESTS_DOCKER_IMAGE)

build-e2e-test-image: ./opencloud
	docker build -f ./test/e2e/Dockerfile -t $(E2E_TESTS_DOCKER_IMAGE) .

./opencloud:
	wget -O ./opencloud $(OC_BINARY_DL_URL) && chmod +x ./opencloud

delete-test-containers:
	@if [ -n "$$(docker ps -a -q --filter ancestor=$(E2E_TESTS_DOCKER_IMAGE))" ]; then \
		docker rm -f $$(docker ps -a -q --filter ancestor=$(E2E_TESTS_DOCKER_IMAGE)); \
	else \
		echo "No containers found for image $(E2E_TESTS_DOCKER_IMAGE)"; \
	fi
