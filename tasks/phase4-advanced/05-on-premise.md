# 05. 온프레미스 / VPC 배포 지원

## 목표
클라우드 SaaS 외에 기업 자체 인프라(온프레미스, VPC, 에어갭 환경)에서 Gateway를 운영할 수 있는 배포 패키지를 제공한다. 단일 바이너리 또는 컨테이너 이미지로 외부 의존성 없이 실행 가능한 최소 배포를 지원한다.

---

## 요구사항 상세

### 배포 옵션
| 배포 유형 | 설명 | 대상 |
|----------|------|------|
| Docker Compose | 단일 서버 배포 | 소규모 팀 |
| Kubernetes Helm Chart | 컨테이너 오케스트레이션 | 중대규모 엔터프라이즈 |
| 단일 바이너리 | 임베디드 SQLite, 의존성 최소화 | 에어갭, 개발 환경 |
| Ansible Playbook | VM 기반 자동화 배포 | 레거시 인프라 |

### 최소 배포 (단일 바이너리)
```go
// 빌드 태그로 임베디드 DB 선택
//go:build embed_sqlite

import "modernc.org/sqlite"

// SQLite로 PostgreSQL 대체 (소규모 배포)
// Redis 없이 인메모리 레이트 리미터 사용
```

```bash
# 단일 파일 실행 (외부 의존성 없음)
./llm-gateway --config config.yaml
# → 포트 8080 (Gateway), 8081 (Admin UI 내장)
```

### Docker Compose 배포
```yaml
# docker/docker-compose.prod.yml
version: "3.9"
services:
  gateway:
    image: ghcr.io/company/llm-gateway:1.0.0
    ports:
      - "8080:8080"
    environment:
      DATABASE_URL: postgres://...
      REDIS_URL: redis://redis:6379
    depends_on:
      db:
        condition: service_healthy
      redis:
        condition: service_healthy

  admin:
    image: ghcr.io/company/llm-gateway-admin:1.0.0
    ports:
      - "3000:3000"

  db:
    image: postgres:16
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD", "pg_isready"]

  redis:
    image: redis:7-alpine
    volumes:
      - redisdata:/data

volumes:
  pgdata:
  redisdata:
```

### Kubernetes Helm Chart
```yaml
# helm/values.yaml
gateway:
  replicaCount: 3
  image:
    repository: ghcr.io/company/llm-gateway
    tag: "1.0.0"

  resources:
    requests:
      cpu: "500m"
      memory: "256Mi"
    limits:
      cpu: "2000m"
      memory: "1Gi"

  autoscaling:
    enabled: true
    minReplicas: 2
    maxReplicas: 10
    targetCPUUtilizationPercentage: 70

postgresql:
  enabled: true
  auth:
    database: llm_gateway
    existingSecret: gateway-db-secret

redis:
  enabled: true
  architecture: standalone
```

### 에어갭 환경 지원
- 모든 컨테이너 이미지를 프라이빗 레지스트리에서 Pull 가능
- 외부 API 호출 없음 (라이센스 서버 등)
- 오프라인 모델 가격 테이블 내장
- 오프라인 업데이트: 이미지 tar 파일 배포

### TLS/HTTPS 설정
```yaml
tls:
  enabled: true
  mode: "auto"  # auto(Let's Encrypt) | manual | cert-manager
  cert_file: /etc/ssl/certs/gateway.crt
  key_file: /etc/ssl/private/gateway.key
  # 온프레미스: 자체 서명 인증서 또는 내부 CA 사용
```

### 고가용성 구성
```
                    ┌──────────────┐
Internet ──────────▶│ Load Balancer │
                    └──────┬───────┘
                    ┌──────▼───────┐
                    │   Gateway     │ × 3 (Active-Active)
                    └──────┬───────┘
                    ┌──────▼───────┐
                    │  PostgreSQL   │ (Primary + Replica)
                    │    Redis      │ (Sentinel or Cluster)
                    └──────────────┘
```

### 업그레이드 전략
- **Rolling Update**: Kubernetes의 기본 업데이트 전략
- **Blue-Green**: 트래픽 전환 전 새 버전 사전 웜업
- **마이그레이션 자동화**: 업그레이드 시 DB 마이그레이션 자동 실행
- **롤백**: Helm rollback 명령으로 이전 버전 복구

### 모니터링 (온프레미스)
- Prometheus + Grafana 포함 배포 옵션
- Helm 차트에 kube-prometheus-stack 서브차트 포함
- 사전 구성 Grafana 대시보드 자동 provisioning

---

## 기술 설계 포인트

- **Stateless Gateway**: 상태는 모두 외부 저장소(Postgres, Redis)에 유지
- **헬스체크 엔드포인트**: Kubernetes liveness/readiness probe 완전 지원
- **우아한 종료**: SIGTERM 수신 후 진행 중인 요청 완료 후 종료 (drain timeout: 30s)
- **설정 비밀**: Kubernetes Secret 또는 Vault에서 API Key 주입

---

## 의존성

- `phase1-mvp/01-project-setup.md` 완료
- `phase1-mvp/09-health-check.md` 완료

---

## 완료 기준

- [ ] `docker-compose up` 명령 1개로 전체 스택 실행 확인
- [ ] Helm Chart로 Kubernetes 배포 성공
- [ ] 단일 바이너리 모드로 외부 DB 없이 실행 확인
- [ ] Rolling Update 중 요청 드롭 없음 확인
- [ ] 에어갭 환경에서 외부 네트워크 없이 실행 확인

---

## 예상 산출물

- `docker/docker-compose.prod.yml`
- `helm/` (Helm Chart 디렉토리)
  - `Chart.yaml`, `values.yaml`, `templates/`
- `Dockerfile.gateway` (멀티 스테이지 빌드, 최소 이미지)
- `docs/deployment/docker-compose.md`
- `docs/deployment/kubernetes.md`
- `docs/deployment/airgap.md`
- `scripts/upgrade.sh`
