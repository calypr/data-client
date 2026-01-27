package upload

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/calypr/data-client/common"
	client "github.com/calypr/data-client/g3client"
	"github.com/calypr/data-client/logs"
)

func UploadSingle(ctx context.Context, profile string, guid string, oid string, filePath string, bucketName string, enableLogs bool, progressCallback common.ProgressCallback) error {

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

	filePaths, err := common.ParseFilePaths(filePath, false)
	if len(filePaths) > 1 {
		return errors.New("more than 1 file location has been found. Do not use \"*\" in file path or provide a folder as file path")
	}
	if err != nil {
		return errors.New("file path parsing error: " + err.Error())
	}
	if len(filePaths) == 1 {
		filePath = filePaths[0]
	}
	filename := filepath.Base(filePath)

	file, err := os.Open(filePath)
	if err != nil {
		if enableLogs {
			sb := g3i.Logger().Scoreboard()
			if sb != nil {
				sb.IncrementSB(len(sb.Counts))
				sb.PrintSB()
			}
		}
		g3i.Logger().Failed(filePath, filename, common.FileMetadata{}, "", 0, false)
		g3i.Logger().Error("File open error", "file", filePath, "error", err)
		return fmt.Errorf("[ERROR] when opening file path %s, an error occurred: %s\n", filePath, err.Error())
	}
	defer file.Close()

	fi, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}
	fileSize := fi.Size()

	furObject := common.FileUploadRequestObject{
		FilePath: filePath,
		Filename: filename,
		GUID:     guid,
		OID:      oid,
		Bucket:   bucketName,
		Progress: progressCallback,
	}

	furObject, err = generateUploadRequest(ctx, g3i, furObject, file, nil)
	if err != nil {
		if enableLogs {
			sb := g3i.Logger().Scoreboard()
			if sb != nil {
				sb.IncrementSB(len(sb.Counts))
				sb.PrintSB()
			}
		}
		g3i.Logger().Failed(furObject.FilePath, furObject.Filename, common.FileMetadata{}, furObject.GUID, 0, false)
		g3i.Logger().Error("Error occurred during request generation", "file", filePath, "error", err)
		return fmt.Errorf("[ERROR] Error occurred during request generation for file %s: %s\n", filePath, err.Error())
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
		g3i.Logger().Succeeded(filePath, guid)
		// g3i.Logger().Info("Upload successful", "file", filePath) // Already logged by Succeeded? No.
		sb := g3i.Logger().Scoreboard()
		if sb != nil {
			sb.IncrementSB(0)
			sb.PrintSB()
		}
	}
	return nil
}
