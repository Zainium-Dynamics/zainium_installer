# Builds both binaries for Zainium OS (x86_64 musl).
#   make            → build/zainium-install (static) and build/zainium-installer (Wayland, cgo)
#   make install DESTDIR=<zairoot>  → puts them and their data into overlayer/zaisys
# The GUI links against Zainium's own libraries from SYSHUB and the host's -devel headers.

SYSHUB  ?= /home/alizain/os/zairoot/overlayer/syshub
MUSLDIR  = $(SYSHUB)/x86_64-zainium-linux-musl
LOADER   = /overlayer/syshub/x86_64-zainium-linux-musl/lib/ld-musl-x86_64.so.1
BUILD    = build
SR       = $(abspath $(BUILD))/sysroot
GOFLAGS  = -trimpath

.PHONY: all engine gui test sysroot install clean

all: engine gui

engine:
	CGO_ENABLED=0 go build $(GOFLAGS) -ldflags='-s -w' -o $(BUILD)/zainium-install ./cmd/zainium-install

test:
	go test $$(go list ./internal/... | grep -v /ui)

# headers from the build host, libraries from Zainium
sysroot:
	rm -rf $(SR) && mkdir -p $(SR)/include $(SR)/lib $(SR)/pkgconfig
	cp -r /usr/include/wayland-client*.h /usr/include/wayland-util.h /usr/include/wayland-version.h \
	      /usr/include/wayland-cursor.h /usr/include/wayland-egl*.h /usr/include/xkbcommon \
	      /usr/include/EGL /usr/include/KHR /usr/include/vulkan /usr/include/vk_video $(SR)/include/
	for l in wayland-client.so.0 wayland-cursor.so.0 wayland-egl.so.1 EGL.so.1 xkbcommon.so.0; do \
	  test -e $(SYSHUB)/lib/lib$$l || { echo "missing $(SYSHUB)/lib/lib$$l"; exit 1; }; \
	  ln -sf $(SYSHUB)/lib/lib$$l $(SR)/lib/lib$${l%%.so*}.so; done
	for p in wayland-client wayland-cursor wayland-egl egl xkbcommon; do \
	  lib=$$p; [ $$p = egl ] && lib=EGL; \
	  printf 'Name: %s\nDescription: %s\nVersion: 99\nCflags: -I%s\nLibs: -L%s -l%s\n' $$p $$p $(SR)/include $(SR)/lib $$lib > $(SR)/pkgconfig/$$p.pc; done

gui: sysroot
	CGO_ENABLED=1 CC=musl-gcc \
	PKG_CONFIG_PATH= PKG_CONFIG_LIBDIR=$(SR)/pkgconfig \
	CGO_LDFLAGS='-Wl,--dynamic-linker=$(LOADER) -Wl,--allow-shlib-undefined -Wl,-rpath-link=$(SYSHUB)/lib:$(MUSLDIR)/lib' \
	go build $(GOFLAGS) -tags nox11 -ldflags='-s -w -linkmode external' -o $(BUILD)/zainium-installer ./cmd/zainium-installer

# DESTDIR is a zairoot staging tree; everything goes to zaisys (live ISO only, the engine removes it on install)
install: all
	install -Dm755 $(BUILD)/zainium-install   $(DESTDIR)/overlayer/zaisys/bin/zainium-install
	install -Dm755 $(BUILD)/zainium-installer $(DESTDIR)/overlayer/zaisys/bin/zainium-installer
	install -Dm644 data/tech.zainiumdynamics.Installer.desktop $(DESTDIR)/overlayer/zaisys/share/applications/tech.zainiumdynamics.Installer.desktop
	install -Dm644 data/tech.zainiumdynamics.Installer.svg $(DESTDIR)/overlayer/zaisys/share/icons/hicolor/scalable/apps/tech.zainiumdynamics.Installer.svg
	install -Dm644 data/tech.zainiumdynamics.installer.policy $(DESTDIR)/overlayer/zaisys/share/polkit-1/actions/tech.zainiumdynamics.installer.policy

clean:
	rm -rf $(BUILD)
