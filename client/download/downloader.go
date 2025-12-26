package download

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/calypr/data-client/client/client"
	"github.com/calypr/data-client/client/common"
	"github.com/calypr/data-client/client/logs"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

// DownloadMultiple is the public entry point called from g3cmd
func DownloadMultiple(
	ctx context.Context,
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
	logger := g3i.Logger()

	// === Input validation ===
	if numParallel < 1 {
		return fmt.Errorf("numparallel must be a positive integer")
	}

	var err error
	downloadPath, err = common.ParseRootPath(downloadPath)
	if err != nil {
		return fmt.Errorf("invalid download path: %w", err)
	}
	if !strings.HasSuffix(downloadPath, "/") {
		downloadPath += "/"
	}

	filenameFormat = strings.ToLower(strings.TrimSpace(filenameFormat))
	if filenameFormat != "original" && filenameFormat != "guid" && filenameFormat != "combined" {
		return fmt.Errorf("filename-format must be one of: original, guid, combined")
	}
	if (filenameFormat == "guid" || filenameFormat == "combined") && rename {
		logger.Println("NOTICE: rename flag is ignored in guid/combined mode")
		rename = false
	}

	// === Warnings and user confirmation ===
	if err := handleWarningsAndConfirmation(logger, downloadPath, filenameFormat, rename, noPrompt); err != nil {
		return err // aborted by user
	}

	// === Create download directory ===
	if err := os.MkdirAll(downloadPath, 0766); err != nil {
		return fmt.Errorf("cannot create directory %s: %w", downloadPath, err)
	}

	// === Prepare files (metadata + local validation) ===
	toDownload, skipped, renamed, err := prepareFiles(ctx, g3i, objects, downloadPath, filenameFormat, rename, skipCompleted, protocol)
	if err != nil {
		return err
	}

	logger.Printf("Total objects: %d | To download: %d | Skipped: %d\n",
		len(objects), len(toDownload), len(skipped))

	// === Download phase ===
	downloaded, downloadErr := downloadFiles(ctx, g3i, toDownload, numParallel, protocol)

	// === Final summary ===
	logger.Printf("%d files downloaded successfully.\n", downloaded)
	printRenamed(logger, renamed)
	printSkipped(logger, skipped)

	if downloadErr != nil {
		logger.Printf("Some downloads failed. See errors above.\n")
	}

	return nil // we log failures but don't fail the whole command unless critical
}

// handleWarningsAndConfirmation prints warnings and asks for confirmation if needed
func handleWarningsAndConfirmation(logger logs.Logger, downloadPath, filenameFormat string, rename, noPrompt bool) error {
	if filenameFormat == "guid" || filenameFormat == "combined" {
		logger.Printf("WARNING: in %q mode, duplicate files in %q will be overwritten\n", filenameFormat, downloadPath)
	} else if !rename {
		logger.Printf("WARNING: rename=false in original mode – duplicates in %q will be overwritten\n", downloadPath)
	} else {
		logger.Printf("NOTICE: rename=true in original mode – duplicates in %q will be renamed with a counter\n", downloadPath)
	}

	if noPrompt {
		return nil
	}
	if !AskForConfirmation(logger, "Proceed? (y/N)") {
		logger.Fatal("Aborted by user")
	}
	return nil
}

// prepareFiles gathers metadata, checks local files, collects skips/renames
func prepareFiles(
	ctx context.Context,
	g3i client.Gen3Interface,
	objects []common.ManifestObject,
	downloadPath, filenameFormat string,
	rename, skipCompleted bool,
	protocol string,
) ([]common.FileDownloadResponseObject, []RenamedOrSkippedFileInfo, []RenamedOrSkippedFileInfo, error) {
	logger := g3i.Logger()
	renamed := make([]RenamedOrSkippedFileInfo, 0)
	skipped := make([]RenamedOrSkippedFileInfo, 0)
	toDownload := make([]common.FileDownloadResponseObject, 0, len(objects))

	p := mpb.New(mpb.WithOutput(os.Stdout))
	bar := p.AddBar(int64(len(objects)),
		mpb.PrependDecorators(decor.Name("Preparing "), decor.CountersNoUnit("%d / %d")),
		mpb.AppendDecorators(decor.Percentage()),
	)

	for _, obj := range objects {
		if obj.ObjectID == "" {
			logger.Println("Empty GUID, skipping entry")
			bar.Increment()
			continue
		}

		info := &IndexdResponse{Name: obj.Title, Size: obj.Size}
		var err error
		if info.Name == "" || info.Size == 0 {
			// Very strict object id checking
			info, err = AskGen3ForFileInfo(ctx, g3i, obj.ObjectID, protocol, downloadPath, filenameFormat, rename, &renamed)
			if err != nil {
				return nil, nil, nil, err
			}
		}

		fdr := common.FileDownloadResponseObject{
			DownloadPath: downloadPath,
			Filename:     info.Name,
			GUID:         obj.ObjectID,
		}

		if !rename {
			validateLocalFileStat(logger, &fdr, int64(info.Size), skipCompleted)
		}

		if fdr.Skip {
			logger.Printf("Skipping %q (GUID: %s) – complete local copy exists\n", fdr.Filename, fdr.GUID)
			skipped = append(skipped, RenamedOrSkippedFileInfo{GUID: fdr.GUID, OldFilename: fdr.Filename})
		} else {
			toDownload = append(toDownload, fdr)
		}

		bar.Increment()
	}
	p.Wait()
	logger.Println("Preparation complete")
	return toDownload, skipped, renamed, nil
}
