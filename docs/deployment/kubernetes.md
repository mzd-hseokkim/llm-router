# Kubernetes / Helm 배포 가이드

엔터프라이즈 규모의 Kubernetes 배포 방법입니다.

## 사전 요구사항

- Kubernetes 1.25+
- Helm 3.12+
- cert-manager (TLS 자동화, 선택)
- ingress-nginx 또는 동등한 컨트롤러 (Ingress 사용 시)

## Helm 저장소 추가

```bash
helm repo add llm-gateway https://charts.company.com
helm repo update
```

## 기본 설치

```bash
# 시크릿 먼저 생성
kubectl create secret generic gateway-db-secret \
  --from-literal=DATABASE_URL="postgres://llmrouter:PASSWORD@db:5432/llmrouter" \
  --from-literal=REDIS_ADDR="redis-master:6379" \
  --from-literal=MASTER_KEY="your-master-key" \
  --from-literal=ENCRYPTION_KEY="your-encryption-key" \
  --from-literal=OPENAI_API_KEY="sk-..." \
  --from-literal=ANTHROPIC_API_KEY="sk-ant-..." \
  --from-literal=GEMINI_API_KEY="..."

# 설치
helm install llm-gateway ./helm \
  --namespace llm-gateway --create-namespace \
  --set existingSecret=gateway-db-secret \
  --wait
```

## 값 커스터마이징

```yaml
# my-values.yaml
replicaCount: 3

autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 20
  targetCPUUtilizationPercentage: 70

ingress:
  enabled: true
  className: nginx
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
  hosts:
    - host: gateway.company.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: gateway-tls
      hosts: [gateway.company.com]

resources:
  requests:
    cpu: "1000m"
    memory: "512Mi"
  limits:
    cpu: "4000m"
    memory: "2Gi"

postgresql:
  enabled: false  # 외부 DB 사용

redis:
  enabled: false  # 외부 Redis 사용
```

```bash
helm install llm-gateway ./helm \
  --namespace llm-gateway \
  -f my-values.yaml \
  --set existingSecret=gateway-db-secret
```

## 업그레이드 (Rolling Update)

```bash
helm upgrade llm-gateway ./helm \
  --namespace llm-gateway \
  --set image.tag=1.1.0 \
  --wait --timeout 5m
```

Rolling Update 전략으로 요청 드롭 없이 업그레이드됩니다 (`maxUnavailable: 0`).

## 롤백

```bash
helm rollback llm-gateway --namespace llm-gateway
```

## DB 마이그레이션 (업그레이드 시)

```bash
kubectl run migration --restart=Never --rm -it \
  --image=ghcr.io/company/llm-gateway:1.1.0 \
  --overrides='{"spec":{"containers":[{"name":"migration","command":["./migrate","up"]}]}}' \
  --env="DATABASE_URL=$(kubectl get secret gateway-db-secret -o jsonpath='{.data.DATABASE_URL}' | base64 -d)"
```

## 모니터링

Prometheus 스크래핑은 pod annotation으로 자동 설정됩니다:

```yaml
podAnnotations:
  prometheus.io/scrape: "true"
  prometheus.io/port: "8080"
  prometheus.io/path: "/metrics"
```

Grafana 대시보드 임포트:
- 대시보드 ID: (회사 내부 ID)

## 트러블슈팅

```bash
# Pod 상태 확인
kubectl get pods -n llm-gateway

# 로그 확인
kubectl logs -n llm-gateway -l app.kubernetes.io/name=llm-gateway --tail=100 -f

# 헬스체크
kubectl exec -n llm-gateway deploy/llm-gateway -- wget -qO- http://localhost:8080/health
```
