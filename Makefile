mac:
	GOOS=linux GOARCH=amd64 \
	CC=x86_64-unknown-linux-gnu-gcc \
	CXX=x86_64-unknown-linux-gnu-g++ \
	CGO_ENABLED=1 \
	go build -o output_linux
linux:
	GOOS=linux GOARCH=amd64 \
	CGO_ENABLED=1 \
	go build