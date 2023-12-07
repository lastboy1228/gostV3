package http2

import (
	"time"

	mdata "github.com/go-gost/core/metadata"
	mdutil "github.com/go-gost/core/metadata/util"
)

type metadata struct {
	ttl time.Duration
}

func (d *http2Dialer) parseMetadata(md mdata.Metadata) (err error) {
	const (
		ttl = "ttl"
	)
	d.md.ttl = mdutil.GetDuration(md, ttl)
	return
}
