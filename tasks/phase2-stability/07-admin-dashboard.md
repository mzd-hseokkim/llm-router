# 07. 기본 대시보드 UI

## 목표
Admin API를 통해 Gateway 운영 현황을 시각적으로 모니터링하고 관리할 수 있는 웹 기반 관리 대시보드를 구현한다. 실시간 메트릭, 사용량 분석, Provider 상태 모니터링, 기본 설정 관리를 제공한다.

---

## 요구사항 상세

### 기술 스택
- **프레임워크**: Next.js 15 (App Router)
- **UI 라이브러리**: shadcn/ui + Tailwind CSS
- **차트**: Recharts 또는 Tremor
- **상태 관리**: React Query (서버 상태) + Zustand (클라이언트 상태)
- **API 연동**: Admin API (인증: 쿠키 기반 세션)

### 페이지 구조
```
/dashboard                 # 메인 대시보드 (실시간 현황)
/dashboard/usage           # 사용량 분석
/dashboard/keys            # Virtual Key 관리
/dashboard/providers       # Provider 관리
/dashboard/models          # 모델 관리
/dashboard/routing         # 라우팅 설정
/dashboard/logs            # 요청 로그 뷰어
/dashboard/budgets         # 예산 설정
/dashboard/settings        # 시스템 설정
```

### 메인 대시보드 (`/dashboard`)
**실시간 요약 카드** (30초 자동 갱신)
- 오늘 총 요청 수 / 전일 대비 증감
- 오늘 총 비용 / 예산 대비 %
- 현재 에러율 (1분 평균)
- 활성 연결 수

**Provider 상태 패널**
- 각 Provider의 상태 (ok/degraded/unhealthy) 색상 표시
- 최근 1시간 에러율
- 평균 레이턴시 (P50, P95)

**요청량 시계열 차트** (최근 24시간, 1시간 단위)
- Provider별 선 차트
- 에러/성공 스택 영역 차트

**비용 현황** (이번 달)
- 모델별 파이 차트
- 팀별 막대 차트

### 사용량 분석 (`/dashboard/usage`)
- 기간 선택: 오늘/7일/30일/커스텀
- 집계 단위: 시간/일/주
- 필터: 모델, 팀, Virtual Key
- 내보내기: CSV 다운로드

**차트 목록**
- 일별 요청/비용 트렌드 (이중 y축)
- 모델별 사용량 분포 (파이 차트)
- 상위 소비 팀/사용자 (막대 차트)
- 토큰 사용 패턴 (히트맵)

### Virtual Key 관리 (`/dashboard/keys`)
- 키 목록 테이블 (검색, 정렬, 페이지네이션)
- 컬럼: 이름, 프리픽스, 상태, 팀, 사용량, 비용, 마지막 사용
- 키 생성 다이얼로그 (RPM, TPM, 예산, 모델 접근 설정)
- 키 상세 슬라이드오버 (사용량 차트 포함)
- 키 비활성화/삭제 확인 모달

### Provider 관리 (`/dashboard/providers`)
- Provider 카드 (상태, 연결된 키 수, 오늘 사용량)
- Provider 키 추가/편집/삭제
- 헬스체크 수동 트리거
- 서킷 브레이커 상태 및 수동 초기화

### 요청 로그 뷰어 (`/dashboard/logs`)
- 실시간 로그 스트림 (WebSocket)
- 필터: 키, 모델, 상태코드, 기간
- 컬럼: 시간, 모델, 레이턴시, 토큰, 비용, 상태
- 행 클릭 시 상세 드로어 (프롬프트/응답은 권한 기반)

### 인증
- 로그인 페이지 (`/login`)
- Master Key 또는 Admin Virtual Key로 인증
- 세션 기반 (httpOnly 쿠키, 24시간)
- 재인증 없이 사용 가능한 페이지는 없음

---

## 기술 설계 포인트

- **SSR/SSG 선택**: 대시보드 데이터는 CSR + React Query (실시간 갱신 필요)
- **WebSocket**: 실시간 로그 스트림용 (Admin API에 WS 엔드포인트 추가)
- **반응형 레이아웃**: 태블릿/모바일 지원
- **에러 바운더리**: 차트 렌더링 오류가 전체 페이지에 영향 주지 않도록
- **접근성**: WCAG 2.1 AA 기준 준수

---

## 의존성

- `phase2-stability/06-admin-api.md` 완료 (모든 API)

---

## 완료 기준

- [ ] 메인 대시보드 30초마다 자동 갱신 확인
- [ ] Virtual Key 생성/수정/삭제 UI 동작 확인
- [ ] 사용량 차트 정확한 데이터 표시 확인
- [ ] 요청 로그 실시간 스트림 확인
- [ ] 모바일 화면(375px) 레이아웃 깨짐 없음 확인

---

## 예상 산출물

- `admin-ui/` (Next.js 프로젝트)
  - `app/dashboard/page.tsx`
  - `app/dashboard/usage/page.tsx`
  - `app/dashboard/keys/page.tsx`
  - `app/dashboard/providers/page.tsx`
  - `app/dashboard/logs/page.tsx`
  - `components/` (재사용 컴포넌트)
  - `lib/api.ts` (Admin API 클라이언트)
- `admin-ui/package.json`
- `admin-ui/Dockerfile`
