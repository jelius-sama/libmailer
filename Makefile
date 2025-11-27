libmailer:
	go build -buildmode=c-archive -tags netgo -ldflags '-extldflags "-static"' -o ./libmailer.a ./libmailer.go
