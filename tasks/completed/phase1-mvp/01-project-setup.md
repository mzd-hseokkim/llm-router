# 01. 프로젝트 초기 설정

## 목표
LLM Router Gateway 프로젝트의 기반 구조를 설정한다. 디렉토리 레이아웃, 의존성 관리, 빌드 시스템, 개발 환경을 확립하여 이후 모든 작업의 토대를 마련한다.

---

## 요구사항 상세

### 기술 스택
- **Gateway Core**: Go 1.22+
  - HTTP 서버: `net/http` + `fasthttp` 또는 `fiber`
  - 설정 파싱: `viper` (YAML/ENV 지원)
  - 로거: `zap` (구조화 로그)
  - DI: `wire` 또는 수동 의존성 주입
- **Admin API**: TypeScript + Node.js (Fastify 또는 Express)
- **Admin UI**: Next.js + React + Tailwind CSS
- **데이터베이스**: PostgreSQL 15+
- **캐시/상태**: Redis 7+
- **ORM**: `sqlc` (Go) + `drizzle-orm` 또는 `prisma` (Node.js)
- **마이그레이션**: `golang-migrate` 또는 `goose`

### 디렉토리 구조
```
llm-router/
├── cmd/
│   ├── gateway/          # Gateway Core 진입점
│   └── admin/            # Admin API 진입점
├── internal/
│   ├── gateway/          # 핵심 게이트웨이 로직
│   │   ├── handler/      # HTTP 핸들러
│   │   ├── middleware/   # 미들웨어 체인
│   │   ├── router/       # 라우팅 엔진
│   │   └── proxy/        # 프록시 로직
│   ├── provider/         # Provider 어댑터
│   │   ├── openai/
│   │   ├── anthropic/
│   │   └── gemini/
│   ├── auth/             # 인증/인가
│   ├── config/           # 설정 관리
│   ├── store/            # 데이터 저장소 레이어
│   │   ├── postgres/
│   │   └── redis/
│   └── telemetry/        # 모니터링/로깅
├── admin-ui/             # Next.js 프론트엔드
├── migrations/           # DB 마이그레이션 파일
├── config/
│   ├── config.example.yaml
│   └── models.yaml       # 모델 가격 정보
├── docker/
│   ├── Dockerfile.gateway
│   ├── Dockerfile.admin
│   └── docker-compose.yml
├── scripts/
│   ├── setup.sh
│   └── migrate.sh
├── docs/
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

### 빌드 시스템 (Makefile 타겟)
- `make build` — Gateway + Admin API 빌드
- `make test` — 전체 테스트 실행
- `make lint` — golangci-lint + eslint
- `make docker-up` — 로컬 개발 환경 실행 (Postgres, Redis 포함)
- `make migrate` — DB 마이그레이션 실행
- `make generate` — sqlc, wire 코드 생성

### 설정 파일 구조 (config.yaml)
```yaml
server:
  port: 8080
  admin_port: 8081
  read_timeout: 30s
  write_timeout: 60s

database:
  url: postgres://...
  max_connections: 20

redis:
  addr: localhost:6379

log:
  level: info
  format: json

providers: []  # Phase별로 추가
```

### 개발 환경
- `docker-compose.yml`: Postgres, Redis, (선택) Adminer
- `.env.example` 제공
- Hot reload: `air` (Go) + Next.js dev server
- Git hooks: `pre-commit` (lint, format)

---

## 기술 설계 포인트

- **모노레포 구조**: Go 모듈과 Node.js 프로젝트를 단일 저장소에서 관리
- **인터페이스 중심 설계**: 각 컴포넌트는 인터페이스로 추상화하여 테스트 가능성 확보
- **설정 계층**: 파일 < 환경변수 < CLI 플래그 순으로 오버라이드
- **제로 값 안전성**: Go 구조체는 기본값으로 안전하게 동작하도록 설계
- **graceful shutdown**: SIGTERM 수신 시 진행 중인 요청 완료 후 종료 (drain timeout: 30s)

---

## 의존성

- 없음 (최초 작업)

---

## 완료 기준

- [ ] `make docker-up` 실행 후 Postgres, Redis 정상 기동 확인
- [ ] `make build` 성공 (Go 바이너리 생성)
- [ ] `make test` 실행 시 빈 테스트 스위트 통과
- [ ] `make lint` 경고/에러 없음
- [ ] `config.example.yaml` 로드 및 파싱 정상 동작
- [ ] 기본 HTTP 서버 기동 및 `/ping` 응답 확인
- [ ] README에 로컬 개발 시작 방법 문서화

---

## 예상 산출물

- `go.mod`, `go.sum`
- `Makefile`
- `docker/docker-compose.yml`
- `config/config.example.yaml`
- `config/models.yaml` (빈 초기 버전)
- `cmd/gateway/main.go`
- `internal/config/config.go`
- `internal/telemetry/logger.go`
- `.env.example`
- `README.md`
