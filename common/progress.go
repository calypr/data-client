package common

import (
	"context"
)

// ProgressEvent matches the Git LFS custom transfer progress payload.
type ProgressEvent struct {
	Event          string `json:"event"`
	Oid            string `json:"oid"`
	BytesSoFar     int64  `json:"bytesSoFar"`
	BytesSinceLast int64  `json:"bytesSinceLast"`
	Message        string `json:"message,omitempty"`
}

// ProgressCallback emits transfer progress updates.
type ProgressCallback func(ProgressEvent) error

type contextKey string

const (
	progressKey contextKey = "progressCallback"
	oidKey      contextKey = "activeOid"
)

// WithProgress returns a new context with the provided ProgressCallback.
func WithProgress(ctx context.Context, cb ProgressCallback) context.Context {
	return context.WithValue(ctx, progressKey, cb)
}

// GetProgress returns the ProgressCallback from the context, or nil if not found.
func GetProgress(ctx context.Context) ProgressCallback {
	if cb, ok := ctx.Value(progressKey).(ProgressCallback); ok {
		return cb
	}
	return nil
}

// WithOid returns a new context with the provided OID.
func WithOid(ctx context.Context, oid string) context.Context {
	return context.WithValue(ctx, oidKey, oid)
}

// GetOid returns the OID from the context, or empty string if not found.
func GetOid(ctx context.Context) string {
	if oid, ok := ctx.Value(oidKey).(string); ok {
		return oid
	}
	return ""
}
