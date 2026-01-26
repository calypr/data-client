package common

// ProgressEvent matches the Git LFS custom transfer progress payload.
type ProgressEvent struct {
	Event          string `json:"event"`
	Oid            string `json:"oid"`
	BytesSoFar     int64  `json:"bytesSoFar"`
	BytesSinceLast int64  `json:"bytesSinceLast"`
}

// ProgressCallback emits transfer progress updates.
type ProgressCallback func(ProgressEvent) error
