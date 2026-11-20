package api

import (
	"bytes"
	"context"
	"encoding/json"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	emailsvc "github.com/spencercornish/booklab/internal/email"
	"github.com/spencercornish/booklab/internal/stripe/stripetest"
)

const testMAC = "AA:BB:CC:DD:EE:01"

type displayEnv struct {
	t  *testing.T
	ts *httptest.Server
}

func newDisplayEnv(t *testing.T) *displayEnv {
	t.Helper()
	ctx := context.Background()
	pool, q := openTestPool(t)
	truncateBookingData(t, ctx, pool)
	resetSettingsUTC(t, ctx, q)
	if _, err := pool.Exec(ctx, `TRUNCATE displays RESTART IDENTITY`); err != nil {
		t.Fatal(err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("s3cret!!"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreateAdminUser(ctx, "admin", string(hash)); err != nil {
		t.Fatal(err)
	}
	emailSvc := emailsvc.New("127.0.0.1", 1025, "", "", "booklab@localhost", testLogger())
	srv := New(testConfig(), q, stripetest.New(), emailSvc, nil, testLogger())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return &displayEnv{t: t, ts: ts}
}

func (e *displayEnv) device(path string, headers map[string]string) (int, map[string]any) {
	e.t.Helper()
	req := mustNewRequest(e.t, http.MethodGet, e.ts.URL+path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp := doReq(e.t, e.ts.Client(), req)
	defer resp.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		e.t.Fatalf("%s: decode: %v", path, err)
	}
	return resp.StatusCode, body
}

func (e *displayEnv) setup(mac string) (int, map[string]any) {
	return e.device("/api/setup", map[string]string{"ID": mac, "FW-Version": "1.6.0", "Model": "og"})
}

func (e *displayEnv) poll(mac, key string) map[string]any {
	e.t.Helper()
	status, body := e.device("/api/display", map[string]string{
		"ID": mac, "Access-Token": key, "Battery-Voltage": "3.41", "RSSI": "-61",
		"FW-Version": "1.6.0", "Model": "og", "Width": "800", "Height": "480",
	})
	if status != http.StatusOK {
		e.t.Fatalf("poll status %d: %v", status, body)
	}
	return body
}

func (e *displayEnv) fetchImage(url string) []byte {
	e.t.Helper()
	resp, err := e.ts.Client().Get(url)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		e.t.Fatalf("image %s: status %d", url, resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "image/png" {
		e.t.Fatalf("image content-type %q (firmware needs image/png)", ct)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil || img.Bounds().Dx() != 800 || img.Bounds().Dy() != 480 {
		e.t.Fatalf("bad image: %v %v", err, img)
	}
	return data
}

func (e *displayEnv) admin(method, path, body string) (int, []byte) {
	e.t.Helper()
	sess := loginAndSession(e.t, e.ts.URL, "admin", "s3cret!!")
	csrf := "testcsrf01234567890123456789012"
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req := mustNewRequest(e.t, method, e.ts.URL+path, rdr)
	addAdminCookies(req, sess, csrf)
	req.Header.Set("X-CSRF-Token", csrf)
	resp := doReq(e.t, e.ts.Client(), req)
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, data
}

func (e *displayEnv) listDisplays() []map[string]any {
	e.t.Helper()
	status, data := e.admin(http.MethodGet, "/api/admin/displays", "")
	if status != http.StatusOK {
		e.t.Fatalf("list displays: %d %s", status, data)
	}
	var out []map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		e.t.Fatal(err)
	}
	return out
}

func TestDisplay_setupApproveAndPoll(t *testing.T) {
	e := newDisplayEnv(t)

	status, setup := e.setup(strings.ToLower(testMAC))
	if status != http.StatusOK || setup["status"] != float64(200) {
		t.Fatalf("setup: %d %v", status, setup)
	}
	key, _ := setup["api_key"].(string)
	friendly, _ := setup["friendly_id"].(string)
	if len(key) < 32 || len(friendly) != 6 {
		t.Fatalf("setup returned key %q friendly %q", key, friendly)
	}

	// Pending: the device gets a waiting-for-approval image and polls every minute.
	pending := e.poll(testMAC, key)
	if pending["status"] != float64(0) || pending["refresh_rate"] != float64(60) {
		t.Fatalf("pending poll: %v", pending)
	}
	pendingURL := pending["image_url"].(string)
	if !strings.HasPrefix(pendingURL, e.ts.URL+"/api/display/image/") {
		t.Fatalf("image_url %q should use the request's base URL", pendingURL)
	}
	e.fetchImage(pendingURL)

	list := e.listDisplays()
	if len(list) != 1 || list[0]["status"] != "pending" || list[0]["mac_address"] != testMAC ||
		list[0]["battery_low"] != true || list[0]["rssi"] != float64(-61) || list[0]["firmware_version"] != "1.6.0" {
		t.Fatalf("admin list: %v", list)
	}
	if _, leaked := list[0]["api_key_hash"]; leaked {
		t.Fatal("admin list must not expose key material")
	}
	id := strconv.Itoa(int(list[0]["id"].(float64)))

	if status, data := e.admin(http.MethodPatch, "/api/admin/displays/"+id, `{"status":"approved","name":"  Darkroom door "}`); status != http.StatusOK {
		t.Fatalf("approve: %d %s", status, data)
	} else if !strings.Contains(string(data), `"name":"Darkroom door"`) {
		t.Fatalf("name not trimmed: %s", data)
	}

	approved := e.poll(testMAC, key)
	if approved["filename"] == pending["filename"] {
		t.Fatal("filename should change once the schedule replaces the pending screen")
	}
	if r := approved["refresh_rate"].(float64); r < 60 || r > 3600 {
		t.Fatalf("refresh_rate %v out of range", r)
	}
	img := e.fetchImage(approved["image_url"].(string))

	// Same state → same filename, so the firmware skips the redraw.
	again := e.poll(testMAC, key)
	if again["filename"] != approved["filename"] {
		t.Fatalf("filename churned: %v vs %v", again["filename"], approved["filename"])
	}
	if !bytes.Equal(img, e.fetchImage(again["image_url"].(string))) {
		t.Fatal("image bytes differ for the same filename")
	}

	// Revoke: old image URLs die (token rotates) and setup is refused.
	if status, data := e.admin(http.MethodPatch, "/api/admin/displays/"+id, `{"status":"revoked"}`); status != http.StatusOK {
		t.Fatalf("revoke: %d %s", status, data)
	}
	if resp, err := e.ts.Client().Get(approved["image_url"].(string)); err != nil || resp.StatusCode != http.StatusNotFound {
		t.Fatalf("old image URL after revoke: %v %v", resp.StatusCode, err)
	}
	revoked := e.poll(testMAC, key)
	if revoked["refresh_rate"] != float64(6*3600) {
		t.Fatalf("revoked poll: %v", revoked)
	}
	if status, _ := e.setup(testMAC); status != http.StatusNotFound {
		t.Fatalf("setup for revoked MAC: %d, want 404", status)
	}
}

func TestDisplay_resetupDropsToPending(t *testing.T) {
	e := newDisplayEnv(t)
	_, first := e.setup(testMAC)
	id := strconv.Itoa(int(e.listDisplays()[0]["id"].(float64)))
	e.admin(http.MethodPatch, "/api/admin/displays/"+id, `{"status":"approved"}`)

	_, second := e.setup(testMAC)
	if second["friendly_id"] != first["friendly_id"] {
		t.Fatal("friendly ID should be stable across re-setup")
	}
	list := e.listDisplays()
	if len(list) != 1 || list[0]["status"] != "pending" {
		t.Fatalf("re-setup must need approval again: %v", list)
	}
	// The old key no longer works.
	if body := e.poll(testMAC, first["api_key"].(string)); body["status"] != float64(202) {
		t.Fatalf("old key: %v", body)
	}
}

func TestDisplay_adoptsUnknownKeyForNewMAC(t *testing.T) {
	e := newDisplayEnv(t)
	body := e.poll(testMAC, "key-issued-by-some-other-server-0123456789")
	if body["status"] != float64(0) || body["refresh_rate"] != float64(60) {
		t.Fatalf("adopt: %v", body)
	}
	list := e.listDisplays()
	if len(list) != 1 || list[0]["status"] != "pending" {
		t.Fatalf("adopted display: %v", list)
	}

	// A different key for the now-known MAC is not adopted.
	if body := e.poll(testMAC, "another-key-entirely-0123456789abcdef"); body["status"] != float64(202) {
		t.Fatalf("hijack attempt: %v", body)
	}
}

func TestDisplay_rejectsBadRequests(t *testing.T) {
	e := newDisplayEnv(t)
	if status, _ := e.setup("not-a-mac"); status != http.StatusNotFound {
		t.Fatalf("bad MAC setup: %d", status)
	}
	if status, body := e.device("/api/display", map[string]string{"ID": testMAC}); status != http.StatusOK || body["status"] != float64(202) {
		t.Fatalf("no key: %d %v", status, body)
	}
	if resp, _ := e.ts.Client().Get(e.ts.URL + "/api/display/image/not-a-uuid/x.png"); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("bad token: %d", resp.StatusCode)
	}

	// Key for MAC A presented with MAC B.
	_, setup := e.setup(testMAC)
	if body := e.poll("AA:BB:CC:DD:EE:02", setup["api_key"].(string)); body["status"] != float64(202) {
		t.Fatalf("mismatched MAC: %v", body)
	}
}

func TestDisplay_pendingCap(t *testing.T) {
	e := newDisplayEnv(t)
	for i := range maxPendingDisplays {
		mac := "AA:BB:CC:DD:EE:" + strings.ToUpper(strconv.FormatInt(int64(0x10+i), 16))
		if status, body := e.setup(mac); status != http.StatusOK {
			t.Fatalf("setup %d: %d %v", i, status, body)
		}
	}
	if status, _ := e.setup("AA:BB:CC:DD:EE:FF"); status != http.StatusNotFound {
		t.Fatalf("setup beyond pending cap: %d, want 404", status)
	}
}

func TestDisplay_adminPreviewAndDelete(t *testing.T) {
	e := newDisplayEnv(t)
	status, data := e.admin(http.MethodGet, "/api/admin/displays/preview.png", "")
	if status != http.StatusOK {
		t.Fatalf("preview: %d", status)
	}
	if _, err := png.Decode(bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}

	e.setup(testMAC)
	id := strconv.Itoa(int(e.listDisplays()[0]["id"].(float64)))
	if status, _ := e.admin(http.MethodPatch, "/api/admin/displays/"+id, `{"status":"bogus"}`); status != http.StatusBadRequest {
		t.Fatalf("bad status: %d", status)
	}
	if status, _ := e.admin(http.MethodDelete, "/api/admin/displays/"+id, ""); status != http.StatusNoContent {
		t.Fatalf("delete: %d", status)
	}
	if status, _ := e.admin(http.MethodDelete, "/api/admin/displays/"+id, ""); status != http.StatusNotFound {
		t.Fatalf("second delete: %d", status)
	}

	// Unauthenticated admin access is refused.
	resp, err := e.ts.Client().Get(e.ts.URL + "/api/admin/displays")
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated list: %v %v", resp.StatusCode, err)
	}
}

func TestDisplay_settingsRefreshMinutes(t *testing.T) {
	e := newDisplayEnv(t)
	if status, _ := e.admin(http.MethodPut, "/api/admin/settings", `{"display_refresh_minutes":0}`); status != http.StatusBadRequest {
		t.Fatalf("0 minutes: %d", status)
	}
	status, data := e.admin(http.MethodPut, "/api/admin/settings", `{"display_refresh_minutes":15}`)
	if status != http.StatusOK || !strings.Contains(string(data), `"display_refresh_minutes":15`) {
		t.Fatalf("15 minutes: %d %s", status, data)
	}
}

func TestNormalizeMAC(t *testing.T) {
	cases := map[string]string{
		"aa:bb:cc:dd:ee:ff":   "AA:BB:CC:DD:EE:FF",
		" AA-BB-CC-DD-EE-FF ": "AA:BB:CC:DD:EE:FF",
	}
	for in, want := range cases {
		if got, ok := normalizeMAC(in); !ok || got != want {
			t.Errorf("normalizeMAC(%q) = %q %v", in, got, ok)
		}
	}
	for _, bad := range []string{"", "AA:BB:CC:DD:EE", "AA:BB:CC:DD:EE:GG", "AABBCCDDEEFF", "A:BB:CC:DD:EE:FFF"} {
		if _, ok := normalizeMAC(bad); ok {
			t.Errorf("normalizeMAC(%q) should fail", bad)
		}
	}
}
