package db

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type DisplayStatus string

const (
	DisplayStatusPending  DisplayStatus = "pending"
	DisplayStatusApproved DisplayStatus = "approved"
	DisplayStatusRevoked  DisplayStatus = "revoked"
)

// Display is an e-ink door sign running TRMNL firmware against this server.
type Display struct {
	ID              int32         `json:"id"`
	MacAddress      string        `json:"mac_address"`
	APIKeyHash      string        `json:"-"`
	FriendlyID      string        `json:"friendly_id"`
	ImageToken      uuid.UUID     `json:"-"`
	Name            string        `json:"name"`
	Status          DisplayStatus `json:"status"`
	Model           string        `json:"model"`
	FirmwareVersion string        `json:"firmware_version"`
	BatteryVoltage  *float32      `json:"battery_voltage"`
	RSSI            *int32        `json:"rssi"`
	Width           *int32        `json:"width"`
	Height          *int32        `json:"height"`
	LastSeenAt      *time.Time    `json:"last_seen_at"`
	LastIP          string        `json:"last_ip"`
	ApprovedAt      *time.Time    `json:"approved_at"`
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
}

const displayColumns = `id, mac_address, api_key_hash, friendly_id, image_token, name, status,
	model, firmware_version, battery_voltage, rssi, width, height, last_seen_at, last_ip,
	approved_at, created_at, updated_at`

func scanDisplay(row pgx.Row) (*Display, error) {
	var d Display
	err := row.Scan(&d.ID, &d.MacAddress, &d.APIKeyHash, &d.FriendlyID, &d.ImageToken, &d.Name, &d.Status,
		&d.Model, &d.FirmwareVersion, &d.BatteryVoltage, &d.RSSI, &d.Width, &d.Height, &d.LastSeenAt, &d.LastIP,
		&d.ApprovedAt, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// RegisterDisplay records a device presenting a new API key. A new MAC becomes
// a pending display. A known, non-revoked MAC gets the new key and drops back to
// pending, so a device that re-runs setup (or anyone spoofing its MAC) needs an
// admin's approval again. Revoked displays are left untouched; callers check
// for them first. friendlyID is only used for new rows.
func (q *Queries) RegisterDisplay(ctx context.Context, mac, keyHash, friendlyID string) (*Display, error) {
	return scanDisplay(q.pool.QueryRow(ctx, `
		INSERT INTO displays (mac_address, api_key_hash, friendly_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (mac_address) DO UPDATE SET
			api_key_hash = EXCLUDED.api_key_hash,
			status       = 'pending',
			approved_at  = NULL,
			updated_at   = NOW()
		WHERE displays.status != 'revoked'
		RETURNING `+displayColumns,
		mac, keyHash, friendlyID,
	))
}

func (q *Queries) CountPendingDisplays(ctx context.Context) (int64, error) {
	var n int64
	err := q.pool.QueryRow(ctx, `SELECT COUNT(*) FROM displays WHERE status = 'pending'`).Scan(&n)
	return n, err
}

func (q *Queries) GetDisplay(ctx context.Context, id int32) (*Display, error) {
	return scanDisplay(q.pool.QueryRow(ctx, `SELECT `+displayColumns+` FROM displays WHERE id = $1`, id))
}

func (q *Queries) GetDisplayByMAC(ctx context.Context, mac string) (*Display, error) {
	return scanDisplay(q.pool.QueryRow(ctx, `SELECT `+displayColumns+` FROM displays WHERE mac_address = $1`, mac))
}

func (q *Queries) GetDisplayByKeyHash(ctx context.Context, keyHash string) (*Display, error) {
	return scanDisplay(q.pool.QueryRow(ctx, `SELECT `+displayColumns+` FROM displays WHERE api_key_hash = $1`, keyHash))
}

func (q *Queries) GetDisplayByImageToken(ctx context.Context, token uuid.UUID) (*Display, error) {
	return scanDisplay(q.pool.QueryRow(ctx, `SELECT `+displayColumns+` FROM displays WHERE image_token = $1`, token))
}

func (q *Queries) ListDisplays(ctx context.Context) ([]*Display, error) {
	rows, err := q.pool.Query(ctx, `SELECT `+displayColumns+` FROM displays ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	displays := []*Display{}
	for rows.Next() {
		d, err := scanDisplay(rows)
		if err != nil {
			return nil, err
		}
		displays = append(displays, d)
	}
	return displays, rows.Err()
}

// DisplayCheckin is what a device reports about itself on each poll.
type DisplayCheckin struct {
	Model           string
	FirmwareVersion string
	BatteryVoltage  *float32
	RSSI            *int32
	Width           *int32
	Height          *int32
	IP              string
}

func (q *Queries) RecordDisplayCheckin(ctx context.Context, id int32, c DisplayCheckin) error {
	_, err := q.pool.Exec(ctx, `
		UPDATE displays SET
			model            = $2,
			firmware_version = $3,
			battery_voltage  = COALESCE($4, battery_voltage),
			rssi             = COALESCE($5, rssi),
			width            = COALESCE($6, width),
			height           = COALESCE($7, height),
			last_ip          = $8,
			last_seen_at     = NOW()
		WHERE id = $1`,
		id, c.Model, c.FirmwareVersion, c.BatteryVoltage, c.RSSI, c.Width, c.Height, c.IP)
	return err
}

// UpdateDisplay changes a display's name and/or status. Approving stamps
// approved_at; leaving approved (revoke, back to pending) rotates the image
// token so previously issued image URLs stop working.
func (q *Queries) UpdateDisplay(ctx context.Context, id int32, name *string, status *DisplayStatus) (*Display, error) {
	return scanDisplay(q.pool.QueryRow(ctx, `
		UPDATE displays SET
			name        = COALESCE($2, name),
			status      = COALESCE($3, status),
			approved_at = CASE
				WHEN $3::text = 'approved' AND status != 'approved' THEN NOW()
				WHEN $3::text IS NOT NULL AND $3::text != 'approved' THEN NULL
				ELSE approved_at END,
			image_token = CASE
				WHEN $3::text IS NOT NULL AND $3::text != 'approved' THEN gen_random_uuid()
				ELSE image_token END,
			updated_at  = NOW()
		WHERE id = $1
		RETURNING `+displayColumns,
		id, name, status,
	))
}

func (q *Queries) DeleteDisplay(ctx context.Context, id int32) error {
	tag, err := q.pool.Exec(ctx, `DELETE FROM displays WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
