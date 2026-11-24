package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/spencercornish/booklab/internal/db"
	"github.com/spencercornish/booklab/internal/display"
)

// Door-sign displays speak the TRMNL firmware protocol ("bring your own
// server"): the device calls GET /api/setup once to get an API key, then
// GET /api/display on every wake for an image URL and a sleep time. See
// docs/eink-display.md.

const (
	// maxPendingDisplays caps unapproved devices so random clients cannot
	// fill the table.
	maxPendingDisplays = 10
	// lowBatteryVolts is where the sign starts showing a low-battery icon
	// (single-cell LiPo, roughly 10-15% left).
	lowBatteryVolts = 3.5

	pendingRefresh = time.Minute
	revokedRefresh = 6 * time.Hour
)

// TRMNL /api/display "status" values (JSON body, not HTTP status).
const (
	trmnlStatusOK         = 0
	trmnlStatusNotClaimed = 202 // device shows its friendly ID and polls fast
)

// displayImageCache keeps the last image rendered for each display, so the
// image URL handed out by /api/display serves exactly what was hashed into
// its filename. The zero value is ready to use.
type displayImageCache struct {
	mu sync.Mutex
	m  map[int32]cachedDisplayImage
}

type cachedDisplayImage struct {
	hash string
	png  []byte
}

func (c *displayImageCache) put(id int32, hash string, png []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[int32]cachedDisplayImage{}
	}
	c.m[id] = cachedDisplayImage{hash: hash, png: png}
}

func (c *displayImageCache) get(id int32, hash string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	img, ok := c.m[id]
	if !ok || img.hash != hash {
		return nil, false
	}
	return img.png, true
}

func (c *displayImageCache) drop(id int32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, id)
}

// handleDisplaySetup is TRMNL's GET /api/setup: the device sends its MAC in
// the ID header and receives an API key. New devices start out pending until
// an admin approves them.
func (s *Server) handleDisplaySetup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	mac, ok := normalizeMAC(r.Header.Get("ID"))
	if !ok {
		s.writeSetupRefusal(w, r, "missing or invalid ID header")
		return
	}

	existing, err := s.queries.GetDisplayByMAC(ctx, mac)
	switch {
	case err == nil && existing.Status == db.DisplayStatusRevoked:
		s.writeSetupRefusal(w, r, "display has been revoked")
		return
	case errors.Is(err, pgx.ErrNoRows):
		if full, err := s.pendingDisplaysFull(r); err != nil {
			s.writeError(w, r, http.StatusInternalServerError, "failed to register display", err)
			return
		} else if full {
			s.writeSetupRefusal(w, r, "too many displays waiting for approval")
			return
		}
	case err != nil:
		s.writeError(w, r, http.StatusInternalServerError, "failed to register display", err)
		return
	}

	key, err := newDisplayAPIKey()
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to register display", err)
		return
	}
	d, err := s.registerDisplay(r, mac, key)
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to register display", err)
		return
	}
	s.displayImages.drop(d.ID)
	s.logger.Info("display_registered", "display_id", d.ID, "friendly_id", d.FriendlyID, "mac", mac, "status", d.Status)

	writeJSON(w, http.StatusOK, map[string]any{
		"status":      http.StatusOK,
		"api_key":     key,
		"friendly_id": d.FriendlyID,
		"image_url":   "",
		"message":     "Approve this display in BookLab admin",
	})
}

// writeSetupRefusal answers /api/setup with 404, which the firmware treats as
// "MAC not registered": it shows a notice and tries again on its next wake.
func (s *Server) writeSetupRefusal(w http.ResponseWriter, r *http.Request, msg string) {
	s.logger.Warn("display_setup_refused", "reason", msg, "client_ip", s.clientIP(r))
	writeJSON(w, http.StatusNotFound, map[string]any{"status": http.StatusNotFound, "message": msg})
}

func (s *Server) pendingDisplaysFull(r *http.Request) (bool, error) {
	n, err := s.queries.CountPendingDisplays(r.Context())
	return n >= maxPendingDisplays, err
}

// registerDisplay stores a device's key, retrying friendly-ID collisions.
func (s *Server) registerDisplay(r *http.Request, mac, key string) (*db.Display, error) {
	var lastErr error
	for range 5 {
		friendly, err := newFriendlyID()
		if err != nil {
			return nil, err
		}
		d, err := s.queries.RegisterDisplay(r.Context(), mac, hashDisplayKey(key), friendly)
		if err == nil {
			return d, nil
		}
		if !isUniqueViolation(err) {
			return nil, err
		}
		lastErr = err
	}
	return nil, lastErr
}

