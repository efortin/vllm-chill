#!/bin/bash
# Example script demonstrating model switching with vllm-chill

VLLM_CHILL_URL="${VLLM_CHILL_URL:-http://localhost:8080}"

echo "=== vLLM-Chill Model Management Demo ==="
echo ""

# 1. List available models
echo "1. Listing available models..."
curl -s "$VLLM_CHILL_URL/proxy/models/available" | jq '.'
echo ""

# 2. Check currently running model
echo "2. Checking currently running model..."
curl -s "$VLLM_CHILL_URL/proxy/models/running" | jq '.'
echo ""

# 3. Switch to a different model
echo "3. Switching to deepseek-r1-fp8..."
curl -s -X POST "$VLLM_CHILL_URL/proxy/models/switch" \
  -H "Content-Type: application/json" \
  -d '{"model_id": "deepseek-r1-fp8"}' | jq '.'
echo ""

# 4. Verify the switch
echo "4. Verifying model switch..."
sleep 2
curl -s "$VLLM_CHILL_URL/proxy/models/running" | jq '.'
echo ""

# 5. Switch back to original model
echo "5. Switching back to qwen3-coder-30b-fp8..."
curl -s -X POST "$VLLM_CHILL_URL/proxy/models/switch" \
  -H "Content-Type: application/json" \
  -d '{"model_id": "qwen3-coder-30b-fp8"}' | jq '.'
echo ""

echo "=== Demo Complete ==="
