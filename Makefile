all: intel

intel:
	GOOS=linux GOARCH=amd64 go build -o home2grafana.amd64