// handleDisplayPoll is TRMNL's GET /api/display, called on every wake.
func (s *Server) handleDisplayPoll(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	key := r.Header.Get("Access-Token")
	mac, macOK := normalizeMAC(r.Header.Get("ID"))
	if key == "" {
		s.writeNotClaimed(w, r, "missing Access-Token")
		return
	}

	d, err := s.queries.GetDisplayByKeyHash(ctx, hashDisplayKey(key))
	if errors.Is(err, pgx.ErrNoRows) {
		d, err = s.adoptUnknownDisplay(r, mac, macOK, key)
		if d == nil && err == nil {
			s.writeNotClaimed(w, r, "unknown display key")
			return
		}
	}
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to load display", err)
		return
	}
	if macOK && d.MacAddress != mac {
		s.writeNotClaimed(w, r, "display key does not match ID header")
		return
	}

	checkin := displayCheckinFromHeaders(r)
	checkin.IP = s.clientIP(r)
	if err := s.queries.RecordDisplayCheckin(ctx, d.ID, checkin); err != nil {
		s.logger.Warn("display_checkin_failed", "display_id", d.ID, "err", err)
	}

	png, refresh, err := s.renderDisplay(r, d, checkin.BatteryVoltage)
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to render display", err)
		return
	}
	sum := sha256.Sum256(png)
	hash := hex.EncodeToString(sum[:8])
	s.displayImages.put(d.ID, hash, png)

	writeJSON(w, http.StatusOK, map[string]any{
		"status":          trmnlStatusOK,
		"image_url":       s.requestBaseURL(r) + "/api/display/image/" + d.ImageToken.String() + "/" + hash + ".png",
		"filename":        "booklab-" + hash,
		"refresh_rate":    int(refresh.Seconds()),
		"reset_firmware":  false,
		"update_firmware": false,
		"firmware_url":    "",
		// Force a full (flashing) refresh. TRMNL firmware otherwise uses
		// partial refreshes, which leave ghosts of the previous screen and
		// only clean up every 8th redraw. The sign only redraws when its
		// content changes, so the flash is rare.
		"maximum_compatibility": true,
	})
}

// adoptUnknownDisplay handles a device presenting a key this server never
// issued, e.g. one already set up against another server, or one whose row an
// admin deleted. If its MAC is new, it is registered as pending with the key it
// already has, so it never needs a reset (TRMNL's reset wipes Wi-Fi settings
// too). A known MAC with the wrong key is not adopted: that would let anyone
// who knows a MAC take over its row. Returns (nil, nil) when not adopted.
func (s *Server) adoptUnknownDisplay(r *http.Request, mac string, macOK bool, key string) (*db.Display, error) {
	if !macOK {
		return nil, nil
	}
	if _, err := s.queries.GetDisplayByMAC(r.Context(), mac); err == nil {
		return nil, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if full, err := s.pendingDisplaysFull(r); err != nil || full {
		return nil, err
	}
	d, err := s.registerDisplay(r, mac, key)
	if err != nil {
		return nil, err
	}
	s.logger.Info("display_adopted", "display_id", d.ID, "friendly_id", d.FriendlyID, "mac", mac)
	return d, nil
}

func (s *Server) writeNotClaimed(w http.ResponseWriter, r *http.Request, msg string) {
	s.logger.Warn("display_not_claimed", "reason", msg, "client_ip", s.clientIP(r))
	writeJSON(w, http.StatusOK, map[string]any{
		"status":       trmnlStatusNotClaimed,
		"refresh_rate": int(pendingRefresh.Seconds()),
	})
}

// renderDisplay draws the screen for d's current state.
func (s *Server) renderDisplay(r *http.Request, d *db.Display, battery *float32) ([]byte, time.Duration, error) {
	switch d.Status {
	case db.DisplayStatusApproved:
		res, err := s.buildSignForNow(r, battery)
		if err != nil {
			return nil, 0, err
		}
		png, err := display.RenderSchedule(res.View)
		return png, res.RefreshAfter, err
	case db.DisplayStatusRevoked:
		png, err := display.RenderNotice(display.NoticeView{
			Title:  "Display disabled",
			Lines:  []string{"This display was revoked in BookLab admin."},
			Code:   d.FriendlyID,
			Footer: "Checks again every few hours",
		})
		return png, revokedRefresh, err
	default:
		png, err := display.RenderNotice(pendingNotice(d))
		return png, pendingRefresh, err
	}
}

func pendingNotice(d *db.Display) display.NoticeView {
	return display.NoticeView{
		Title:  "Waiting for approval",
		Lines:  []string{"Approve this display in BookLab", "under Admin → Displays."},
		Code:   d.FriendlyID,
		Footer: "Checks again every minute",
	}
}

// buildSignForNow loads what the sign needs and lays it out.
func (s *Server) buildSignForNow(r *http.Request, battery *float32) (signResult, error) {
	ctx := r.Context()
	settings, err := s.queries.GetSettings(ctx)
	if err != nil {
		return signResult{}, fmt.Errorf("load settings: %w", err)
	}
	loc := settingsLocation(settings)
	now := time.Now().In(loc)
	from := dateOnly(now, loc)
	to := from.AddDate(0, 0, signLookaheadDays+1)

	bookings, err := s.queries.ListBookingsOverlapping(ctx, from, to)
	if err != nil {
		return signResult{}, fmt.Errorf("load bookings: %w", err)
	}
	// ListClosuresInRange compares calendar dates, passed as UTC midnights.
	closures, err := s.queries.ListClosuresInRange(ctx,
		time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC),
		time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC))
	if err != nil {
		return signResult{}, fmt.Errorf("load closures: %w", err)
	}

	qrURL := strings.TrimRight(s.cfg.AppURL, "/")
	return buildSign(signInput{
		Now:          now,
		Settings:     settings,
		Loc:          loc,
		Bookings:     bookings,
		Closures:     closures,
		QRURL:        qrURL,
		QRLabel:      "Book at " + displayHost(qrURL),
		BatteryLow:   battery != nil && *battery > 0 && *battery < lowBatteryVolts,
		OpenInterval: time.Duration(max(settings.DisplayRefreshMinutes, 1)) * time.Minute,
	}), nil
}

