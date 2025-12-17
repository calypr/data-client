package g3cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/calypr/data-client/client/common"
	client "github.com/calypr/data-client/client/gen3Client"
	"github.com/calypr/data-client/client/logs"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"

	"github.com/spf13/cobra"
)

// mockgen -destination=../mocks/mock_gen3interface.go -package=mocks . Gen3Interface

func AskGen3ForFileInfo(g3i client.Gen3Interface, guid string, protocol string, downloadPath string, filenameFormat string, rename bool, renamedFiles *[]RenamedOrSkippedFileInfo) (string, int64) {
	var fileName string
	var fileSize int64

	// If the commons has the newer Shepherd API deployed, get the filename and file size from the Shepherd API.
	// Otherwise, fall back on Indexd and Fence.
	hasShepherd, err := g3i.CheckForShepherdAPI()
	if err != nil {
		g3i.Logger().Println("Error occurred when checking for Shepherd API: " + err.Error())
		g3i.Logger().Println("Falling back to Indexd...")
	}
	if hasShepherd {
		endPointPostfix := common.ShepherdEndpoint + "/objects/" + guid
		_, res, err := g3i.GetResponse(endPointPostfix, "GET", "", nil)
		if err != nil {
			g3i.Logger().Println("Error occurred when querying filename from Shepherd: " + err.Error())
			g3i.Logger().Println("Using GUID for filename instead.")
			if filenameFormat != "guid" {
				*renamedFiles = append(*renamedFiles, RenamedOrSkippedFileInfo{GUID: guid, OldFilename: "N/A", NewFilename: guid})
			}
			return guid, 0
		}

		decoded := struct {
			Record struct {
				FileName string `json:"file_name"`
				Size     int64  `json:"size"`
			}
		}{}
		err = json.NewDecoder(res.Body).Decode(&decoded)
		if err != nil {
			g3i.Logger().Println("Error occurred when reading response from Shepherd: " + err.Error())
			g3i.Logger().Println("Using GUID for filename instead.")
			if filenameFormat != "guid" {
				*renamedFiles = append(*renamedFiles, RenamedOrSkippedFileInfo{GUID: guid, OldFilename: "N/A", NewFilename: guid})
			}
			return guid, 0
		}
		defer res.Body.Close()

		fileName = decoded.Record.FileName
		fileSize = decoded.Record.Size

	} else {
		// Attempt to get the filename from Indexd
		endPointPostfix := common.IndexdIndexEndpoint + "/" + guid
		indexdMsg, err := g3i.DoRequestWithSignedHeader(endPointPostfix, "", nil)
		if err != nil {
			g3i.Logger().Println("Error occurred when querying filename from IndexD: " + err.Error())
			g3i.Logger().Println("Using GUID for filename instead.")
			if filenameFormat != "guid" {
				*renamedFiles = append(*renamedFiles, RenamedOrSkippedFileInfo{GUID: guid, OldFilename: "N/A", NewFilename: guid})
			}
			return guid, 0
		}

		if filenameFormat == "guid" {
			return guid, indexdMsg.Size
		}

		actualFilename := indexdMsg.FileName
		if actualFilename == "" {
			if len(indexdMsg.URLs) > 0 {
				// Indexd record has no file name but does have URLs, try to guess file name from URL
				var indexdURL = indexdMsg.URLs[0]
				if protocol != "" {
					for _, url := range indexdMsg.URLs {
						if strings.HasPrefix(url, protocol) {
							indexdURL = url
						}
					}
				}

				actualFilename = guessFilenameFromURL(indexdURL)
				if actualFilename == "" {
					g3i.Logger().Println("Error occurred when guessing filename for object " + guid)
					g3i.Logger().Println("Using GUID for filename instead.")
					*renamedFiles = append(*renamedFiles, RenamedOrSkippedFileInfo{GUID: guid, OldFilename: "N/A", NewFilename: guid})
					return guid, indexdMsg.Size
				}
			} else {
				// Neither file name nor URLs exist in the Indexd record
				// Indexd record is busted for that file, just return as we are renaming the file for now
				// The download logic will handle the errors
				g3i.Logger().Println("Neither file name nor URLs exist in the Indexd record of " + guid)
				g3i.Logger().Println("The attempt of downloading file is likely to fail! Check Indexd record!")
				g3i.Logger().Println("Using GUID for filename instead.")
				*renamedFiles = append(*renamedFiles, RenamedOrSkippedFileInfo{GUID: guid, OldFilename: "N/A", NewFilename: guid})
				return guid, indexdMsg.Size
			}
		}

		fileName = actualFilename
		fileSize = indexdMsg.Size
	}

	if filenameFormat == "original" {
		if !rename { // no renaming in original mode
			return fileName, fileSize
		}
		newFilename := processOriginalFilename(downloadPath, fileName)
		if fileName != newFilename {
			*renamedFiles = append(*renamedFiles, RenamedOrSkippedFileInfo{GUID: guid, OldFilename: fileName, NewFilename: newFilename})
		}
		return newFilename, fileSize
	}
	// filenameFormat == "combined"
	combinedFilename := guid + "_" + fileName
	return combinedFilename, fileSize
}

