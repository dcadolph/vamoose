# vamoose build targets.

.PHONY: build eventkit test clean

# build compiles the vamoose CLI.
build:
	go build -o vamoose .

# eventkit compiles the macOS EventKit helper, which enables native approval
# detection on iCloud. The Info.plist is embedded into the Mach-O so macOS shows
# the calendar-access prompt; without its usage-description key macOS 14+ denies
# access outright. Requires the Swift toolchain and runs on macOS only.
eventkit:
	swiftc -O internal/eventkit/eventkit-helper.swift -o vamoose-eventkit \
		-Xlinker -sectcreate -Xlinker __TEXT -Xlinker __info_plist \
		-Xlinker internal/eventkit/Info.plist
	codesign --force --sign - vamoose-eventkit

# test runs the Go test suite.
test:
	go test ./...

# clean removes the built binaries.
clean:
	rm -f vamoose vamoose-eventkit
