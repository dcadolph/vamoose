# vamoose build targets.

.PHONY: build eventkit test clean

# build compiles the vamoose CLI.
build:
	go build -o vamoose .

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
	rm -rf vamoose vamoose-eventkit vamoose-eventkit.app
