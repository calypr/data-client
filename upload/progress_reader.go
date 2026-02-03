package upload

import (
	"fmt"
	"io"

	"github.com/calypr/data-client/common"
)

type progressReader struct {
	reader     io.Reader
	onProgress common.ProgressCallback
	hash       string
	total      int64
	bytesSoFar int64
}

func newProgressReader(reader io.Reader, onProgress common.ProgressCallback, hash string, total int64) *progressReader {
	return &progressReader{
		reader:     reader,
		onProgress: onProgress,
		hash:       hash,
		total:      total,
	}
}

func resolveUploadOID(req common.FileUploadRequestObject) string {
	if req.ObjectKey != "" {
		return req.ObjectKey
	}
	return req.GUID
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	if n > 0 && pr.onProgress != nil {
		delta := int64(n)
		pr.bytesSoFar += delta
		if progressErr := pr.onProgress(common.ProgressEvent{
			Event:          "progress",
			Oid:            pr.hash,
			BytesSoFar:     pr.bytesSoFar,
			BytesSinceLast: delta,
		}); progressErr != nil {
			return n, progressErr
		}
	}
	return n, err
}

func (pr *progressReader) Finalize() error {
	if pr.total > 0 && pr.bytesSoFar < pr.total {
		delta := pr.total - pr.bytesSoFar
		pr.bytesSoFar = pr.total
		if pr.onProgress != nil {
			_ = pr.onProgress(common.ProgressEvent{
				Event:          "progress",
				Oid:            pr.hash,
				BytesSoFar:     pr.bytesSoFar,
				BytesSinceLast: delta,
			})
		}
		return fmt.Errorf("upload incomplete: %d/%d bytes", pr.bytesSoFar-delta, pr.total)
	}
	return nil
}
