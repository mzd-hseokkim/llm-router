# CLAUDE.md

Behavioral guidelines to reduce common LLM coding mistakes. Merge with project-specific instructions as needed.

**Tradeoff:** These guidelines bias toward caution over speed. For trivial tasks, use judgment.

## 1. Think Before Coding

**Don't assume. Don't hide confusion. Surface tradeoffs.**

Before implementing:
- State your assumptions explicitly. If uncertain, ask.
- If multiple interpretations exist, present them - don't pick silently.
- If a simpler approach exists, say so. Push back when warranted.
- If something is unclear, stop. Name what's confusing. Ask.

## 2. Simplicity First

**Minimum code that solves the problem. Nothing speculative.**

- No features beyond what was asked.
- No abstractions for single-use code.
- No "flexibility" or "configurability" that wasn't requested.
- No error handling for impossible scenarios.
- If you write 200 lines and it could be 50, rewrite it.

Ask yourself: "Would a senior engineer say this is overcomplicated?" If yes, simplify.

## 3. Surgical Changes

**Touch only what you must. Clean up only your own mess.**

When editing existing code:
- Don't "improve" adjacent code, comments, or formatting.
- Don't refactor things that aren't broken.
- Match existing style, even if you'd do it differently.
- If you notice unrelated dead code, mention it - don't delete it.

When your changes create orphans:
- Remove imports/variables/functions that YOUR changes made unused.
- Don't remove pre-existing dead code unless asked.

The test: Every changed line should trace directly to the user's request.

## 4. Goal-Driven Execution

**Define success criteria. Loop until verified.**

Transform tasks into verifiable goals:
- "Add validation" → "Write tests for invalid inputs, then make them pass"
- "Fix the bug" → "Write a test that reproduces it, then make it pass"
- "Refactor X" → "Ensure tests pass before and after"

For multi-step tasks, state a brief plan:
```
1. [Step] → verify: [check]
2. [Step] → verify: [check]
3. [Step] → verify: [check]
```

Strong success criteria let you loop independently. Weak criteria ("make it work") require constant clarification.

---

**These guidelines are working if:** fewer unnecessary changes in diffs, fewer rewrites due to overcomplication, and clarifying questions come before implementation rather than after mistakes.

---

## LLM-ROUTER

### Project Overview
LLM Router Gateway — 다양한 LLM Provider(OpenAI, Anthropic, Gemini 등)를 단일 OpenAI-compatible 엔드포인트로 통합하는 API Gateway.

### Tech Stack
- **Language**: Go 1.22+ (Gateway Core)
- **HTTP**: `net/http` + `chi` (SSE 스트리밍을 위해 stdlib 기반 필수)
- **Config**: `koanf` (YAML + 환경변수)
- **Logging**: `slog` (Go stdlib)
- **DI**: 수동 생성자 주입 (main.go에서 명시적 wiring)
- **DB**: PostgreSQL 16 + `pgx` v5 + `sqlc`
- **Cache**: Redis 7
- **Migration**: `goose`
- **Docker**: distroless/static (멀티스테이지 빌드)

### Architecture Principles
- Stateless Gateway: 상태는 PostgreSQL/Redis에만 저장
- Interface-driven: 모든 컴포넌트는 인터페이스로 추상화
- Graceful shutdown: SIGTERM 시 30초 drain timeout

### Project Structure
- `cmd/gateway/` — 진입점 (thin main, 의존성 wiring만)
- `internal/` — 모든 비즈니스 로직 (외부 import 불가)
- `config/` — YAML 설정 파일
- `docker/` — Dockerfile, docker-compose
- `migrations/` — SQL 마이그레이션 (goose)
- `tasks/` — 기획 문서 (phase별 작업 계획)

### Conventions
- Go 패키지는 도메인 중심으로 구성 (handler, provider, auth 등)
- 새로운 패키지 추가 시 해당 태스크에서만 추가 (사전 생성 금지)
- 테스트: `_test.go` 파일, table-driven tests 선호
- 에러: `fmt.Errorf("context: %w", err)` 패턴으로 래핑
- 로깅: `slog.Info/Error/Debug` with structured key-value pairs

### Task Tracking
작업이 완료된 경우 `tasks/README.md` 파일에 완료된 작업을 표시해서 어느 수준까지 작업이 완료되었는지 확인할 수 있도록 한다.

