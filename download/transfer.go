package download

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/calypr/data-client/backend"
	"github.com/calypr/data-client/common"
	"golang.org/x/sync/errgroup"
)

type DownloadOptions struct {
	MultipartThreshold int64
	ChunkSize          int64
	Concurrency        int
}

func defaultDownloadOptions() DownloadOptions {
	return DownloadOptions{
		MultipartThreshold: common.GB,
		ChunkSize:          64 * common.MB,
		Concurrency:        8,
	}
}

// DownloadSingleWithProgress downloads a single object while emitting progress events.
func DownloadSingleWithProgress(
	ctx context.Context,
	bk backend.Backend,
	guid string,
	downloadPath string,
	protocol string,
) error {
	progress := common.GetProgress(ctx)
	var err error
	downloadPath, err = common.ParseRootPath(downloadPath)
	if err != nil {
		return fmt.Errorf("invalid download path: %w", err)
	}
	if !strings.HasSuffix(downloadPath, "/") {
		downloadPath += "/"
	}

	renamed := make([]RenamedOrSkippedFileInfo, 0)
	info, err := GetFileInfo(ctx, bk, guid, protocol, downloadPath, "original", false, &renamed)
	if err != nil {
		return err
	}

	fdr := common.FileDownloadResponseObject{
		DownloadPath: downloadPath,
		Filename:     info.Name,
		GUID:         guid,
	}

	protocolText := ""
	if protocol != "" {
		protocolText = "?protocol=" + protocol
	}
	if err := GetDownloadResponse(ctx, bk, &fdr, protocolText); err != nil {
		return err
	}

	fullPath := filepath.Join(fdr.DownloadPath, fdr.Filename)
	if dir := filepath.Dir(fullPath); dir != "." {
		if err = os.MkdirAll(dir, 0766); err != nil {
			_ = fdr.Response.Body.Close()
			return fmt.Errorf("mkdir for %s: %w", fullPath, err)
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
		return fmt.Errorf("open local file %s: %w", fullPath, err)
	}

	total := info.Size
	var writer io.Writer = file
	var tracker *progressWriter
	if progress != nil {
		tracker = newProgressWriter(file, progress, guid, total)
		writer = tracker
	}

	_, copyErr := io.Copy(writer, fdr.Response.Body)
	_ = fdr.Response.Body.Close()
	_ = file.Close()
	if tracker != nil {
		if finalizeErr := tracker.Finalize(); finalizeErr != nil && copyErr == nil {
			copyErr = finalizeErr
		}
	}
	if copyErr != nil {
		return fmt.Errorf("download failed for %s: %w", fdr.Filename, copyErr)
	}
	return nil
}

// DownloadToPath downloads a single object using the provided backend
func DownloadToPath(
	ctx context.Context,
	bk backend.Backend,
	logger *slog.Logger,
	guid string,
	dstPath string,
	protocol string,
) error {
	opts := defaultDownloadOptions()
	return DownloadToPathWithOptions(ctx, bk, logger, guid, dstPath, protocol, opts)
}

func DownloadToPathWithOptions(
	ctx context.Context,
	bk backend.Backend,
	logger *slog.Logger,
	guid string,
	dstPath string,
	protocol string,
	opts DownloadOptions,
) error {
	if opts.MultipartThreshold <= 0 {
		opts.MultipartThreshold = defaultDownloadOptions().MultipartThreshold
	}
	if opts.ChunkSize <= 0 {
		opts.ChunkSize = defaultDownloadOptions().ChunkSize
	}
	if opts.Concurrency <= 0 {
		opts.Concurrency = defaultDownloadOptions().Concurrency
	}

	info, err := bk.GetFileDetails(ctx, guid)
	if err != nil {
		return fmt.Errorf("get file details failed: %w", err)
	}

	// If size is unknown or small, single stream is safest.
	if info.Size <= 0 || info.Size < opts.MultipartThreshold {
		return downloadToPathSingle(ctx, bk, logger, guid, dstPath, protocol)
	}

	if err := downloadToPathMultipart(ctx, bk, logger, guid, dstPath, protocol, info.Size, opts); err != nil {
		return err
	}

	return nil
}

func downloadToPathSingle(
	ctx context.Context,
	bk backend.Backend,
	logger *slog.Logger,
	guid string,
	dstPath string,
	protocol string,
) error {
	progress := common.GetProgress(ctx)
	hash := common.GetOid(ctx)

	fdr := common.FileDownloadResponseObject{
		GUID: guid,
	}

	protocolText := ""
	if protocol != "" {
		protocolText = "?protocol=" + protocol
	}

	if err := GetDownloadResponse(ctx, bk, &fdr, protocolText); err != nil {
		// Mimic failed context logging from original
		// We'd need to reconstruct the "logger.FailedContext" logic if using raw slog
		// For now, simple error logging or rely on caller to log context?
		// The original code used g3i.Logger().FailedContext...
		// Let's just log error
		logger.Error("Download failed", "error", err, "path", dstPath, "guid", guid)
		return err
	}
	defer fdr.Response.Body.Close()

	if dir := filepath.Dir(dstPath); dir != "." {
		if err := os.MkdirAll(dir, 0766); err != nil {
			logger.Error("Mkdir failed", "error", err, "path", dstPath)
			return fmt.Errorf("mkdir for %s: %w", dstPath, err)
		}
	}

	file, err := os.Create(dstPath)
	if err != nil {
		logger.Error("Create file failed", "error", err, "path", dstPath)
		return fmt.Errorf("create local file %s: %w", dstPath, err)
	}
	defer file.Close()

	var writer io.Writer = file
	if progress != nil {
		total := fdr.Response.ContentLength
		tracker := newProgressWriter(file, progress, hash, total)
		writer = tracker
		defer tracker.Finalize()
	}

	if _, err := io.Copy(writer, fdr.Response.Body); err != nil {
		logger.Error("Copy failed", "error", err, "path", dstPath)
		return fmt.Errorf("copy to %s: %w", dstPath, err)
	}

	// Success logging is up to caller or we can do simple info
	// logger.Info("Download succeeded", "path", dstPath, "guid", guid)
	return nil
}

func downloadToPathMultipart(
	ctx context.Context,
	bk backend.Backend,
	logger *slog.Logger,
	guid string,
	dstPath string,
	protocol string,
	totalSize int64,
	opts DownloadOptions,
) error {
	protocolText := ""
	if protocol != "" {
		protocolText = "?protocol=" + protocol
	}

	signedURL, err := bk.GetDownloadURL(ctx, guid, protocolText)
	if err != nil {
		return fmt.Errorf("failed to resolve download URL for %s: %w", guid, err)
	}

	// Preflight first ranged read to verify server honors ranges.
	rangeStart := int64(0)
	rangeEnd := opts.ChunkSize - 1
	if rangeEnd >= totalSize {
		rangeEnd = totalSize - 1
	}
	preflight := &common.FileDownloadResponseObject{
		GUID:         guid,
		PresignedURL: signedURL,
		RangeStart:   &rangeStart,
		RangeEnd:     &rangeEnd,
	}

	resp, err := bk.Download(ctx, preflight)
	if err != nil {
		return fmt.Errorf("multipart preflight request failed: %w", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 206 {
		return fmt.Errorf("range requests not supported (status %d)", resp.StatusCode)
	}

	if dir := filepath.Dir(dstPath); dir != "." {
		if err := os.MkdirAll(dir, 0766); err != nil {
			return fmt.Errorf("mkdir for %s: %w", dstPath, err)
		}
	}

	file, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		return fmt.Errorf("create local file %s: %w", dstPath, err)
	}
	defer file.Close()

	if err := file.Truncate(totalSize); err != nil {
		return fmt.Errorf("pre-allocate %s: %w", dstPath, err)
	}

	progress := common.GetProgress(ctx)
	hash := common.GetOid(ctx)
	if hash == "" {
		hash = guid
	}
	var soFar atomic.Int64

	totalParts := int((totalSize + opts.ChunkSize - 1) / opts.ChunkSize)
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(opts.Concurrency)

	for i := 0; i < totalParts; i++ {
		partStart := int64(i) * opts.ChunkSize
		partEnd := partStart + opts.ChunkSize - 1
		if partEnd >= totalSize {
			partEnd = totalSize - 1
		}
		ps := partStart
		pe := partEnd

		g.Go(func() error {
			fdr := &common.FileDownloadResponseObject{
				GUID:         guid,
				PresignedURL: signedURL,
				RangeStart:   &ps,
				RangeEnd:     &pe,
			}

			partResp, err := bk.Download(gctx, fdr)
			if err != nil {
				return fmt.Errorf("range download %d-%d failed: %w", ps, pe, err)
			}
			defer partResp.Body.Close()

			if partResp.StatusCode != 206 {
				return fmt.Errorf("range download %d-%d returned status %d", ps, pe, partResp.StatusCode)
			}

			buf, err := io.ReadAll(partResp.Body)
			if err != nil {
				return fmt.Errorf("range read %d-%d failed: %w", ps, pe, err)
			}

			if _, err := file.WriteAt(buf, ps); err != nil {
				return fmt.Errorf("range write %d-%d failed: %w", ps, pe, err)
			}

			if progress != nil {
				current := soFar.Add(int64(len(buf)))
				_ = progress(common.ProgressEvent{
					Event:          "progress",
					Oid:            hash,
					BytesSinceLast: int64(len(buf)),
					BytesSoFar:     current,
				})
			}

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	if progress != nil {
		final := soFar.Load()
		if final < totalSize {
			_ = progress(common.ProgressEvent{
				Event:          "progress",
				Oid:            hash,
				BytesSinceLast: totalSize - final,
				BytesSoFar:     totalSize,
			})
		}
	}

	logger.Info("multipart download completed", "guid", guid, "size", totalSize)
	return nil
}
