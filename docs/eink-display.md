# E-ink door sign

BookLab can drive an e-ink display mounted outside the room. It shows whether the room is in use right now, today's hourly schedule, and a QR code that opens the booking page. BookLab acts as the display's server, so no TRMNL account or cloud service is involved.

It was built for the **Seeed Studio TRMNL 7.5" (OG) DIY Kit** (XIAO ESP32-S3 Plus, 800×480 black-and-white panel). Any device running the [TRMNL firmware](https://github.com/usetrmnl/trmnl-firmware) with an 800×480 panel should work.

## What the screen shows

- **Left panel:** the resource name, a big status banner, and details underneath:
  - **IN USE:** shows when the current booking ends and who booked it (first name and last initial only).
  - **OPEN:** shows how long the room stays free.
  - **CLOSED:** shows when it next opens, or a closure's reason.
- **Below that:** a QR code linking to `APP_URL`.
- **Right panel:** one row per hour of today's bookable hours:
  - **Booked:** a solid black row with the booker's first name and last initial.
  - **Open:** an outlined white row.
  - **Partial closure:** a hatched row.
  - **Past hours:** drawn lighter.
  - **The current hour:** marked with ◄.
- **After closing time, or on a closure day:** the timeline switches to the next day with bookable hours.

The sign never shows email addresses or full last names. Cancelled bookings never appear.

To see exactly what approved displays are showing, open **Admin → Displays → Preview**. You don't need any hardware for this.

## Setting up a display

1. **Make sure the device can reach BookLab.** The display needs to reach your BookLab server over HTTP(S), either at its public `APP_URL` or a LAN address. It only connects outward, so nothing needs to be opened up for it.

2. **Check the firmware.** New Seeed kits ship with TRMNL firmware. Seeed recommends firmware **1.5.12 or newer**. The *Custom Server* option first appeared in 1.4.6, so older firmware needs flashing first (see Seeed's [getting-started guide](https://wiki.seeedstudio.com/trmnl_7inch5_diy_kit_main_page/)).

3. **Run the device's Wi-Fi setup.** Power it on (or hold the reset button) to open its Wi-Fi setup portal and connect to the `TRMNL` network it creates. Enter your Wi-Fi details, choose **Custom Server**, and enter your BookLab base URL, for example `https://book.example.org`. Don't add `/api`; the firmware adds the paths itself.

4. **Note the code on screen.** The display registers itself and shows **Waiting for approval** with a six-character code, such as `7F3K2Q`.

5. **Approve it in BookLab.** Open **Admin → Displays**. The device appears as *Pending* with the same code. Check that the codes match, give it a name if you like, and click **Approve**. The schedule appears on the display's next check-in, within about a minute.

## Refresh and battery

The display spends most of its time asleep and wakes to ask BookLab what to show:

- **During bookable hours:** it wakes every *N* minutes, set under **Admin → Displays → Refresh during open hours** (default 5, allowed range 1-60). It also wakes just after each hour boundary and at booking start and end times, so the status and the ◄ marker change on time.
- **Outside bookable hours:** it checks in hourly, plus once 15 minutes before opening. A booking made overnight shows up within an hour.
- **Pending displays:** these check in every minute until approved.

When nothing has changed, the image name stays the same and the firmware skips the redraw. That saves battery and avoids e-ink flashing.

The admin page shows each display's battery voltage, Wi-Fi signal, firmware version and last check-in. Below about 3.5 V the battery is marked as low, and the sign shows a small battery icon in its bottom corner.

## Managing displays

- **Revoke:** the display shows *Display disabled* and checks back every few hours. Setup requests from its MAC address are refused, and image links it was given stop working. You can approve it again at any time.
- **Delete:** removes the record. If the device is still powered on, it re-registers as *Pending* on its next check-in.
- **Re-running setup on a device:** for example after a factory reset. The device gets a new key and drops back to *Pending*, so you need to approve it again. This also means anyone who spoofs a display's MAC address can't take over its slot without your approval.

At most 10 displays can be pending at once. Further setup attempts are refused until you approve, revoke or delete some.

## How it works

BookLab implements the parts of TRMNL's "bring your own server" protocol that the firmware uses:

| Endpoint | Purpose |
|---|---|
| `GET /api/setup` | First boot. The device sends its MAC address in the `ID` header and receives an API key and a friendly ID. The key is stored only as a SHA-256 hash. |
| `GET /api/display` | Every wake. The device sends `Access-Token`, `ID`, battery and Wi-Fi headers. The reply holds `image_url`, `filename` and `refresh_rate` (seconds to sleep). |
| `GET /api/display/image/{token}/{hash}.png` | The 1-bit 800×480 PNG. The firmware fetches it without headers, so a per-display random token in the path acts as the credential. The token changes when a display is revoked or set back to pending. |
| `POST /api/log` | Device logs. They are accepted and discarded. |

Things worth knowing if you're debugging:

- `image_url` is built from the scheme and host the device used to reach BookLab. Behind a reverse proxy, `X-Forwarded-Proto` is trusted only from `TRUSTED_PROXIES`. Caddy and nginx set it by default.
- BookLab never sends the firmware's `reset_firmware` instruction. On TRMNL firmware, a reset also wipes the Wi-Fi settings.
- If a device presents a key BookLab doesn't recognize (for example one issued by another server), BookLab registers it as *Pending* with that key, as long as its MAC address is new. If the MAC address is already registered, the device is told it isn't registered and keeps showing its code.
- The screen is drawn in Go (`internal/display`) using IBM Plex Mono, which is embedded in the binary and licensed under the SIL Open Font License (see `internal/display/fonts/OFL.txt`). No headless browser is used.
