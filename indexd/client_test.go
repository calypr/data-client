package indexd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/calypr/data-client/conf"
	drs "github.com/calypr/data-client/drs"
	"github.com/calypr/data-client/hash"
	"github.com/calypr/data-client/logs"
	"github.com/calypr/data-client/request"
)

type mockIndexdServer struct {
	mu                sync.Mutex
	listProjectPages  int
	listObjectsPages  int
	lastUpdatePayload UpdateInputInfo
}

func (m *mockIndexdServer) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && path == "/index/index":
			if hashQuery := r.URL.Query().Get("hash"); hashQuery != "" {
				record := sampleOutputInfo()
				page := ListRecords{Records: []OutputInfo{record}}
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(page)
				return
			}
			if r.URL.Query().Get("authz") != "" {
				m.mu.Lock()
				page := m.listProjectPages
				m.listProjectPages++
				m.mu.Unlock()
				w.WriteHeader(http.StatusOK)
				if page == 0 {
					_ = json.NewEncoder(w).Encode(ListRecords{Records: []OutputInfo{sampleOutputInfo()}})
				} else {
					_ = json.NewEncoder(w).Encode(ListRecords{Records: []OutputInfo{}})
				}
				return
			}

		case r.Method == http.MethodPost && path == "/index/index":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"did":"did-1"}`))
			return
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/ga4gh/drs/v1/objects"):
			if path == "/ga4gh/drs/v1/objects" {
				m.mu.Lock()
				page := m.listObjectsPages
				m.listObjectsPages++
				m.mu.Unlock()
				w.WriteHeader(http.StatusOK)
				if page == 0 {
					_ = json.NewEncoder(w).Encode(drs.DRSPage{DRSObjects: []drs.DRSObject{sampleDRSObject()}})
				} else {
					_ = json.NewEncoder(w).Encode(drs.DRSPage{DRSObjects: []drs.DRSObject{}})
				}
				return
			}
			obj := sampleOutputObject()
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(obj)
			return
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/index/index/"):
			record := sampleOutputInfo()
			record.Rev = "rev-1"
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(record)
			return
		case r.Method == http.MethodPut && strings.HasPrefix(path, "/index/index/"):
			body, _ := io.ReadAll(r.Body)
			payload := UpdateInputInfo{}
			_ = json.Unmarshal(body, &payload)
			m.mu.Lock()
			m.lastUpdatePayload = payload
			m.mu.Unlock()
			w.WriteHeader(http.StatusOK)
			return
		case r.Method == http.MethodDelete && strings.HasPrefix(path, "/index/index/"):
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}
}

func sampleOutputInfo() OutputInfo {
	return OutputInfo{
		Did:      "did-1",
		FileName: "file.txt",
		URLs:     []string{"s3://bucket/key"},
		Authz:    []string{"/programs/test/projects/proj"},
		Hashes:   hash.HashInfo{SHA256: "sha-256"},
		Size:     123,
	}
}

func sampleDRSObject() drs.DRSObject {
	return drs.DRSObject{
		Id:   "did-1",
		Name: "file.txt",
		Size: 123,
		Checksums: hash.HashInfo{
			SHA256: "sha-256",
		},
		AccessMethods: []drs.AccessMethod{
			{
				Type:           "s3",
				AccessURL:      drs.AccessURL{URL: "s3://bucket/key"},
				Authorizations: &drs.Authorizations{Value: "/programs/test/projects/proj"},
			},
		},
	}
}

func sampleOutputObject() OutputObject {
	return OutputObject{
		Id:   "did-1",
		Name: "file.txt",
		Size: 123,
		Checksums: []hash.Checksum{
			{Checksum: "sha-256", Type: hash.ChecksumTypeSHA256},
		},
	}
}

func newTestClient(server *httptest.Server) IndexdInterface {
	cred := &conf.Credential{APIEndpoint: server.URL, Profile: "test", AccessToken: "test-token"}
	logger, _ := logs.New("test")
	config := conf.NewConfigure(logger.Logger)
	req := request.NewRequestInterface(logger, cred, config)
	return NewIndexdClient(req, cred, logger.Logger)
}

func TestIndexdClient_ListAndQueryDirect(t *testing.T) {
	mock := &mockIndexdServer{}
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	client := newTestClient(server)

	records, err := client.GetObjectByHash(context.Background(), "sha256", "sha-256")
	if err != nil {
		t.Fatalf("GetObjectByHash error: %v", err)
	}
	if len(records) != 1 || records[0].Id != "did-1" {
		t.Fatalf("unexpected records: %+v", records)
	}

	objChan, err := client.ListObjectsByProject(context.Background(), "test-proj")
	if err != nil {
		t.Fatalf("ListObjectsByProject error: %v", err)
	}
	var found bool
	for res := range objChan {
		if res.Error != nil {
			t.Fatalf("ListObjectsByProject result error: %v", res.Error)
		}
		if res.Object != nil && res.Object.Id == "did-1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected object from ListObjectsByProject")
	}

	listChan, err := client.ListObjects(context.Background())
	if err != nil {
		t.Fatalf("ListObjects error: %v", err)
	}
	var listCount int
	for res := range listChan {
		if res.Error != nil {
			t.Fatalf("ListObjects result error: %v", res.Error)
		}
		if res.Object != nil {
			listCount++
		}
	}
	if listCount != 1 {
		t.Fatalf("expected 1 object from ListObjects, got %d", listCount)
	}
}

func TestIndexdClient_RegisterAndUpdateDirect(t *testing.T) {
	mock := &mockIndexdServer{}
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	client := newTestClient(server)

	drsObj := &drs.DRSObject{
		Id:        "did-1",
		Name:      "file.txt",
		Size:      123,
		Checksums: hash.HashInfo{SHA256: "sha-256"},
		AccessMethods: []drs.AccessMethod{
			{
				Type:           "s3",
				AccessURL:      drs.AccessURL{URL: "s3://bucket/key"},
				Authorizations: &drs.Authorizations{Value: "/programs/test/projects/proj"},
			},
		},
	}

	obj, err := client.RegisterRecord(context.Background(), drsObj)
	if err != nil {
		t.Fatalf("RegisterRecord error: %v", err)
	}
	if obj.Id != "did-1" {
		t.Fatalf("unexpected DRS object: %+v", obj)
	}

	update := &drs.DRSObject{
		Name:        "file-updated.txt",
		Version:     "v2",
		Description: "updated",
		AccessMethods: []drs.AccessMethod{
			{
				Type:           "s3",
				AccessURL:      drs.AccessURL{URL: "s3://bucket/other"},
				Authorizations: &drs.Authorizations{Value: "/programs/test/projects/proj"},
			},
		},
	}

	_, err = client.UpdateRecord(context.Background(), update, "did-1")
	if err != nil {
		t.Fatalf("UpdateRecord error: %v", err)
	}

	mock.mu.Lock()
	payload := mock.lastUpdatePayload
	mock.mu.Unlock()

	if len(payload.URLs) != 2 {
		t.Fatalf("expected URLs to include appended entries, got %+v", payload.URLs)
	}
}

func TestIndexdClient_GetObjectDirect(t *testing.T) {
	mock := &mockIndexdServer{}
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	client := newTestClient(server)

	record, err := client.GetObject(context.Background(), "did-1")
	if err != nil {
		t.Fatalf("GetObject error: %v", err)
	}
	if record.Id != "did-1" {
		t.Fatalf("unexpected record: %+v", record)
	}
}
