package download

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"fmt"
	"log/slog"
)

// AskForConfirmation asks user for confirmation before proceed, will wait if user entered garbage
func AskForConfirmation(logger *slog.Logger, s string) bool {
	reader := bufio.NewReader(os.Stdin)

	for {
		logger.Info(fmt.Sprintf("%s [y/n]: ", s))

		response, err := reader.ReadString('\n')
		if err != nil {
			logger.Error("Error occurred during parsing user's confirmation: " + err.Error())
			os.Exit(1)
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

// truncateFilename shortens long filenames for progress bar display
func truncateFilename(name string, max int) string {
	if len(name) <= max {
		return name
	}
	return "..." + name[len(name)-max+3:]
}

// printRenamed shows renamed files in final summary
func printRenamed(logger *slog.Logger, renamed []RenamedOrSkippedFileInfo) {
	if len(renamed) == 0 {
		return
	}
	logger.Info(fmt.Sprintf("%d files renamed:", len(renamed)))
	for _, r := range renamed {
		logger.Info(fmt.Sprintf("  %q (GUID: %s) → %q", r.OldFilename, r.GUID, r.NewFilename))
	}
}

// printSkipped shows skipped files in final summary
func printSkipped(logger *slog.Logger, skipped []RenamedOrSkippedFileInfo) {
	if len(skipped) == 0 {
		return
	}
	logger.Info(fmt.Sprintf("%d files skipped (complete local copy exists)", len(skipped)))
}
