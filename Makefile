GO_FILES := $(shell find . -type f -name '*.go')

opencloud-sftp: $(GO_FILES)
	go build

.PHONY: test-e2e
test-e2e:
	go test -v ./e2e_tests -tags=e2e