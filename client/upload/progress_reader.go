package upload

import (
	"io"

	"github.com/calypr/data-client/client/common"
)

type progressReader struct {
	reader     io.Reader
	onProgress common.ProgressCallback
	oid        string
	total      int64
	bytesSoFar int64
}

func newProgressReader(reader io.Reader, onProgress common.ProgressCallback, oid string, total int64) *progressReader {
	return &progressReader{
		reader:     reader,
		onProgress: onProgress,
		oid:        oid,
		total:      total,
	}
}

func resolveUploadOID(req common.FileUploadRequestObject) string {
	if req.OID != "" {
		return req.OID
	}
	if req.GUID != "" {
		return req.GUID
	}
	return req.Filename
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	if n > 0 && pr.onProgress != nil {
		delta := int64(n)
		pr.bytesSoFar += delta
		if progressErr := pr.onProgress(common.ProgressEvent{
			Event:          "progress",
			Oid:            pr.oid,
			BytesSoFar:     pr.bytesSoFar,
			BytesSinceLast: delta,
		}); progressErr != nil {
			return n, progressErr
		}
	}
	return n, err
}

func (pr *progressReader) Finalize() error {
	if pr.onProgress == nil {
		return nil
	}
	if pr.total == 0 || pr.bytesSoFar >= pr.total {
		return nil
	}
	delta := pr.total - pr.bytesSoFar
	pr.bytesSoFar = pr.total
	return pr.onProgress(common.ProgressEvent{
		Event:          "progress",
		Oid:            pr.oid,
		BytesSoFar:     pr.bytesSoFar,
		BytesSinceLast: delta,
	})
}
