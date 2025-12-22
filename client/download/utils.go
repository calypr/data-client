package download

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/calypr/data-client/client/logs"
)

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