// handleDisplayImage serves the PNG named by /api/display. The device fetches
// it without headers, so the per-display image token in the path is the
// credential. If the cached image has moved on (or the server restarted), the
// current screen is rendered instead.
func (s *Server) handleDisplayImage(w http.ResponseWriter, r *http.Request) {
	token, err := uuid.Parse(chi.URLParam(r, "token"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	d, err := s.queries.GetDisplayByImageToken(r.Context(), token)
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	} else if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to load display", err)
		return
	}
	hash := strings.TrimSuffix(chi.URLParam(r, "file"), ".png")
	png, ok := s.displayImages.get(d.ID, hash)
	if !ok {
		png, _, err = s.renderDisplay(r, d, d.BatteryVoltage)
		if err != nil {
			s.writeError(w, r, http.StatusInternalServerError, "failed to render display", err)
			return
		}
	}
	writePNG(w, png)
}

func writePNG(w http.ResponseWriter, png []byte) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", strconv.Itoa(len(png)))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	w.Write(png)
}

// handleDisplayLog accepts TRMNL's POST /api/log so devices do not retry it.
// Only authenticated devices are logged, and only the size of the body.
func (s *Server) handleDisplayLog(w http.ResponseWriter, r *http.Request) {
	io.Copy(io.Discard, http.MaxBytesReader(w, r.Body, maxJSONBodyBytes))
	if key := r.Header.Get("Access-Token"); key != "" {
		if d, err := s.queries.GetDisplayByKeyHash(r.Context(), hashDisplayKey(key)); err == nil {
			s.logger.Info("display_log_received", "display_id", d.ID, "content_length", r.ContentLength)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// ----- Admin -----

func displayToAdmin(d *db.Display) map[string]any {
	return map[string]any{
		"id":               d.ID,
		"mac_address":      d.MacAddress,
		"friendly_id":      d.FriendlyID,
		"name":             d.Name,
		"status":           d.Status,
		"model":            d.Model,
		"firmware_version": d.FirmwareVersion,
		"battery_voltage":  d.BatteryVoltage,
		"battery_low":      d.BatteryVoltage != nil && *d.BatteryVoltage > 0 && *d.BatteryVoltage < lowBatteryVolts,
		"rssi":             d.RSSI,
		"width":            d.Width,
		"height":           d.Height,
		"last_seen_at":     d.LastSeenAt,
		"last_ip":          d.LastIP,
		"approved_at":      d.ApprovedAt,
		"created_at":       d.CreatedAt,
	}
}

func (s *Server) handleAdminListDisplays(w http.ResponseWriter, r *http.Request) {
	displays, err := s.queries.ListDisplays(r.Context())
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to list displays", err)
		return
	}
	out := make([]map[string]any, 0, len(displays))
	for _, d := range displays {
		out = append(out, displayToAdmin(d))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleAdminUpdateDisplay(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, "invalid display id", err)
		return
	}
	var req struct {
		Name   *string           `json:"name"`
		Status *db.DisplayStatus `json:"status"`
	}
	if !s.readJSONRequest(w, r, &req) {
		return
	}
	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		if len([]rune(trimmed)) > 100 {
			s.writeError(w, r, http.StatusBadRequest, "name must be at most 100 characters", nil)
			return
		}
		req.Name = &trimmed
	}
	if req.Status != nil {
		switch *req.Status {
		case db.DisplayStatusApproved, db.DisplayStatusRevoked, db.DisplayStatusPending:
		default:
			s.writeError(w, r, http.StatusBadRequest, "status must be approved, revoked or pending", nil)
			return
		}
	}
	d, err := s.queries.UpdateDisplay(r.Context(), id, req.Name, req.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		s.writeError(w, r, http.StatusNotFound, "display not found", err)
		return
	} else if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to update display", err)
		return
	}
	s.displayImages.drop(d.ID)
	s.logger.Info("display_updated", "display_id", d.ID, "status", d.Status)
	writeJSON(w, http.StatusOK, displayToAdmin(d))
}

