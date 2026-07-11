# vamoose build targets.

.PHONY: build eventkit app test clean

# build compiles the vamoose CLI.
build:
	go build -o vamoose .

# app packages the vamoose CLI as a macOS .app bundle that launches the local dashboard,
# so there is a Vamoose icon in Applications. It is ad-hoc signed; for distribution, sign
# with a Developer ID and notarize. Requires macOS (sips and iconutil).
APPBUNDLE := Vamoose.app
ICON_SRC := assets/vamoose-slack-icon.png
app: build
	rm -rf $(APPBUNDLE) vamoose.iconset
	mkdir -p $(APPBUNDLE)/Contents/MacOS $(APPBUNDLE)/Contents/Resources
	cp packaging/macos/Info.plist $(APPBUNDLE)/Contents/Info.plist
	cp vamoose $(APPBUNDLE)/Contents/MacOS/vamoose
	cp packaging/macos/launch.sh $(APPBUNDLE)/Contents/MacOS/vamoose-app
	chmod +x $(APPBUNDLE)/Contents/MacOS/vamoose-app
	mkdir vamoose.iconset
	for s in 16 32 128 256 512; do \
		sips -z $$s $$s $(ICON_SRC) --out vamoose.iconset/icon_$${s}x$${s}.png >/dev/null; \
		d=$$((s * 2)); \
		sips -z $$d $$d $(ICON_SRC) --out vamoose.iconset/icon_$${s}x$${s}@2x.png >/dev/null; \
	done
	iconutil -c icns vamoose.iconset -o $(APPBUNDLE)/Contents/Resources/vamoose.icns
	rm -rf vamoose.iconset
	codesign --force --deep --sign - $(APPBUNDLE)
	@echo "Built $(APPBUNDLE). Drag it to /Applications."

# eventkit compiles the macOS EventKit helper and packages it as a signed .app
# bundle. TCC treats a bundled, signed app as its own subject, so macOS shows the
# calendar-access prompt and the grant sticks to the app; a bare CLI binary is
# attributed to the parent terminal and denied without a prompt. Requires the
# Swift toolchain and runs on macOS only.
APP := vamoose-eventkit.app
eventkit:
	rm -rf $(APP)
	mkdir -p $(APP)/Contents/MacOS
	cp internal/eventkit/Info.plist $(APP)/Contents/Info.plist
	swiftc -O internal/eventkit/eventkit-helper.swift -o $(APP)/Contents/MacOS/vamoose-eventkit \
		-Xlinker -sectcreate -Xlinker __TEXT -Xlinker __info_plist \
		-Xlinker internal/eventkit/Info.plist
	codesign --force --sign - $(APP)

# test runs the Go test suite.
test:
	go test ./...

# clean removes the built binaries.
clean:
	rm -rf vamoose vamoose-eventkit vamoose-eventkit.app Vamoose.app vamoose.iconset
