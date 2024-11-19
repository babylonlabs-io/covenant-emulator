package tracing

import (
	"context"
	"github.com/google/uuid"
)

type TraceContextKey string

const TraceInfoKey = TraceContextKey("requestTracingInfo")
const TraceIdKey = TraceContextKey("requestTraceId")

type SpanDetail struct {
	Name     string
	Duration int64
}

type TracingInfo struct {
	SpanDetails []SpanDetail
}

func AttachTracingIntoContext(ctx context.Context) context.Context {
	// Attach traceId into context
	traceID := uuid.New().String()
	ctx = context.WithValue(ctx, TraceIdKey, traceID)

	// Start tracingInfo
	return context.WithValue(ctx, TraceInfoKey, &TracingInfo{})
}