func (s *Server) handleAdminDeleteDisplay(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, "invalid display id", err)
		return
	}
	if err := s.queries.DeleteDisplay(r.Context(), id); errors.Is(err, pgx.ErrNoRows) {
		s.writeError(w, r, http.StatusNotFound, "display not found", err)
		return
	} else if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to delete display", err)
		return
	}
	s.displayImages.drop(id)
	w.WriteHeader(http.StatusNoContent)
}

// handleAdminDisplayPreview renders the sign as an approved display would see
// it right now, so admins can check it without hardware.
func (s *Server) handleAdminDisplayPreview(w http.ResponseWriter, r *http.Request) {
	res, err := s.buildSignForNow(r, nil)
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to build preview", err)
		return
	}
	png, err := display.RenderSchedule(res.View)
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "failed to render preview", err)
		return
	}
	writePNG(w, png)
}

// ----- helpers -----

// normalizeMAC accepts "aa:bb:cc:dd:ee:ff" or "AA-BB-…" and returns the
// upper-case, colon-separated form.
func normalizeMAC(raw string) (string, bool) {
	s := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(raw), "-", ":"))
	parts := strings.Split(s, ":")
	if len(parts) != 6 {
		return "", false
	}
	for _, p := range parts {
		if len(p) != 2 {
			return "", false
		}
		if _, err := hex.DecodeString(p); err != nil {
			return "", false
		}
	}
	return s, true
}

func newDisplayAPIKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashDisplayKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// friendlyIDAlphabet leaves out look-alikes (0/O, 1/I/L).
const friendlyIDAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

func newFriendlyID() (string, error) {
	var b strings.Builder
	limit := big.NewInt(int64(len(friendlyIDAlphabet)))
	for range 6 {
		n, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", err
		}
		b.WriteByte(friendlyIDAlphabet[n.Int64()])
	}
	return b.String(), nil
}

func displayCheckinFromHeaders(r *http.Request) db.DisplayCheckin {
	h := r.Header
	c := db.DisplayCheckin{
		Model:           truncateHeader(h.Get("Model")),
		FirmwareVersion: truncateHeader(h.Get("FW-Version")),
	}
	if v, err := strconv.ParseFloat(h.Get("Battery-Voltage"), 32); err == nil && v >= 0 && v < 10 {
		f := float32(v)
		c.BatteryVoltage = &f
	}
	c.RSSI = headerInt(h.Get("RSSI"), -200, 0)
	c.Width = headerInt(h.Get("Width"), 1, 10000)
	c.Height = headerInt(h.Get("Height"), 1, 10000)
	return c
}

func headerInt(v string, lo, hi int64) *int32 {
	n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 32)
	if err != nil || n < lo || n > hi {
		return nil
	}
	i := int32(n)
	return &i
}

func truncateHeader(v string) string {
	v = strings.TrimSpace(v)
	if len(v) > 64 {
		v = v[:64]
	}
	return v
}

// requestBaseURL is the scheme and host the device used to reach us, so the
// image URL works however the device is pointed at the server (LAN address or
// public URL). X-Forwarded-Proto is only believed from a trusted proxy.
func (s *Server) requestBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	} else if peer, ok := parseHostAddr(r.RemoteAddr); ok && s.isTrustedProxy(peer) {
		if p := strings.ToLower(r.Header.Get("X-Forwarded-Proto")); p == "https" || p == "http" {
			scheme = p
		}
	}
	return scheme + "://" + r.Host
}

// displayHost strips the scheme and trailing slash for printing a URL.
func displayHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	return strings.TrimRight(u.Host+u.Path, "/")
}
