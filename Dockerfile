# syntax=docker/dockerfile:1

# Build stage runs on the build host's architecture and cross-compiles, so
# multi-arch builds do not need emulation for the Go compiler.
FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine@sha256:d530cf60723abf1ffae0cd16bb546b379ec0550d2f3815e5b16ed69dbc00fba4 AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath \
      -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
      -o /out/oraclewatch ./cmd/oraclewatch

# Runtime: no shell, no package manager, uid 65532.
FROM gcr.io/distroless/static:nonroot@sha256:18f4cc26c9ffa7f20a359298be7deadac5c607c8e03a02b8119f1a5fc2f82c99
COPY --from=build /out/oraclewatch /usr/local/bin/oraclewatch
USER nonroot:nonroot
EXPOSE 9090
ENTRYPOINT ["/usr/local/bin/oraclewatch"]
CMD ["run", "--config", "/etc/oraclewatch/config.yaml", "--listen", "0.0.0.0:9090"]
