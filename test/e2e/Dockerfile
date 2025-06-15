FROM golang:1.24-bookworm AS deps_cache

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

FROM golang:1.24-bookworm

# Copy dependencies from previous stage
COPY --from=deps_cache /go/pkg /go/pkg
COPY --from=deps_cache /go/src /go/src
COPY --from=deps_cache /go /go


COPY . /src/opencloud-sftp
WORKDIR /src/opencloud-sftp

EXPOSE 9200

RUN cp -v ./test/e2e/docker/init.sh ./test/e2e/docker/entrypoint.sh . && \
    chmod +x ./init.sh ./entrypoint.sh && \
    ./init.sh

ENTRYPOINT ["./entrypoint.sh"]



