// Estimasi kekuatan password sederhana (client-side, tanpa dependency).
// BUKAN pengganti kebijakan server (min 8 karakter) — hanya panduan visual.

export interface PasswordScore {
  score: 0 | 1 | 2 | 3 | 4
  hints: string[]
}

const COMMON = ['password', '12345678', 'qwerty', 'admin', '11111111', 'iloveyou', 'letmein']

export function scorePassword(pw: string): PasswordScore {
  const hints: string[] = []
  if (!pw) return { score: 0, hints: [] }

  let score = 0
  if (pw.length >= 8) score++
  else hints.push('minimal 8 karakter')
  if (pw.length >= 12) score++
  else if (pw.length >= 8) hints.push('12+ karakter lebih baik')

  const classes = [/[a-z]/, /[A-Z]/, /[0-9]/, /[^A-Za-z0-9]/].filter((re) => re.test(pw)).length
  if (classes >= 3) score++
  else hints.push('campur huruf besar/kecil, angka, simbol')
  if (classes === 4 && pw.length >= 10) score++

  if (COMMON.some((c) => pw.toLowerCase().includes(c))) {
    score = Math.min(score, 1)
    hints.unshift('hindari kata umum')
  }
  if (/^(.)\1+$/.test(pw) || /^(0123|1234|2345|3456|4567|5678|6789|abcd)/.test(pw.toLowerCase())) {
    score = Math.min(score, 1)
    hints.unshift('hindari pola berulang/berurutan')
  }

  return { score: Math.max(0, Math.min(4, score)) as PasswordScore['score'], hints: hints.slice(0, 2) }
}
