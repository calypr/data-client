package upload

import (
	"testing"

	"github.com/calypr/data-client/client/common"
)

func TestOptimalChunkSize(t *testing.T) {
	tests := []struct {
		name          string
		fileSize      int64
		wantChunkSize int64
		wantChunks    int64
	}{
		{
			name:          "100MB",
			fileSize:      100 * common.MB,
			wantChunkSize: 100 * common.MB,
			wantChunks:    1,
		},
		{
			name:          "1GB",
			fileSize:      1 * common.GB,
			wantChunkSize: 10 * common.MB,
			wantChunks:    103,
		},
		{
			name:          "5GB",
			fileSize:      5 * common.GB,
			wantChunkSize: 70 * common.MB,
			wantChunks:    74,
		},
		{
			name:          "10GB",
			fileSize:      10 * common.GB,
			wantChunkSize: 128 * common.MB,
			wantChunks:    80,
		},
		{
			name:          "50GB",
			fileSize:      50 * common.GB,
			wantChunkSize: 256 * common.MB,
			wantChunks:    200,
		},
		{
			name:          "100GB",
			fileSize:      100 * common.GB,
			wantChunkSize: 256 * common.MB,
			wantChunks:    400,
		},
		{
			name:          "1TB",
			fileSize:      1 * common.TB,
			wantChunkSize: 1 * common.GB,
			wantChunks:    1024,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunkSize := OptimalChunkSize(tt.fileSize)
			if chunkSize != tt.wantChunkSize {
				t.Fatalf("chunk size = %d, want %d", chunkSize, tt.wantChunkSize)
			}

			chunks := (tt.fileSize + chunkSize - 1) / chunkSize
			if chunks != tt.wantChunks {
				t.Fatalf("chunks = %d, want %d", chunks, tt.wantChunks)
			}
		})
	}
}
