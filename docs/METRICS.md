# Metrics

vLLM-Chill exposes Prometheus/OpenMetrics format metrics at the `/proxy/metrics` endpoint.

**Note:** The proxy uses `/proxy/metrics` to avoid conflicts with vLLM's own `/metrics` endpoint. Both endpoints are accessible:
- `/proxy/metrics` - vLLM-Chill proxy metrics (autoscaling, requests, etc.)
- `/metrics` - vLLM backend metrics (when vLLM is running)

## Enabling Metrics

Proxy metrics are always enabled and cannot be disabled.

## Available Metrics

### Request Metrics

#### `vllm_chill_requests_total`
**Type:** Counter
**Labels:** `method`, `path`, `status`
**Description:** Total number of requests received

Example:
```
vllm_chill_requests_total{method="POST",path="/v1/chat/completions",status="200"} 1523
```

#### `vllm_chill_request_duration_seconds`
**Type:** Histogram
**Labels:** `method`, `path`, `status`
**Description:** Request duration in seconds

Buckets: `[0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10]`

Example:
```
vllm_chill_request_duration_seconds_bucket{method="POST",path="/v1/chat/completions",status="200",le="1"} 234
vllm_chill_request_duration_seconds_sum{method="POST",path="/v1/chat/completions",status="200"} 845.23
vllm_chill_request_duration_seconds_count{method="POST",path="/v1/chat/completions",status="200"} 1523
```

#### `vllm_chill_request_payload_bytes`
**Type:** Histogram
**Labels:** `method`, `path`
**Description:** Request payload size in bytes

Buckets: `[100, 1000, 10000, 100000, 1000000, 10000000]`

#### `vllm_chill_response_payload_bytes`
**Type:** Histogram
**Labels:** `method`, `path`, `status`
**Description:** Response payload size in bytes

Buckets: `[100, 1000, 10000, 100000, 1000000, 10000000]`

### Model Switching Metrics

#### `vllm_chill_model_switches_total`
**Type:** Counter
**Labels:** `from_model`, `to_model`, `status`
**Description:** Total number of model switches

Example:
```
vllm_chill_model_switches_total{from_model="qwen3-coder-30b-fp8",to_model="deepseek-r1-fp8",status="success"} 12
```

#### `vllm_chill_model_switch_duration_seconds`
**Type:** Histogram
**Labels:** `from_model`, `to_model`
**Description:** Model switch duration in seconds

Buckets: `[10, 30, 60, 120, 300, 600]`

Example:
```
vllm_chill_model_switch_duration_seconds_bucket{from_model="qwen3-coder-30b-fp8",to_model="deepseek-r1-fp8",le="120"} 8
```

### Scaling Metrics

#### `vllm_chill_scale_operations_total`
**Type:** Counter
**Labels:** `direction`, `status`
**Description:** Total number of scale operations (up/down)

Example:
```
vllm_chill_scale_operations_total{direction="up",status="success"} 45
vllm_chill_scale_operations_total{direction="down",status="success"} 43
```

#### `vllm_chill_scale_operation_duration_seconds`
**Type:** Histogram
**Labels:** `direction`
**Description:** Scale operation duration in seconds

Buckets: `[1, 5, 10, 30, 60, 120]`

### State Metrics

#### `vllm_chill_current_replicas`
**Type:** Gauge
**Description:** Current number of replicas

Example:
```
vllm_chill_current_replicas 1
```

#### `vllm_chill_idle_time_seconds`
**Type:** Gauge
**Description:** Time since last activity in seconds

Example:
```
vllm_chill_idle_time_seconds 142.5
```

#### `vllm_chill_current_model`
**Type:** Gauge
**Labels:** `model_name`
**Description:** Current model loaded (1 if loaded, 0 otherwise)

Example:
```
vllm_chill_current_model{model_name="deepseek-r1-fp8"} 1
vllm_chill_current_model{model_name="qwen3-coder-30b-fp8"} 0
```

## Grafana Dashboard

Example PromQL queries for monitoring:

### Request Rate
```promql
rate(vllm_chill_requests_total[5m])
```

### Request Latency (p95)
```promql
histogram_quantile(0.95, rate(vllm_chill_request_duration_seconds_bucket[5m]))
```

### Error Rate
```promql
sum(rate(vllm_chill_requests_total{status=~"5.."}[5m])) / sum(rate(vllm_chill_requests_total[5m]))
```

### Model Switch Success Rate
```promql
sum(rate(vllm_chill_model_switches_total{status="success"}[5m])) / sum(rate(vllm_chill_model_switches_total[5m]))
```

### Average Model Switch Duration
```promql
rate(vllm_chill_model_switch_duration_seconds_sum[5m]) / rate(vllm_chill_model_switch_duration_seconds_count[5m])
```

### Current State
```promql
vllm_chill_current_replicas
vllm_chill_idle_time_seconds
```

## Prometheus Configuration

Add this to your `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'vllm-chill'
    static_configs:
      - targets: ['vllm-chill-svc:8080']
    metrics_path: '/proxy/metrics'
    scrape_interval: 15s
```

For Kubernetes with ServiceMonitor:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: vllm-chill
  namespace: ai-apps
spec:
  selector:
    matchLabels:
      app: vllm-chill
  endpoints:
    - port: http
      path: /proxy/metrics
      interval: 15s
```

## Alerting Rules

Example Prometheus alerting rules:

```yaml
groups:
  - name: vllm-chill
    rules:
      - alert: HighErrorRate
        expr: |
          sum(rate(vllm_chill_requests_total{status=~"5.."}[5m]))
          / sum(rate(vllm_chill_requests_total[5m])) > 0.05
        for: 5m
        annotations:
          summary: "High error rate on vLLM-Chill"
          description: "Error rate is {{ $value | humanizePercentage }}"

      - alert: ModelSwitchFailing
        expr: |
          sum(rate(vllm_chill_model_switches_total{status="failure"}[15m]))
          / sum(rate(vllm_chill_model_switches_total[15m])) > 0.2
        for: 10m
        annotations:
          summary: "Model switches are failing"
          description: "{{ $value | humanizePercentage }} of model switches are failing"

      - alert: SlowModelSwitch
        expr: |
          histogram_quantile(0.95,
            rate(vllm_chill_model_switch_duration_seconds_bucket[10m])
          ) > 300
        for: 10m
        annotations:
          summary: "Model switches are taking too long"
          description: "P95 model switch duration is {{ $value | humanizeDuration }}"

      - alert: HighRequestLatency
        expr: |
          histogram_quantile(0.95,
            rate(vllm_chill_request_duration_seconds_bucket[5m])
          ) > 30
        for: 5m
        annotations:
          summary: "High request latency"
          description: "P95 latency is {{ $value | humanizeDuration }}"
```

## Performance Impact

The metrics collection has minimal overhead:
- **Memory:** ~5-10MB additional memory usage
- **CPU:** <1% CPU overhead for typical workloads
- **Latency:** <100μs per request

For high-throughput scenarios (>1000 req/s), consider:
- Increasing scrape intervals to reduce cardinality
- Using histogram bucketing more aggressively
- Disabling metrics entirely if not needed
