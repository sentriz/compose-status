FROM golang:1.23-alpine AS builder
RUN apk add -U --no-cache git
WORKDIR /src
COPY go.mod .
COPY go.sum .
RUN go mod download
COPY . .
RUN GOOS=linux CGO_ENABLED=0 go build -ldflags="-w -s" -o compose-status cmd/compose-status/main.go
FROM scratch
COPY --from=builder /src/compose-status /bin/
ENV CS_SAVE_PATH /data/save.json
ENV CS_LISTEN_ADDR :80
ENV HOST_PROC /host_proc
EXPOSE 80
VOLUME ["/var/run/docker.sock", "/data"]
CMD ["/bin/compose-status"]
