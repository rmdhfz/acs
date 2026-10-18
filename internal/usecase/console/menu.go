// Package console = fitur Console bertabel sendiri (migrations/0033): layout menu per pengguna, laporan bug, dan
// riwayat impor/ekspor. Aturan akses ada di sini; repository hanya query, handler hanya parsing.
package console

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"acs/internal/domain"
)

// Batas layout menu (juga dijaga klien): mencegah JSON raksasa atau simpul berlebih dari pengguna sendiri.
const (
	maxMenuJSONBytes = 8 << 10
	maxMenuNodes     = 60
	maxMenuGroups    = 12
	maxMenuChildren  = 40
	maxMenuHidden    = 60
)

var (
	menuKeyRe   = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,47}$`)
	menuGroupRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,23}$`)
)

// MenuNode = satu simpul tingkat atas: item menu ("item") atau grup/sub menu ("group", berisi item).
type MenuNode struct {
	Type      string   `json:"type"`
	Key       string   `json:"key,omitempty"`       // item
	ID        string   `json:"id,omitempty"`        // grup
	Label     string   `json:"label,omitempty"`     // grup
	Collapsed bool     `json:"collapsed,omitempty"` // grup
	Children  []string `json:"children,omitempty"`  // grup: kunci item
}

// MenuLayout = susunan menu satu pengguna. Kunci item mengacu katalog menu di Console; kunci yang tidak dikenal klien
// diabaikan saat render, dan hak akses menu TETAP ditegakkan oleh peran (bukan oleh layout).
type MenuLayout struct {
	Version int        `json:"version"`
	Nodes   []MenuNode `json:"nodes"`
	Hidden  []string   `json:"hidden"`
}

type MenuService struct {
	repo domain.MenuLayoutRepository
}

func NewMenuService(repo domain.MenuLayoutRepository) *MenuService { return &MenuService{repo: repo} }

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", domain.ErrInvalidInput, fmt.Sprintf(format, a...))
}

// Get mengembalikan layout milik actor sendiri, atau (nil, nil) bila belum ada/sudah di-reset.
func (s *MenuService) Get(ctx context.Context, actor domain.Actor) (json.RawMessage, error) {
	if actor.UserID == 0 {
		return nil, domain.ErrForbidden
	}
	raw, err := s.repo.Get(ctx, actor.UserID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return json.RawMessage(raw), nil
}

// Put memvalidasi lalu menyimpan layout milik actor sendiri. Sesi impersonate tidak boleh mengubah preferensi
// pengguna yang sedang ditiru.
func (s *MenuService) Put(ctx context.Context, actor domain.Actor, body []byte) (json.RawMessage, error) {
	if actor.UserID == 0 || actor.ImpersonatorID != 0 {
		return nil, domain.ErrForbidden
	}
	norm, err := ValidateMenuLayout(body)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Upsert(ctx, actor.UserID, string(norm)); err != nil {
		return nil, err
	}
	return norm, nil
}

func (s *MenuService) Reset(ctx context.Context, actor domain.Actor) error {
	if actor.UserID == 0 || actor.ImpersonatorID != 0 {
		return domain.ErrForbidden
	}
	return s.repo.Reset(ctx, actor.UserID)
}

// ValidateMenuLayout menolak bentuk/ukuran yang tidak sah dan mengembalikan JSON ternormalisasi (bidang tak dikenal
// ditolak, bukan diabaikan, supaya tidak ada data tak terkontrol yang tersimpan).
func ValidateMenuLayout(body []byte) (json.RawMessage, error) {
	if len(body) == 0 || len(body) > maxMenuJSONBytes {
		return nil, invalid("layout menu kosong atau lebih dari %d KB", maxMenuJSONBytes>>10)
	}
	var l MenuLayout
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&l); err != nil {
		return nil, invalid("bentuk layout menu tidak valid")
	}
	if dec.More() {
		return nil, invalid("bentuk layout menu tidak valid")
	}
	if l.Version != 1 {
		return nil, invalid("versi layout menu harus 1")
	}
	if len(l.Nodes) > maxMenuNodes || len(l.Hidden) > maxMenuHidden {
		return nil, invalid("layout menu terlalu besar")
	}
	seen := map[string]bool{}
	use := func(key string) error {
		if !menuKeyRe.MatchString(key) {
			return invalid("kunci menu %q tidak valid", key)
		}
		if seen[key] {
			return invalid("kunci menu %q muncul lebih dari sekali", key)
		}
		seen[key] = true
		return nil
	}
	groups, groupIDs := 0, map[string]bool{}
	for i := range l.Nodes {
		n := &l.Nodes[i]
		switch n.Type {
		case "item":
			if n.ID != "" || n.Label != "" || len(n.Children) != 0 || n.Collapsed {
				return nil, invalid("item menu tidak boleh memuat bidang grup")
			}
			if err := use(n.Key); err != nil {
				return nil, err
			}
		case "group":
			if n.Key != "" {
				return nil, invalid("grup menu tidak boleh memuat key")
			}
			groups++
			if groups > maxMenuGroups {
				return nil, invalid("grup menu maksimal %d", maxMenuGroups)
			}
			if !menuGroupRe.MatchString(n.ID) || groupIDs[n.ID] {
				return nil, invalid("id grup menu tidak valid atau ganda")
			}
			groupIDs[n.ID] = true
			label, err := domain.CleanText("nama grup menu", n.Label, 1, 32, false)
			if err != nil {
				return nil, err
			}
			n.Label = label
			if len(n.Children) > maxMenuChildren {
				return nil, invalid("isi grup menu maksimal %d item", maxMenuChildren)
			}
			for _, k := range n.Children {
				if err := use(k); err != nil {
					return nil, err
				}
			}
		default:
			return nil, invalid("jenis simpul menu harus item atau group")
		}
	}
	hidden := map[string]bool{}
	for _, k := range l.Hidden {
		if !menuKeyRe.MatchString(k) || hidden[k] {
			return nil, invalid("kunci menu tersembunyi %q tidak valid atau ganda", k)
		}
		hidden[k] = true
	}
	if l.Hidden == nil {
		l.Hidden = []string{}
	}
	out, err := json.Marshal(l)
	if err != nil {
		return nil, invalid("layout menu tidak dapat diproses")
	}
	return out, nil
}
