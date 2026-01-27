package download

import (
	"fmt"
	"io"

	"github.com/calypr/data-client/common"
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
	if pw.total > 0 && pw.bytesSoFar < pw.total {
		delta := pw.total - pw.bytesSoFar
		pw.bytesSoFar = pw.total
		if pw.onProgress != nil {
			_ = pw.onProgress(common.ProgressEvent{
				Event:          "progress",
				Oid:            pw.oid,
				BytesSoFar:     pw.bytesSoFar,
				BytesSinceLast: delta,
			})
		}
		return fmt.Errorf("download incomplete: %d/%d bytes", pw.bytesSoFar-delta, pw.total)
	}
	return nil
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
