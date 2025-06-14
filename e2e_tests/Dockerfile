FROM golang:1.24-bookworm
EXPOSE 9200
WORKDIR /src/opencloud-sftp
COPY . /src/opencloud-sftp
RUN make && \
    cp opencloud-sftp e2e_tests/config.env /usr/local/bin/ && \
    wget https://github.com/opencloud-eu/opencloud/releases/download/v2.3.0/opencloud-2.3.0-linux-amd64 -O /usr/local/bin/opencloud && \
    chmod +x /usr/local/bin/opencloud && \
    opencloud init -ap admin -f --insecure "yes" && \
    mkdir -p ~/.opencloud/sftp && \
    ssh-keygen -t rsa -b 4096 -f ~/.opencloud/sftp/xyz -N "" && \
    go get && go build -tags=e2e ./...

COPY e2e_tests/start.sh /src/opencloud-sftp/start.sh
RUN chmod +x /src/opencloud-sftp/start.sh

CMD ["./start.sh"]


