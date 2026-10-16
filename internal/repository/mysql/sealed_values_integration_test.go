package mysql

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"

	"acs/internal/domain"
)

// Integrasi MariaDB NYATA: nilai rahasia profil & preset benar-benar tersegel di kolom DB,
// terbaca plaintext lewat repository, dan backfill mengenkripsi baris lama secara idempoten.
// Dilewati tanpa ACS_TEST_DSN.
func TestSealedSecrets_Integration(t *testing.T) {
	dsn := os.Getenv("ACS_TEST_DSN")
	if dsn == "" {
		t.Skip("ACS_TEST_DSN tidak diisi — test integrasi MariaDB dilewati")
	}
	db, err := Connect(dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	s := testSealer(t)

	profiles := NewProvisioningProfileRepository(db)
	params := NewProvisioningProfileParameterRepository(db, s)
	presets := NewPresetRepository(db, s)

	// ---- profil ----
	prof := &domain.ProvisioningProfile{ProfileUUID: uuid.NewString(), Name: "it-seal-" + uuid.NewString()[:8], IsActive: true}
	if err := profiles.Create(ctx, prof); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM provisioning_profile_parameters WHERE profile_id = ?`, prof.ID)
		_, _ = db.Exec(`DELETE FROM provisioning_profiles WHERE id = ?`, prof.ID)
	})
	pw, ssid := "PPPoE-Rahasia-123", "ISP-Home"
	in := []domain.ProvisioningProfileParameter{
		{ParameterName: "InternetGatewayDevice.WANDevice.1.WANConnectionDevice.1.WANPPPConnection.1.Password", ParameterValue: &pw},
		{ParameterName: "InternetGatewayDevice.LANDevice.1.WLANConfiguration.1.SSID", ParameterValue: &ssid},
	}
	if err := params.Replace(ctx, prof.ID, in); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if *in[0].ParameterValue != pw {
		t.Fatalf("slice pemanggil tidak boleh berisi ciphertext setelah Replace: %q", *in[0].ParameterValue)
	}
	var raw []string
	if err := db.Select(&raw, `SELECT parameter_value FROM provisioning_profile_parameters WHERE profile_id = ? ORDER BY id`, prof.ID); err != nil {
		t.Fatal(err)
	}
	if len(raw) != 2 || !strings.HasPrefix(raw[0], sealedPrefix) || strings.Contains(raw[0], pw) {
		t.Fatalf("kolom DB harus tersegel: %v", raw)
	}
	if raw[1] != ssid {
		t.Fatalf("nilai tak sensitif harus plaintext di DB: %q", raw[1])
	}
	got, err := params.ListByProfile(ctx, prof.ID)
	if err != nil || len(got) != 2 || *got[0].ParameterValue != pw || *got[1].ParameterValue != ssid {
		t.Fatalf("ListByProfile harus plaintext: %+v err=%v", got, err)
	}

	// ---- backfill: baris LAMA plaintext -> tersegel, idempoten ----
	if _, err := db.Exec(`UPDATE provisioning_profile_parameters SET parameter_value = ? WHERE profile_id = ? AND parameter_name LIKE '%Password'`, "lama-plain", prof.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := params.ListByProfile(ctx, prof.ID); *got[0].ParameterValue != "lama-plain" {
		t.Fatalf("baris lama plaintext harus tetap terbaca: %q", *got[0].ParameterValue)
	}
	pr, _, err := SealExistingSecrets(ctx, db, s)
	if err != nil || pr < 1 {
		t.Fatalf("backfill: pr=%d err=%v", pr, err)
	}
	_ = db.Select(&raw, `SELECT parameter_value FROM provisioning_profile_parameters WHERE profile_id = ? AND parameter_name LIKE '%Password'`, prof.ID)
	if !strings.HasPrefix(raw[0], sealedPrefix) {
		t.Fatalf("setelah backfill harus tersegel: %q", raw[0])
	}
	if pr2, _, err := SealExistingSecrets(ctx, db, s); err != nil || pr2 != 0 {
		t.Fatalf("backfill harus idempoten: pr2=%d err=%v", pr2, err)
	}
	if got, _ := params.ListByProfile(ctx, prof.ID); *got[0].ParameterValue != "lama-plain" {
		t.Fatalf("setelah backfill nilai harus tetap sama: %q", *got[0].ParameterValue)
	}

	// ---- preset ----
	cfg := `[{"op":"set_parameter","key":"wifi.password","value":"kunci-preset"},{"op":"set_parameter","key":"wifi.ssid","value":"ISP"}]`
	pre := &domain.Preset{Name: "it-seal-" + uuid.NewString()[:8], Weight: 1, Precondition: "{}", Configurations: cfg, IsActive: true, Enforce: true}
	if err := presets.Create(ctx, pre); err != nil {
		t.Fatalf("create preset: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM presets WHERE id = ?`, pre.ID) })
	if pre.Configurations != cfg {
		t.Fatalf("struct pemanggil tidak boleh berubah jadi ciphertext: %s", pre.Configurations)
	}
	var rawCfg string
	_ = db.Get(&rawCfg, `SELECT configurations FROM presets WHERE id = ?`, pre.ID)
	if strings.Contains(rawCfg, "kunci-preset") || !strings.Contains(rawCfg, sealedPrefix) {
		t.Fatalf("kolom preset harus tersegel: %s", rawCfg)
	}
	back, err := presets.GetByID(ctx, pre.ID)
	if err != nil || !strings.Contains(back.Configurations, "kunci-preset") || strings.Contains(back.Configurations, sealedPrefix) {
		t.Fatalf("GetByID harus plaintext: %v %s", err, back.Configurations)
	}
	active, err := presets.ListActiveEnforce(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range active {
		if a.ID == pre.ID {
			found = strings.Contains(a.Configurations, "kunci-preset")
		}
	}
	if !found {
		t.Fatal("ListActiveEnforce harus mengembalikan plaintext untuk engine preset")
	}
	// Update menyegel lagi
	pre.Configurations = `[{"op":"set_parameter","key":"wifi.password","value":"kunci-baru"}]`
	if err := presets.Update(ctx, pre); err != nil {
		t.Fatal(err)
	}
	_ = db.Get(&rawCfg, `SELECT configurations FROM presets WHERE id = ?`, pre.ID)
	if strings.Contains(rawCfg, "kunci-baru") {
		t.Fatalf("Update harus menyegel: %s", rawCfg)
	}
}
