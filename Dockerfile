FROM golang:alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 go build -o /truenas-power-manager ./cmd/truenas-power-manager/

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata ipmitool
COPY --from=builder /truenas-power-manager /usr/local/bin/truenas-power-manager
ENTRYPOINT ["/usr/local/bin/truenas-power-manager"]
