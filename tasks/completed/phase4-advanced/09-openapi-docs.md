# 09. OpenAPI 문서 (Swagger UI)

## 목표
Admin API와 Gateway API에 대한 OpenAPI 3.0 명세를 제공하고 Swagger UI로 서빙한다. 클라이언트 개발자가 별도 문서 없이 API를 탐색하고 테스트할 수 있도록 한다.

---

## 요구사항 상세

### 대상 API
- `/admin/*` — Provider Key·Virtual Key CRUD, 라우팅 설정 등 Admin API 전체
- `/v1/*` — OpenAI-compatible 엔드포인트 (OpenAI 공식 스펙 참조 링크 제공)

### 제공 엔드포인트
```
GET /docs              → Swagger UI HTML
GET /docs/openapi.json → OpenAPI 3.0 JSON 명세
GET /docs/openapi.yaml → OpenAPI 3.0 YAML 명세
```

### 구현 방법 선택지
1. **swaggo/swag** — 핸들러 주석 파싱으로 자동 생성 (코드-명세 동기화 용이)
2. **수동 openapi.yaml** — 명세를 직접 작성 후 static 파일로 서빙 (단순·정확)
3. **huma 마이그레이션** — OpenAPI-first 프레임워크로 전환 (대규모 변경 필요)

권장: **swaggo/swag** (기존 chi 코드 유지하면서 점진적 적용 가능)

### Swagger UI 인증
- Admin API: `Authorization: Bearer {master_key}` 헤더를 Swagger UI에서 직접 입력
- `/v1/*`: `Authorization: Bearer {virtual_key}` 입력

---

## 기술 설계 포인트
- Swagger UI는 CDN 또는 내장 static 파일로 서빙 (`swaggerui` embed)
- `/docs` 경로는 인증 없이 접근 가능 (명세 자체는 공개)
- 프로덕션 빌드 시 빌드 태그(`//go:build !prod`)로 비활성화 옵션 고려

---

## 의존성
- Phase 2-06 Admin API 완료

---

## 완료 기준
- [ ] `GET /docs` 접속 시 Swagger UI 렌더링
- [ ] 모든 Admin API 엔드포인트 명세 포함
- [ ] Try it out 기능으로 실제 API 호출 가능
- [ ] OpenAPI JSON/YAML 다운로드 가능

---

## 예상 산출물
- `internal/docs/openapi.go` 또는 `docs/openapi.yaml`
- `internal/gateway/handler/docs.go` (Swagger UI 서빙)
