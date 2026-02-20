# 08. MCP Gateway (Model Context Protocol)

## 목표
Anthropic의 Model Context Protocol(MCP)을 지원하는 게이트웨이 레이어를 구현한다. MCP 서버들을 중앙에서 관리하고, LLM 에이전트가 외부 도구(파일 시스템, 데이터베이스, API 등)에 안전하게 접근할 수 있는 통합 MCP 허브를 제공한다.

---

## 요구사항 상세

### MCP 개요
- **MCP 서버**: 특정 기능을 제공하는 서버 (파일 접근, DB 쿼리, 웹 검색 등)
- **MCP 클라이언트**: LLM 에이전트 또는 AI 코딩 어시스턴트
- **역할**: Gateway가 중간에서 MCP 서버를 관리하고 접근 제어

### Gateway MCP 허브 아키텍처
```
Claude / LLM Agent
        ↓ (MCP 프로토콜)
LLM Gateway MCP Hub
        ↓ (프록시)
   ┌────┴────┐
   MCP 서버1  MCP 서버2  MCP 서버3
  (파일시스템) (PostgreSQL) (웹검색)
```

### 지원 MCP 전송 계층
- **Stdio**: 로컬 프로세스 (개발 환경)
- **SSE (Server-Sent Events)**: HTTP 기반 원격 MCP 서버
- **WebSocket**: 양방향 실시간 통신

### MCP 서버 레지스트리
```yaml
mcp_servers:
  - name: "filesystem"
    type: stdio
    command: "npx"
    args: ["-y", "@modelcontextprotocol/server-filesystem", "/data"]
    env:
      PATH: /usr/bin

  - name: "postgres"
    type: stdio
    command: "npx"
    args: ["-y", "@modelcontextprotocol/server-postgres", "postgresql://..."]

  - name: "web-search"
    type: sse
    url: "https://mcp-search.company.com/mcp"
    api_key: ${MCP_SEARCH_KEY}

  - name: "internal-api"
    type: websocket
    url: "wss://internal.company.com/mcp"
    auth:
      type: bearer
      token: ${INTERNAL_MCP_TOKEN}
```

### MCP 프록시 엔드포인트
```
# Claude Desktop → Gateway MCP Hub
POST /mcp/v1/initialize               # MCP 세션 초기화
POST /mcp/v1/tools/list               # 사용 가능한 도구 목록
POST /mcp/v1/tools/call               # 도구 실행
POST /mcp/v1/resources/list           # 리소스 목록
POST /mcp/v1/resources/read           # 리소스 읽기
POST /mcp/v1/prompts/list             # 프롬프트 목록
POST /mcp/v1/prompts/get              # 프롬프트 조회
```

### MCP 도구 접근 제어
```go
type MCPPolicy struct {
    AllowedServers []string   // 접근 가능한 MCP 서버
    AllowedTools   []string   // 허용된 도구 목록
    BlockedTools   []string   // 차단된 도구
    RequireApproval []string  // 실행 전 인간 승인 필요
}

func (p *MCPPolicy) CanCallTool(server, tool string) bool {
    if slices.Contains(p.BlockedTools, tool) {
        return false
    }
    if len(p.AllowedTools) > 0 && !slices.Contains(p.AllowedTools, tool) {
        return false
    }
    return true
}
```

### MCP 도구 실행 감사 로그
```json
{
  "event": "mcp_tool_called",
  "server": "postgres",
  "tool": "query",
  "arguments": {"query": "SELECT * FROM users LIMIT 10"},
  "result_size_bytes": 2048,
  "duration_ms": 45,
  "actor": "virtual_key_id",
  "request_id": "req_xxx",
  "timestamp": "2026-01-01T00:00:00Z"
}
```

### MCP 서버 관리 API
```
GET    /admin/mcp/servers              # 등록된 MCP 서버 목록
POST   /admin/mcp/servers              # MCP 서버 등록
PUT    /admin/mcp/servers/:name        # MCP 서버 수정
DELETE /admin/mcp/servers/:name        # MCP 서버 제거
GET    /admin/mcp/servers/:name/health  # 헬스체크
GET    /admin/mcp/servers/:name/tools  # 서버의 도구 목록
POST   /admin/mcp/policies             # 접근 정책 설정
```

### 프록시 보안
- MCP 서버 간 격리 (한 서버의 오류가 다른 서버에 영향 없음)
- 도구 실행 결과 크기 제한 (기본 1MB)
- 실행 타임아웃 (기본 30초)
- 프롬프트 인젝션 탐지 (도구 결과에서)

### 도구 결과 캐싱 (선택적)
- 동일 도구 + 동일 인수 → 캐시된 결과 반환 (짧은 TTL)
- 파일 읽기, DB 쿼리 등 멱등 작업에 적용
- 실시간 데이터 필요한 경우 캐시 비활성화

---

## 기술 설계 포인트

- **MCP SDK**: `github.com/mark3labs/mcp-go` 또는 자체 구현
- **Stdio 프로세스 관리**: 프로세스 풀링, 크래시 시 자동 재시작
- **SSE 프록시**: 클라이언트 ↔ Hub ↔ MCP 서버의 SSE 스트림 양방향 프록시
- **컨텍스트 격리**: 각 MCP 세션은 독립된 컨텍스트와 권한 유지

---

## 의존성

- `phase1-mvp/02-openai-compatible-api.md` 완료
- `phase3-enterprise/02-rbac.md` 완료
- `phase3-enterprise/09-audit-log.md` 완료

---

## 완료 기준

- [ ] Claude Desktop에서 Gateway MCP Hub를 통해 파일시스템 접근 성공
- [ ] PostgreSQL MCP 서버를 통한 DB 쿼리 성공
- [ ] 접근 정책으로 특정 MCP 도구 차단 확인
- [ ] MCP 도구 실행 감사 로그 기록 확인
- [ ] MCP 서버 장애 시 클라이언트에 명확한 오류 반환 확인

---

## 예상 산출물

- `internal/mcp/` (디렉토리)
  - `hub.go`, `proxy.go`, `policy.go`
  - `server/stdio.go`, `server/sse.go`, `server/websocket.go`
- `internal/gateway/handler/mcp.go`
- `internal/gateway/handler/admin/mcp_servers.go`
- `config/mcp-servers.yaml` (예시)
- `docs/mcp-gateway-setup.md`
- `internal/mcp/proxy_test.go`
