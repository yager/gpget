<p align="center">
  <img src="docs/img/gpget-logo.png" alt="gpget" width="120">
</p>

# gpget

**English** | [日本語](README.ja.md)

Connect a GoPro to your computer with a USB cable and copy the shoot onto your
disk — **without taking out the microSD card or opening the battery door**.

> gpget is an independent tool and is not affiliated with, sponsored by, or
> endorsed by GoPro. "GoPro" is a trademark of GoPro, Inc.

---

## What it does

- **Only the new stuff** — copies what is not already in your destination, filed
  by capture date. Run it after every shoot
- **Resumes across runs** — Ctrl-C, a pulled cable, a closed laptop: run it again
  and it continues from the byte it stopped at, even in a later session
- **Transfers don't leave broken files** — each file is written under a temporary
  name and renamed only once every byte is in and the size checks out. An
  interruption leaves a `.part`, never a file that looks done but is not
- **4GB+ videos are fine** — each file is streamed straight to disk, never held
  in memory, no matter the size
- **Light and quick** — no cloud sync, no transcoding: it just moves files. With
  "only the new stuff" and autostart, clearing a shoot goes fast
- **Bursts, timelapses and intervals as one unit** — hundreds or thousands of
  frames land in one subfolder, in a single step
- **Chaptered MP4s renamed to sort right** — `GX012495.MP4`, `GX022495.MP4` …
  become `GX2495_01`, `GX2495_02`, so they line up in order (it does not join them)
- **Never touches the camera** — strictly read-only. Nothing on the card is
  deleted, renamed or moved, and the microSD and battery door stay shut

GoPro's own tools are the phone app and the web media library; there is no
desktop app from GoPro for this anymore. gpget fills that one gap — getting a
shoot onto a computer.

## Requirements

