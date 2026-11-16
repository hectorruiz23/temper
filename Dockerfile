FROM --platform=$BUILDPLATFORM golang:1.27.4-bookworm AS build

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src/api

COPY api/go.mod api/go.sum ./
RUN go mod download

COPY api/ ./
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/temper ./cmd/temper

FROM alpine:3.21

RUN apk add --no-cache sqlite

WORKDIR /app

COPY --from=build /out/temper /app/temper
COPY api/scripts/backup.sh /app/backup.sh
RUN chmod 0755 /app/backup.sh

USER 1000:1000

ENV TEMPER_ADDR=0.0.0.0:8787 \
    TEMPER_DB=/data/temper.db

VOLUME ["/data"]

EXPOSE 8787

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1:8787/api/health || exit 1

ENTRYPOINT ["/app/temper"]
