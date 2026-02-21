# 에어갭(Air-gap) 환경 배포 가이드

외부 인터넷 접근이 없는 격리된 환경(에어갭, On-Premise VPC)에서의 배포 방법입니다.

## 이미지 준비 (인터넷 연결 서버에서)

```bash
# 1. 필요 이미지 Pull
docker pull ghcr.io/company/llm-gateway:1.0.0
docker pull ghcr.io/company/llm-gateway-admin:1.0.0
docker pull pgvector/pgvector:pg16
docker pull redis:7-alpine

# 2. tar 아카이브로 저장
docker save \
  ghcr.io/company/llm-gateway:1.0.0 \
  ghcr.io/company/llm-gateway-admin:1.0.0 \
  pgvector/pgvector:pg16 \
  redis:7-alpine \
  | gzip > llm-gateway-images-1.0.0.tar.gz
```

## 에어갭 환경으로 이전

```bash
# 물리 매체(USB, 보안 파일 전송) 또는 내부 네트워크로 복사
scp llm-gateway-images-1.0.0.tar.gz user@airgap-server:/opt/llm-gateway/
```

## 에어갭 서버에서 설치

```bash
# 1. 이미지 로드
docker load < llm-gateway-images-1.0.0.tar.gz

# 2. 로컬 레지스트리에 태깅 (선택 — 프라이빗 레지스트리가 있는 경우)
docker tag ghcr.io/company/llm-gateway:1.0.0 registry.internal/llm-gateway:1.0.0
docker push registry.internal/llm-gateway:1.0.0

# 3. 환경 변수 설정 후 기동
cp docker/docker-compose.prod.yml .
# image를 로컬 이미지로 수정 후:
docker compose -f docker-compose.prod.yml up -d
```

## 오프라인 모델 가격 테이블

Gateway는 `config/models.yaml`에 내장된 가격 테이블을 사용합니다.
외부 API 호출 없이 비용 계산이 가능합니다.

```bash
# 가격 테이블 업데이트 시 (이미지 재빌드 불필요)
cp new-models.yaml config/models.yaml
docker compose -f docker-compose.prod.yml restart gateway
```

## 라이센스 서버 없음

LLM Gateway는 라이센스 서버나 외부 API 호출 없이 완전히 오프라인으로 동작합니다.
모든 설정과 인증은 로컬 PostgreSQL/Redis에 저장됩니다.

## TLS 설정 (내부 CA)

```yaml
# config/config.yaml
server:
  tls:
    enabled: true
    cert_file: /certs/gateway.crt
    key_file: /certs/gateway.key
```

```bash
# 자체 서명 인증서 생성 (개발/테스트용)
openssl req -x509 -newkey rsa:4096 -keyout gateway.key \
  -out gateway.crt -days 365 -nodes \
  -subj "/CN=gateway.internal"
```

## 업그레이드 절차

```bash
# 새 이미지 tar 준비 → 전송 → 로드
docker load < llm-gateway-images-1.1.0.tar.gz

# 마이그레이션 실행
docker run --rm \
  -e DATABASE_URL="${DATABASE_URL}" \
  ghcr.io/company/llm-gateway:1.1.0 \
  /app/gateway migrate up

# Rolling 업그레이드
GATEWAY_VERSION=1.1.0 docker compose -f docker-compose.prod.yml up -d gateway
```