func guessFilenameFromURL(URL string) string {
	splittedURLWithFilename := strings.Split(URL, "/")
	actualFilename := splittedURLWithFilename[len(splittedURLWithFilename)-1]
	return actualFilename
}

func processOriginalFilename(downloadPath string, actualFilename string) string {
	_, err := os.Stat(downloadPath + actualFilename)
	if os.IsNotExist(err) {
		return actualFilename
	}
	extension := filepath.Ext(actualFilename)
	filename := strings.TrimSuffix(actualFilename, extension)
	counter := 2
	for {
		newFilename := filename + "_" + strconv.Itoa(counter) + extension
		_, err := os.Stat(downloadPath + newFilename)
		if os.IsNotExist(err) {
			return newFilename
		}
		counter++
	}
}

func validateLocalFileStat(logger logs.Logger, downloadPath string, filename string, filesize int64, skipCompleted bool) common.FileDownloadResponseObject {
	fi, err := os.Stat(downloadPath + filename) // check filename for local existence
	if err != nil {
		if os.IsNotExist(err) {
			return common.FileDownloadResponseObject{DownloadPath: downloadPath, Filename: filename} // no local file, normal full length download
		}
		logger.Printf("Error occurred when getting information for file \"%s\": %s\n", downloadPath+filename, err.Error())
		logger.Println("Will try to download the whole file")
		return common.FileDownloadResponseObject{DownloadPath: downloadPath, Filename: filename} // errorred when trying to get local FI, normal full length download
	}

	// have existing local file and may want to skip, check more conditions
	if !skipCompleted {
		return common.FileDownloadResponseObject{DownloadPath: downloadPath, Filename: filename, Overwrite: true} // not skipping any local files, normal full length download
	}

	localFilesize := fi.Size()
	if localFilesize == filesize {
		return common.FileDownloadResponseObject{DownloadPath: downloadPath, Filename: filename, Skip: true} // both filename and filesize matches, consider as completed
	}
	if localFilesize > filesize {
		return common.FileDownloadResponseObject{DownloadPath: downloadPath, Filename: filename, Overwrite: true} // local filesize is greater than INDEXD record, overwrite local existing
	}
	// local filesize is less than INDEXD record, try ranged download
	return common.FileDownloadResponseObject{DownloadPath: downloadPath, Filename: filename, Range: localFilesize}
}

