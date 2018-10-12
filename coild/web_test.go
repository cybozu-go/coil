package coild

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cybozu-go/coil/model"
	"github.com/google/go-cmp/cmp"
)

func testNewServer() *Server {
	mockDB := model.NewMock()
	s := NewServer(mockDB, 119, 30)
	s.dryRun = true
	return s
}

func testGetStatus(t *testing.T) {
	t.Parallel()
	server := testNewServer()
	server.podIPs = map[string]net.IP{
		"default/pod-1": net.ParseIP("10.0.0.1"),
	}

	_, subnet1, _ := net.ParseCIDR("10.0.0.0/27")
	server.addressBlocks = map[string][]*net.IPNet{
		"default": {subnet1},
	}
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
	if !cmp.Equal(st.AddressBlocks["default"], []string{"10.0.0.0/27"}) {
		t.Error(`expected: []string{"10.0.0.0/27"}, actual:`, st.AddressBlocks)
	}
	if !cmp.Equal(st.Pods, map[string]string{
		"default/pod-1": "10.0.0.1",
	}) {
		t.Error(`expected: "default/pod-1": "10.0.0.1", actual:`, st.Pods)
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
	server := testNewServer()

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
	r = httptest.NewRequest("POST", "/ip", strings.NewReader(`{"pod-namespace": "aaa"}`))
	server.ServeHTTP(w, r)
	resp = w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Error("http status should be 400, actual:", resp.StatusCode)
	}

	response := addressInfo{}

	w = httptest.NewRecorder()
	r = httptest.NewRequest("POST", "/ip",
		strings.NewReader(`{"pod-namespace": "aaa", "pod-name": "bbb"}`))
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
		strings.NewReader(`{"pod-namespace": "aaa", "pod-name": "bbb"}`))
	server.ServeHTTP(w, r)
	resp = w.Result()
	if resp.StatusCode != http.StatusConflict {
		t.Error("http status should be 409, actual:", resp.StatusCode)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest("POST", "/ip",
		strings.NewReader(`{"pod-namespace": "aaa", "pod-name": "ccc"}`))
	server.ServeHTTP(w, r)
	resp = w.Result()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Error("http status should be 503, actual:", resp.StatusCode)
	}
}

func testIPGet(t *testing.T) {
	t.Parallel()
	server := testNewServer()
	server.podIPs = map[string]net.IP{
		"default/pod-1": net.ParseIP("10.0.0.1"),
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/ip/foo/bar", nil)
	server.ServeHTTP(w, r)
	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Error("http status should be 404, actual:", resp.StatusCode)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest("GET", "/ip/default/pod-1", nil)
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
	server := testNewServer()
	server.podIPs = map[string]net.IP{
		"default/pod-1": net.ParseIP("10.0.0.1"),
	}
	_, subnet1, _ := net.ParseCIDR("10.0.0.0/27")
	server.addressBlocks = map[string][]*net.IPNet{
		"default": {subnet1},
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("DELETE", "/ip/foo/bar", nil)
	server.ServeHTTP(w, r)
	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Error("http status should be 404, actual:", resp.StatusCode)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest("DELETE", "/ip/default/pod-1", nil)
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

	w = httptest.NewRecorder()
	r = httptest.NewRequest("DELETE", "/ip/default/pod-1", nil)
	server.ServeHTTP(w, r)
	resp = w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Error("http status should be 404, actual:", resp.StatusCode)
	}
}

func testIP(t *testing.T) {
	t.Run("new", testIPNew)
	t.Run("get", testIPGet)
	t.Run("delete", testIPDelete)
}

func testNotFound(t *testing.T) {
	t.Parallel()
	server := testNewServer()

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
