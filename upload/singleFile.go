package upload

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/calypr/data-client/common"
	client "github.com/calypr/data-client/g3client"
	"github.com/calypr/data-client/logs"
)

func UploadSingle(ctx context.Context, profile string, req common.FileUploadRequestObject, enableLogs bool) error {

	logger, closer := logs.New(profile, logs.WithSucceededLog(), logs.WithFailedLog())
	if enableLogs {
		logger, closer = logs.New(
			profile,
			logs.WithSucceededLog(),
			logs.WithFailedLog(),
			logs.WithScoreboard(),
			logs.WithConsole(),
		)
	}
	defer closer()

	// Instantiate interface to Gen3
	g3i, err := client.NewGen3Interface(
		profile,
		logger,
	)
	if err != nil {
		return fmt.Errorf("failed to parse config on profile %s: %w", profile, err)
	}

	// Helper to handle * in path if it was passed, though optimally caller handles this.
	// We will trust the SourcePath in the request object mostly, but for safety we can check existence.
	// But commonly parsing happens before creating the object usually.
	// Let's assume req.SourcePath is a single valid file path for now as per design.

	file, err := os.Open(req.SourcePath)
	if err != nil {
		if enableLogs {
			sb := g3i.Logger().Scoreboard()
			if sb != nil {
				sb.IncrementSB(len(sb.Counts))
				sb.PrintSB()
			}
		}
		g3i.Logger().Failed(req.SourcePath, req.ObjectKey, common.FileMetadata{}, "", 0, false)
		g3i.Logger().Error("File open error", "file", req.SourcePath, "error", err)
		return fmt.Errorf("[ERROR] when opening file path %s, an error occurred: %s\n", req.SourcePath, err.Error())
	}
	defer file.Close()

	fi, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}
	fileSize := fi.Size()

	furObject, err := generateUploadRequest(ctx, g3i, req, file, nil)
	if err != nil {
		if enableLogs {
			sb := g3i.Logger().Scoreboard()
			if sb != nil {
				sb.IncrementSB(len(sb.Counts))
				sb.PrintSB()
			}
		}
		g3i.Logger().Failed(req.SourcePath, req.ObjectKey, common.FileMetadata{}, req.GUID, 0, false)
		g3i.Logger().Error("Error occurred during request generation", "file", req.SourcePath, "error", err)
		return fmt.Errorf("[ERROR] Error occurred during request generation for file %s: %s\n", req.SourcePath, err.Error())
	}

	var reader io.Reader = file
	var progressTracker *progressReader
	if furObject.Progress != nil {
		progressTracker = newProgressReader(file, furObject.Progress, resolveUploadOID(furObject), fileSize)
		reader = progressTracker
	}

	_, err = uploadPart(ctx, furObject.PresignedURL, reader, fileSize)
	if progressTracker != nil {
		if finalizeErr := progressTracker.Finalize(); finalizeErr != nil && err == nil {
			err = finalizeErr
		}
	}
	if enableLogs {
		g3i.Logger().Succeeded(req.SourcePath, req.GUID)
		sb := g3i.Logger().Scoreboard()
		if sb != nil {
			sb.IncrementSB(0)
			sb.PrintSB()
		}
	}
	return nil
}
