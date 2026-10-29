package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"acs/internal/domain"
	"acs/internal/usecase/task"
)

// NBI tahap 2 (kompatibel-sebagian GenieACS): membuat, mencoba ulang, dan menghapus tugas. Penerjemah tipis ke usecase task yang sudah ada —
// validasi parameter, kuota tenant, scope tenant, dan penyegelan nilai rahasia tetap dilakukan task.Service (CreateTask/Retry/Cancel); handler ini
// hanya menerjemahkan bentuk body/respons dan menegakkan RBAC per jenis tugas yang sama dengan POST /tasks.
//
// Subset (selain ini ditolak 400 dengan daftar yang didukung — tidak diabaikan diam-diam):
//
//	POST   /nbi/devices/{device_uuid}/tasks   body {"name":"setParameterValues","parameterValues":[["path","nilai","xsd:string"],...]}
//	                                                {"name":"getParameterValues","parameterNames":["path",...]}
//	                                                {"name":"reboot"}
//	POST   /nbi/tasks/{task_uuid}/retry
//	DELETE /nbi/tasks/{task_uuid}
//
// Beda yang disengaja dari GenieACS: ACS tidak pernah MENUNGGU perangkat. Respons selalu 202 dengan dokumen tugas berstatus antre; `?timeout=` diterima
// tetapi tidak berefek, dan `?connection_request` memicu Connection Request secara best-effort (kegagalannya tidak menggagalkan pembuatan tugas).
// Id perangkat dan tugas adalah UUID publik (`_id`), bukan primary key.

const (
	nbiMaxTaskBody   = 256 << 10
	nbiTaskSetValues = "setParameterValues"
	nbiTaskGetValues = "getParameterValues"
	nbiTaskReboot    = "reboot"
)

var nbiSupportedTasks = nbiTaskSetValues + ", " + nbiTaskGetValues + ", " + nbiTaskReboot

type nbiTaskRequest struct {
	Name            string              `json:"name"`
	ParameterValues [][]json.RawMessage `json:"parameterValues"`
	ParameterNames  []string            `json:"parameterNames"`
}

// readLimitedBody membaca body maks max byte; lebih besar = 413 (bukan dipotong diam-diam).
func readLimitedBody(c *echo.Context, max int64) ([]byte, error) {
	b, err := io.ReadAll(http.MaxBytesReader(c.Response(), c.Request().Body, max))
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return nil, echo.NewHTTPError(http.StatusRequestEntityTooLarge, "body terlalu besar")
		}
		return nil, nbiBadRequest("body tidak dapat dibaca")
	}
	return b, nil
}

