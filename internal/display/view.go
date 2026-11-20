// Package display renders the e-ink door-sign screen that TRMNL-firmware
// devices show outside the room. It only draws: callers hand it a fully
// formatted View and get back a 1-bit 800x480 PNG.
package display

// Panel dimensions for the TRMNL 7.5" (OG) panel.
const (
	Width  = 800
	Height = 480
)

// StatusKind picks the treatment of the big status banner.
type StatusKind int

const (
	StatusOpen   StatusKind = iota // outlined banner
	StatusInUse                    // solid black banner
	StatusClosed                   // dithered banner
)

// RowState is what one hour of the timeline shows.
type RowState int

const (
	RowOpen   RowState = iota // white bar with an outline
	RowBooked                 // solid black bar with the booker's short name
	RowClosed                 // hatched bar: a partial closure covers this hour
)

// Row is one hour in the timeline.
type Row struct {
	Label   string // "10 AM"
	State   RowState
	Text    string // name for booked rows; ignored for open rows ("open" is drawn)
	Current bool   // the hour that contains "now"; gets the ◄ marker
	Past    bool   // already finished; drawn lighter
}

// View is everything the schedule screen needs, already formatted.
type View struct {
	Title string // resource name, e.g. "Darkroom"

	Status       StatusKind
	Headline     string // "IN USE", "OPEN", "CLOSED"
	Detail       string // "until 3:00 PM"
	DetailStrong string // optional bold line under Detail, e.g. "Maya R."
	Next         string // "Next open: 3:00 PM"

	DayLabel string // "FRI NOV 20" or "TOMORROW · SAT NOV 21"
	Rows     []Row
	// EmptyMessage replaces the timeline when there are no rows
	// (closure day, no bookable hours).
	EmptyMessage string

	QRURL   string // encoded into the QR code; no QR when empty
	QRLabel string // printed beside the QR code

	Updated    string // "updated 2:14 PM"
	BatteryLow bool
}

// NoticeView is a full-screen message used for device states that have no
// schedule to show, such as "waiting for approval".
type NoticeView struct {
	Title  string   // big heading
	Lines  []string // body lines
	Code   string   // optional large code (e.g. the device's friendly ID)
	Footer string
}
