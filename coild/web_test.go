package coild

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cybozu-go/coil"
	"github.com/cybozu-go/coil/model"
	"github.com/google/go-cmp/cmp"
)

func testNewServer(t *testing.T) *Server {
	db := model.NewTestEtcdModel(t)
	s := NewServer(db, 119, 30)
	s.nodeName = "node-0"
	s.dryRun = true

	_, gsubnet, _ := net.ParseCIDR("99.88.77.0/28")
	_, lsubnet, _ := net.ParseCIDR("10.10.0.0/28")
	err := s.db.AddPool(context.Background(), "global", gsubnet, 0)
	if err != nil {
		t.Fatal(err)
	}
	err = s.db.AddPool(context.Background(), "default", lsubnet, 2)
	if err != nil {
		t.Fatal(err)
	}
	block, err := s.db.AcquireBlock(context.Background(), "node-0", "default")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.db.AllocateIP(context.Background(), block, coil.IPAssignment{
		ContainerID: "container-0",
		Namespace:   "default",
		Pod:         "pod-0",
		CreatedAt:   time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func testGetStatus(t *testing.T) {
	t.Parallel()
	server := testNewServer(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/status", nil)
	server.ServeHTTP(w, r)
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Error("http status should be 200, actual:", resp.StatusCode)
	}
	st := status{}
	err := json.NewDecoder(resp.Body).Decode(&st)
	if err != nil {
		t.Fatal(err)
	}
	if !cmp.Equal(st.AddressBlocks["default"], []string{"10.10.0.0/30"}) {
		t.Error(`expected: []string{"10.10.0.0/30"}, actual:`, st.AddressBlocks)
	}
	if !cmp.Equal(st.Pods, map[string]string{
		"container-0": "10.10.0.0",
	}) {
		t.Error(`expected: "container-0": "10.10.0.0", actual:`, st.Pods)
	}
	if st.Status != http.StatusOK {
		t.Error("expected: 200, actual:", st.Status)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest("POST", "/status", nil)
	server.ServeHTTP(w, r)
	resp = w.Result()
	if resp.StatusCode == http.StatusOK {
		t.Error(`should not be 200`)
	}
}

func testIPNew(t *testing.T) {
	t.Parallel()
	server := testNewServer(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/ip", nil)
	server.ServeHTTP(w, r)
	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Error("http status should be 400, actual:", resp.StatusCode)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest("POST", "/ip", strings.NewReader(`{"pod-name": "aaa"}`))
	server.ServeHTTP(w, r)
	resp = w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Error("http status should be 400, actual:", resp.StatusCode)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest("POST", "/ip", strings.NewReader(`{"pod-namespace": "default"}`))
	server.ServeHTTP(w, r)
	resp = w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Error("http status should be 400, actual:", resp.StatusCode)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest("POST", "/ip", strings.NewReader(`{"container-id": "aaa"}`))
	server.ServeHTTP(w, r)
	resp = w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Error("http status should be 400, actual:", resp.StatusCode)
	}

	response := addressInfo{}

	w = httptest.NewRecorder()
	r = httptest.NewRequest("POST", "/ip",
		strings.NewReader(`{"pod-namespace": "default", "pod-name": "bbb", "container-id": "ccc"}`))
	server.ServeHTTP(w, r)
	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Error("http status should be 200, actual:", resp.StatusCode)
	}

	err := json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		t.Fatal(err)
	}
	if net.ParseIP(response.Address).IsUnspecified() {
		t.Error("invalid IP address:", response.Address)
	}
	if response.Status != http.StatusOK {
		t.Error("invalid status:", response.Status)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest("POST", "/ip",
		strings.NewReader(`{"pod-namespace": "default", "pod-name": "bbb", "container-id": "ccc"}`))
	server.ServeHTTP(w, r)
	resp = w.Result()
	if resp.StatusCode != http.StatusConflict {
		t.Error("http status should be 409, actual:", resp.StatusCode)
	}

	for i := 0; i < 14; i++ {
		w = httptest.NewRecorder()
		r = httptest.NewRequest("POST", "/ip",
			strings.NewReader(`{"pod-namespace": "default", "pod-name": "bbb", "container-id": "`+fmt.Sprintf("ddd-%d", i)+`"}`))
		server.ServeHTTP(w, r)
		resp = w.Result()
		if resp.StatusCode != http.StatusOK {
			t.Error("http status should be 200, actual:", resp.StatusCode)
		}
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest("POST", "/ip",
		strings.NewReader(`{"pod-namespace": "default", "pod-name": "ddd", "container-id": "eee"}`))
	server.ServeHTTP(w, r)
	resp = w.Result()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Error("http status should be 503, actual:", resp.StatusCode)
	}
}

func testIPGet(t *testing.T) {
	t.Parallel()
	server := testNewServer(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/ip/foo/bar/30449aba-01cf-4cb6-b4ed-4d17fa8af1a6", nil)
	server.ServeHTTP(w, r)
	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Error("http status should be 404, actual:", resp.StatusCode)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest("GET", "/ip/default/pod-0/container-0", nil)
	server.ServeHTTP(w, r)
	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Error("http status should be 200, actual:", resp.StatusCode)
	}

	response := addressInfo{}
	err := json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		t.Fatal(err)
	}
	if net.ParseIP(response.Address).IsUnspecified() {
		t.Error("invalid IP address:", response.Address)
	}
	if response.Status != http.StatusOK {
		t.Error("invalid status:", response.Status)
	}
}

func testIPDelete(t *testing.T) {
	t.Parallel()
	server := testNewServer(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("DELETE", "/ip/foo/bar/30449aba-01cf-4cb6-b4ed-4d17fa8af1a6", nil)
	server.ServeHTTP(w, r)
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Error("http status should be 200, actual:", resp.StatusCode)
	}

	response := addressInfo{}
	err := json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Address) != 0 {
		t.Error("IP address is not empty:", response.Address)
	}
	if response.Status != http.StatusOK {
		t.Error("invalid status:", response.Status)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest("DELETE", "/ip/default/pod-0/container-0", nil)
	server.ServeHTTP(w, r)
	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Error("http status should be 200, actual:", resp.StatusCode)
	}

	response = addressInfo{}
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		t.Fatal(err)
	}
	if net.ParseIP(response.Address).IsUnspecified() {
		t.Error("invalid IP address:", response.Address)
	}
	if response.Status != http.StatusOK {
		t.Error("invalid status:", response.Status)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest("DELETE", "/ip/default/pod-0/container-0", nil)
	server.ServeHTTP(w, r)
	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Error("http status should be 200, actual:", resp.StatusCode)
	}

	response = addressInfo{}
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Address) != 0 {
		t.Error("IP address is not empty:", response.Address)
	}
	if response.Status != http.StatusOK {
		t.Error("invalid status:", response.Status)
	}
}

func testIP(t *testing.T) {
	t.Run("new", testIPNew)
	t.Run("get", testIPGet)
	t.Run("delete", testIPDelete)
}

func testNotFound(t *testing.T) {
	t.Parallel()
	server := testNewServer(t)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/notfound", nil)
	server.ServeHTTP(w, r)
	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Error("http status should be 404, actual:", resp.StatusCode)
	}
}

func TestServeHTTP(t *testing.T) {
	t.Run("status", testGetStatus)
	t.Run("ip", testIP)
	t.Run("notfound", testNotFound)
}
