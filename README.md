# gpget

**English** | [日本語](README.ja.md)

A CLI that pulls media off a GoPro **without removing the microSD card or opening
the battery door**. It talks to the local HTTP API that GoPro already exposes,
over a wired USB connection.

> gpget is an independent tool and is not affiliated with, sponsored by, or
> endorsed by GoPro. "GoPro" is a trademark of GoPro, Inc.

---

## What it does

- **Incremental offload** — copies only the media that is not already in your
  destination, into folders by capture date
- **Resumes after an interruption** — Ctrl-C, a pulled cable: run it again and it
  picks up where it stopped
- **Renames chaptered MP4s** — so they sort correctly by name (it does not join them)
- **Treats bursts, intervals and timelapses as one unit** — hundreds or thousands
  of frames go into a single subfolder
- **Never writes to the camera** — strictly read-only; there is no delete code

## Requirements

| | |
|---|---|
| Camera | Verified on the **GoPro MISSION 1 PRO ILS**. Other MISSION 1 models, HERO11–13 and MAX2 should work if they expose `/gopro/media/list` over the wire |
| Connection | A **USB-C cable that carries data**. The camera appears as a wired network interface |
| OS | macOS / Windows / Linux |
| Note | **Lenses and capture modes are irrelevant.** gpget copies whatever the camera recorded |

**You never have to take the microSD card out.** It also works with a media mod
attached, as long as the side USB-C port passes data.

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

### Supported targets

| OS | Architecture | Minimum |
|---|---|---|
| macOS | Apple Silicon | macOS 15 |
| Windows | x64 | Windows 10 |
| Linux | x86_64 / aarch64 | systemd (only for autostart) |

