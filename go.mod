module github.com/lkmavi/osg-runtime

go 1.27.0

require (
	github.com/docker/docker v27.5.1+incompatible
	github.com/docker/go-connections v0.5.0
	github.com/lkmavi/osg-core v0.0.0
	golang.org/x/term v0.46.0
)

replace github.com/lkmavi/osg-core => ../osg-core

exclude github.com/docker/go-connections v0.8.0

exclude github.com/docker/go-connections v0.8.1
