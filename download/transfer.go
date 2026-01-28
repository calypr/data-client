package download

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/calypr/data-client/common"
	"github.com/calypr/data-client/g3client"
)

// DownloadSingleWithProgress downloads a single object while emitting progress events.
func DownloadSingleWithProgress(
	ctx context.Context,
	g3i g3client.Gen3Interface,
	guid string,
	downloadPath string,
	protocol string,
	progress common.ProgressCallback,
) error {
	var err error
	downloadPath, err = common.ParseRootPath(downloadPath)
	if err != nil {
		return fmt.Errorf("invalid download path: %w", err)
	}
	if !strings.HasSuffix(downloadPath, "/") {
		downloadPath += "/"
	}

	renamed := make([]RenamedOrSkippedFileInfo, 0)
	info, err := AskGen3ForFileInfo(ctx, g3i, guid, protocol, downloadPath, "original", false, &renamed)
	if err != nil {
		return err
	}

	fdr := common.FileDownloadResponseObject{
		DownloadPath: downloadPath,
		Filename:     info.Name,
		GUID:         guid,
		Progress:     progress,
	}

	protocolText := ""
	if protocol != "" {
		protocolText = "?protocol=" + protocol
	}
	if err := GetDownloadResponse(ctx, g3i, &fdr, protocolText); err != nil {
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
	if fdr.Progress != nil {
		tracker = newProgressWriter(file, fdr.Progress, fdr.GUID, total)
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

// DownloadToPath downloads a single object by GUID to a specific destination file path.
// It bypasses the name lookup from Gen3 and uses the provided dstPath directly.
func DownloadToPath(
	ctx context.Context,
	g3i g3client.Gen3Interface,
	guid string,
	dstPath string,
	hash string, // Content hash (e.g. SHA256) for progress tracking
	progress common.ProgressCallback,
) error {
	fdr := common.FileDownloadResponseObject{
		GUID:     guid,
		Progress: progress,
	}

	if err := GetDownloadResponse(ctx, g3i, &fdr, ""); err != nil {
		return err
	}
	defer fdr.Response.Body.Close()

	if dir := filepath.Dir(dstPath); dir != "." {
		if err := os.MkdirAll(dir, 0766); err != nil {
			return fmt.Errorf("mkdir for %s: %w", dstPath, err)
		}
	}

	file, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("create local file %s: %w", dstPath, err)
	}
	defer file.Close()

	var writer io.Writer = file
	if fdr.Progress != nil {
		total := fdr.Response.ContentLength
		tracker := newProgressWriter(file, fdr.Progress, hash, total)
		writer = tracker
		defer tracker.Finalize()
	}

	if _, err := io.Copy(writer, fdr.Response.Body); err != nil {
		return fmt.Errorf("copy to %s: %w", dstPath, err)
	}

	return nil
}
