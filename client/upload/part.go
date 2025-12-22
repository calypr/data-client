package upload

import (
	"fmt"
	"io"
	"net/http"
)

// Upload single part via presigned URL
func uploadPart(url string, data io.Reader, etagOut *string) error {
	req, err := http.NewRequest(http.MethodPut, url, data)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	*etagOut = resp.Header.Get("ETag")
	if *etagOut == "" {
		return fmt.Errorf("no ETag in response")
	}
	return nil
}
