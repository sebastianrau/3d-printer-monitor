FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/3d-printer-monitor ./cmd/3d-printer-monitor

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/3d-printer-monitor /3d-printer-monitor
ENTRYPOINT ["/3d-printer-monitor"]
CMD ["--config", "/etc/3d-printer-monitor/config.yaml"]