func batchDownload(g3 client.Gen3Interface, progress *mpb.Progress, batchFDRSlice []common.FileDownloadResponseObject, protocolText string, workers int, errCh chan error) int {
	fdrs := make([]common.FileDownloadResponseObject, 0)
	for _, fdrObject := range batchFDRSlice {
		err := GetDownloadResponse(g3, &fdrObject, protocolText)
		if err != nil {
			errCh <- err
			continue
		}

		fileFlag := os.O_CREATE | os.O_RDWR
		if fdrObject.Range != 0 {
			fileFlag = os.O_APPEND | os.O_RDWR
		} else if fdrObject.Overwrite {
			fileFlag = os.O_TRUNC | os.O_RDWR
		}

		subDir := filepath.Dir(fdrObject.Filename)
		if subDir != "." && subDir != "/" {
			err = os.MkdirAll(fdrObject.DownloadPath+subDir, 0766)
			if err != nil {
				errCh <- err
				continue
			}
		}
		file, err := os.OpenFile(fdrObject.DownloadPath+fdrObject.Filename, fileFlag, 0666)
		if err != nil {
			errCh <- errors.New("Error occurred during opening local file: " + err.Error())
			continue
		}
		total := fdrObject.Response.ContentLength + fdrObject.Range
		bar := progress.AddBar(total,
			mpb.PrependDecorators(
				decor.Name(fdrObject.Filename+" "),
				decor.CountersKibiByte("% .1f / % .1f"),
			),
			mpb.AppendDecorators(
				decor.Percentage(),
				decor.AverageSpeed(decor.SizeB1024(0), " % .1f"),
			),
		)
		if fdrObject.Range > 0 {
			bar.SetCurrent(fdrObject.Range)
		}
		writer := bar.ProxyWriter(file)
		fdrObject.Writer = writer
		fdrs = append(fdrs, fdrObject)
		defer file.Close()
		defer fdrObject.Response.Body.Close()
	}

	fdrCh := make(chan common.FileDownloadResponseObject, len(fdrs))
	wg := sync.WaitGroup{}
	succeeded := 0
	var err error
	for range workers {
		wg.Add(1)
		go func() {
			for fdr := range fdrCh {
				if _, err = io.Copy(fdr.Writer, fdr.Response.Body); err != nil {
					errCh <- errors.New("io.Copy error: " + err.Error())
					return
				}
				succeeded++
			}
			wg.Done()
		}()
	}

	for _, fdr := range fdrs {
		fdrCh <- fdr
	}
	close(fdrCh)

	wg.Wait()
	return succeeded
}

// AskForConfirmation asks user for confirmation before proceed, will wait if user entered garbage
func AskForConfirmation(logger logs.Logger, s string) bool {
	reader := bufio.NewReader(os.Stdin)

	for {
		logger.Printf("%s [y/n]: ", s)

		response, err := reader.ReadString('\n')
		if err != nil {
			logger.Fatal("Error occurred during parsing user's confirmation: " + err.Error())
		}

		switch strings.ToLower(strings.TrimSpace(response)) {
		case "y", "yes":
			return true
		case "n", "no":
			return false
		default:
			return false // Example of defaulting to false
		}
	}
}

