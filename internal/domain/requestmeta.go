package domain

import "context"

// RequestMeta — konteks permintaan HTTP (IP klien & user agent) yang dibawa lewat context
// supaya audit trail (activity_logs.ip_address) terisi tanpa tiap usecase mengopernya.
type RequestMeta struct {
	IP        string
	UserAgent string
	// ImpersonatorID — id SUPERADMIN bila request berasal dari sesi impersonate; audit otomatis mencatatnya.
	ImpersonatorID uint64
}

type requestMetaKey struct{}

func WithRequestMeta(ctx context.Context, m RequestMeta) context.Context {
	return context.WithValue(ctx, requestMetaKey{}, m)
}

func RequestMetaFrom(ctx context.Context) (RequestMeta, bool) {
	m, ok := ctx.Value(requestMetaKey{}).(RequestMeta)
	return m, ok
}
