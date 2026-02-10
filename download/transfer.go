package download

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/calypr/data-client/backend"
	"github.com/calypr/data-client/common"
)

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