func downloadFile(g3i client.Gen3Interface, objects []ManifestObject, downloadPath string, filenameFormat string, rename bool, noPrompt bool, protocol string, numParallel int, skipCompleted bool) error {
	if numParallel < 1 {
		return fmt.Errorf("invalid value for option \"numparallel\": must be a positive integer! Please check your input")
	}

	downloadPath, err := common.ParseRootPath(downloadPath)
	if err != nil {
		return fmt.Errorf("downloadFile Error: %s", err.Error())
	}
	if !strings.HasSuffix(downloadPath, "/") {
		downloadPath += "/"
	}
	filenameFormat = strings.ToLower(strings.TrimSpace(filenameFormat))
	if (filenameFormat == "guid" || filenameFormat == "combined") && rename {
		g3i.Logger().Println("NOTICE: flag \"rename\" only works if flag \"filename-format\" is \"original\"")
		rename = false
	}

	if filenameFormat != "original" && filenameFormat != "guid" && filenameFormat != "combined" {
		return fmt.Errorf("invalid option found! option \"filename-format\" can either be \"original\", \"guid\" or \"combined\" only")
	}
	if filenameFormat == "guid" || filenameFormat == "combined" {
		g3i.Logger().Printf("WARNING: in \"guid\" or \"combined\" mode, duplicated files under \"%s\" will be overwritten\n", downloadPath)
		if !noPrompt && !AskForConfirmation(g3i.Logger(), "Proceed?") {
			g3i.Logger().Fatal("Aborted by user")
		}
	} else if !rename {
		g3i.Logger().Printf("WARNING: flag \"rename\" was set to false in \"original\" mode, duplicated files under \"%s\" will be overwritten\n", downloadPath)
		if !noPrompt && !AskForConfirmation(g3i.Logger(), "Proceed?") {
			g3i.Logger().Fatal("Aborted by user")
		}
	} else {
		g3i.Logger().Printf("NOTICE: flag \"rename\" was set to true in \"original\" mode, duplicated files under \"%s\" will be renamed by appending a counter value to the original filenames\n", downloadPath)
	}

	protocolText := ""
	if protocol != "" {
		protocolText = "?protocol=" + protocol
	}

	err = os.MkdirAll(downloadPath, 0766)
	if err != nil {
		return fmt.Errorf("cannot create folder %s", downloadPath)
	}

	renamedFiles := make([]RenamedOrSkippedFileInfo, 0)
	skippedFiles := make([]RenamedOrSkippedFileInfo, 0)
	fdrObjects := make([]common.FileDownloadResponseObject, 0)

	g3i.Logger().Printf("Total number of objects in manifest: %d\n", len(objects))
	g3i.Logger().Println("Preparing file info for each file, please wait...")
	fileInfoProgress := mpb.New(mpb.WithOutput(os.Stdout))
	fileInfoBar := fileInfoProgress.AddBar(int64(len(objects)),
		mpb.PrependDecorators(
			decor.Name("Preparing files "),
			decor.CountersNoUnit("%d / %d"),
		),
		mpb.AppendDecorators(decor.Percentage()),
	)
	for _, obj := range objects {
		if obj.ObjectID == "" {
			g3i.Logger().Println("Found empty object_id (GUID), skipping this entry")
			continue
		}
		var fdrObject common.FileDownloadResponseObject
		filename := obj.Filename
		filesize := obj.Filesize
		// only queries Gen3 services if any of these 2 values doesn't exists in manifest
		if filename == "" || filesize == 0 {
			filename, filesize = AskGen3ForFileInfo(g3i, obj.ObjectID, protocol, downloadPath, filenameFormat, rename, &renamedFiles)
		}
		fdrObject = common.FileDownloadResponseObject{DownloadPath: downloadPath, Filename: filename}
		if !rename {
			fdrObject = validateLocalFileStat(g3i.Logger(), downloadPath, filename, filesize, skipCompleted)
		}
		fdrObject.GUID = obj.ObjectID
		fdrObjects = append(fdrObjects, fdrObject)
		fileInfoBar.Increment()
	}
	fileInfoProgress.Wait()
	g3i.Logger().Println("File info prepared successfully")

	totalCompeleted := 0
	workers, _, errCh, _ := initBatchUploadChannels(numParallel, len(fdrObjects))
	downloadProgress := mpb.New(mpb.WithOutput(os.Stdout))
	batchFDRSlice := make([]common.FileDownloadResponseObject, 0)
	for _, fdrObject := range fdrObjects {
		if fdrObject.Skip {
			g3i.Logger().Printf("File \"%s\" (GUID: %s) has been skipped because there is a complete local copy\n", fdrObject.Filename, fdrObject.GUID)
			skippedFiles = append(skippedFiles, RenamedOrSkippedFileInfo{GUID: fdrObject.GUID, OldFilename: fdrObject.Filename})
			continue
		}

		if len(batchFDRSlice) < workers {
			batchFDRSlice = append(batchFDRSlice, fdrObject)
		} else {
			totalCompeleted += batchDownload(g3i, downloadProgress, batchFDRSlice, protocolText, workers, errCh)
			batchFDRSlice = make([]common.FileDownloadResponseObject, 0)
			batchFDRSlice = append(batchFDRSlice, fdrObject)
		}
	}
	totalCompeleted += batchDownload(g3i, downloadProgress, batchFDRSlice, protocolText, workers, errCh) // download remainders
	downloadProgress.Wait()

	g3i.Logger().Printf("%d files downloaded.\n", totalCompeleted)

	if len(renamedFiles) > 0 {
		g3i.Logger().Printf("%d files have been renamed as the following:\n", len(renamedFiles))
		for _, rfi := range renamedFiles {
			g3i.Logger().Printf("File \"%s\" (GUID: %s) has been renamed as: %s\n", rfi.OldFilename, rfi.GUID, rfi.NewFilename)
		}
	}
	if len(skippedFiles) > 0 {
		g3i.Logger().Printf("%d files have been skipped\n", len(skippedFiles))
	}
	if len(errCh) > 0 {
		close(errCh)
		g3i.Logger().Printf("%d files have encountered an error during downloading, detailed error messages are:\n", len(errCh))
		for err := range errCh {
			g3i.Logger().Println(err.Error())
		}
	}
	return nil
}

