# vamoose build targets.

.PHONY: build eventkit test clean

# build compiles the vamoose CLI.
build:
	go build -o vamoose .

# eventkit compiles the macOS EventKit helper, which enables native approval
# detection on iCloud. Requires the Swift toolchain and runs on macOS only.
eventkit:
	swiftc -O internal/eventkit/eventkit-helper.swift -o vamoose-eventkit

# test runs the Go test suite.
test:
	go test ./...

# clean removes the built binaries.
clean:
	rm -f vamoose vamoose-eventkit
