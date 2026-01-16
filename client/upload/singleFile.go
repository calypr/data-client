package upload

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	client "github.com/calypr/data-client/client/client"
	"github.com/calypr/data-client/client/common"
	"github.com/calypr/data-client/client/logs"
)

func UploadSingle(ctx context.Context, profile string, guid string, filePath string, bucketName string, enableLogs bool) error {

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
			sb.IncrementSB(len(sb.Counts))
			sb.PrintSB()
		}
		g3i.Logger().Failed(filePath, filename, common.FileMetadata{}, "", 0, false)
		g3i.Logger().Println("File open error: " + err.Error())

		return fmt.Errorf("[ERROR] when opening file path %s, an error occurred: %s\n", filePath, err.Error())
	}
	defer file.Close()

	fi, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}
	fileSize := fi.Size()

	furObject := common.FileUploadRequestObject{FilePath: filePath, Filename: filename, GUID: guid, Bucket: bucketName}

	furObject, err = generateUploadRequest(ctx, g3i, furObject, file, nil)
	if err != nil {
		if enableLogs {
			sb := g3i.Logger().Scoreboard()
			sb.IncrementSB(len(sb.Counts))
			sb.PrintSB()
		}
		g3i.Logger().Failed(furObject.FilePath, furObject.Filename, common.FileMetadata{}, furObject.GUID, 0, false)
		g3i.Logger().Printf("Error occurred during request generation: %s", err.Error())
		return fmt.Errorf("[ERROR] Error occurred during request generation for file %s: %s\n", filePath, err.Error())
	}

	_, err = uploadPart(ctx, furObject.PresignedURL, file, fileSize)
	if err != nil {
		if enableLogs {
			g3i.Logger().Scoreboard().IncrementSB(1) // Increment failure
		}
		return fmt.Errorf("[ERROR] Error uploading file content for %s: %w", filePath, err)
	}

	if enableLogs {
		g3i.Logger().Scoreboard().IncrementSB(0)
		g3i.Logger().Scoreboard().PrintSB()
	}
	return nil
}
