// Beranda statis (SEO) hanya untuk "/": di route lain disembunyikan sebelum React mengambil alih (tanpa kedip).
// Berkas eksternal (bukan skrip inline) supaya Content-Security-Policy dapat memakai script-src 'self' tanpa 'unsafe-inline'.
if (location.pathname !== '/') {
  var r = document.getElementById('root')
  if (r) r.style.visibility = 'hidden'
}
