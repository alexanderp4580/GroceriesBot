FROM docker.io/library/golang:1.27.2-bookworm@sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY migrations ./migrations
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=1 go build -trimpath -ldflags='-s -w' -o /out/groceriesbot ./cmd/groceriesbot

FROM docker.io/library/debian:bookworm-slim@sha256:a4672c0cb26fbdde88e38fa2dfb6c681942306680e41e4378b28770b6e79ee91
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata && rm -rf /var/lib/apt/lists/* && mkdir /data /app && chown 10001:10001 /data && chmod 700 /data
COPY --from=build /out/groceriesbot /app/groceriesbot
USER 10001:10001
WORKDIR /app
ENV DATABASE_PATH=/data/groceriesbot.sqlite3 RUN_MODE=paused GOMEMLIMIT=64MiB GOMAXPROCS=1
ENTRYPOINT ["/app/groceriesbot"]
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s CMD ["/app/groceriesbot", "healthcheck"]
