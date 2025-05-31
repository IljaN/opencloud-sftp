GO_FILES := $(shell find . -type f -name '*.go')

opencloud-sftp: $(GO_FILES)
	go build