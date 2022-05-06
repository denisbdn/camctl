
all: compile

compile:
	go build

clean:
	rm camctl

install:
	install ./camctl /usr/local/bin/
	mkdir -p /tmpl/camctl/ffmpeg/
	mkdir -p /usr/local/etc/camctl/tmpl/
	mkdir -p /usr/local/etc/camctl/cmd/
	mkdir -p /usr/local/etc/camctl/static/
	cp ./tmpl/* /usr/local/etc/camctl/tmpl/
	cp ./cmd/* /usr/local/etc/camctl/cmd/
	cp ./static/* /usr/local/etc/camctl/static/

uninstall:
	rm -rf /tmpl/camctl/ffmpeg/
	rm -rf /usr/local/etc/camctl/
	rm /usr/local/bin/camctl
