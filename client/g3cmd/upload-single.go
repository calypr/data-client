package g3cmd

// Deprecated: Use upload instead.
import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/calypr/data-client/client/common"
	client "github.com/calypr/data-client/client/gen3Client"
	"github.com/calypr/data-client/client/logs"
	"github.com/spf13/cobra"
)

func init() {
	var guid string
	var filePath string
	var bucketName string

	var uploadSingleCmd = &cobra.Command{
		Use:     "upload-single",
		Short:   "Upload a single file to a GUID",
		Long:    `Gets a presigned URL for which to upload a file associated with a GUID and then uploads the specified file.`,
		Example: `./data-client upload-single --profile=<profile-name> --guid=f6923cf3-xxxx-xxxx-xxxx-14ab3f84f9d6 --file=<path-to-file>`,
		Run: func(cmd *cobra.Command, args []string) {
			// initialize transmission logs
			err := UploadSingle(profile, guid, filePath, bucketName, true)
			if err != nil {
				log.Fatalln(err.Error())
			}
		},
	}
	uploadSingleCmd.Flags().StringVar(&profile, "profile", "", "Specify profile to use")
	uploadSingleCmd.MarkFlagRequired("profile") //nolint:errcheck
	uploadSingleCmd.Flags().StringVar(&guid, "guid", "", "Specify the guid for the data you would like to work with")
	uploadSingleCmd.MarkFlagRequired("guid") //nolint:errcheck
	uploadSingleCmd.Flags().StringVar(&filePath, "file", "", "Specify file to upload to with --file=~/path/to/file")
	uploadSingleCmd.MarkFlagRequired("file") //nolint:errcheck
	uploadSingleCmd.Flags().StringVar(&bucketName, "bucket", "", "The bucket to which files will be uploaded. If not provided, defaults to Gen3's configured DATA_UPLOAD_BUCKET.")
	RootCmd.AddCommand(uploadSingleCmd)
}

func UploadSingle(profile string, guid string, filePath string, bucketName string, enableLogs bool) error {

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
		context.Background(),
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

	furObject, err = GenerateUploadRequest(g3i, furObject, file, nil)
	if err != nil {
		file.Close()
		g3i.Logger().Failed(furObject.FilePath, furObject.Filename, common.FileMetadata{}, furObject.GUID, 0, false)
		sb := g3i.Logger().Scoreboard()
		sb.IncrementSB(len(sb.Counts))
		sb.PrintSB()
		g3i.Logger().Fatalf("Error occurred during request generation: %s", err.Error())
		return fmt.Errorf("[ERROR] Error occurred during request generation for file %s: %s\n", filePath, err.Error())
	}
	err = uploadFile(g3i, furObject, 0)
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
