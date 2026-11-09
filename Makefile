# Builds both binaries for Zainium OS (x86_64 musl).
#   make            → build/zainium-install (static) and build/zainium-installer (Wayland, cgo)
#   make install DESTDIR=<zairoot>  → puts them and their data into overlayer/zexlib/union

SYSHUB  ?= /overlayer/syshub
MUSLDIR  = $(SYSHUB)/x86_64-zainium-linux-musl
LOADER   = $(MUSLDIR)/lib/ld-musl-x86_64.so.1
CC       = $(SYSHUB)/bin/gcc
SPECS    = $(wildcard $(MUSLDIR)/lib/musl-gcc.specs)
BUILD    = build
GOFLAGS  = -trimpath

.PHONY: all engine gui test install clean

all: engine gui

engine:
	CGO_ENABLED=0 go build $(GOFLAGS) -ldflags='-s -w' -o $(BUILD)/zainium-install ./cmd/zainium-install

test:
	go test $$(go list ./internal/... | grep -v /ui)

$(BUILD)/pkg-config:
	mkdir -p $(BUILD)
	printf '#!/bin/sh\n\
	if printf "%%s\n" "$$@" | grep -q -- "--cflags"; then\n\
		exec $(SYSHUB)/bin/pkg-config "$$@"\n\
	else\n\
		exec $(SYSHUB)/bin/pkg-config "$$@" | sed \\\n\
			-e "s/-lEGL/-l:libEGL.so.1/g" \\\n\
			-e "s/-lxkbcommon/-l:libxkbcommon.so.0/g" \\\n\
			-e "s/-lwayland-client/-l:libwayland-client.so.0/g" \\\n\
			-e "s/-lwayland-cursor/-l:libwayland-cursor.so.0/g" \\\n\
			-e "s/-lwayland-egl/-l:libwayland-egl.so.1/g"\n\
	fi\n' > $@
	chmod +x $@

gui: $(BUILD)/pkg-config
	PATH="$(abspath $(BUILD)):$(SYSHUB)/bin:$$PATH" \
	CGO_ENABLED=1 CC="$(CC)" \
	PKG_CONFIG_PATH= PKG_CONFIG_LIBDIR=$(SYSHUB)/lib/pkgconfig:$(SYSHUB)/share/pkgconfig:/overlayer/zexlib/union/lib/pkgconfig:/overlayer/zexlib/union/share/pkgconfig \
	CGO_CFLAGS="$(if $(SPECS),-specs=$(SPECS)) -I$(MUSLDIR)/include -I$(SYSHUB)/include -I/overlayer/zexlib/union/include" \
	CGO_LDFLAGS="$(if $(SPECS),-specs=$(SPECS)) -L$(MUSLDIR)/lib -L$(SYSHUB)/lib -L/overlayer/zexlib/union/lib -Wl,--dynamic-linker=$(LOADER) -Wl,--allow-shlib-undefined -Wl,--rpath-link=$(SYSHUB)/lib:$(MUSLDIR)/lib:/overlayer/zexlib/union/lib" \
	go build $(GOFLAGS) -tags nox11 -ldflags='-s -w -linkmode external' -o $(BUILD)/zainium-installer ./cmd/zainium-installer

install: all
	install -Dm755 $(BUILD)/zainium-install   $(DESTDIR)/overlayer/zaisys/bin/zainium-install
	install -Dm755 $(BUILD)/zainium-installer $(DESTDIR)/overlayer/zaisys/bin/zainium-installer
	install -Dm644 data/tech.zainiumdynamics.Installer.desktop $(DESTDIR)/overlayer/zaisys/share/applications/tech.zainiumdynamics.Installer.desktop
	install -Dm644 data/tech.zainiumdynamics.Installer.svg $(DESTDIR)/overlayer/zaisys/share/icons/hicolor/scalable/apps/tech.zainiumdynamics.Installer.svg
	install -Dm644 data/tech.zainiumdynamics.installer.policy $(DESTDIR)/overlayer/zaisys/share/polkit-1/actions/tech.zainiumdynamics.installer.policy
	# polkitd reads actions only from the base; RemoveInstaller deletes it on install
	install -Dm644 data/tech.zainiumdynamics.installer.policy $(DESTDIR)/overlayer/syshub/share/polkit-1/actions/tech.zainiumdynamics.installer.policy

clean:
	rm -rf $(BUILD)