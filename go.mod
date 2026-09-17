module github.com/zorneth/osg-runtime

go 1.27.0

require (
	github.com/landlock-lsm/go-landlock v0.10.0
	github.com/zorneth/osg-core v0.1.0-alpha.1
	github.com/zorneth/osg-driver v0.1.0-alpha.1
	github.com/zorneth/osg-proxy v0.1.0-alpha.1
	golang.org/x/crypto v0.56.0
	golang.org/x/sys v0.48.0
	gopkg.in/yaml.v3 v3.0.1
)

require kernel.org/pub/linux/libs/security/libcap/psx v1.2.77 // indirect

replace (
	github.com/zorneth/osg-core => ../osg-core
	github.com/zorneth/osg-driver => ../osg-driver
	github.com/zorneth/osg-proxy => ../osg-proxy
)
