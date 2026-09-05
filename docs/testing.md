# Testing gpget on Windows or Linux

**English** | [日本語](testing.ja.md)

gpget copies photos and videos off a GoPro over USB.
**It has never been run on a real Windows or Linux machine.** Development
happens on a Mac and there is no other hardware here to check against — which is
what this document is asking for help with.

It is fine if it does not work. **A report that it did not work is exactly what
is useful.**

- Takes 15–30 minutes
- You need a GoPro you can connect over USB, a cable, and some free disk space
- **Nothing on the camera is modified.** gpget is strictly read-only and never deletes

---

## 1. Install

### Windows (10 or 11, 64-bit)

Open PowerShell and run these one at a time:

```powershell
mkdir "$env:USERPROFILE\bin" -Force
curl.exe -L -o "$env:USERPROFILE\bin\gpget.exe" https://github.com/yager/gpget/releases/latest/download/gpget-windows-amd64.exe
$env:Path += ";$env:USERPROFILE\bin"
gpget version
```

Success looks like `gpget v0.1.0`.

> **That `$env:Path` change only applies to this window.** It is gone when you
> open a new PowerShell. To make it permanent, add `%USERPROFILE%\bin` to `Path`
> in your system environment variables — but you do not need to for this test.

> Windows may warn that the publisher cannot be verified.
> **Whether that happens is itself something worth reporting.** If you see it,
> please tell us what the dialog said.

### Linux (x86_64)

```bash
mkdir -p ~/bin
curl -L -o ~/bin/gpget https://github.com/yager/gpget/releases/latest/download/gpget-linux-amd64
chmod +x ~/bin/gpget
export PATH="$HOME/bin:$PATH"
gpget version
```

### Linux (aarch64, e.g. a Raspberry Pi)

Change `gpget-linux-amd64` in the URL above to **`gpget-linux-arm64`**.
`uname -m` tells you which you need (`x86_64` → amd64, `aarch64` → arm64).

---

## 2. First-time setup

```
gpget init
```

It asks a few questions — the important one is the **destination folder**. Pick
somewhere with free space. The rest can stay at their defaults.

---

## 3. What to try

### (1) Is the camera detected?

Turn the GoPro on and connect it over USB. **Wait a few seconds**, until the
camera itself shows something like "USB connected", then run:

```
gpget probe
```

If it works you get the model, serial, firmware and so on. **If this step fails,
nothing after it will work** — please report it at that point.

### (2) Can it see what is on the card?

```
gpget status
gpget list
```

### (3) Can it transfer?

```
gpget sync
```

Answer `y` when asked. Progress is printed per file, and it ends with something
like `transferred N, skipped 0, failed 0`.

**Please also check that the transferred files actually open** — that photos
display and videos play. A file with the right size but broken contents is
exactly the kind of bug worth catching.

### (4) Autostart (the part we are least sure about)

```
gpget autostart install
gpget autostart status
```

Once it is installed, **unplug the camera, wait about ten seconds, and plug it
back in.**

- Windows: does a transfer start, or a notification appear, within a minute?
- Linux: same. With no desktop environment there is no notification, so it only
  goes to the log

After waiting a while:

```
gpget autostart log
```

If anything was recorded, please include it. **If nothing happened at all, that
is still worth reporting.**

You can undo this when you are done:

```
gpget autostart uninstall
```

---

## 4. How to report

**Run these four and paste the output**, as far as each one runs. If a command
failed, include its error message too.

```
gpget version
gpget probe
gpget autostart status
gpget autostart log
```

Please also include:

- **Your OS and version**
  - Windows: what `winver` shows, or Settings > System > About
  - Linux: `uname -a` and the first line of `/etc/os-release`
- **Which GoPro** you used
- **What you did and what happened** — specifically where it differed from what
  you expected

There is an issue form that asks for all of this:
[Test report](https://github.com/yager/gpget/issues/new?template=test-report.yml).

### Where the log file lives

If `gpget autostart log` does not work, read the file directly.

| OS | Path |
|---|---|
| Windows | `%LOCALAPPDATA%\gpget\autostart.log` |
| Linux | `~/.local/state/gpget/autostart.log` |
| macOS | `~/Library/Logs/gpget-autostart.log` |

### Especially useful to know

- **Where the instructions tripped you up** — a command that did not run, wording
  that was unclear
- **Any security or permission dialog**, and what it said
- **Whether autostart worked at all.** Windows uses Task Scheduler and Linux a
  systemd user timer; neither has ever been verified
- **Whether any text was garbled**

---

## 5. About safety

- **Files on the camera are only read.** Nothing is deleted, changed or renamed
- Files already in the destination are **not overwritten** by default
- A transfer is written to a temporary `.part` file and only gets its real name
  once every byte has arrived. An interrupted transfer never leaves a
  half-written file looking finished
- Nothing is sent to any external server. The only network traffic is to the
  camera

To uninstall, delete the binary you downloaded (run `gpget autostart uninstall`
first). The config file lives in `%APPDATA%\gpget` on Windows and
`~/.config/gpget` on Linux.
