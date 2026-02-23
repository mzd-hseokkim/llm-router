# ATL-279: [LLM Router] Embeddings API 구현 (/v1/embeddings)

## Issue Details
- **Key**: ATL-279
- **Summary**: [LLM Router] Embeddings API 구현 (/v1/embeddings)
- **Type**: 작업
- **Priority**: 주요
- **Status**: In Progress
- **Branch**: feature/ATL-279
- **Worktree**: C:/WORK/workspace/llm-router_worktree/ATL-279
- **Started**: 2026-02-23

## Description

### 현황
`internal/gateway/handler/embeddings.go` 골격만 존재, 실제 처리 로직 미구현

### 필요 작업
- Provider별 임베딩 API 호출 구현 (OpenAI, Gemini, Cohere 지원)
- 임베딩 결과 캐싱 연동 (exact-match)
- 비용 추적 (임베딩 토큰 단가 테이블)
- 라우터 등록 (`/v1/embeddings`)
- E2E 테스트 추가

## Acceptance Criteria
- `/v1/embeddings` 엔드포인트 등록 및 정상 동작
- OpenAI, Gemini, Cohere provider 라우팅 지원
- 임베딩 결과 캐싱 (exact-match, Redis)
- 토큰 사용량 및 비용 추적 (provider별 임베딩 단가)
- E2E 테스트 작성

## Workflow
1. ✅ `init` — worktree 생성
2. ✅ `start` — 작업 시작 (현재)
3. `/jira-task plan ATL-279` — 기획 문서 작성
4. `/jira-task design ATL-279` — 설계 문서 작성
5. `/jira-task impl ATL-279` — 구현
6. `/jira-task test ATL-279` — 테스트 실행
7. `/jira-task review ATL-279` — 코드 리뷰
8. `/jira-task pr ATL-279` — PR 생성
9. `/jira-task done ATL-279` — 완료 처리