func init() {
	var manifestPath string
	var downloadPath string
	var filenameFormat string
	var rename bool
	var noPrompt bool
	var protocol string
	var numParallel int
	var skipCompleted bool

	var downloadMultipleCmd = &cobra.Command{
		Use:     "download-multiple",
		Short:   "Download multiple of files from a specified manifest",
		Long:    `Get presigned URLs for multiple of files specified in a manifest file and then download all of them.`,
		Example: `./data-client download-multiple --profile=<profile-name> --manifest=<path-to-manifest/manifest.json> --download-path=<path-to-file-dir/>`,
		Run: func(cmd *cobra.Command, args []string) {
			// don't initialize transmission logs for non-uploading related commands

			logger, logCloser := logs.New(profile, logs.WithConsole(), logs.WithFailedLog(), logs.WithScoreboard(), logs.WithSucceededLog())
			defer logCloser()

			g3i, err := client.NewGen3Interface(context.Background(), profile, logger)
			if err != nil {
				log.Fatalf("Failed to parse config on profile %s, %v", profile, err)
			}

			manifestPath, _ = common.GetAbsolutePath(manifestPath)
			manifestFile, err := os.Open(manifestPath)
			if err != nil {
				g3i.Logger().Fatalf("Failed to open manifest file %s, %v\n", manifestPath, err)
			}
			defer manifestFile.Close()
			manifestFileStat, err := manifestFile.Stat()
			if err != nil {
				g3i.Logger().Fatalf("Failed to get manifest file stats %s, %v\n", manifestPath, err)
			}
			g3i.Logger().Println("Reading manifest...")
			manifestFileSize := manifestFileStat.Size()
			manifestProgress := mpb.New(mpb.WithOutput(os.Stdout))
			manifestFileBar := manifestProgress.AddBar(manifestFileSize,
				mpb.PrependDecorators(
					decor.Name("Manifest "),
					decor.CountersKibiByte("% .1f / % .1f"),
				),
				mpb.AppendDecorators(decor.Percentage()),
			)

			manifestFileReader := manifestFileBar.ProxyReader(manifestFile)

			manifestBytes, err := io.ReadAll(manifestFileReader)
			if err != nil {
				g3i.Logger().Fatalf("Failed reading manifest %s, %v\n", manifestPath, err)
			}
			manifestProgress.Wait()

			var objects []ManifestObject
			err = json.Unmarshal(manifestBytes, &objects)
			if err != nil {
				g3i.Logger().Fatalf("Error has occurred during unmarshalling manifest object: %v\n", err)
			}

			err = downloadFile(g3i, objects, downloadPath, filenameFormat, rename, noPrompt, protocol, numParallel, skipCompleted)
			if err != nil {
				g3i.Logger().Fatal(err.Error())
			}
		},
	}

	downloadMultipleCmd.Flags().StringVar(&profile, "profile", "", "Specify profile to use")
	downloadMultipleCmd.MarkFlagRequired("profile") //nolint:errcheck
	downloadMultipleCmd.Flags().StringVar(&manifestPath, "manifest", "", "The manifest file to read from. A valid manifest can be acquired by using the \"Download Manifest\" button in Data Explorer from a data common's portal")
	downloadMultipleCmd.MarkFlagRequired("manifest") //nolint:errcheck
	downloadMultipleCmd.Flags().StringVar(&downloadPath, "download-path", ".", "The directory in which to store the downloaded files")
	downloadMultipleCmd.Flags().StringVar(&filenameFormat, "filename-format", "original", "The format of filename to be used, including \"original\", \"guid\" and \"combined\"")
	downloadMultipleCmd.Flags().BoolVar(&rename, "rename", false, "Only useful when \"--filename-format=original\", will rename file by appending a counter value to its filename if set to true, otherwise the same filename will be used")
	downloadMultipleCmd.Flags().BoolVar(&noPrompt, "no-prompt", false, "If set to true, will not display user prompt message for confirmation")
	downloadMultipleCmd.Flags().StringVar(&protocol, "protocol", "", "Specify the preferred protocol with --protocol=s3")
	downloadMultipleCmd.Flags().IntVar(&numParallel, "numparallel", 1, "Number of downloads to run in parallel")
	downloadMultipleCmd.Flags().BoolVar(&skipCompleted, "skip-completed", false, "If set to true, will check for filename and size before download and skip any files in \"download-path\" that matches both")
	RootCmd.AddCommand(downloadMultipleCmd)
}
