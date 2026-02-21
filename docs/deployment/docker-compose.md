# Docker Compose 배포 가이드

단일 서버 또는 소규모 팀을 위한 Docker Compose 배포 방법입니다.

## 사전 요구사항

- Docker Engine 24+
- Docker Compose v2+
- 최소 사양: CPU 2코어, 메모리 4GB, 디스크 20GB

## 빠른 시작

### 1. 환경 변수 설정

```bash
cp .env.example .env
```

`.env` 파일 편집:

```env
GATEWAY_VERSION=1.0.0

# 데이터베이스
DB_PASSWORD=your-strong-db-password
DATABASE_URL=postgres://llmrouter:${DB_PASSWORD}@db:5432/llmrouter?sslmode=disable

# Gateway
MASTER_KEY=your-master-key-min-32-chars
ENCRYPTION_KEY=base64-encoded-32-byte-key

# Provider API 키 (선택)
OPENAI_API_KEY=sk-...
ANTHROPIC_API_KEY=sk-ant-...
GEMINI_API_KEY=...
```

### 2. 데이터베이스 마이그레이션

```bash
docker compose -f docker/docker-compose.prod.yml up db -d
docker run --rm --network host \
  -v $(pwd)/migrations:/migrations \
  ghcr.io/pressly/goose:latest \
  postgres "${DATABASE_URL}" up -dir /migrations
```

### 3. 서비스 시작

```bash
docker compose -f docker/docker-compose.prod.yml up -d
```

### 4. 정상 동작 확인

```bash
curl http://localhost:8080/health/ready
# {"status":"ok","version":"1.0.0",...}
```

## 업그레이드

```bash
./scripts/upgrade.sh 1.1.0
```

## 고가용성 구성

단일 서버에서 Gateway 복제본을 늘리려면:

```yaml
# docker/docker-compose.prod.yml 수정
services:
  gateway:
    deploy:
      replicas: 2
```

외부 로드 밸런서(Nginx, HAProxy 등)를 게이트웨이 앞에 배치하세요.

## 로그 확인

```bash
docker compose -f docker/docker-compose.prod.yml logs -f gateway
```

## 백업

```bash
# PostgreSQL 덤프
docker compose -f docker/docker-compose.prod.yml exec db \
  pg_dump -U llmrouter llmrouter > backup-$(date +%Y%m%d).sql
```
