package cli

import (
	"fmt"
	"io"

	"github.com/alex-mccollum/igw-cli/internal/gateway"
)

const callStatsSchemaVersion = 1

type rpcQueueStats struct {
	QueueWaitMs int64 `json:"queueWaitMs"`
	QueueDepth  int   `json:"queueDepth"`
}

type callStats struct {
	Version   int                 `json:"version"`
	TimingMs  int64               `json:"timingMs"`
	BodyBytes int64               `json:"bodyBytes"`
	HTTP      *gateway.CallTiming `json:"http,omitempty"`
	Truncated bool                `json:"truncated,omitempty"`
	RPC       *rpcQueueStats      `json:"rpc,omitempty"`
}

func buildCallStats(resp *gateway.CallResponse, timingMs int64) callStats {
	stats := callStats{
		Version:   callStatsSchemaVersion,
		TimingMs:  timingMs,
		BodyBytes: 0,
	}
	if resp == nil {
		return stats
	}

	stats.BodyBytes = resp.BodyBytes
	stats.HTTP = resp.Timing
	stats.Truncated = resp.Truncated
	return stats
}

func withRPCQueueStats(stats callStats, queueWaitMs int64, queueDepth int) callStats {
	stats.RPC = &rpcQueueStats{
		QueueWaitMs: queueWaitMs,
		QueueDepth:  queueDepth,
	}
	return stats
}

func printTimingSummary(w io.Writer, payload callStats) {
	if w == nil {
		return
	}
	if payload.HTTP != nil {
		fmt.Fprintf(w, "timing\thttp=%v\tbodyBytes=%v\ttruncated=%t\n", payload.HTTP, payload.BodyBytes, payload.Truncated)
		return
	}
	if payload.Truncated || payload.BodyBytes > 0 {
		fmt.Fprintf(w, "timing\tbodyBytes=%v\ttruncated=%t\n", payload.BodyBytes, payload.Truncated)
	}
}
