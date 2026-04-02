package drs

import (
	"time"

	"github.com/calypr/data-client/hash"
	syclient "github.com/calypr/syfon/client"
)

func drsObjectToSyfonInternalRecord(obj *DRSObject) (syclient.InternalRecord, error) {
	if obj == nil {
		return syclient.InternalRecord{}, nil
	}
	out := syclient.InternalRecord{}
	out.SetDid(obj.Id)
	if obj.Name != "" {
		out.SetFileName(obj.Name)
	}
	out.SetSize(obj.Size)
	out.SetUrls(InternalURLFromDrsAccessURLs(obj.AccessMethods))
	out.SetAuthz(InternalAuthzFromDrsAccessMethods(obj.AccessMethods))
	out.SetHashes(hash.ConvertDrsChecksumsToMap(obj.Checksums))
	return out, nil
}

func syfonInternalRecordToDRSObject(rec syclient.InternalRecord) (*DRSObject, error) {
	accessMethods, err := DRSAccessMethodsFromInternalURLs(rec.GetUrls(), rec.GetAuthz())
	if err != nil {
		return nil, err
	}
	checksums := hash.ConvertMapToDrsChecksums(rec.GetHashes())
	did := rec.GetDid()
	obj := &DRSObject{
		Id:            did,
		SelfUri:       "drs://" + did,
		Size:          rec.GetSize(),
		AccessMethods: accessMethods,
		Checksums:     checksums,
	}
	if rec.GetFileName() != "" {
		obj.Name = rec.GetFileName()
	}
	if t, ok := parseRFC3339(rec.GetCreatedDate()); ok {
		obj.CreatedTime = t
	}
	if t, ok := parseRFC3339(rec.GetUpdatedDate()); ok {
		obj.UpdatedTime = t
	}
	return obj, nil
}

func parseRFC3339(v string) (time.Time, bool) {
	if v == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
