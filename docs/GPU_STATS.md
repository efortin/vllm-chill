# GPU Statistics API

The `/proxy/stats` endpoint provides real-time GPU statistics using NVIDIA NVML (NVIDIA Management Library) in JSON format.

## Endpoint

```
GET /proxy/stats
```

## Response Format

```json
{
  "timestamp": "2025-11-09T10:30:45.123456Z",
  "gpus": [
    {
      "index": 0,
      "name": "NVIDIA H100 PCIe",
      "uuid": "GPU-abc123def456...",
      "utilization_percent": 85.5,
      "memory_used_mb": 40960,
      "memory_total_mb": 81920,
      "memory_util_percent": 50.0,
      "temperature_c": 65,
      "power_draw_w": 350.5,
      "power_limit_w": 400.0,
      "fan_speed_percent": 45,
      "encoder_util_percent": 0,
      "decoder_util_percent": 0
    },
    {
      "index": 1,
      "name": "NVIDIA H100 PCIe",
      "uuid": "GPU-xyz789abc123...",
      "utilization_percent": 92.1,
      "memory_used_mb": 65536,
      "memory_total_mb": 81920,
      "memory_util_percent": 80.0,
      "temperature_c": 72,
      "power_draw_w": 385.2,
      "power_limit_w": 400.0,
      "fan_speed_percent": 60,
      "encoder_util_percent": 0,
      "decoder_util_percent": 0
    }
  ]
}
```

## Fields

| Field | Type | Description |
|-------|------|-------------|
| `timestamp` | string | ISO 8601 timestamp of when the stats were collected |
| `gpus` | array | Array of GPU objects |
| `index` | int | GPU index (0, 1, 2, etc.) |
| `name` | string | GPU model name |
| `uuid` | string | Unique GPU identifier |
| `utilization_percent` | float | GPU utilization (0-100) |
| `memory_used_mb` | int | GPU memory used in MiB |
| `memory_total_mb` | int | Total GPU memory in MiB |
| `memory_util_percent` | float | Memory utilization (0-100) |
| `temperature_c` | int | GPU temperature in Celsius |
| `power_draw_w` | float | Current power draw in Watts |
| `power_limit_w` | float | Power limit in Watts |
| `fan_speed_percent` | int | Fan speed (0-100) |
| `encoder_util_percent` | int | Video encoder utilization (0-100) |
| `decoder_util_percent` | int | Video decoder utilization (0-100) |

## Caching

The endpoint caches results for **1 second** to avoid excessive NVML calls.

Check the `X-Cache` response header:
- `X-Cache: HIT` - Returned from cache
- `X-Cache: MISS` - Fresh query from NVML

## Usage Examples

### cURL

```bash
# Basic request
curl http://localhost:8080/proxy/stats

# With pretty printing
curl http://localhost:8080/proxy/stats | jq .

# Check cache status
curl -v http://localhost:8080/proxy/stats 2>&1 | grep X-Cache
```

### JavaScript (for Crush UI)

```javascript
// Fetch GPU stats every second
async function fetchGPUStats() {
  const response = await fetch('http://vllm-chill:8080/proxy/stats');
  const data = await response.json();

  console.log('GPU Stats:', data);
  console.log('Cache:', response.headers.get('X-Cache'));

  return data;
}

// Poll every second
setInterval(fetchGPUStats, 1000);
```

### Python

```python
import requests
import time

def get_gpu_stats():
    response = requests.get('http://localhost:8080/proxy/stats')
    data = response.json()
    cache_status = response.headers.get('X-Cache', 'UNKNOWN')

    print(f"Cache: {cache_status}")
    for gpu in data['gpus']:
        print(f"GPU {gpu['index']}: {gpu['utilization_percent']}% @ {gpu['temperature_c']}°C")

    return data

# Poll every second
while True:
    get_gpu_stats()
    time.sleep(1)
```

### Go

```go
package main

import (
    "encoding/json"
    "fmt"
    "net/http"
    "time"
)

type GPUStats struct {
    Timestamp time.Time `json:"timestamp"`
    GPUs      []struct {
        Index         int     `json:"index"`
        Utilization   float64 `json:"utilization_percent"`
        Temperature   int     `json:"temperature_c"`
        MemoryUsed    int64   `json:"memory_used_mb"`
        MemoryTotal   int64   `json:"memory_total_mb"`
    } `json:"gpus"`
}

func getGPUStats() (*GPUStats, error) {
    resp, err := http.Get("http://localhost:8080/proxy/stats")
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    var stats GPUStats
    if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
        return nil, err
    }

    cacheStatus := resp.Header.Get("X-Cache")
    fmt.Printf("Cache: %s\n", cacheStatus)

    return &stats, nil
}

func main() {
    ticker := time.NewTicker(1 * time.Second)
    defer ticker.Stop()

    for range ticker.C {
        stats, err := getGPUStats()
        if err != nil {
            fmt.Printf("Error: %v\n", err)
            continue
        }

        for _, gpu := range stats.GPUs {
            fmt.Printf("GPU %d: %.1f%% @ %d°C\n",
                gpu.Index, gpu.Utilization, gpu.Temperature)
        }
    }
}
```

## Performance

- **Latency**: <5ms (from cache), ~5-10ms (fresh NVML call)
- **CPU overhead**: Minimal (<0.5% CPU per request)
- **Recommended poll rate**: 1-2 seconds
- **Maximum poll rate**: Up to 100 Hz (100 requests/sec) supported

**Note:** NVML is significantly faster than nvidia-smi (5-10ms vs 50-100ms) thanks to native library access.

## Requirements

The proxy **must run as a sidecar** in the same pod as vLLM to have direct access to NVIDIA GPUs through NVML.

## Error Responses

### No GPUs Found

```json
{
  "error": "Failed to query GPU: no GPUs found"
}
```

**HTTP Status**: 500 Internal Server Error

### NVML Not Initialized

```json
{
  "error": "Failed to query GPU: NVML not initialized"
}
```

**HTTP Status**: 500 Internal Server Error

This occurs when NVML fails to initialize (e.g., GPU drivers not loaded or incompatible).

## Troubleshooting

1. **"NVML not initialized"**
   - Ensure NVIDIA GPU drivers are installed and loaded
   - Check that the container has access to `/dev/nvidia*` devices
   - Verify the NVIDIA container runtime is configured

2. **"no GPUs found"**
   - Check that the pod has GPU resources allocated
   - Verify `nvidia.com/gpu` resource requests/limits
   - Ensure the NVIDIA device plugin is running in the cluster

3. **High latency**
   - Check if caching is working (X-Cache header)
   - NVML calls should be <10ms, if higher check GPU driver health
   - Reduce poll frequency if needed

4. **Stale data**
   - Cache is only 1 second, so data should be fresh
   - Check timestamp field in response
