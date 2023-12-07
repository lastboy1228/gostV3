package http2

import (
	"time"

	mdata "github.com/go-gost/core/metadata"
	mdutil "github.com/go-gost/core/metadata/util"
)

const (
	defaultBacklog = 128
)

type metadata struct {
	backlog        int
	mptcp          bool
	maxIdleTimeout time.Duration
}

func (l *http2Listener) parseMetadata(md mdata.Metadata) (err error) {
	const (
		backlog        = "backlog"
		maxIdleTimeout = "idle"
	)

	l.md.backlog = mdutil.GetInt(md, backlog)
	if l.md.backlog <= 0 {
		l.md.backlog = defaultBacklog
	}
	l.md.mptcp = mdutil.GetBool(md, "mptcp")
	l.md.maxIdleTimeout = mdutil.GetDuration(md, maxIdleTimeout)
	if l.md.maxIdleTimeout <= 0 {
		l.md.maxIdleTimeout = 5 * time.Minute
	}
	return
}
