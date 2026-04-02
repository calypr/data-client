package drs

import (
	"context"
	"fmt"
	"strings"

	"github.com/calypr/data-client/hash"
)

// ResolveObject centralizes object-id vs checksum resolution logic.
func ResolveObject(ctx context.Context, client Client, guid string) (*DRSObject, error) {
	if oid := NormalizeOid(guid); oid != "" {
		if cached, ok := PrefetchedBySHA(ctx, oid); ok {
			obj := cached
			return &obj, nil
		}
		if recs, err := client.GetObjectByHash(ctx, &hash.Checksum{Type: "sha256", Checksum: oid}); err == nil && len(recs) > 0 {
			return &recs[0], nil
		}
	}
	return client.GetObject(ctx, guid)
}

// ResolveDownloadURL resolves access method and object id when caller does not already provide a concrete access id.
func ResolveDownloadURL(ctx context.Context, client Client, guid string, accessID string) (string, error) {
	obj, err := ResolveObject(ctx, client, guid)
	if err != nil {
		return "", err
	}

	resolvedID := strings.TrimSpace(obj.Id)
	if resolvedID == "" {
		resolvedID = guid
	}

	if accessID == "" {
		for _, am := range obj.AccessMethods {
			if am.AccessId != "" {
				accessID = am.AccessId
				break
			}
		}
		if accessID == "" {
			for _, am := range obj.AccessMethods {
				if am.AccessUrl.Url != "" {
					return am.AccessUrl.Url, nil
				}
			}
			return "", fmt.Errorf("no suitable access method found for object %s", guid)
		}
	}

	accessURL, err := client.GetDownloadURL(ctx, resolvedID, accessID)
	if err != nil {
		return "", err
	}
	if accessURL == nil || accessURL.Url == "" {
		return "", fmt.Errorf("empty access URL for object %s", guid)
	}
	return accessURL.Url, nil
}
