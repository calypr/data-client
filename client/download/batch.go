package download

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	client "github.com/calypr/data-client/client/client"
	"github.com/calypr/data-client/client/common"
	"github.com/calypr/data-client/client/logs"
	"github.com/hashicorp/go-multierror"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
	"golang.org/x/sync/errgroup"
)

// downloadFiles performs bounded parallel downloads and collects ALL errors
func downloadFiles(
	ctx context.Context,
	g3i client.Gen3Interface,
	files []common.FileDownloadResponseObject,
	numParallel int,
	protocol string,
) (int, error) {
	if len(files) == 0 {
		return 0, nil
	}

	logger := g3i.Logger()

	protocolText := ""
	if protocol != "" {
		protocolText = "?protocol=" + protocol
	}

	// Scoreboard: maxRetries = 0 for now (no retry logic yet)
	sb := logs.NewSB(0, logger)

	p := mpb.New(mpb.WithOutput(os.Stdout))

	var eg errgroup.Group
	eg.SetLimit(numParallel)

	var success atomic.Int64
	var mu sync.Mutex
	var allErrors []*multierror.Error

	for i := range files {
		fdr := &files[i] // capture loop variable

		eg.Go(func() error {
			var err error

			defer func() {
				if err != nil {
					// Final failure bucket
					sb.IncrementSB(len(sb.Counts) - 1)

					mu.Lock()
					allErrors = append(allErrors, multierror.Append(nil, err))
					mu.Unlock()
				} else {
					success.Add(1)
					sb.IncrementSB(0) // success, no retries
				}
			}()

			// Get presigned URL
			if err = GetDownloadResponse(ctx, g3i, fdr, protocolText); err != nil {
				err = fmt.Errorf("get URL for %s (GUID: %s): %w", fdr.Filename, fdr.GUID, err)
				return err
			}

			// Prepare directories
			fullPath := filepath.Join(fdr.DownloadPath, fdr.Filename)
			if dir := filepath.Dir(fullPath); dir != "." {
				if err = os.MkdirAll(dir, 0766); err != nil {
					_ = fdr.Response.Body.Close()
					err = fmt.Errorf("mkdir for %s: %w", fullPath, err)
					return err
				}
			}

			flags := os.O_CREATE | os.O_WRONLY
			if fdr.Range > 0 {
				flags |= os.O_APPEND
			} else if fdr.Overwrite {
				flags |= os.O_TRUNC
			}

			file, err := os.OpenFile(fullPath, flags, 0666)
			if err != nil {
				_ = fdr.Response.Body.Close()
				err = fmt.Errorf("open local file %s: %w", fullPath, err)
				return err
			}

			// Progress bar for this file
			total := fdr.Response.ContentLength + fdr.Range
			bar := p.AddBar(total,
				mpb.PrependDecorators(
					decor.Name(truncateFilename(fdr.Filename, 40)+" "),
					decor.CountersKibiByte("% .1f / % .1f"),
				),
				mpb.AppendDecorators(
					decor.Percentage(),
					decor.AverageSpeed(decor.SizeB1024(0), "% .1f"),
				),
			)

			if fdr.Range > 0 {
				bar.SetCurrent(fdr.Range)
			}

			writer := bar.ProxyWriter(file)

			_, copyErr := io.Copy(writer, fdr.Response.Body)
			_ = fdr.Response.Body.Close()
			_ = file.Close()

			if copyErr != nil {
				bar.Abort(true)
				err = fmt.Errorf("download failed for %s: %w", fdr.Filename, copyErr)
				return err
			}

			return nil
		})
	}

	// Wait for all downloads
	_ = eg.Wait()
	p.Wait()

	// Combine errors
	var combinedError error
	mu.Lock()
	if len(allErrors) > 0 {
		multiErr := multierror.Append(nil, nil)
		for _, e := range allErrors {
			multiErr = multierror.Append(multiErr, e.Errors...)
		}
		combinedError = multiErr.ErrorOrNil()
	}
	mu.Unlock()

	downloaded := int(success.Load())

	// Print scoreboard summary
	sb.PrintSB()

	if combinedError != nil {
		logger.Printf("%d files downloaded, but %d failed:\n", downloaded, len(allErrors))
		logger.Println(combinedError.Error())
	} else {
		logger.Printf("%d files downloaded successfully.\n", downloaded)
	}

	return downloaded, combinedError
}
