package device

import (
	"context"
	"errors"
	"net/http"

	"acs/pkg/digestauth"
)

// doConnectionRequest mengirim HTTP GET Connection Request ke CPE dan menjawab
// tantangan autentikasi bila ada.
//
// TR-069 §3.2.2 mewajibkan HTTP Digest di endpoint Connection Request CPE;
// sebagian CPE hanya menawarkan Basic. Request pertama SENGAJA dikirim tanpa
// kredensial (bukan Basic preemptive): password tidak ikut terkirim sebelum CPE
// memilih skema, dan Digest tidak pernah turun ke Basic secara diam-diam.
// Digest diprioritaskan bila CPE menawarkan keduanya.
//
// Pemanggil bertanggung jawab menutup resp.Body dan memastikan URL sudah lolos
// SSRF guard serta klien menolak redirect.
func doConnectionRequest(ctx context.Context, client *http.Client, rawURL, username, password string) (*http.Response, error) {
	newReq := func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	}
	req, err := newReq()
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusUnauthorized || username == "" {
		return resp, err
	}

	// Maks. 2 jawaban: satu normal, satu lagi bila server menandai nonce stale.
	for attempt := 0; attempt < 2 && resp.StatusCode == http.StatusUnauthorized; attempt++ {
		challenges := resp.Header.Values("WWW-Authenticate")
		next, err := newReq()
		if err != nil {
			resp.Body.Close()
			return nil, err
		}
		ch, derr := digestauth.ParseChallenge(challenges)
		switch {
		case derr == nil:
			authz, aerr := ch.Authorize(http.MethodGet, next.URL.RequestURI(), username, password, "", 1)
			if aerr != nil {
				resp.Body.Close()
				return nil, aerr
			}
			next.Header.Set("Authorization", authz)
		case errors.Is(derr, digestauth.ErrNoDigestChallenge) && digestauth.HasScheme(challenges, "basic"):
			next.SetBasicAuth(username, password)
		default:
			// Skema tak dikenal / tantangan rusak: kembalikan 401 apa adanya
			// agar pemanggil memperlakukannya sebagai "device menolak".
			return resp, nil
		}
		resp.Body.Close()

		resp, err = client.Do(next)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == http.StatusUnauthorized {
			if c2, e2 := digestauth.ParseChallenge(resp.Header.Values("WWW-Authenticate")); e2 != nil || !c2.Stale {
				break
			}
		}
	}
	return resp, nil
}