| | |
|---|---|
| Camera | **Verified: GoPro MISSION 1 PRO ILS**, plus early user reports on HERO13 Black. Other current GoPros are expected to work — see [Which cameras](#which-cameras) |
| Connection | A **USB-C cable that carries data** (not a charge-only one) |
| macOS | Apple Silicon, macOS 15 or newer |
| Windows | 64-bit, Windows 10 or newer |
| Linux | x86_64 or aarch64 |
| Note | **Lenses and capture modes do not matter.** gpget copies whatever the camera recorded |

**You never take the microSD card out.** It also works with a media mod attached,
as long as the side USB-C port passes data.

### Which cameras

Over the cable, gpget speaks the **Open GoPro HTTP API** — the same media-list
and file-download requests the camera already answers for GoPro's own software.
GoPro documents that API as working on **HERO9 through HERO13 Black, HERO11 Black
Mini, MISSION 1, MISSION 1 PRO, MAX2 and LIT HERO**
([compatibility list](https://gopro.github.io/OpenGoPro/docs)), so gpget should
run on any of them.

Tested here: the **MISSION 1 PRO ILS** only. **HERO13 Black** has early reports
from users. The rest are unconfirmed — if you try one,
[tell us how it went](https://github.com/yager/gpget/issues). Keep the camera on
current firmware. LIT HERO turns on USB access a different way and may need
changes.

---

## Install

Grab one binary from [Releases](https://github.com/yager/gpget/releases).
There is no runtime to install.

> **Do not download it with a browser.** On macOS, files fetched by a browser are
> quarantined, and Gatekeeper blocks gpget because it is not signed. The `curl`
> commands below avoid that entirely.

### macOS (Apple Silicon, macOS 15 or newer)

```bash
mkdir -p ~/bin
curl -L -o ~/bin/gpget https://github.com/yager/gpget/releases/latest/download/gpget-darwin-arm64
chmod +x ~/bin/gpget
echo 'export PATH="$HOME/bin:$PATH"' >> ~/.zshrc && source ~/.zshrc
gpget version
```

### Linux (x86_64 / aarch64)

```bash
mkdir -p ~/bin
curl -L -o ~/bin/gpget https://github.com/yager/gpget/releases/latest/download/gpget-linux-amd64
chmod +x ~/bin/gpget
export PATH="$HOME/bin:$PATH"
gpget version
```

On aarch64 (a Raspberry Pi, for example) change `gpget-linux-amd64` in the URL to
`gpget-linux-arm64`. `uname -m` tells you which one you need.

### Windows (64-bit, Windows 10 or newer)

In PowerShell:

```powershell
mkdir "$env:USERPROFILE\bin" -Force
curl.exe -L -o "$env:USERPROFILE\bin\gpget.exe" https://github.com/yager/gpget/releases/latest/download/gpget-windows-amd64.exe
$env:Path += ";$env:USERPROFILE\bin"
gpget version
```

That `$env:Path` change only lasts for the current window. To make it permanent,
add `%USERPROFILE%\bin` to `Path` in your system environment variables.

### Building from source

```bash
git clone https://github.com/yager/gpget && cd gpget
go build -o gpget .
```

Go 1.21 or newer. On macOS, build with cgo enabled (`CGO_ENABLED=1`, the default):
notifications and USB watching need it.

---

## Usage

### The short version

```bash
gpget init     # set the destination and a few options (first run only)
gpget sync     # plug the camera in and run this; only new media is copied
```

### Commands

```
gpget [sync]      incremental offload into dated folders (no argument = sync)
    --dest <dir> --since <date> --date <date> --label <s>
    --type mp4,jpg --sidecars gpr,lrv --dry-run -y --ip <addr>

gpget list        an ls -l style listing (one line per file; a group is one line)
    --expand --new --date <date> --since <date> --video --photo --json --ip <addr>

gpget status      card summary (counts and size by date) + how much is not
                  offloaded yet + leftover .part files

gpget get <name|glob>...   cherry-pick by name or pattern

gpget init        interactive setup, writes config.ini
gpget config get <key> | set <key> <val> | path | edit

gpget autostart install | uninstall | status

gpget probe       connection diagnostics
```

### What `list` looks like

```
TYPE  DATE              SIZE   NAME                DEST
V     2026-09-04 14:32  2.4M   GX010014.MP4        -
P     2026-09-04 14:35  2.1M   GP010009.JPG(+GPR)  ✓
G     2026-09-03 08:00  20.1G  [interval ×1000]    GPAA0015…
```

Long listings are printed as-is — pipe them through `less`, `grep` or `awk`.
gpget has no pager of its own.

---

## Destination and folder names

Files land in `<dest>/<folder>/<file>`; group frames in
`<dest>/<folder>/<group_dir>/<frame>`.

By default everything is filed by capture date:

```
<videos dir>/GoPro/          # macOS ~/Movies, Windows ~\Videos, Linux ~/Videos
├── 2026-09-04_GoPro/
│   ├── GX2495_01.MP4
│   ├── GX2495_02.MP4
│   ├── GP010009.JPG
│   ├── GP010009.GPR
│   └── GPAA0015/          ← a 20-frame burst
│       ├── GPAA0015.JPG
│       └── …
└── 2026-09-05_GoPro/
```

**The folder date is the camera's own clock, exactly as it recorded it.** After a
transfer, gpget also sets each file's modification time back to the capture time.

### Destinations gpget refuses

For safety, these are **rejected** before a transfer starts:

- **Cloud-synced folders** (iCloud, Google Drive, Dropbox, OneDrive)
- **Network volumes** (SMB, NFS)
- Read-only drives
- Drives without enough free space
- FAT32 / FAT16 when a file would exceed the volume's single-file limit

Large video files in a sync folder cause trouble often enough to be worth blocking.
An external SSD is a good choice.

---

## Configuration

Most people never open this — `gpget init` sets the essentials and the defaults
are fine. This is the reference for when you want to change something.

### Where it lives

One config file, in the usual place for your OS. The default destination is
per-OS too:

| OS | Config file | Default destination |
|---|---|---|
| macOS | `~/Library/Application Support/gpget/config.ini` | `~/Movies/GoPro` |
| Windows | `%AppData%\gpget\config.ini` | `%USERPROFILE%\Videos\GoPro` |
| Linux | `${XDG_CONFIG_HOME:-~/.config}/gpget/config.ini` | `$XDG_VIDEOS_DIR/GoPro` (default `~/Videos/GoPro`) |

`gpget config path` prints the exact path. `--config <path>` points at a
different file; nothing else is read. Order of precedence: a `--flag` beats
`config.ini`, which beats the built-in default.

### config.ini

```ini
[general]
dest       = /Users/you/Movies/GoPro   ; under the OS videos folder (gpget init writes the real path)
folder     = {date:%Y-%m-%d}_GoPro     ; one subfolder per capture date
group_dir  = {stem}                    ; subfolder for a group (empty = directly under the date)
sidecars   = gpr,lrv                   ; none | gpr,lrv | all
overwrite  = skip                      ; skip | rename | replace
confirm    = true
timezone   = camera                    ; camera = the camera's own time; or +9 / -05:30 to correct a wrong clock

[chapters]
regroup      = multi                   ; always | multi | never
chapter_name = {prefix}{clip}_{chapter:02d}   ; → GX2495_01.MP4, GX2495_02.MP4 …

[camera]
ip =                                   ; empty = auto-discover

[autostart]
mode = notify                          ; notify | auto
```

### Templates

`{token}` substitution. `{date:...}` takes strftime specifiers.

- In `folder` and `group_dir`: `{date:%Y-%m-%d}`, `{label}` (from `--label`),
  `{model}`, `{cam}`, `{stem}` (a group's base filename, e.g. `GPAA0015`)
- In `chapter_name`: `{prefix}` (`GX` etc.), `{clip}` (last 4 digits),
  `{chapter:02d}`, `{date:...}`
- Output path: `<dest>/<folder>/<file>`, group frames
  `<dest>/<folder>/<group_dir>/<frame>`
- **What `{date:...}` is based on:** the camera's own clock, as it recorded it.
  That is right as long as the camera's clock is set to your local time. If it
  was wrong, or set for another time zone, correct it with an offset like
  `timezone = +9`.

### `gpget init`

```
$ gpget init
Destination base directory                [<OS videos folder>/GoPro]:
Dated folder template                     [{date:%Y-%m-%d}_GoPro]:
Camera clock offset (camera / +9 etc.)    [camera]:
Subfolder for groups                      [{stem}]:
Also fetch sidecars (gpr,lrv / none / all)[gpr,lrv]:
Existing files (skip/rename/replace)      [skip]:
Chapter renaming (always/multi/never)     [multi]:
Confirm before transferring (y/n)         [y]:

→ saved to ~/Library/Application Support/gpget/config.ini
```

---

## Autostart on connect

Optional, and **off until you run `gpget autostart install`**. Once enabled,
gpget runs on its own whenever you plug the camera in, so offloading is not
something you have to remember.

What it does on connect is set by `[autostart] mode` in the config:

- **`notify`** (the default) — checks what has not been offloaded yet and shows a
  desktop notification. It transfers nothing; you still run `gpget sync` when you
  want the files
- **`auto`** — transfers everything new with no prompt, then notifies you when it
  finishes or if it fails

Either way, if the camera has nothing new or does not answer, gpget does nothing
— plugging in only to charge is harmless. Repeated firing on one connection is
suppressed until you unplug.

How it detects the camera, and what it needs your permission for, differ by OS —
macOS especially has rough edges. The sections below cover each one.

```bash
gpget autostart install       # set it up for your OS
gpget autostart status        # on/off, mode, whether a transfer is running now
gpget autostart log           # recent log lines
gpget autostart log --follow  # keep watching the log
gpget autostart uninstall
gpget autostart install --print   # print the trigger definition instead of installing
```

### macOS

`gpget autostart install` creates **`~/Applications/gpget.app`** and registers it
to run in the background. This is not a second copy to install — it is just how
the background service has to be packaged on macOS. (The camera connection and
notifications need an *app* for macOS to grant permission to; a plain
command-line binary cannot receive it. It is built from the gpget you already
have.)

It shows no window and no Dock icon. A compact **menu-bar indicator** shows an
action-cam icon when idle and a two-line file count while transferring (click
for Sync Now / Open Destination / Show Log / About gpget / Quit gpget; hover
for a `gpget — …` tooltip). It reacts within a second or two of the cable
going in. Deleting the app is safe — `gpget autostart install` builds it again.

Quitting from the menu (or `gpget autostart pause`) only stops gpget for the
current login session — it comes back on its own next time you log in.
`gpget autostart resume` restarts it right away without reinstalling, and
`gpget autostart uninstall` removes autostart entirely.

#### The first time: allow two permissions

**Local network** — an ordinary dialog. Click **Allow**.

**Notifications** — a **banner** at the top right, titled "gpget", saying that
notifications may include text, sounds and icon badges. It reads like an
announcement, but it is the permission request:

- Move the pointer over it, open the **"Options"** menu, and choose **Allow**
- Clicking the banner itself only opens System Settings; it does not allow
  anything

`gpget autostart install` finishes by sending a test notification and tells you
whether it actually went through, so you can tell "allowed" apart from "silently
off". If it says gpget does not have permission, see
[When something goes wrong](#when-something-goes-wrong).

- Notifications come from gpget itself, so it shows up as "gpget" in
  System Settings > Notifications. **No Homebrew package or other extra install
  is needed**
- In `auto` mode a **menu-bar indicator** shows an action-cam icon when idle and
  a compact two-line file count while transferring; desktop notifications fire
  only for **complete** and **fail** (and for connection errors). Per-file lines
  still go to the log
- **Failures are always notified.** With no window, finishing quietly would be
  indistinguishable from success

### Windows and Linux — handled in the background

`gpget autostart install` registers a scheduled task (Task Scheduler on Windows,
a systemd user timer on Linux) that checks for the camera once a minute and acts
on the `mode` above. Nothing is shown while it waits. Notifications are a
PowerShell toast on Windows and `notify-send` on Linux (which needs a
notification daemon); if neither works, the message goes to the log.

**This has not been tested on real Windows or Linux hardware yet.** Manual `sync`
and `get` use the same code and are fine; only `autostart` is unverified. If you
try it, please [report what happened](https://github.com/yager/gpget/issues) —
the steps are in [docs/testing.md](docs/testing.md). `gpget autostart install
--print` shows the exact task it would register, if you would rather set it up by
hand.

### The log

`gpget autostart log` shows it (`--follow` to keep watching); `gpget autostart
status` says whether a transfer is running right now.

| OS | Log file |
|---|---|
| macOS | `~/Library/Logs/gpget-autostart.log` |
| Windows | `%LOCALAPPDATA%\gpget\autostart.log` |
| Linux | `${XDG_STATE_HOME:-~/.local/state}/gpget/autostart.log` |

---

## Updating and uninstalling

### Update

Re-download the binary for your platform — it overwrites in place. Your config is
untouched.

#### macOS

```bash
curl -L -o ~/bin/gpget https://github.com/yager/gpget/releases/latest/download/gpget-darwin-arm64
chmod +x ~/bin/gpget
gpget version
```

#### Linux

```bash
curl -L -o ~/bin/gpget https://github.com/yager/gpget/releases/latest/download/gpget-linux-amd64
chmod +x ~/bin/gpget
gpget version
```

On aarch64, use `gpget-linux-arm64`.

#### Windows (PowerShell)

```powershell
curl.exe -L -o "$env:USERPROFILE\bin\gpget.exe" https://github.com/yager/gpget/releases/latest/download/gpget-windows-amd64.exe
gpget version
```

If it fails with the file in use, the autostart task is holding `gpget.exe` — run
`gpget autostart uninstall` first, then re-download.

**If you use autostart**, run `gpget autostart install` again afterwards: gpget
copies itself into the thing that runs on connect, and that copy has to be
refreshed.

### Uninstall

```bash
gpget autostart uninstall   # only if you turned it on
rm ~/bin/gpget
```

That removes the autostart trigger and, on macOS, the app bundle it built. Your
config and everything you already offloaded stay. To remove the config too,
delete the folder `gpget config path` points at:

| OS | Configuration |
|---|---|
| macOS | `~/Library/Application Support/gpget/` |
| Windows | `%AppData%\gpget\` |
| Linux | `~/.config/gpget/` |

---

## When something goes wrong

**The camera is not found**

```bash
gpget probe        # what it found: the network and IP, the camera's info, and whether the file list loads
```

- **Check that the cable carries data.** With a charge-only cable the camera says
  "USB connected" while the computer sees nothing
- Check that the camera is powered on
- `gpget --ip <addr>` lets you point at it directly

**A Docker or VPN address collides**

The camera shows up in the `172.16`–`172.31` private range, and Docker bridges and
VPNs sometimes use the same range. gpget checks each candidate is actually a GoPro
before using it, but if more than one qualifies, name the right one with `--ip`.

**The destination is rejected**

See "Destinations gpget refuses" above. Point it at an external drive or a local
disk.

**A transfer stopped partway**

```bash
gpget status       # finds and reports the .part files left behind
gpget sync         # run it again and it resumes
```

gpget never deletes a `.part` on its own. Use `--clean` when you want them gone.

**GoPro Labs**

With Labs **Altered File Naming** (basename) enabled, listing and offloading over
USB may fail. Labs itself says renamed files are for **direct SD-card copies
only** — not USB transfer, Quik, or the cloud. There are reports that returning
to stock firmware restored `gpget list` / `sync`. gpget talks to the camera over
the Open GoPro HTTP API (USB network). If you have renamed files with Labs,
switch back to the default naming, or copy from the card with a reader.

### Autostart on macOS

**No notification ever appears**

Three things cause this, in rough order of likelihood.

**1. The permission banner was missed.** It disappears after about a minute, and
letting it expire counts as a refusal — macOS will not ask again. Turn gpget on
by hand:

```
System Settings > Notifications > gpget
```

**2. Your screen is being recorded or shared.** macOS treats screen recording
(QuickTime Player, any capture app) the same as sharing your display and
**suppresses banners** so they cannot end up in the recording. gpget looks
broken: the notification is delivered and lands in Notification Center, but
nothing appears. Turn on **System Settings > Notifications > Allow notifications
when mirroring or sharing the display** — the bottom section, off by default.

**3. Something else.** Ask macOS directly:

```bash
"$HOME/Applications/gpget.app/Contents/MacOS/gpget" autostart test-notify
```

Run the copy inside the bundle, not the one on your PATH — only that one can
post as gpget. It prints what macOS itself reports, so you do not have to guess:

```
authorizationStatus  2   (2 = allowed)
alertSetting         2   (2 = enabled)
alertStyle           1   (1 = banner)
```

**Autostart stops working after an update**

Updating gpget changes its code signature, and macOS can treat the new signature
as a different app. Usually the agent retries and recovers on its own — the log
(`gpget autostart log`) shows the retries.

More rarely, System Settings > Local Network ends up with several entries of the
same name, all switched on, and connections keep being refused anyway.
**gpget cannot prevent or repair that**; recovering from it needs Recovery mode.

---

## Documentation

| | |
|---|---|
| [docs/design.md](docs/design.md) | Why gpget is built this way, what is in and out of scope, requirements, the safety model |
| [docs/gopro-api.md](docs/gopro-api.md) | The GoPro local API, and what was measured on an ILS |
| [docs/status.md](docs/status.md) | What is implemented, what is verified on hardware, what is not |
| [docs/testing.md](docs/testing.md) | How to try gpget on Windows or Linux, and how to report what happened |
| [docs/release.md](docs/release.md) | Build and release procedure (for maintainers) |
| [tools/probe_group.py](tools/probe_group.py) | Script for measuring the camera's API behaviour |

---

## License

[MIT](LICENSE)
