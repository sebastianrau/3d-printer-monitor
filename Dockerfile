FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN for attempt in 1 2 3; do \
        go mod download && break; \
        if [ "$attempt" -eq 3 ]; then exit 1; fi; \
        sleep $((attempt * 5)); \
    done
COPY . .
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
RUN CGO_ENABLED=0 GOOS="$TARGETOS" GOARCH="$TARGETARCH" GOARM="${TARGETVARIANT#v}" \
    go build -trimpath -ldflags="-s -w" -o /out/3d-printer-monitor ./cmd/3d-printer-monitor

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates ffmpeg && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/3d-printer-monitor /3d-printer-monitor
USER 65532:65532
ENTRYPOINT ["/3d-printer-monitor"]
CMD ["--config", "/etc/3d-printer-monitor/config.yaml"]
