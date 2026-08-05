.PHONY: build test workcell-test stringproof-test test-race demo multi-demo contention proof recording proof-recording stringproof-release-asset stringproof-recording release-assets site-install site-build clean

WORKCELL_DIR := $(CURDIR)/workcell
STRINGPROOF_DIR := $(CURDIR)/stringproof
WORKCELL_BIN := $(WORKCELL_DIR)/bin/workcell
STRINGPROOF_ZIPAPP := $(CURDIR)/site/public/downloads/stringproof.pyz

build:
	@mkdir -p $(WORKCELL_DIR)/bin
	cd $(WORKCELL_DIR) && go build -o $(WORKCELL_BIN) ./cmd/workcell

test: workcell-test stringproof-test

workcell-test:
	cd $(WORKCELL_DIR) && go vet ./... && go test ./...

stringproof-test:
	cd $(STRINGPROOF_DIR) && python3 -m unittest discover -s tests

test-race:
	cd $(WORKCELL_DIR) && go test -race ./...

demo: build
	WORKCELL_BIN=$(WORKCELL_BIN) $(WORKCELL_DIR)/demo/demo.sh

multi-demo: build
	WORKCELL_BIN=$(WORKCELL_BIN) $(WORKCELL_DIR)/demo/multi-resource-demo.sh

contention: build
	WORKCELL_BIN=$(WORKCELL_BIN) $(WORKCELL_DIR)/demo/contention.sh --workers 24 --resource macos-xcode

proof: build
	WORKCELL_BIN=$(WORKCELL_BIN) $(WORKCELL_DIR)/demo/agent-proof/run.sh

recording: proof-recording
	@mkdir -p $(WORKCELL_DIR)/artifacts/recordings
	cd $(WORKCELL_DIR) && vhs demo/multi-resource.tape

proof-recording: build
	@mkdir -p $(CURDIR)/site/src/assets/demo $(CURDIR)/artifacts/recordings
	@rm -rf /tmp/workcell-real-agent-proof-frames
	cd /tmp && WORKCELL_REPO=$(WORKCELL_DIR) vhs $(WORKCELL_DIR)/demo/agent-proof.tape
	ffmpeg -loglevel error -y -framerate 30 -start_number 1 \
		-i /tmp/workcell-real-agent-proof-frames/frame-text-%05d.png \
		-vf "pad=1920:1200:(ow-iw)/2:(oh-ih)/2:color=0x171717,scale=1600:1000:flags=lanczos,format=yuv420p,setparams=range=limited:color_primaries=bt709:color_trc=bt709:colorspace=bt709" \
		-c:v libx264 -preset slow -tune animation -crf 13 -profile:v high -level:v 4.1 \
		-x264-params "colorprim=bt709:transfer=bt709:colormatrix=bt709:fullrange=off" \
		-color_primaries bt709 -color_trc bt709 -colorspace bt709 \
		-movflags +faststart $(CURDIR)/site/src/assets/demo/workcell-real-agent-proof-v1.mp4
	ffmpeg -loglevel error -y \
		-i $(CURDIR)/site/src/assets/demo/workcell-real-agent-proof-v1.mp4 \
		-filter_complex "[0:v]fps=15,scale=1200:750:flags=lanczos,split[frames][palette_source];[palette_source]palettegen=max_colors=128:stats_mode=diff[palette];[frames][palette]paletteuse=dither=bayer:bayer_scale=3:diff_mode=rectangle" \
		-loop 0 $(CURDIR)/site/src/assets/demo/workcell-real-agent-proof-v1.gif
	ffmpeg -loglevel error -y -sseof -4 \
		-i $(CURDIR)/site/src/assets/demo/workcell-real-agent-proof-v1.mp4 \
		-frames:v 1 $(CURDIR)/site/src/assets/demo/workcell-real-agent-proof-v1-poster.png
	@rm -rf /tmp/workcell-real-agent-proof-frames

stringproof-release-asset:
	@mkdir -p $(CURDIR)/site/public/downloads
	@mkdir -p /Volumes/thunderware/tmp/workcell
	@stage_dir=$$(mktemp -d /Volumes/thunderware/tmp/workcell/stringproof-zipapp.XXXXXX); \
		trap 'rm -rf "$$stage_dir"' EXIT; \
		cp -R $(STRINGPROOF_DIR)/stringproof "$$stage_dir/stringproof"; \
		find "$$stage_dir" -type d -name __pycache__ -prune -exec rm -rf {} +; \
		python3 -m zipapp "$$stage_dir" --main stringproof.__main__:run --python "/usr/bin/env python3" --compress --output $(STRINGPROOF_ZIPAPP); \
		chmod 0755 $(STRINGPROOF_ZIPAPP)

stringproof-recording: stringproof-release-asset
	@mkdir -p $(CURDIR)/site/src/assets/demo
	cd $(STRINGPROOF_DIR) && STRINGPROOF_REPO=$(STRINGPROOF_DIR) STRINGPROOF_ZIPAPP=$(STRINGPROOF_ZIPAPP) vhs demo/stringproof.tape

release-assets:
	$(WORKCELL_DIR)/scripts/build-release-assets.sh
	$(MAKE) stringproof-release-asset

site-install:
	cd site && pnpm install --frozen-lockfile

site-build: stringproof-release-asset
	cd site && pnpm build

clean:
	rm -rf $(WORKCELL_DIR)/bin $(CURDIR)/site/dist
