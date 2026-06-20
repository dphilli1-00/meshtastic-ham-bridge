BINARY=meshtastic-ham-bridge
MODULE=github.com/dphilli/meshtastic-ham-bridge/cmd/bridge

# Local build
build:
	go build -o $(BINARY) $(MODULE)

# Cross-compile for Pi 4/5 (64-bit ARM)
build-pi:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=1 CC=aarch64-linux-gnu-gcc \
		go build -o $(BINARY)-arm64 $(MODULE)

# Cross-compile for older Pi (32-bit ARM)
build-pi32:
	GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=1 CC=arm-linux-gnueabihf-gcc \
		go build -o $(BINARY)-arm $(MODULE)

# Run tests
test:
	go test ./internal/...

# Deploy to Pi over SSH
# Usage: make deploy PI=pi@raspberrypi.local
deploy: build-pi
	ssh $(PI) "sudo systemctl stop $(BINARY) || true"
	scp $(BINARY)-arm64 $(PI):~/$(BINARY)
	ssh $(PI) "sudo mv ~/$(BINARY) /usr/local/bin/$(BINARY)"
	scp deploy/systemd/$(BINARY).service $(PI):~/
	ssh $(PI) "sudo mv ~/$(BINARY).service /etc/systemd/system/ && \
		sudo systemctl daemon-reload && \
		sudo systemctl enable $(BINARY) && \
		sudo systemctl start $(BINARY)"
	@echo "Deployed and started on $(PI)"

# Check service status on Pi
# Usage: make status PI=pi@raspberrypi.local
status:
	ssh $(PI) "sudo systemctl status $(BINARY)"

.PHONY: build build-pi build-pi32 test deploy status
