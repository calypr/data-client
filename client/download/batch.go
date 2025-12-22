package download

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	client "github.com/calypr/data-client/client/client"
	"github.com/calypr/data-client/client/common"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

func batchDownload(
	g3 client.Gen3Interface,
	progress *mpb.Progress,
	globalBar *mpb.Bar, // counts completed files
	batch []common.FileDownloadResponseObject,
	protocolText string,
	workers int,
	errCh chan error,
) int {
	var ready []common.FileDownloadResponseObject

	// ---- prepare download responses + per-file bars ----
	for i := range batch {
		fdr := batch[i]

		if err := GetDownloadResponse(g3, &fdr, protocolText); err != nil {
			errCh <- err
			continue
		}

		flags := os.O_CREATE | os.O_RDWR
		if fdr.Range > 0 {
			flags = os.O_APPEND | os.O_RDWR
		} else if fdr.Overwrite {
			flags = os.O_TRUNC | os.O_RDWR
		}

		if sub := filepath.Dir(fdr.Filename); sub != "." && sub != "/" {
			_ = os.MkdirAll(fdr.DownloadPath+sub, 0766)
		}

		file, err := os.OpenFile(fdr.DownloadPath+fdr.Filename, flags, 0666)
		if err != nil {
			errCh <- fmt.Errorf("open local file failed: %w", err)
			continue
		}

		total := fdr.Response.ContentLength + fdr.Range

		fileBar := progress.AddBar(
			total,
			mpb.PrependDecorators(
				decor.Name(fdr.Filename+" "),
				decor.CountersKibiByte("% .1f / % .1f"),
			),
			mpb.AppendDecorators(
				decor.Percentage(),
				decor.AverageSpeed(decor.SizeB1024(0), " % .1f"),
			),
		)

		if fdr.Range > 0 {
			fileBar.SetCurrent(fdr.Range)
		}

		fdr.Writer = fileBar.ProxyWriter(file)

		ready = append(ready, fdr)
	}

	// ---- worker pool ----
	fdrCh := make(chan common.FileDownloadResponseObject)
	var wg sync.WaitGroup
	var success int64

	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for fdr := range fdrCh {
				_, err := io.Copy(fdr.Writer, fdr.Response.Body)

				// close resources deterministically
				_ = fdr.Response.Body.Close()

				if err != nil {
					errCh <- fmt.Errorf("copy failed for %s: %w", fdr.Filename, err)
					continue
				}

				atomic.AddInt64(&success, 1)

				globalBar.Increment()
			}
		}()
	}

	for _, fdr := range ready {
		fdrCh <- fdr
	}
	close(fdrCh)

	wg.Wait()
	return int(success)
}
