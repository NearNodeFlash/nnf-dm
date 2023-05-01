MY_INSTALL_DIR=$(HOME)/.local

all: clean cmake build

clean:
	rm -rf cmake

cmake:
	mkdir -p cmake/build
	cd cmake/build; cmake -DgRPC_INSTALL=ON \
		-DgRPC_BUILD_TESTS=OFF \
		-DCMAKE_INSTALL_PREFIX=$MY_INSTALL_DIR \
		../..

build:
	make -C cmake/build -j4