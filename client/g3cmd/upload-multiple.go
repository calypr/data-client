package g3cmd

// Deprecated: Use upload instead.
import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/calypr/data-client/client/common"
	client "github.com/calypr/data-client/client/gen3Client"
	"github.com/calypr/data-client/client/logs"
	"github.com/spf13/cobra"
)

func init() {
	var bucketName string
	var manifestPath string
	var uploadPath string
	var batch bool
	var numParallel int
	var forceMultipart bool
	var includeSubDirName bool

	var uploadMultipleCmd = &cobra.Command{
		Use:     "upload-multiple",
		Short:   "Upload multiple of files from a specified manifest",
		Long:    `Get presigned URLs for multiple of files specified in a manifest file and then upload all of them. Options to run multipart uploads for large files and running multiple workers to batch upload available.`,
		Example: `./data-client upload-multiple --profile=<profile-name> --manifest=<path-to-manifest/manifest.json> --upload-path=<path-to-file-dir/> --bucket=<bucket-name> --force-multipart=<boolean> --include-subdirname=<boolean> --batch=<boolean>`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Notice: this is the upload method which requires the user to provide GUIDs. In this method files will be uploaded to specified GUIDs.\nIf your intention is to upload files without pre-existing GUIDs, consider to use \"./data-client upload\" instead.\n\n")

			// Instantiate interface to Gen3
			g3i, err := client.NewGen3InterfaceWithLogger(context.Background(), profile,
				logs.New(profile, logs.WithSucceededLog(), logs.WithFailedLog(), logs.WithScoreboard()))
			if err != nil {
				log.Fatalf("Failed to parse config on profile %s, %v", profile, err)
			}

			host, err := g3i.GetHost()
			if err != nil {
				log.Fatalln("Error occurred during parsing config file for hostname: " + err.Error())
			}
			dataExplorerURL := host.Scheme + "://" + host.Host + "/explorer"

			var objects []ManifestObject

			manifestFile, err := os.Open(manifestPath)
			if err != nil {
				log.Println("Failed to open manifest file")
				log.Fatalln("A valid manifest can be acquired by using the \"Download Manifest\" button on " + dataExplorerURL)
			}
			defer manifestFile.Close()
			switch {
			case strings.EqualFold(filepath.Ext(manifestPath), ".json"):
				manifestBytes, err := os.ReadFile(manifestPath)
				if err != nil {
					log.Printf("Failed reading manifest %s, %v\n", manifestPath, err)
					log.Fatalln("A valid manifest can be acquired by using the \"Download Manifest\" button on " + dataExplorerURL)
				}
				err = json.Unmarshal(manifestBytes, &objects)
				if err != nil {
					log.Fatalln("Unmarshalling manifest failed with error: " + err.Error())
				}
			default:
				log.Println("Unsupported manifast format")
				log.Fatalln("A valid manifest can be acquired by using the \"Download Manifest\" button on " + dataExplorerURL)
			}

			absUploadPath, err := common.GetAbsolutePath(uploadPath)
			if err != nil {
				log.Fatalf("Error when parsing file paths: %s", err.Error())
			}

			// Create unified upload request objects
			uploadRequestObjects := make([]common.FileUploadRequestObject, 0, len(objects))

			for _, object := range objects {
				var localFilePath string
				// Determine the local file path
				if object.Filename != "" {
					// conform to fence naming convention
					localFilePath, err = getFullFilePath(absUploadPath, object.Filename)
				} else {
					// Otherwise, here we are assuming the local filename will be the same as GUID
					localFilePath, err = getFullFilePath(absUploadPath, object.ObjectID)
				}

				if err != nil {
					log.Println(err.Error())
					continue
				}

				fileInfo, err := ProcessFilename(absUploadPath, localFilePath, object.ObjectID, includeSubDirName, false)
				if err != nil {
					g3i.Logger().Failed(localFilePath, filepath.Base(localFilePath), common.FileMetadata{}, object.ObjectID, 0, false)
					log.Println("Process filename error: " + err.Error())
					continue
				}

				// Convert FileInfo to the unified common.FileUploadRequestObject
				furObject := common.FileUploadRequestObject{
					FilePath:     fileInfo.FilePath,
					Filename:     fileInfo.Filename,
					FileMetadata: fileInfo.FileMetadata,
					GUID:         fileInfo.GUID,
				}
				uploadRequestObjects = append(uploadRequestObjects, furObject)
			}

			// Separate into single-part and multipart objects
			singlePartObjects, multipartObjects := separateSingleAndMultipartUploads(g3i, uploadRequestObjects, forceMultipart)
			// Pass the unified objects to the upload handlers
			if batch {
				workers, respCh, errCh, batchFURObjects := initBatchUploadChannels(numParallel, len(singlePartObjects))
				for i, furObject := range singlePartObjects {
					// FileInfo processing and path normalization are already done, so we use the object directly
					if len(batchFURObjects) < workers {
						batchFURObjects = append(batchFURObjects, furObject)
					} else {
						batchUpload(g3i, batchFURObjects, workers, respCh, errCh, bucketName)
						batchFURObjects = []common.FileUploadRequestObject{furObject}
					}
					if !forceMultipart && i == len(singlePartObjects)-1 && len(batchFURObjects) > 0 { // upload remainders
						batchUpload(g3i, batchFURObjects, workers, respCh, errCh, bucketName)
					}
				}
			} else {
				processSingleUploads(g3i, singlePartObjects, bucketName, includeSubDirName, absUploadPath) // Assuming updated
			}

			if len(multipartObjects) > 0 {
				err := processMultipartUpload(g3i, multipartObjects, bucketName, includeSubDirName, absUploadPath)
				if err != nil {
					log.Fatalln(err.Error())
				}
			}

			if len(g3i.Logger().GetSucceededLogMap()) == 0 {
				retryUpload(g3i, g3i.Logger().GetFailedLogMap())
			}

			g3i.Logger().Scoreboard().PrintSB()
		},
	}

	uploadMultipleCmd.Flags().StringVar(&profile, "profile", "", "Specify profile to use")
	uploadMultipleCmd.MarkFlagRequired("profile") //nolint:errcheck
	uploadMultipleCmd.Flags().StringVar(&manifestPath, "manifest", "", "The manifest file to read from. A valid manifest can be acquired by using the \"Download Manifest\" button in Data Explorer for Common portal")
	uploadMultipleCmd.MarkFlagRequired("manifest") //nolint:errcheck
	uploadMultipleCmd.Flags().StringVar(&uploadPath, "upload-path", "", "The directory in which contains files to be uploaded")
	uploadMultipleCmd.MarkFlagRequired("upload-path") //nolint:errcheck
	uploadMultipleCmd.Flags().BoolVar(&batch, "batch", true, "Upload in parallel")
	uploadMultipleCmd.Flags().IntVar(&numParallel, "numparallel", 3, "Number of uploads to run in parallel")
	uploadMultipleCmd.Flags().StringVar(&bucketName, "bucket", "", "The bucket to which files will be uploaded. If not provided, defaults to Gen3's configured DATA_UPLOAD_BUCKET.")
	uploadMultipleCmd.Flags().BoolVar(&forceMultipart, "force-multipart", false, "Force to use multipart upload when possible (file size >= 5MB)")
	uploadMultipleCmd.Flags().BoolVar(&includeSubDirName, "include-subdirname", true, "Include subdirectory names in file name")
	RootCmd.AddCommand(uploadMultipleCmd)
}