**Autostart has not been tested on real Windows or Linux machines.** Manual `sync`
and `get` go through the same code, but `autostart` is unverified there. If you
try it, please report what happened in
[Issues](https://github.com/yager/gpget/issues) — the steps are in
[docs/testing.md](docs/testing.md).

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

**The date comes from the camera's own wall clock**, exactly as recorded. After a
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

### Where it lives

`os.UserConfigDir()` + `/gpget/config.ini`. The default destination is
OS-specific too:

| OS | Config file | Default destination |
|---|---|---|
| macOS | `~/Library/Application Support/gpget/config.ini` | `~/Movies/GoPro` |
| Windows | `%AppData%\gpget\config.ini` | `%USERPROFILE%\Videos\GoPro` |
| Linux | `${XDG_CONFIG_HOME:-~/.config}/gpget/config.ini` | `$XDG_VIDEOS_DIR/GoPro` (default `~/Videos/GoPro`) |

`--config <path>` overrides it. No other config file is read — there is exactly
one location. Resolution order: `--flag` > `config.ini` > built-in default.
`gpget config path` prints the real path.

### config.ini

```ini
[general]
dest       = /Users/you/Movies/GoPro   ; under the OS videos folder (gpget init writes the real path)
folder     = {date:%Y-%m-%d}_GoPro     ; one subfolder per capture date
group_dir  = {stem}                    ; subfolder for a group (empty = directly under the date)
sidecars   = gpr,lrv                   ; none | gpr,lrv | all
overwrite  = skip                      ; skip | rename | replace
confirm    = true
timezone   = camera                    ; camera (no conversion) or +9 / -05:30 to correct a wrong clock

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
  `{model}`, `{cam}`, `{stem}` (the group's representative filename stem)
- In `chapter_name`: `{prefix}` (`GX` etc.), `{clip}` (last 4 digits),
  `{chapter:02d}`, `{date:...}`
- Output path: `<dest>/<folder>/<file>`, group frames
  `<dest>/<folder>/<group_dir>/<frame>`
- **What `{date:...}` is based on:** GoPro's `cre` is the camera's wall clock,
  encoded as if it were UTC. gpget uses those digits without converting them, so
  the result is right whenever the camera clock is set to local time. Use an
  offset like `timezone = +9` only when the clock was wrong or set for another
  time zone.

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

```bash
gpget autostart install       # register the trigger for your OS
gpget autostart status        # enabled/disabled, mode, whether a transfer is running
gpget autostart log           # tail of the log (current file and .old together)
gpget autostart log --follow  # keep following it
gpget autostart uninstall
gpget autostart install --print   # just print what would be registered
```

### macOS — no window, only a notification

A background LaunchAgent is silently denied access to the camera (172.x) by
macOS Local Network privacy. No permission prompt appears, and flipping the
toggle in System Settings does not help a process started by launchd — **a bare
executable has no app identity for macOS to attach the permission to.**

So gpget splits the work in two:

1. **The LaunchAgent** (every 5 seconds) **never touches the network.** It only
   checks whether a GoPro-shaped wired interface has appeared (under 10 ms per
   run). If the interface is not up yet, it asks `ioreg` whether a GoPro is
   plugged in and, if so, waits up to 30 seconds for it
2. When it finds one, it launches
   `~/Library/Application Support/gpget/gpget.app`. That is what actually talks
   to the camera — **it has an identity, so the connection is allowed**

That `.app` is marked `LSUIElement`, so **no window, no Dock icon, no menu bar
item**. Inside it is the gpget binary itself; `gpget autostart install` assembles
the bundle and ad-hoc signs it. What you download is still a single binary.

The first time, macOS asks whether to allow notifications and local network
access. Allow both. `gpget autostart install` ends by sending a test
notification: **make sure you actually see that banner** — without notifications
you cannot tell what happened. If it does not appear, turn gpget on in
System Settings > Notifications.

| mode | behaviour |
|---|---|
| **`notify`** (default) | counts what is not offloaded yet and notifies. Transfers nothing |
| **`auto`** | transfers. Progress replaces the same notification (about every 10 files or 15 seconds), then becomes the completion or failure message. Per-file detail goes to the log |

- Notifications are posted by gpget.app itself (`UserNotifications`), so it shows
  up as "gpget" in System Settings > Notifications. **No Homebrew package or
  other extra install is needed**
- **Failures are always notified.** With no window, finishing quietly would be
  indistinguishable from success
- The app lives at `~/Library/Application Support/gpget/gpget.app` and is rebuilt
  on reinstall
- Read the log with `gpget autostart log` (current file plus one `.old`
  generation, rotated at about 1 MB). `gpget autostart status` prints its path
- `gpget autostart status` also tells you whether a transfer is in progress, by
  looking at `.gpget.lock` in the destination

**A known macOS bug:** updating gpget changes its code signature, and the first
connection right after an update is sometimes refused. gpget retries quietly and
recovers (up to three attempts). More rarely, System Settings > Local Network
ends up with several entries of the same name, all switched on, yet connections
keep being denied. Recovering from that needs Recovery mode; gpget cannot prevent
it.

### Windows and Linux — handled in the background

There is no privacy gate here, so the trigger (Task Scheduler or a systemd user
timer, both polling once a minute) runs `autostart run` directly: it counts
`media/list` on the spot and raises a desktop notification.
**This is unverified** — `--print` emits the trigger definition if you would
rather register it by hand.

| mode | behaviour |
|---|---|
| **`notify`** (default) | counts what is not offloaded yet and notifies. Transfers nothing |
| **`auto`** | transfers, replacing the same notification as it goes, then completion or failure. Per-file detail goes to the log |

Notifications use a WinRT toast via PowerShell on Windows and `notify-send` on
Linux (which needs a notification daemon). If neither works, the message goes to
stderr.

### Both platforms

- If nothing new is on the camera, or the camera does not answer, gpget does
  nothing. Plugging in "just to charge" is harmless
- Repeated firing on one connection is suppressed with a state file
  (`os.UserCacheDir()/gpget/autostart*.state`). Unplugging clears it, so the next
  connect fires again
- To check a transfer: `gpget autostart status` (the lock) and
  `gpget autostart log` (`--follow` to keep watching)

| OS | Log file |
|---|---|
| macOS | `~/Library/Logs/gpget-autostart.log` |
| Windows | `%LOCALAPPDATA%\gpget\autostart.log` |
| Linux | `${XDG_STATE_HOME:-~/.local/state}/gpget/autostart.log` |

---

## When something goes wrong

**The camera is not found**

```bash
gpget probe        # shows the interface and IP it found, camera/info, and whether media/list responds
```

- **Check that the cable carries data.** With a charge-only cable the camera says
  "USB connected" while the computer sees nothing
- Check that the camera is powered on
- `gpget --ip <addr>` lets you point at it directly

**A Docker or VPN address collides**

The camera shows up in the `172.16`–`172.31` private range, and Docker bridges and
VPNs sometimes use the same range. gpget confirms each candidate by asking
`camera/info` whether it is a GoPro, but if several still qualify, name the right
one with `--ip`.

**The destination is rejected**

See "Destinations gpget refuses" above. Point it at an external drive or a local
disk.

**A transfer stopped partway**

```bash
gpget status       # finds and reports the .part files left behind
gpget sync         # run it again and it resumes
```

gpget never deletes a `.part` on its own. Use `--clean` when you want them gone.

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
