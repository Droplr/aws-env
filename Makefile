export GO111MODULE=on
export BUILD_DIR=bin

install:
	@go install

build:
	@./build.sh

bin:
	@mkdir -p $(BIN)
