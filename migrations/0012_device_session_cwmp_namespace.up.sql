-- Menyimpan namespace XML CWMP yang dideklarasikan CPE pada Inform-nya
-- (mis. "urn:dslforum-org:cwmp-1-0", "cwmp-1-2"), supaya SELURUH RPC
-- proaktif yang dikirim ACS sepanjang sesi yang sama (bukan cuma balasan
-- InformResponse) memakai namespace yang sama, bukan default hardcode.
--
-- [BUG DITEMUKAN saat review kode setelah fix awal namespace echo]
-- Fix awal (lihat pkg/cwmpxml/envelope.go Body.Namespace()) hanya membaca
-- namespace dari REQUEST HTTP saat ini -- tapi RPC proaktif pertama dalam
-- sesi (task Reboot/SetParameterValues/Download dsb.) selalu dikirim
-- sbg balasan atas POST KOSONG CPE ("siap terima RPC berikutnya"), yang
-- SAMA SEKALI tidak membawa body XML sehingga tidak ada namespace utk
-- dibaca dari request itu -- inilah jalur MAYORITAS pengiriman RPC dalam
-- praktik, bukan kasus langka seperti diasumsikan komentar fix awal.
-- Dengan kolom ini, namespace dicatat SEKALI saat Inform lalu dipakai
-- ulang utk seluruh RPC proaktif dalam sesi yang sama (lihat
-- usecase/session/service.go NextRequest).
ALTER TABLE device_sessions
    ADD COLUMN cwmp_namespace VARCHAR(64) NULL
        COMMENT 'Namespace CWMP yang dideklarasikan CPE pada Inform sesi ini, mis. urn:dslforum-org:cwmp-1-2'
        AFTER cwmp_id;
