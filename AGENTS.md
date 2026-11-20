# Repository Guidelines

## Project Structure

- `backend/` is the Go API using Gin, GORM, and MySQL 8. The server entry point is `cmd/server/`. Route registration lives in `internal/router/`, HTTP handlers in `internal/handler/`, business logic in `internal/service/`, and persistence models in `internal/model/`.
- `frontend/` is the Vue 3 and Vite app. Pages live in `src/views/`, reusable components in `src/components/`, API clients in `src/api/`, and Pinia stores in `src/store/`. Assets live in `src/assets/` and `public/`.
- `docs/requirements.md` describes the product. `deploy/host/` contains systemd and Nginx deployment files.

## Build and Development

Run these commands from the repository root.

- `(cd backend && go run ./cmd/server)` starts the API. Configure a local MySQL instance using `backend/config.yaml`.
- `(cd frontend && npm ci && npm run dev)` installs locked dependencies and starts Vite on port 5173. Vite proxies `/api` to port 8080.
- `(cd frontend && npm run build)` builds production assets into `frontend/dist/`.
- `cp .env.example .env` sets up Compose configuration. Review the values, then run `docker compose up -d --build` to start the stack.
- `bash scripts/build.sh` rebuilds the host deployment bundle in `release/`.

## Coding Style

Run `gofmt` on changed Go files and keep package names lowercase. Match nearby Vue and JavaScript files with two-space indentation, single-quoted strings, and PascalCase component filenames such as `ModelDetail.vue`. Use the shared Axios client in `src/api/request.js` for API calls.

## Testing

Run `(cd backend && go test ./...)` for backend changes. Place Go tests beside the package code in `*_test.go` files and name test functions `TestXxx`. For frontend changes, run the production build and exercise the affected flows against the local API. For a stack smoke check, request `/api/health` on the configured HTTP port.

## Commits and Pull Requests

Use short, descriptive commit subjects. In pull requests, explain the change, list verification steps, link the issue when available, and include screenshots for visible UI changes. Document any new configuration or migration steps. Keep database credentials and JWT secrets out of commits.
