FROM golang:1.25-alpine AS agent-builder
WORKDIR /src
COPY agent/codex-guard/ ./
RUN mkdir -p /artifacts && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o /artifacts/codex-guard-linux-amd64 ./cmd/codex-guard && \
    CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags='-s -w' -o /artifacts/codex-guard-linux-arm64 ./cmd/codex-guard && \
    CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o /artifacts/codex-guard-darwin-amd64 ./cmd/codex-guard && \
    CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags='-s -w' -o /artifacts/codex-guard-darwin-arm64 ./cmd/codex-guard

FROM alpine:3.22 AS artifacts
RUN apk add --no-cache curl tar coreutils
COPY --from=agent-builder /artifacts /artifacts
COPY scripts/fetch-ccusage.sh /fetch-ccusage.sh
COPY installer/install.sh installer/uninstall.sh /artifacts/
RUN chmod +x /fetch-ccusage.sh /artifacts/install.sh /artifacts/uninstall.sh && /fetch-ccusage.sh /artifacts && cd /artifacts && sha256sum codex-guard-* ccusage-* > checksums.txt

FROM python:3.13-slim
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1 ARTIFACTS_DIR=/app/artifacts
WORKDIR /app
COPY server/requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY server/app ./app
COPY --from=artifacts /artifacts ./artifacts
EXPOSE 8000
CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000", "--proxy-headers", "--forwarded-allow-ips=*"]
