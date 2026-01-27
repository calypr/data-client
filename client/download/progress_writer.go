package download

import (
	"io"

	"github.com/calypr/data-client/client/common"
)

type progressWriter struct {
	writer     io.Writer
	onProgress common.ProgressCallback
	oid        string
	total      int64
	bytesSoFar int64
}

func newProgressWriter(writer io.Writer, onProgress common.ProgressCallback, oid string, total int64) *progressWriter {
	return &progressWriter{
		writer:     writer,
		onProgress: onProgress,
		oid:        oid,
		total:      total,
	}
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n, err := pw.writer.Write(p)
	if n > 0 && pw.onProgress != nil {
		delta := int64(n)
		pw.bytesSoFar += delta
		if progressErr := pw.onProgress(common.ProgressEvent{
			Event:          "progress",
			Oid:            pw.oid,
			BytesSoFar:     pw.bytesSoFar,
			BytesSinceLast: delta,
		}); progressErr != nil {
			return n, progressErr
		}
	}
	return n, err
}

func (pw *progressWriter) Finalize() error {
	if pw.onProgress == nil {
		return nil
	}
	if pw.total == 0 || pw.bytesSoFar >= pw.total {
		return nil
	}
	delta := pw.total - pw.bytesSoFar
	pw.bytesSoFar = pw.total
	return pw.onProgress(common.ProgressEvent{
		Event:          "progress",
		Oid:            pw.oid,
		BytesSoFar:     pw.bytesSoFar,
		BytesSinceLast: delta,
	})
}

func resolveDownloadOID(fdr common.FileDownloadResponseObject) string {
	if fdr.OID != "" {
		return fdr.OID
	}
	if fdr.GUID != "" {
		return fdr.GUID
	}
	return fdr.Filename
}
