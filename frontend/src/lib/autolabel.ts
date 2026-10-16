// Menautkan <label> ke kontrol formulir di dekatnya secara otomatis (aksesibilitas).
//
// Hampir semua formulir Console memakai pola `<label>Teks</label><input/>` (kontrol sebagai saudara
// berikutnya, atau di dalam pembungkus) TANPA htmlFor/id — pembaca layar tidak tahu nama kolomnya dan
// klik pada label tidak memfokuskan kolom. Daripada menyunting ratusan JSX satu per satu (dan lupa saat
// menambah form baru), pemetaan deterministik ini dijalankan untuk semua DOM, termasuk modal yang
// muncul belakangan. Hanya menyentuh label yang belum punya `for` dan tidak membungkus kontrol; kontrol
// yang sudah ber-id dipakai apa adanya.

const CONTROL = 'input:not([type="hidden"]), select, textarea'
let seq = 0

function linkLabels(root: ParentNode) {
  root.querySelectorAll<HTMLLabelElement>('label:not([for])').forEach((label) => {
    if (label.querySelector(CONTROL)) return // sudah membungkus kontrol
    const next = label.nextElementSibling
    if (!next) return
    const control = next.matches(CONTROL) ? next : next.querySelector(CONTROL)
    if (!control) return
    if (!control.id) control.id = `al-${++seq}`
    label.htmlFor = control.id
  })
}

export function installAutoLabels() {
  if (typeof document === 'undefined') return
  let scheduled = 0
  const run = () => {
    scheduled = 0
    linkLabels(document)
  }
  new MutationObserver(() => {
    if (!scheduled) scheduled = requestAnimationFrame(run)
  }).observe(document.body, { childList: true, subtree: true })
  run()
}
