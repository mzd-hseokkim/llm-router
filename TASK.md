# ATL-469: [LLM Router] Admin UI 목록 페이지 페이지네이션 구현

## Issue Details
- **Status**: In Progress
- **Priority**: 주요
- **Assignee**: Unassigned
- **Branch**: feature/ATL-469
- **Started**: 2026-02-24

## Description

Admin UI의 목록형 페이지들에 페이지 이동 UI가 없어 대량 데이터 조회 시 불편함이 있음.
`audit` 페이지만 서버사이드 페이지네이션이 완료된 상태.

## 현황 분석

| 페이지 | 현재 상태 | API page 지원 |
|--------|----------|--------------|
| audit | ✅ 완료 | ✅ |
| logs | limit 셀렉터만 있음, 페이지 이동 없음 | ❌ |
| keys | 페이지네이션 없음 | ❌ |
| prompts | 페이지네이션 없음 | ❌ |
| providers | 페이지네이션 없음 (소량, 낮은 우선순위) | ❌ |

## 작업 범위

### Step 1. Backend — Go 핸들러 페이지네이션 추가

대상 엔드포인트: `GET /admin/logs`, `GET /admin/keys`, `GET /admin/prompts`

- `page` (1-based), `limit` 쿼리 파라미터 추가
- 응답에 `total` 필드 추가: `{ data: [...], total: N }`
- 파라미터 미전달 시 기존 동작 유지 (하위호환)
- 관련 파일: `internal/handler/` 하위 각 핸들러, sqlc 쿼리에 LIMIT/OFFSET 추가

### Step 2. Frontend — lib/api.ts 타입/파라미터 업데이트

- `logs.list()`, `keys.list()`, `prompts.list()` 에 `page`/`limit` 파라미터 추가
- 각 응답 타입에 `total: number` 추가

### Step 3. Frontend — 각 페이지 UI

`audit/page.tsx`의 패턴을 동일하게 적용:
- 페이지 이동 버튼: `« ‹ {page} / {totalPages} › »`
- Per page 셀렉터: 25 / 50 / 100 / 200
- 필터 변경 시 page=1 리셋

대상: `logs/page.tsx`, `keys/page.tsx`, `prompts/page.tsx`

## 참고

- `audit/page.tsx`를 레퍼런스 구현으로 사용
- Go 패키지: `internal/handler/`, DB 쿼리: sqlc LIMIT/OFFSET 패턴
