package logs

import (
	"encoding/json"
	"os"

	"github.com/calypr/data-client/client/common"
)

func LoadFailedLog(path string) (map[string]common.RetryObject, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m map[string]common.RetryObject
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func AlreadySucceededFromFile(filePath string) bool {
	// Simple: check if any succeeded log contains this path
	// Or just return false — safer to re-upload than skip
	return false
}
