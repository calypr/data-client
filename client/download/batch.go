package download

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/calypr/data-client/client/common"
	client "github.com/calypr/data-client/client/gen3Client"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

// batchDownload downloads a batch in parallel and returns success count
func batchDownload(
	g3 client.Gen3Interface,
	progress *mpb.Progress,
	batch []common.FileDownloadResponseObject,
	protocolText string,
	workers int,
	errCh chan error,
) int {
	var ready []common.FileDownloadResponseObject
	for _, fdr := range batch {
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
			os.MkdirAll(fdr.DownloadPath+sub, 0766) // ignore error — already checked upstream
		}

		file, err := os.OpenFile(fdr.DownloadPath+fdr.Filename, flags, 0666)
		if err != nil {
			errCh <- errors.New("Open local file failed: " + err.Error())
			continue
		}

		total := fdr.Response.ContentLength + fdr.Range
		bar := progress.AddBar(total,
			mpb.PrependDecorators(decor.Name(fdr.Filename+" "), decor.CountersKibiByte("% .1f / % .1f")),
			mpb.AppendDecorators(decor.Percentage(), decor.AverageSpeed(decor.SizeB1024(0), " % .1f")),
		)
		if fdr.Range > 0 {
			bar.SetCurrent(fdr.Range)
		}

		fdr.Writer = bar.ProxyWriter(file)
		ready = append(ready, fdr)

		defer file.Close()
		defer fdr.Response.Body.Close()
	}

	fdrCh := make(chan common.FileDownloadResponseObject, len(ready))
	var wg sync.WaitGroup
	success := 0

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for fdr := range fdrCh {
				if _, err := io.Copy(fdr.Writer, fdr.Response.Body); err != nil {
					errCh <- errors.New("Copy failed: " + err.Error())
					return
				}
				success++
			}
		}()
	}

	for _, fdr := range ready {
		fdrCh <- fdr
	}
	close(fdrCh)
	wg.Wait()

	return success
}
