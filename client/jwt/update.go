package jwt

import (
	"fmt"
	"log"
	"strings"

	"github.com/calypr/data-client/client/commonUtils"
	"github.com/calypr/data-client/client/logs"
	"github.com/hashicorp/go-version"
)

func UpdateConfig(cred *Credential) error {

	var conf Configure
	var req Request

	cred.APIEndpoint = strings.TrimSpace(cred.APIEndpoint)
	if cred.APIEndpoint[len(cred.APIEndpoint)-1:] == "/" {
		cred.APIEndpoint = cred.APIEndpoint[:len(cred.APIEndpoint)-1]
	}
	parsedURL, err := conf.ValidateUrl(cred.APIEndpoint)
	if err != nil {
		return fmt.Errorf("Errr occurred when validating apiendpoint URL: %s", err.Error())
	}

	prefixEndPoint := parsedURL.Scheme + "://" + parsedURL.Host
	fileCredential, err := conf.ParseConfig(cred.Profile)
	if err != nil {
		return err
	}
	if cred.APIKey == "" {
		cred.APIKey = fileCredential.APIKey
	}
	if cred.AccessToken == "" {
		err = req.RequestNewAccessToken(prefixEndPoint+commonUtils.FenceAccessTokenEndpoint, cred)
		if err != nil {
			receivedErrorString := err.Error()
			errorMessageString := receivedErrorString
			if strings.Contains(receivedErrorString, "401") {
				errorMessageString = `Invalid credentials for apiendpoint '` + prefixEndPoint + `': check if your credentials are expired or incorrect`
			} else if strings.Contains(receivedErrorString, "404") || strings.Contains(receivedErrorString, "405") || strings.Contains(receivedErrorString, "no such host") {
				errorMessageString = `The provided apiendpoint '` + prefixEndPoint + `' is possibly not a valid Gen3 data commons`
			}
			return fmt.Errorf("Error occurred when validating profile config: %s", errorMessageString)
		}
	}

	cred.UseShepherd = strings.TrimSpace(cred.UseShepherd)
	cred.MinShepherdVersion = strings.TrimSpace(cred.MinShepherdVersion)
	if cred.MinShepherdVersion != "" {
		_, err = version.NewVersion(cred.MinShepherdVersion)
		if err != nil {
			return fmt.Errorf("Error occurred when validating minShepherdVersion: %s", err.Error())
		}
	}

	// Store user info in ~/.gen3/gen3_client_config.ini
	err = conf.UpdateConfigFile(*cred)
	if err != nil {
		return err
	}
	log.Println(`Profile '` + cred.Profile + `' has been configured successfully.`)
	err = logs.CloseMessageLog()
	if err != nil {
		log.Println(err.Error())
		return err
	}
	return nil

}
