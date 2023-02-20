.PHONY: all build run gotool clean help

BINARY="probermesh"
BINARY_DIR="./cmd/probermesh/"

all: gotool build

build:
	cd ${BINARY_DIR} && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o ${BINARY} . && upx -q -9 ${BINARY}

run:
	@cd ${BINARY_DIR} && go run ./

gotool:
	cd ${BINARY_DIR} && go fmt ./
	cd ${BINARY_DIR} && go vet ./

clean:
	@cd ${BINARY_DIR} && if [ -f ${BINARY} ] ; then rm ${BINARY} ; fi

help:
	@echo "make - 格式化 Go 代码, 并编译生成二进制文件"
	@echo "make build - 编译 Go 代码, 生成二进制文件"
	@echo "make run - 直接运行 Go 代码"
	@echo "make clean - 移除二进制文件和 vim swap files"
	@echo "make gotool - 运行 Go 工具 'fmt' and 'vet'"
