// Batas panjang input, dicerminkan dari lebar kolom di `schema.sql` dan
// divalidasi ulang di backend (`internal/delivery/http/validate.go`).
//
// KENAPA ADA: sebelum ini tidak ada satu pun atribut `maxLength` di frontend.
// Input yang melebihi lebar kolom lolos sampai MariaDB dan keluar sebagai
// HTTP 500 ("Data too long for column"), bukan pesan validasi yang bisa
// dimengerti user. Temuan audit parity 2026-09-05.
//
// Ini lapisan UX (mencegah user mengetik terlalu panjang sejak awal), BUKAN
// pengganti validasi backend — backend tetap sumber kebenaran.
//
// Kalau migrasi mengubah lebar kolom, ubah tiga tempat sekaligus:
// `schema.sql`, konstanta di `validate.go`, dan berkas ini.
export const LIMITS = {
  tenant: {
    code: 32, // tenants.code VARCHAR(32)
    name: 128, // tenants.name VARCHAR(128)
    cwmpUsername: 128, // tenants.cwmp_inform_username VARCHAR(128)
    brandName: 128, // tenants.brand_name VARCHAR(128)
    logoUrl: 512, // tenants.logo_url VARCHAR(512)
    primaryColor: 7, // tenants.primary_color CHAR(7) — "#RRGGBB"
  },
  user: {
    username: 64, // users.username VARCHAR(64)
    email: 191, // users.email VARCHAR(191)
    fullName: 128, // users.full_name VARCHAR(128)
  },
  apiToken: {
    name: 128, // api_tokens.name VARCHAR(128)
  },
  tag: {
    name: 255, // tags.name VARCHAR(255)
    color: 7, // tags.color VARCHAR(7)
  },
  preset: {
    name: 255, // presets.name VARCHAR(255)
    channel: 64, // presets.channel VARCHAR(64)
  },
  vendor: {
    code: 32, // ref_vendors.code VARCHAR(32)
    name: 128, // ref_vendors.name VARCHAR(128)
    oui: 6, // vendor_ouis.oui CHAR(6)
    notes: 255, // vendor_ouis.notes VARCHAR(255)
  },
  deviceModel: {
    modelName: 128, // device_models.model_name VARCHAR(128)
    productClass: 128, // device_models.product_class VARCHAR(128)
  },
  mapping: {
    logicalKey: 128, // vendor_parameter_mappings.logical_key VARCHAR(128)
    tr069Path: 512, // vendor_parameter_mappings.tr069_path VARCHAR(512)
    softwareVersionPattern: 255, // *.software_version_pattern VARCHAR(255)
  },
  profile: {
    name: 128, // provisioning_profiles.name VARCHAR(128)
  },
  ztpRule: {
    serialPattern: 255, // zero_touch_rules.serial_pattern VARCHAR(255)
    matchParameterName: 512, // zero_touch_rules.match_parameter_name VARCHAR(512)
    matchParameterValuePattern: 255, // zero_touch_rules.match_parameter_value_pattern VARCHAR(255)
  },
  webhook: {
    name: 128, // webhook_subscriptions.name VARCHAR(128)
    targetUrl: 500, // webhook_subscriptions.target_url VARCHAR(500)
  },
  // Kolom `description VARCHAR(255)` dipakai di banyak tabel (ref_vendors,
  // device_models, vendor_parameter_mappings, provisioning_profiles,
  // webhook_subscriptions) — satu konstanta untuk semuanya.
  description: 255,
} as const
