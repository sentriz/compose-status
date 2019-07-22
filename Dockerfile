FROM golang:1.12-alpine AS builder
WORKDIR /src
COPY . .
RUN \
    apk add -U --no-cache git && \
    GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-w -s" -o compose-status *.go

FROM scratch
COPY --from=builder /src/compose-status /bin/
ENV GONIC_DB_PATH /data/db.db
ENV GONIC_LISTEN_ADDR :80
EXPOSE 80
VOLUME ["/var/run/docker.sock", "/data"]
CMD ["/bin/compose-status"]
