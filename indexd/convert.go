package indexd

// Conversion functions between drs.DRSObject and IndexdRecord

import (
	"fmt"
	"net/url"

	"github.com/calypr/data-client/drs"
)

// IndexdRecordFromDrsObject represents a simplified version of an indexd record for conversion purposes
func IndexdRecordFromDrsObject(drsObj *drs.DRSObject) (*IndexdRecord, error) {
	indexdObj := &IndexdRecord{
		Did:      drsObj.Id,
		Size:     drsObj.Size,
		FileName: drsObj.Name,
		URLs:     IndexdURLFromDrsAccessURLs(drsObj.AccessMethods),
		Authz:    IndexdAuthzFromDrsAccessMethods(drsObj.AccessMethods),
		Hashes:   drsObj.Checksums,
	}
	return indexdObj, nil
}

func IndexdRecordToDrsObject(indexdObj *IndexdRecord) (*drs.DRSObject, error) {
	accessMethods, err := DRSAccessMethodsFromIndexdURLs(indexdObj.URLs, indexdObj.Authz)
	if err != nil {
		return nil, err
	}
	for _, am := range accessMethods {
		if am.Authorizations == nil || len(am.Authorizations.BearerAuthIssuers) == 0 {
			return nil, fmt.Errorf("access method missing authorization %v, %v", indexdObj, indexdObj.Authz)
		}
	}

	return &drs.DRSObject{
		Id:            indexdObj.Did,
		Size:          indexdObj.Size,
		Name:          indexdObj.FileName,
		AccessMethods: accessMethods,
		Checksums:     indexdObj.Hashes,
	}, nil
}

func DRSAccessMethodsFromIndexdURLs(urls []string, authz []string) ([]drs.AccessMethod, error) {
	var accessMethods []drs.AccessMethod
	for _, urlString := range urls {
		var method drs.AccessMethod
		method.AccessURL = drs.AccessURL{URL: urlString}

		parsed, err := url.Parse(urlString)
		if err != nil {
			return nil, fmt.Errorf("failed to parse url %q: %v", urlString, err)
		}
		if parsed.Scheme == "" {
			// default to https if no scheme or parse error
			method.Type = "https"
		} else {
			method.Type = parsed.Scheme
		}

		// check if authz is null or 0-length, then error
		if authz == nil {
			return nil, fmt.Errorf("authz is required")
		}

		// NOTE: a record can only have 1 authz entry atm
		method.Authorizations = &drs.Authorizations{BearerAuthIssuers: []string{authz[0]}}
		accessMethods = append(accessMethods, method)
	}
	return accessMethods, nil
}

// IndexdAuthzFromDrsAccessMethods extracts authz values from DRS access methods
func IndexdAuthzFromDrsAccessMethods(accessMethods []drs.AccessMethod) []string {
	var authz []string
	for _, drsURL := range accessMethods {
		if drsURL.Authorizations != nil && len(drsURL.Authorizations.BearerAuthIssuers) > 0 {
			authz = append(authz, drsURL.Authorizations.BearerAuthIssuers[0])
		}
	}
	return authz
}

func IndexdURLFromDrsAccessURLs(accessMethods []drs.AccessMethod) []string {
	var urls []string
	for _, drsURL := range accessMethods {
		urls = append(urls, drsURL.AccessURL.URL)
	}
	return urls
}

func (inr *IndexdRecord) ToDrsObject() (*drs.DRSObject, error) {
	o, err := IndexdRecordToDrsObject(inr)
	if err != nil {
		return nil, err
	}
	return o, nil
}
