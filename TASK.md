# ATL-287: Admin UI - 차지백/쇼백 리포트 페이지 (/reports)

## Issue Details
- **Status**: 진행 중 (In Progress)
- **Priority**: 주요 (High)
- **Assignee**: Kim Hyungsuk
- **Branch**: feature/ATL-287
- **Started**: 2026-02-24

## Description

### 관련 백엔드 API
- `GET /admin/reports`
- `POST /admin/reports/generate`
- `GET /admin/reports/:id`

### 필요 작업
- 기간별 팀/키별 비용 집계 테이블
- 리포트 생성 트리거 (수동/자동 예약)
- CSV/PDF 내보내기
- `lib/api.ts`에 `reports` 객체 추가
- Sidebar 링크 추가

### 우선순위
높음 — 백엔드 완성, UI만 없어서 운영 불가