// nbiParamValue menerima nilai GenieACS berupa string, angka, atau boolean (disimpan sebagai teks, seperti nilai TR-069).
func nbiParamValue(raw json.RawMessage) (string, error) {
	if strings.TrimSpace(string(raw)) == "null" { // json.Unmarshal(null, &string) tidak galat dan menghasilkan "": jangan diam-diam menulis nilai kosong
		return "", errors.New("nilai tidak boleh null")
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, nil
	}
	var n json.Number
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	if err := dec.Decode(&n); err == nil {
		return n.String(), nil
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err == nil {
		if b {
			return "true", nil
		}
		return "false", nil
	}
	return "", errors.New("nilai harus string, angka, atau boolean")
}

// parseNBITask menerjemahkan body NBI menjadi (jenis tugas, parameter) untuk CreateTask.
func parseNBITask(body []byte) (taskType string, params map[string]interface{}, err error) {
	dec := json.NewDecoder(strings.NewReader(string(body)))
	var req nbiTaskRequest
	if e := dec.Decode(&req); e != nil {
		return "", nil, nbiBadRequest("body harus berupa objek JSON tugas")
	}
	if dec.More() {
		return "", nil, nbiBadRequest("body memuat data setelah objek JSON")
	}
	switch req.Name {
	case nbiTaskReboot:
		if len(req.ParameterValues) > 0 || len(req.ParameterNames) > 0 {
			return "", nil, nbiBadRequest("tugas reboot tidak memakai parameterValues/parameterNames")
		}
		return domain.TaskTypeReboot, map[string]interface{}{}, nil
	case nbiTaskGetValues:
		if len(req.ParameterValues) > 0 {
			return "", nil, nbiBadRequest("getParameterValues memakai parameterNames, bukan parameterValues")
		}
		if len(req.ParameterNames) == 0 {
			return "", nil, nbiBadRequest("parameterNames wajib berisi minimal 1 nama parameter")
		}
		return domain.TaskTypeGetParameterValues, map[string]interface{}{"names": req.ParameterNames}, nil
	case nbiTaskSetValues:
		if len(req.ParameterNames) > 0 {
			return "", nil, nbiBadRequest("setParameterValues memakai parameterValues, bukan parameterNames")
		}
		if len(req.ParameterValues) == 0 {
			return "", nil, nbiBadRequest("parameterValues wajib berisi minimal 1 pasangan [nama, nilai, tipe?]")
		}
		if len(req.ParameterValues) > task.MaxSetValues {
			return "", nil, nbiBadRequest("parameterValues maksimal %d per tugas", task.MaxSetValues)
		}
		values, types := map[string]string{}, map[string]string{}
		for i, pv := range req.ParameterValues {
			if len(pv) < 2 || len(pv) > 3 {
				return "", nil, nbiBadRequest("parameterValues[%d] harus [nama, nilai] atau [nama, nilai, tipe]", i)
			}
			var name string
			if json.Unmarshal(pv[0], &name) != nil || name == "" {
				return "", nil, nbiBadRequest("parameterValues[%d]: nama harus string tidak kosong", i)
			}
			if _, dup := values[name]; dup {
				return "", nil, nbiBadRequest("parameterValues: nama %q muncul lebih dari sekali", name)
			}
			v, verr := nbiParamValue(pv[1])
			if verr != nil {
				return "", nil, nbiBadRequest("parameterValues[%d]: %s", i, verr.Error())
			}
			values[name] = v
			if len(pv) == 3 {
				var ty string
				if json.Unmarshal(pv[2], &ty) != nil || domain.NormalizeXSDType(ty) == "" {
					return "", nil, nbiBadRequest("parameterValues[%d]: tipe xsd tidak dikenal", i)
				}
				types[name] = ty
			}
		}
		out := map[string]interface{}{"values": values}
		if len(types) > 0 {
			out["types"] = types
		}
		return domain.TaskTypeSetParameterValues, out, nil
	case "":
		return "", nil, nbiBadRequest("name wajib diisi (didukung: %s)", nbiSupportedTasks)
	}
	return "", nil, nbiBadRequest("tugas %q belum didukung (didukung: %s)", req.Name, nbiSupportedTasks)
}

// nbiTaskName memetakan kode jenis task ke nama tugas GenieACS (jenis lain: kode apa adanya).
func nbiTaskName(code string) string {
	switch code {
	case domain.TaskTypeSetParameterValues:
		return nbiTaskSetValues
	case domain.TaskTypeGetParameterValues:
		return nbiTaskGetValues
	case domain.TaskTypeReboot:
		return nbiTaskReboot
	}
	return code
}

// nbiTaskDoc membentuk dokumen tugas bergaya GenieACS. Nilai parameter bernama rahasia SELALU disamarkan (nilai tersimpan tersegel dan tidak
// pernah dikembalikan), apa pun peran pemanggil. status/fault adalah tambahan di luar GenieACS.
func nbiTaskDoc(t *domain.Task, deviceUUID, typeCode, statusCode string) map[string]any {
	doc := map[string]any{
		"_id":       t.TaskUUID,
		"device":    deviceUUID,
		"name":      nbiTaskName(typeCode),
		"timestamp": t.CreatedAt.UTC().Format(time.RFC3339),
		"status":    statusCode,
	}
	var p struct {
		Names  []string          `json:"names"`
		Values map[string]string `json:"values"`
		Types  map[string]string `json:"types"`
	}
	if len(t.Parameters) > 0 {
		_ = json.Unmarshal(t.Parameters, &p)
	}
	switch typeCode {
	case domain.TaskTypeGetParameterValues:
		doc["parameterNames"] = append([]string{}, p.Names...)
	case domain.TaskTypeSetParameterValues:
		names := make([]string, 0, len(p.Values))
		for n := range p.Values {
			names = append(names, n)
		}
		sort.Strings(names)
		pvs := make([][]string, 0, len(names))
		for _, n := range names {
			v := p.Values[n]
			if domain.IsSensitiveParamName(n) {
				v = domain.RedactedValue
			}
			row := []string{n, v}
			if ty := p.Types[n]; ty != "" {
				row = append(row, ty)
			}
			pvs = append(pvs, row)
		}
		doc["parameterValues"] = pvs
	}
	if statusCode == domain.TaskStatusFailed && t.ErrorMessage != nil {
		doc["fault"] = map[string]any{"message": *t.ErrorMessage}
	}
	return doc
}

// nbiTaskContext melengkapi dokumen dengan kode jenis & status (lookup katalog ref yang di-cache).
func (r *Router) nbiTaskContext(c *echo.Context, t *domain.Task) (typeCode, statusCode string) {
	ctx := c.Request().Context()
	if ref, err := r.Refs.GetByID(ctx, domain.RefTableTaskTypes, t.TaskTypeID); err == nil {
		typeCode = ref.Code
	}
	if ref, err := r.Refs.GetByID(ctx, domain.RefTableTaskStatus, t.TaskStatusID); err == nil {
		statusCode = ref.Code
	}
	return typeCode, statusCode
}

// nbiNotFoundOnForbidden: UUID tugas/perangkat tenant lain tidak boleh dapat dibedakan dari UUID yang tidak ada.
func nbiNotFoundOnForbidden(err error) error {
	if errors.Is(err, domain.ErrForbidden) {
		return domain.ErrNotFound
	}
	return err
}

// nbiCreateTask — POST /nbi/devices/:id/tasks (id = device_uuid).
func (r *Router) nbiCreateTask(c *echo.Context) error {
	actor := ActorFrom(c)
	ctx := c.Request().Context()
	for k := range c.QueryParams() {
		if k != "timeout" && k != "connection_request" {
			return nbiBadRequest("parameter query %q tidak didukung (didukung: timeout, connection_request)", k)
		}
	}
	body, err := readLimitedBody(c, nbiMaxTaskBody)
	if err != nil {
		return err
	}
	typeCode, params, err := parseNBITask(body)
	if err != nil {
		return err
	}
	// RBAC sama dengan POST /tasks: NOC hanya untuk tipe baca/ringan; mengubah parameter hanya ADMIN.
	if !actor.IsSuperadmin() && !actor.HasRole(domain.RoleAdmin) && !nocMayCreateTaskType(typeCode) {
		return echo.NewHTTPError(http.StatusForbidden, "role Anda tidak boleh membuat tugas ini (hanya ADMIN)")
	}
	dev, err := r.Devices.GetByUUID(ctx, actor, c.Param("id"))
	if err != nil {
		return handleErr(c, err)
	}
	t, err := r.Tasks.CreateTask(ctx, actor, domain.CreateTaskInput{DeviceID: dev.ID, TaskType: typeCode, Priority: 5, Parameters: params})
	if err != nil {
		return handleErr(c, err)
	}
	if _, wake := c.QueryParams()["connection_request"]; wake {
		// Best-effort: perangkat yang tak terjangkau tidak menggagalkan pembuatan tugas (tugas tetap antre sampai Inform berikutnya).
		_ = r.Devices.TriggerConnectionRequest(ctx, actor, dev.ID)
	}
	_, status := r.nbiTaskContext(c, t)
	if status == "" {
		status = domain.TaskStatusPending
	}
	return c.JSON(http.StatusAccepted, nbiTaskDoc(t, dev.DeviceUUID, typeCode, status))
}

// nbiTaskByUUID memuat tugas lewat UUID dan memastikan perangkatnya terlihat oleh pemanggil (scope tenant), tanpa membedakan
// "tidak ada" dari "milik tenant lain".
func (r *Router) nbiTaskByUUID(c *echo.Context) (*domain.Task, *domain.Device, error) {
	ctx := c.Request().Context()
	t, err := r.Tasks.GetByUUID(ctx, c.Param("id"))
	if err != nil {
		return nil, nil, err
	}
	dev, err := r.Devices.Get(ctx, ActorFrom(c), t.DeviceID)
	if err != nil {
		return nil, nil, nbiNotFoundOnForbidden(err)
	}
	return t, dev, nil
}

// nbiRetryTask — POST /nbi/tasks/:id/retry (id = task_uuid). Membangkitkan tugas yang sama (parameter sama), bukan membuat tugas baru.
func (r *Router) nbiRetryTask(c *echo.Context) error {
	t, dev, err := r.nbiTaskByUUID(c)
	if err != nil {
		return handleErr(c, err)
	}
	actor := ActorFrom(c)
	if tc, _ := r.nbiTaskContext(c, t); !actor.IsSuperadmin() && !actor.HasRole(domain.RoleAdmin) && !nocMayCreateTaskType(tc) {
		return echo.NewHTTPError(http.StatusForbidden, "role Anda tidak boleh mengulang tugas ini (hanya ADMIN)")
	}
	nt, err := r.Tasks.Retry(c.Request().Context(), actor, t.ID)
	if err != nil {
		return handleErr(c, err)
	}
	typeCode, status := r.nbiTaskContext(c, nt)
	return c.JSON(http.StatusOK, nbiTaskDoc(nt, dev.DeviceUUID, typeCode, status))
}

// nbiDeleteTask — DELETE /nbi/tasks/:id (id = task_uuid). Padanan GenieACS "hapus tugas" = membatalkan tugas yang belum selesai; riwayat
// tugas selesai/gagal tidak dihapus (409), sama dengan POST /tasks/{id}/cancel.
func (r *Router) nbiDeleteTask(c *echo.Context) error {
	t, _, err := r.nbiTaskByUUID(c)
	if err != nil {
		return handleErr(c, err)
	}
	if err := r.Tasks.Cancel(c.Request().Context(), ActorFrom(c), t.ID); err != nil {
		return handleErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}
