.PHONY: test test-go test-server test-installer release vercel-release demo up down logs

test: test-go test-server test-installer

test-go:
	cd agent/codex-guard && go test ./...

test-server:
	PYTHONPATH=server python3 -m pytest -q server/tests

test-installer:
	sh tests/test_installer.sh

release:
	./scripts/build-release.sh

vercel-release:
	./scripts/build-vercel-release.sh

demo:
	docker compose exec -T server python -m app.demo

up:
	docker compose up -d --build

down:
	docker compose down

logs:
	docker compose logs -f server caddy
