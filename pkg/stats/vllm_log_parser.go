package stats

import (
	"regexp"
	"strconv"
	"strings"
)

// KVCacheInfo contains information about the KV cache parsed from vLLM logs
type KVCacheInfo struct {
	AvailableMemoryGiB float64
	AvailableMemoryMiB float64
	BlockSize          int
	NumGPUBlocks       int
	NumCPUBlocks       int
}

var (
	// Regex patterns for parsing vLLM logs
	kvCacheMemoryPattern = regexp.MustCompile(`(?i)Available KV cache memory:\s+([\d.]+)\s+(GiB|MiB)`)
	blockSizePattern     = regexp.MustCompile(`(?i)Block size:\s+(\d+)`)
	numGPUBlocksPattern  = regexp.MustCompile(`(?i)# GPU blocks:\s+(\d+)`)
	numCPUBlocksPattern  = regexp.MustCompile(`(?i)# CPU blocks:\s+(\d+)`)
)

// ParseKVCacheInfo parses KV cache information from vLLM logs
func ParseKVCacheInfo(logs string) *KVCacheInfo {
	info := &KVCacheInfo{}

	// Parse available KV cache memory
	if matches := kvCacheMemoryPattern.FindStringSubmatch(logs); len(matches) == 3 {
		value, err := strconv.ParseFloat(matches[1], 64)
		if err == nil {
			unit := strings.ToUpper(matches[2])
			switch unit {
			case "GIB":
				info.AvailableMemoryGiB = value
				info.AvailableMemoryMiB = value * 1024
			case "MIB":
				info.AvailableMemoryMiB = value
				info.AvailableMemoryGiB = value / 1024
			}
		}
	}

	// Parse block size
	if matches := blockSizePattern.FindStringSubmatch(logs); len(matches) == 2 {
		value, err := strconv.Atoi(matches[1])
		if err == nil {
			info.BlockSize = value
		}
	}

	// Parse GPU blocks
	if matches := numGPUBlocksPattern.FindStringSubmatch(logs); len(matches) == 2 {
		value, err := strconv.Atoi(matches[1])
		if err == nil {
			info.NumGPUBlocks = value
		}
	}

	// Parse CPU blocks
	if matches := numCPUBlocksPattern.FindStringSubmatch(logs); len(matches) == 2 {
		value, err := strconv.Atoi(matches[1])
		if err == nil {
			info.NumCPUBlocks = value
		}
	}

	return info
}

// IsValid returns true if the KV cache info has valid data
func (info *KVCacheInfo) IsValid() bool {
	return info.AvailableMemoryGiB > 0 || info.NumGPUBlocks > 0
}