func processSingleUploads(g3i client.Gen3Interface, singleObjects []common.FileUploadRequestObject, bucketName string, includeSubDirName bool, uploadPath string) {
	for _, furObject := range singleObjects {
		filePath := furObject.FilePath
		file, err := os.Open(filePath)
		if err != nil {
			g3i.Logger().Failed(furObject.FilePath, furObject.Filename, furObject.FileMetadata, furObject.GUID, 0, false)
			log.Println("File open error: " + err.Error())
			continue
		}
		startSingleFileUpload(g3i, furObject, file, bucketName)
		file.Close()
	}
}

func startSingleFileUpload(g3i client.Gen3Interface, furObject common.FileUploadRequestObject, file *os.File, bucketName string) {

	fi, err := file.Stat()
	if err != nil {
		g3i.Logger().Failed(furObject.FilePath, furObject.Filename, furObject.FileMetadata, furObject.GUID, 0, false)
		g3i.Logger().Println("File stat error for file" + fi.Name() + ", file may be missing or unreadable because of permissions.\n")
		return
	}

	respURL, guid, err := GeneratePresignedURL(g3i, furObject.Filename, furObject.FileMetadata, bucketName)
	if err != nil {
		g3i.Logger().Failed(furObject.FilePath, furObject.Filename, furObject.FileMetadata, guid, 0, false)
		log.Println(err.Error())
		return
	}
	furObject.GUID = guid
	g3i.Logger().Failed(furObject.FilePath, furObject.Filename, furObject.FileMetadata, furObject.GUID, 0, false)
	furObject.PresignedURL = respURL

	furObject, err = GenerateUploadRequest(g3i, furObject, file, nil)
	if err != nil {
		file.Close()
		log.Printf("Error occurred during request generation: %s\n", err.Error())
		return
	}

	err = uploadFile(g3i, furObject, 0)
	if err != nil {
		log.Println(err.Error())
	} else {
		g3i.Logger().Scoreboard().IncrementSB(0)
	}

	file.Close()
}

func processMultipartUpload(g3i client.Gen3Interface, multipartObjects []common.FileUploadRequestObject, bucketName string, includeSubDirName bool, uploadPath string) error {
	cred := g3i.GetCredential()
	if cred.UseShepherd == "true" ||
		cred.UseShepherd == "" && common.DefaultUseShepherd == true {
		return fmt.Errorf("error: Shepherd currently does not support multipart uploads. For the moment, please disable Shepherd with\n    $ data-client configure --profile=%v --use-shepherd=false\nand try again", cred.Profile)
	}
	g3i.Logger().Println("Multipart uploading...")

	for _, furObject := range multipartObjects {
		// No more redundant ProcessFilename call!
		// Pass the complete FileUploadRequestObject to the streamlined multipartUpload.
		// Enable progress bar for batch uploads (interactive CLI use)
		err := MultipartUpload(context.Background(), g3i, furObject, bucketName, true)

		if err != nil {
			g3i.Logger().Println(err.Error())
		} else {
			g3i.Logger().Scoreboard().IncrementSB(0)
		}
	}
	return nil
}
