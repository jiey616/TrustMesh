# Repository Guidelines

## Project Structure & Module Organization
`TrustMesh` is split into a Go backend and a React frontend. `backend/cmd/server` contains the entrypoint, and `backend/internal` holds the application layers: `app`, `handler`, `store`, `middleware`, `clawsynapse`, `auth`, and `config`. Backend tests live beside the code as `*_test.go`. `frontend/src` is organized by feature: `pages`, `components`, `api`, `hooks`, `stores`, `lib`, and `types`. Shared docs live in `docs/`, and full-stack local orchestration lives in `docker-compose.yml`.

## Build, Test, and Development Commands
From the repo root, start the full stack with `docker compose up -d --build`; stop it with `docker compose down`.

Backend:
- `cd backend && go run ./cmd/server` starts the API locally.
- `cd backend && go test ./...` runs all unit tests.
- `cd backend && go build ./cmd/server` verifies the server binary builds.
- `cd backend && bash ./scripts/smoke-task-flow.sh` runs the basic end-to-end smoke flow.

Frontend:
- `cd frontend-v2 && npm install` installs dependencies.
- `cd frontend-v2 && npm run dev` starts the Vite dev server.
- `cd frontend-v2 && npm run build` runs `tsc -b` and builds production assets.
- `cd frontend-v2 && npm run lint` runs ESLint.

## Coding Style & Naming Conventions
Go code should stay `gofmt`-formatted, with packages in lowercase and exported identifiers in `PascalCase`. Keep backend logic inside `internal/store` and `internal/handler` rather than `main`. Frontend code uses TypeScript, ES modules, and 2-space indentation. Name React components and pages in `PascalCase` (`ProjectBoardPage.tsx`), hooks with `use*`, and Zustand stores with `*Store`. Use the existing ESLint setup in `frontend/eslint.config.js`; do not introduce a separate formatter without agreement.

## Testing Guidelines
Backend coverage is test-driven today; add table-driven Go tests next to changed packages using `*_test.go`. Run `go test ./...` before opening a PR, and run the smoke scripts when changing webhook, task flow, or ClawSynapse integration. There is no frontend test suite yet, so at minimum run `npm run build` and `npm run lint` for UI changes.

## Commit & Pull Request Guidelines
Recent history uses short imperative subjects such as `Refactor layout components...` and `Enhance notification system...`. Keep commits focused and descriptive. PRs should include a concise summary, the commands you ran to validate changes, linked issues if any, and screenshots or short recordings for visible frontend updates. Never commit secrets; keep local overrides in `backend/.env` and environment variables such as `NATS_SERVERS`.
