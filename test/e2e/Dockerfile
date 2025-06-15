FROM golang:1.24-bookworm

COPY . /src/opencloud-sftp
WORKDIR /src/opencloud-sftp

EXPOSE 9200

RUN cp -v ./test/e2e/docker/init.sh ./test/e2e/docker/entrypoint.sh . && \
    chmod +x ./init.sh ./entrypoint.sh && \
    ./init.sh

ENTRYPOINT ["./entrypoint.sh"]



