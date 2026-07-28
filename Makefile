.PHONY: build test test-race demo multi-demo contention proof recording proof-recording release-assets site-install site-build clean

WORKCELL_BIN := $(CURDIR)/bin/workcell

build:
	@mkdir -p $(CURDIR)/bin
	go build -o $(WORKCELL_BIN) ./cmd/workcell

test:
	go vet ./...
	go test ./...

test-race:
	go test -race ./...

demo: build
	WORKCELL_BIN=$(WORKCELL_BIN) ./demo/demo.sh

multi-demo: build
	WORKCELL_BIN=$(WORKCELL_BIN) ./demo/multi-resource-demo.sh

contention: build
	WORKCELL_BIN=$(WORKCELL_BIN) ./demo/contention.sh --workers 24 --resource macos-xcode

proof: build
	WORKCELL_BIN=$(WORKCELL_BIN) ./demo/agent-proof/run.sh

recording: proof-recording
	@mkdir -p $(CURDIR)/artifacts/recordings
	vhs demo/multi-resource.tape

proof-recording: build
	@mkdir -p $(CURDIR)/site/src/assets/demo $(CURDIR)/artifacts/recordings
	@rm -rf /tmp/workcell-real-agent-proof-frames
	cd /tmp && WORKCELL_REPO=$(CURDIR) vhs $(CURDIR)/demo/agent-proof.tape
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

release-assets:
	./scripts/build-release-assets.sh

site-install:
	cd site && pnpm install --frozen-lockfile

site-build:
	cd site && pnpm build

clean:
	rm -rf $(CURDIR)/bin $(CURDIR)/site/dist
