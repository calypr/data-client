package download

import (
	"context"

	"github.com/calypr/data-client/common"
	"github.com/calypr/data-client/drs"
	"github.com/calypr/data-client/transfer"
)

// DownloadFile is a high-level orchestrator that downloads a file using the provided backend.
func DownloadFile(ctx context.Context, dc drs.Client, bk transfer.Downloader, guid, destPath string) error {
	opts := DownloadOptions{
		MultipartThreshold: int64(5 * common.GB),
	}
	// Note: We could expose more options here if needed
	return DownloadToPathWithOptions(ctx, dc, bk, bk.Logger().Logger, guid, destPath, "", opts)
}
