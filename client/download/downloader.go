package download

import (
	"fmt"
	"os"
	"strings"

	"github.com/calypr/data-client/client/common"
	client "github.com/calypr/data-client/client/gen3Client"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

// DownloadMultiple is the public entry point called from g3cmd
func DownloadMultiple(
	g3i client.Gen3Interface,
	objects []common.ManifestObject,
	downloadPath string,
	filenameFormat string,
	rename bool,
	noPrompt bool,
	protocol string,
	numParallel int,
	skipCompleted bool,
) error {
	// === Input validation ===
	if numParallel < 1 {
		return fmt.Errorf("invalid value for option \"numparallel\": must be a positive integer! Please check your input")
	}

	var err error
	downloadPath, err = common.ParseRootPath(downloadPath)
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

	// === Warnings and user confirmation ===
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

	// === Setup ===
	protocolText := ""
	if protocol != "" {
		protocolText = "?protocol=" + protocol
	}

	if err := os.MkdirAll(downloadPath, 0766); err != nil {
		return fmt.Errorf("cannot create folder %s", downloadPath)
	}

	// === Prepare phase ===
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
			fileInfoBar.Increment()
			continue
		}

		filename := obj.Filename
		filesize := obj.Filesize

		// Query Gen3 only if filename or size missing in manifest
		if filename == "" || filesize == 0 {
			filename, filesize = AskGen3ForFileInfo(g3i, obj.ObjectID, protocol, downloadPath, filenameFormat, rename, &renamedFiles)
		}

		fdr := common.FileDownloadResponseObject{
			DownloadPath: downloadPath,
			Filename:     filename,
			GUID:         obj.ObjectID,
		}

		// Only validate local file if we're not renaming (rename mode always downloads)
		if !rename {
			fdr = validateLocalFileStat(g3i.Logger(), downloadPath, filename, filesize, skipCompleted)
		}

		if fdr.Skip {
			g3i.Logger().Printf("File \"%s\" (GUID: %s) has been skipped because there is a complete local copy\n", fdr.Filename, fdr.GUID)
			skippedFiles = append(skippedFiles, RenamedOrSkippedFileInfo{
				GUID:        fdr.GUID,
				OldFilename: fdr.Filename,
			})
		} else {
			fdrObjects = append(fdrObjects, fdr)
		}

		fileInfoBar.Increment()
	}
	fileInfoProgress.Wait()
	g3i.Logger().Println("File info prepared successfully")

	// === Download phase ===
	totalCompleted := 0
	workers := numParallel
	if workers > len(fdrObjects) {
		workers = len(fdrObjects)
	}
	if workers < 1 {
		workers = 1
	}

	errCh := make(chan error, len(fdrObjects))

	downloadProgress := mpb.New(mpb.WithOutput(os.Stdout))
	batch := make([]common.FileDownloadResponseObject, 0, workers)

	for _, fdr := range fdrObjects {
		batch = append(batch, fdr)
		if len(batch) == workers {
			totalCompleted += batchDownload(g3i, downloadProgress, batch, protocolText, workers, errCh)
			batch = batch[:0] // reset batch
		}
	}
	// Download any remaining files
	if len(batch) > 0 {
		totalCompleted += batchDownload(g3i, downloadProgress, batch, protocolText, workers, errCh)
	}
	downloadProgress.Wait()

	// === Final summary ===
	g3i.Logger().Printf("%d files downloaded.\n", totalCompleted)

	if len(renamedFiles) > 0 {
		g3i.Logger().Printf("%d files have been renamed as the following:\n", len(renamedFiles))
		for _, rfi := range renamedFiles {
			g3i.Logger().Printf("File \"%s\" (GUID: %s) has been renamed as: %s\n", rfi.OldFilename, rfi.GUID, rfi.NewFilename)
		}
	}

	if len(skippedFiles) > 0 {
		g3i.Logger().Printf("%d files have been skipped\n", len(skippedFiles))
	}

	var failures []error
	if len(errCh) > 0 {
		close(errCh)
		g3i.Logger().Printf("%d files have encountered an error during downloading, detailed error messages are:\n", len(errCh))
		for err := range errCh {
			g3i.Logger().Println(err.Error())
			failures = append(failures, err)
		}
	}

	return nil
}
