package display

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func sampleRows(currentIdx int) []Row {
	labels := []string{"10 AM", "11 AM", "12 PM", "1 PM", "2 PM", "3 PM", "4 PM", "5 PM", "6 PM", "7 PM", "8 PM"}
	booked := map[int]string{2: "Maya R.", 3: "Maya R.", 4: "Jon P.", 7: "Alexandria-Rose W."}
	rows := make([]Row, len(labels))
	for i, l := range labels {
		rows[i] = Row{Label: l, Current: i == currentIdx, Past: currentIdx >= 0 && i < currentIdx}
		if n, ok := booked[i]; ok {
			rows[i].State = RowBooked
			rows[i].Text = n
		}
	}
	rows[9].State = RowClosed
	return rows
}

func sampleViews() map[string]View {
	return map[string]View{
		"in_use": {
			Title: "Darkroom", Status: StatusInUse, Headline: "IN USE",
			Detail: "until 3:00 PM", DetailStrong: "Jon P.", Next: "Next open: 3:00 PM",
			DayLabel: "FRI NOV 20", Rows: sampleRows(4),
			QRURL: "https://book.artlab.example.org", QRLabel: "Book at book.artlab.example.org",
			Updated: "updated 2:14 PM", BatteryLow: true,
		},
		"open": {
			Title: "Darkroom", Status: StatusOpen, Headline: "OPEN",
			Detail: "free until 5:00 PM", Next: "Next booking: 5:00 PM",
			DayLabel: "FRI NOV 20", Rows: sampleRows(5),
			QRURL: "https://book.artlab.example.org", QRLabel: "Book at book.artlab.example.org",
			Updated: "updated 3:02 PM",
		},
		"after_hours": {
			Title: "Darkroom", Status: StatusClosed, Headline: "CLOSED",
			Detail: "opens 10:00 AM", DetailStrong: "tomorrow",
			DayLabel: "TOMORROW · SAT NOV 21", Rows: sampleRows(-1),
			QRURL: "https://book.artlab.example.org", QRLabel: "Book at book.artlab.example.org",
			Updated: "updated 9:41 PM",
		},
		"closure_day": {
			Title: "Darkroom", Status: StatusClosed, Headline: "CLOSED",
			Detail: "today", Next: "Opens 10:00 AM Mon, Nov 23",
			DayLabel: "SAT NOV 21", EmptyMessage: "Closed for chemistry change",
			QRURL: "https://book.artlab.example.org", QRLabel: "Book at book.artlab.example.org",
			Updated: "updated 8:00 AM",
		},
	}
}

func checkPNG(t *testing.T, name string, data []byte) {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("%s: decode: %v", name, err)
	}
	if b := img.Bounds(); b.Dx() != Width || b.Dy() != Height {
		t.Fatalf("%s: got %dx%d, want %dx%d", name, b.Dx(), b.Dy(), Width, Height)
	}
	// 1-bit PNG: bit depth byte in IHDR is at offset 24.
	if data[24] != 1 {
		t.Fatalf("%s: bit depth %d, want 1", name, data[24])
	}
	if dir := os.Getenv("DISPLAY_PREVIEW_DIR"); dir != "" {
		if err := os.WriteFile(filepath.Join(dir, name+".png"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRenderSchedule(t *testing.T) {
	for name, v := range sampleViews() {
		data, err := RenderSchedule(v)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		checkPNG(t, name, data)
	}
}

func TestRenderNotice(t *testing.T) {
	data, err := RenderNotice(NoticeView{
		Title:  "Waiting for approval",
		Lines:  []string{"An admin needs to approve this display", "in BookLab → Admin → Displays."},
		Code:   "7F3K2Q",
		Footer: "Checks again every minute",
	})
	if err != nil {
		t.Fatal(err)
	}
	checkPNG(t, "pending", data)
}

func TestRenderDeterministic(t *testing.T) {
	v := sampleViews()["in_use"]
	a, _ := RenderSchedule(v)
	b, _ := RenderSchedule(v)
	if !bytes.Equal(a, b) {
		t.Fatal("same view rendered to different bytes; filename hashing would churn")
	}
}
