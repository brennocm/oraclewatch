# syntax=docker/dockerfile:1

# Build stage runs on the build host's architecture and cross-compiles, so
# multi-arch builds do not need emulation for the Go compiler.
FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine@sha256:d200d2dd76b03a99b52651fd1325aff7dcd35422c35c01bac56571b0507f3deb AS build
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
FROM gcr.io/distroless/static:nonroot@sha256:d62067879de890a1def9da0e7766b7d20810a5e5555cef8506b752437d7a3e88
COPY --from=build /out/oraclewatch /usr/local/bin/oraclewatch
USER nonroot:nonroot
EXPOSE 9090
ENTRYPOINT ["/usr/local/bin/oraclewatch"]
CMD ["run", "--config", "/etc/oraclewatch/config.yaml", "--listen", "0.0.0.0:9090"]
