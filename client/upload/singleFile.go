package upload

import (
	"bytes"
	"context"
	"encoding/json"
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
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		g3i.Logger().Failed(filePath, filename, common.FileMetadata{}, "", 0, false)
		sb := g3i.Logger().Scoreboard()
		sb.IncrementSB(len(sb.Counts))
		sb.PrintSB()
		return fmt.Errorf("[ERROR] The file you specified \"%s\" does not exist locally\n", filePath)
	}

	file, err := os.Open(filePath)
	if err != nil {
		sb := g3i.Logger().Scoreboard()
		sb.IncrementSB(len(sb.Counts))
		sb.PrintSB()
		g3i.Logger().Failed(filePath, filename, common.FileMetadata{}, "", 0, false)
		g3i.Logger().Println("File open error: " + err.Error())
		return fmt.Errorf("[ERROR] when opening file path %s, an error occurred: %s\n", filePath, err.Error())
	}
	defer file.Close()

	furObject := common.FileUploadRequestObject{FilePath: filePath, Filename: filename, GUID: guid, Bucket: bucketName}

	furObject, err = generateUploadRequest(ctx, g3i, furObject, file, nil)
	if err != nil {
		file.Close()
		g3i.Logger().Failed(furObject.FilePath, furObject.Filename, common.FileMetadata{}, furObject.GUID, 0, false)
		sb := g3i.Logger().Scoreboard()
		sb.IncrementSB(len(sb.Counts))
		sb.PrintSB()
		g3i.Logger().Fatalf("Error occurred during request generation: %s", err.Error())
		return fmt.Errorf("[ERROR] Error occurred during request generation for file %s: %s\n", filePath, err.Error())
	}
	jsonData, err := json.Marshal(furObject)
	if err != nil {
		return fmt.Errorf("failed to marshal furObject: %w", err)
	}

	_, err = uploadPart(ctx, g3i, furObject.PresignedURL, bytes.NewReader(jsonData))
	if err != nil {
		sb := g3i.Logger().Scoreboard()
		sb.IncrementSB(len(sb.Counts))
		return fmt.Errorf("[ERROR] Error uploading file %s: %s\n", filePath, err.Error())
	} else {
		g3i.Logger().Scoreboard().IncrementSB(0)
	}
	g3i.Logger().Scoreboard().PrintSB()
	return nil
}
