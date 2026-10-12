# Zainium Installer — design

The installer that puts Zainium OS from the live ISO onto a disk. Written in Go: a
graphical front end (`zainium-installer`, runs as the live user) and an install engine
(`zainium-install`, runs as root). The engine is also usable on its own for scripted installs.

## What gets installed

The live ISO boots `zaisys/zairoot.img`, an erofs image holding the whole base:

```
/                       (erofs image root)
├── overlayer/syshub    base OS (immutable layer)
├── overlayer/zaisys    kernel, initramfs, Limine files, live-only tools
├── home/               live user's home skeleton
└── root/
```

On an installed system quantra-ramfs mounts `root=UUID=…` (ext4) and builds the same
overlay as on the live system, with the disk's `overlayer/zexlib/union` as the writable
layer. So an installed disk is:

```
GPT: 1 ESP (FAT32, 1 GiB) · 2 root (ext4, rest, label zainium-root) · 3 BIOS boot (the 1 MiB gap, Limine stage 2)

ESP (FAT32, 1 GiB)                       root (ext4, rest of disk, label zainium-root)
├── EFI/BOOT/BOOTX64.EFI   (Limine)      ├── overlayer/syshub   ← copied from erofs
├── limine.conf            (from         ├── overlayer/zaisys   ← copied from erofs
│                          limine.install.conf)                  (installer removed)
├── limine-bios.sys        (BIOS boot)   ├── overlayer/zexlib/{union,work,var}  empty
└── overlayer/zaisys/kernel/             ├── home/<user>
    ├── linux-evl                        └── root/
    ├── linux-panic
    └── quantra-ramfs.img
```

Limine only reads FAT, so the kernel and initramfs live on the ESP at the same paths the
`boot():/overlayer/zaisys/kernel/…` entries in `limine.install.conf` use.
`${ROOT_UUID}` and `${ROOT_FSTYPE}` in that template are filled in by the engine.

## Install steps (engine)

| # | Step | How |
|---|------|-----|
| 1 | Check plan | target is not the live medium, big enough, user/password/hostname valid |
| 2 | Partition | unmount anything on the disk, zero old tables, GPT written by the engine (ESP, root, BIOS boot), BLKRRPART |
| 3 | Format | FAT32 written by the engine itself (Zainium has no mkfs.vfat), `mkfs.ext4 -L zainium-root -U <uuid>` |
| 4 | Mount | root and ESP under `/run/zainium-install/` |
| 5 | Copy base | mount the live erofs read-only and copy it: owners, modes, symlinks, hard links, xattrs, times |
| 6 | Boot loader | ESP files above; `limine bios-install <disk> 3`; UEFI `Boot####` entry first in BootOrder (efivarfs) |
| 7 | Configure | see below |
| 8 | Finish | sync, unmount, report |

### Configure: base vs. union

The copied base (`overlayer/syshub`, `overlayer/zaisys`) stays the ISO image minus everything
live-only, so `zex syshub -u` upgrades and rollbacks work against a known base:

- owners: the build host's uid/gid (≥ 1000) leak into the erofs image; the copy gives them to root
- removed from zaisys and syshub: `bin/zainium-installer`, `bin/zainium-install`, `bin/carve`,
  `bin/cosmic-session-start` (live autologin launcher), the installer's desktop file, icon,
  polkit policy and autostart entry
- greetd `[initial_session]` (live autologin) stripped
- live user `zainium` removed from passwd/shadow/group, root locked, shadow 0600 — a zexlib reset
  can never bring back the ISO's well-known passwords

Machine settings go to `overlayer/zexlib/union`, exactly where they land when changed on the
running system: passwd/shadow/group with the new user (uid 1000, the live user's groups, wheel if
administrator), hostname + hosts, `localtime`, `locale.conf` + `profile.d/zainium-locale.sh` +
systemd `DefaultEnvironment=LANG`, `vconsole.conf`, COSMIC `xkb_config` (system default for the
greeter and new users). Non-Latin layouts get `us` as a second layout (Alt+Shift).

`home/<user>` is created from the live user's home, owned by the new user, mode 0700.

Not offered anywhere (languages, countries, time zones, keyboard layouts): Israel (IL, he_IL,
Asia/Jerusalem, Asia/Tel_Aviv, Israel, layout il). The engine rejects them too.

Progress is reported as JSON lines on stdout: `{"step":"copy","percent":42,"message":"…"}`,
and a final `{"done":true}` or `{"error":"…"}`.

## Privilege

The GUI never runs as root. When the user clicks **Begin Installation** it starts
`pkexec /overlayer/zaisys/bin/zainium-install --plan -` and writes the plan (JSON) to its
stdin. COSMIC's polkit agent asks for the password (no terminal needed). The polkit action
`tech.zainiumdynamics.installer` allows the active local session.

## User interface

Modelled on Fedora's Anaconda (hub and spoke), in a dark COSMIC style:

1. **Welcome** — Zainium logo, "Welcome to Zainium OS", language list with search,
   Continue.
2. **Installation Summary** (hub) — tiles, each with an icon, a title and its current
   value; required tiles that are not done are marked and keep **Begin Installation**
   disabled:
   - LOCALIZATION: Keyboard · Language · Time & Date
   - SYSTEM: Installation Destination · Host Name
   - USER SETTINGS: User Creation · Root Account
3. **Spokes** — one page per tile, header with the title, **Done** (back to the hub) and
   "ZAINIUM OS INSTALLATION".
4. **Installing** — progress bar, current step, details on demand.
5. **Complete** — Restart.

The engine is the source of truth for validation; the GUI only collects the plan.

Host name defaults to the detected device (DMI vendor + product, e.g. `lenovo-thinkpad-x1-carbon-gen-9`),
shown on the Host Name spoke and editable. Picking a language moves the keyboard and time zone
defaults until the user picks those; the keyboard choice is applied to the live COSMIC session at
once so passwords are typed in the layout used at login.

## Build

`make` builds `build/zainium-install` (static, CGO off) and `build/zainium-installer` (Gio,
Wayland only `-tags nox11`, musl-gcc, linked against Zainium's own libraries from `SYSHUB` with the
Zainium loader as interpreter). `make install DESTDIR=<zairoot>` puts both, the desktop file, icon
and polkit policy into `overlayer/zaisys` — live ISO only.
