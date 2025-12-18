package g3cmd

/*
// DownloadSignedURL downloads a file from a signed URL with:
// - Resumable single-stream download (if partial file exists)
// - Concurrent multipart download for large files (>1GB)
// - Retries via go-retryablehttp
// - Progress bar support via mpb
func DownloadSignedURL(signedURL, dstPath string) error {
	// Setup retryable client
	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = 10
	retryClient.RetryWaitMin = 1 * time.Second
	retryClient.RetryWaitMax = 30 * time.Second
	retryClient.Logger = nil // silent
	client := retryClient.StandardClient()
	client.Timeout = 0 // no timeout for large downloads

	// HEAD to get size and Accept-Ranges support
	headResp, err := client.Head(signedURL)
	if err != nil {
		return fmt.Errorf("HEAD request failed: %w", err)
	}
	defer headResp.Body.Close()

	if headResp.StatusCode != http.StatusOK {
		return fmt.Errorf("HEAD failed: %s", headResp.Status)
	}

	contentLength := headResp.ContentLength
	if contentLength <= 0 {
		return fmt.Errorf("invalid Content-Length")
	}

	acceptRanges := headResp.Header.Get("Accept-Ranges") == "bytes"
	if !acceptRanges {
		return fmt.Errorf("server does not support range requests")
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return fmt.Errorf("mkdir failed: %w", err)
	}

	// Check if partial file exists
	stat, _ := os.Stat(dstPath)
	existingSize := int64(0)
	if stat != nil {
		existingSize = stat.Size()
	}

	// If we have a partial file, resume with single stream (safer and simpler)
	if existingSize > 0 && existingSize < contentLength {
		return downloadResumableSingle(signedURL, dstPath, contentLength, existingSize, client, progress)
	}

	// For complete downloads: use multipart if file is large enough
	if contentLength >= multiPartThreshold {
		return downloadConcurrentMultipart(signedURL, dstPath, contentLength, client, progress)
	}

	// Otherwise: simple single-stream download
	return downloadResumableSingle(signedURL, dstPath, contentLength, 0, client, progress)
}

// downloadResumableSingle handles single-stream (possibly resumed) download
func downloadResumableSingle(signedURL, dstPath string, totalSize, startByte int64, client *http.Client, progress *mpb.Progress) error {
	req, err := http.NewRequest("GET", signedURL, nil)
	if err != nil {
		return err
	}
	if startByte > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", startByte))
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("GET failed: %w", err)
	}
	defer resp.Body.Close()

	if startByte > 0 && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("expected 206 Partial Content, got %d", resp.StatusCode)
	}
	if startByte == 0 && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("expected 200 OK, got %d", resp.StatusCode)
	}

	file, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	if startByte > 0 {
		if _, err := file.Seek(startByte, io.SeekStart); err != nil {
			return err
		}
	} else {
		if err := file.Truncate(0); err != nil {
			return err
		}
	}

	var writer io.Writer = file
	if progress != nil {
		bar := progress.AddBar(totalSize,
			mpb.PrependDecorators(
				decor.Name(filepath.Base(dstPath)+" "),
				decor.CountersKibiByte("% .1f / % .1f"),
			),
			mpb.AppendDecorators(
				decor.Percentage(),
				decor.AverageSpeed(decor.SizeB1024(0), "% .1f"),
			),
		)
		if startByte > 0 {
			bar.SetCurrent(startByte)
		}
		writer = bar.ProxyWriter(file)
	}

	_, err = io.Copy(writer, resp.Body)
	return err
}

// downloadConcurrentMultipart downloads in parallel chunks
func downloadConcurrentMultipart(signedURL, dstPath string, totalSize int64, client *http.Client, progress *mpb.Progress) error {
	numChunks := int((totalSize + chunkSize - 1) / chunkSize)
	if numChunks < defaultConcurrency {
		numChunks = defaultConcurrency
	}
	chunkSizeActual := (totalSize + int64(numChunks) - 1) / int64(numChunks)

	// Pre-allocate file
	file, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	if err := file.Truncate(totalSize); err != nil {
		file.Close()
		return err
	}
	file.Close()

	var wg sync.WaitGroup
	var mu sync.Mutex
	var downloadErr error

	// Shared progress bar for total
	var totalBar *mpb.Bar
	if progress != nil {
		totalBar = progress.AddBar(totalSize,
			mpb.PrependDecorators(
				decor.Name(filepath.Base(dstPath)+" (multipart) "),
				decor.CountersKibiByte("% .1f / % .1f"),
			),
			mpb.AppendDecorators(
				decor.Percentage(),
				decor.AverageSpeed(decor.SizeB1024(0), "% .1f"),
			),
		)
	}

	concurrency := defaultConcurrency
	sem := make(chan struct{}, concurrency)

	for i := 0; i < int(numChunks); i++ {
		start := int64(i) * chunkSizeActual
		end := start + chunkSizeActual - 1
		if end >= totalSize {
			end = totalSize - 1
		}
		if start > end {
			break
		}

		wg.Add(1)
		sem <- struct{}{}

		go func(start, end int64, chunkIdx int) {
			defer wg.Done()
			defer func() { <-sem }()

			req, _ := http.NewRequest("GET", signedURL, nil)
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))

			resp, err := client.Do(req)
			if err != nil {
				mu.Lock()
				if downloadErr == nil {
					downloadErr = fmt.Errorf("chunk %d failed: %w", chunkIdx, err)
				}
				mu.Unlock()
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusPartialContent {
				mu.Lock()
				if downloadErr == nil {
					downloadErr = fmt.Errorf("chunk %d expected 206, got %d", chunkIdx, resp.StatusCode)
				}
				mu.Unlock()
				return
			}

			file, err := os.OpenFile(dstPath, os.O_WRONLY, 0644)
			if err != nil {
				mu.Lock()
				downloadErr = err
				mu.Unlock()
				return
			}
			file.Seek(start, io.SeekStart)
			writer := io.Writer(file)

			var chunkWriter io.Writer = writer
			if progress != nil {
				chunkBar := progress.AddBar(end-start+1,
					mpb.BarRemoveOnComplete(),
					mpb.PrependDecorators(decor.Name(fmt.Sprintf("chunk %d ", chunkIdx))),
				)
				chunkWriter = chunkBar.ProxyWriter(writer)
				defer file.Close()
			}

			if _, err := io.Copy(chunkWriter, resp.Body); err != nil {
				mu.Lock()
				if downloadErr == nil {
					downloadErr = fmt.Errorf("chunk %d copy failed: %w", chunkIdx, err)
				}
				mu.Unlock()
			} else {
				if totalBar != nil {
					totalBar.IncrBy(int(end - start + 1))
				}
			}
			if progress == nil {
				file.Close()
			}
		}(start, end, i)
	}

	wg.Wait()

	if downloadErr != nil {
		if totalBar != nil {
			totalBar.Abort(true)
		}
		return downloadErr
	}

	if totalBar != nil {
		totalBar.SetCurrent(totalSize)
	}

	return nil
}

*/
