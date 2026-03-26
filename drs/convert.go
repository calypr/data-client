package drs

import (
	"fmt"
	"net/url"

	"github.com/calypr/data-client/apigen/internalapi"
	"github.com/calypr/data-client/hash"
)

// InternalRecordFromDrsObject conversion purposes
func InternalRecordFromDrsObject(drsObj *DRSObject) (*InternalRecord, error) {
	hashesMap := hash.ConvertDrsChecksumsToMap(drsObj.Checksums)

	indexdObj := &InternalRecord{
		InternalRecord: internalapi.InternalRecord{
			Did:      internalapi.PtrString(drsObj.Id),
			Size:     internalapi.PtrInt64(drsObj.Size),
			FileName: drsObj.Name,
			Urls:     InternalURLFromDrsAccessURLs(drsObj.AccessMethods),
			Authz:    InternalAuthzFromDrsAccessMethods(drsObj.AccessMethods),
			Hashes:   &hashesMap,
		},
	}
	return indexdObj, nil
}

func getVal[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

func InternalRecordToDrsObject(indexdObj *InternalRecord) (*DRSObject, error) {
	authz := indexdObj.Authz
	urls := indexdObj.Urls

	accessMethods, err := DRSAccessMethodsFromInternalURLs(urls, authz)
	if err != nil {
		return nil, err
	}

	res := &DRSObject{
		Id:            getVal(indexdObj.Did),
		Size:          getVal(indexdObj.Size),
		Name:          indexdObj.FileName,
		AccessMethods: accessMethods,
	}

	if indexdObj.Hashes != nil {
		res.Checksums = hash.ConvertMapToDrsChecksums(*indexdObj.Hashes)
	}

	return res, nil
}

func DRSAccessMethodsFromInternalURLs(urls []string, authz []string) ([]AccessMethod, error) {
	var accessMethods []AccessMethod
	for _, urlString := range urls {
		var method AccessMethod
		method.AccessUrl = &AccessURL{Url: urlString}

		parsed, err := url.Parse(urlString)
		if err != nil {
			return nil, fmt.Errorf("failed to parse url %q: %v", urlString, err)
		}
		if parsed.Scheme == "" {
			method.Type = "https"
		} else {
			method.Type = parsed.Scheme
		}

		if len(authz) > 0 {
			method.Authorizations = &Authorizations{BearerAuthIssuers: []string{authz[0]}}
		}
		accessMethods = append(accessMethods, method)
	}
	return accessMethods, nil
}

// InternalAuthzFromDrsAccessMethods extracts authz values from DRS access methods
func InternalAuthzFromDrsAccessMethods(accessMethods []AccessMethod) []string {
	var authz []string
	for _, drsURL := range accessMethods {
		if drsURL.Authorizations != nil && len(drsURL.Authorizations.BearerAuthIssuers) > 0 {
			authz = append(authz, drsURL.Authorizations.BearerAuthIssuers[0])
		}
	}
	return authz
}

func InternalURLFromDrsAccessURLs(accessMethods []AccessMethod) []string {
	var urls []string
	for _, drsURL := range accessMethods {
		if drsURL.AccessUrl != nil {
			urls = append(urls, drsURL.AccessUrl.Url)
		}
	}
	return urls
}

// ToDrsObject converts an InternalRecordResponse (OutputInfo) to a DRSObject
func (outputInfo *OutputInfo) ToDrsObject() (*DRSObject, error) {
	return InternalRecordToDrsObject(outputInfo.ToInternalRecord())
}
