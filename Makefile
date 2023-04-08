buildit: pico-sdk
	rm -rf build
	mkdir build
	cd build && cmake ..
	cd build && make -j4
	du -sh build/*elf
	mkdir -p /tmp/build
	cp build/picore.uf2 /tmp/build
	nautilus /tmp/build

quick:
	rm -rf build
	mkdir build
	cd build && cmake ..
	cd build && make -j4

all: audio buildit

audio:
	cd audio2h && rm -rf converted
	cd audio2h && mkdir converted
	cd audio2h && go run main.go --limit 30 --bpm 180 --sr 19200

clean:
	rm -rf build


pico-sdk:
	git clone https://github.com/raspberrypi/pico-sdk
	cd pico-sdk && git submodule update --init
