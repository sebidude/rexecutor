.PHONY: all test clean
APPNAME := rexecutor
APPSRC := ./cmd/$(APPNAME)

DOCKER_REPO := sebidude/$(APPNAME)

GITCOMMITHASH := $(shell git log --max-count=1 --pretty="format:%h" HEAD)
GITCOMMIT := -X main.gitcommit=$(GITCOMMITHASH)

VERSIONTAG := $(shell git describe --tags --abbrev=0)
VERSION := -X main.appversion=$(VERSIONTAG)

BUILDTIMEVALUE := $(shell date +%Y-%m-%dT%H:%M:%S%z)
BUILDTIME := -X main.buildtime=$(BUILDTIMEVALUE)

LDFLAGS := '-extldflags "-static" -d -s -w $(GITCOMMIT) $(VERSION) $(BUILDTIME)'

all:info clean build

clean:
	rm -rf build rendered

info: 
	@echo - appname:   $(APPNAME)
	@echo - version:   $(VERSIONTAG)
	@echo - commit:    $(GITCOMMITHASH)
	@echo - buildtime: $(BUILDTIMEVALUE) 

dep:
	@go get -v -d ./...

dep-update:
	@go get -u -v -d ./...

run:
	build/linux/$(APPNAME)

build-linux: info dep
	@echo Building for linux
	@mkdir -p build/linux
	CGO_ENABLED=0 \
	GOOS=linux \
	go build -o build/linux/$(APPNAME)-$(VERSIONTAG)-$(GITCOMMITHASH) -a -ldflags $(LDFLAGS) $(APPSRC)
	@cp build/linux/$(APPNAME)-$(VERSIONTAG)-$(GITCOMMITHASH) build/linux/$(APPNAME)

image:
	@echo Building docker image
	docker build -t $(DOCKER_REPO):latest --file=Dockerfile.local .

pack: build
	@cd build/linux && tar cvfz $(APPNAME)-$(VERSIONTAG).tar.gz $(APPNAME)
