import { useState } from 'react'
import {
  FileText,
  Upload,
  Trash2,
  AlertCircle,
  Package,
} from 'lucide-react'
import {
  useFiles,
  useUploadFile,
  useDeleteFile,
} from '../lib/hooks'
import { Modal } from '../components/Modal'
import type { GenericFile } from '../lib/types'

export default function FilesPage() {
  const { data: filesData, isLoading, isError } = useFiles({ pageSize: 50 })
  const uploadMut = useUploadFile()
  const deleteMut = useDeleteFile()

  const [isUploadOpen, setIsUploadOpen] = useState(false)
  const [fileType, setFileType] = useState('1 Firmware Upgrade Image')
  const [fileName, setFileName] = useState('')
  const [selectedFile, setSelectedFile] = useState<globalThis.File | null>(null)
  
  const [isDeleteOpen, setIsDeleteOpen] = useState(false)
  const [fileToDelete, setFileToDelete] = useState<GenericFile | null>(null)

  const handleUpload = (e: React.FormEvent) => {
    e.preventDefault()
    if (!selectedFile) return
    const fd = new FormData()
    fd.append('file_type', fileType)
    fd.append('file_name', fileName || selectedFile.name)
    fd.append('file', selectedFile)

    uploadMut.mutate(fd, {
      onSuccess: () => {
        setIsUploadOpen(false)
        setFileName('')
        setSelectedFile(null)
      },
      onError: (err: any) => {
        alert(err.response?.data?.message || 'Gagal mengunggah file')
      }
    })
  }

  const handleDelete = () => {
    if (!fileToDelete) return
    deleteMut.mutate(fileToDelete.id, {
      onSuccess: () => {
        setIsDeleteOpen(false)
        setFileToDelete(null)
      },
      onError: (err: any) => {
        alert(err.response?.data?.message || 'Gagal menghapus file')
      }
    })
  }

  const formatBytes = (bytes: number) => {
    if (bytes === 0) return '0 B'
    const k = 1024
    const sizes = ['B', 'KB', 'MB', 'GB']
    const i = Math.floor(Math.log(bytes) / Math.log(k))
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
  }

  if (isLoading) return <div className="p-6 text-slate-400">Loading files...</div>
  if (isError) return <div className="p-6 text-red-400">Failed to load files</div>

  return (
    <div className="p-6 max-w-7xl mx-auto space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white flex items-center gap-2">
            <Package className="w-6 h-6 text-blue-400" />
            File Management
          </h1>
          <p className="text-sm text-slate-400 mt-1">
            Kelola file firmware, konfigurasi vendor, dan log dari CPE.
          </p>
        </div>
        <button
          onClick={() => setIsUploadOpen(true)}
          className="bg-blue-600 hover:bg-blue-500 text-white px-4 py-2 rounded-lg text-sm font-medium transition-colors flex items-center gap-2"
        >
          <Upload className="w-4 h-4" />
          Upload File
        </button>
      </div>

      <div className="bg-slate-800 rounded-xl border border-slate-700 overflow-hidden">
        <table className="w-full text-left text-sm text-slate-300">
          <thead className="bg-slate-900/50 border-b border-slate-700 text-xs uppercase text-slate-400">
            <tr>
              <th className="px-6 py-4 font-medium">Nama File</th>
              <th className="px-6 py-4 font-medium">Tipe</th>
              <th className="px-6 py-4 font-medium">Ukuran</th>
              <th className="px-6 py-4 font-medium">Waktu Upload</th>
              <th className="px-6 py-4 font-medium text-right">Aksi</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-700">
            {(filesData?.data || []).map((f: GenericFile) => (
              <tr key={f.id} className="hover:bg-slate-700/30 transition-colors">
                <td className="px-6 py-4 font-medium text-white flex items-center gap-3">
                  <FileText className="w-5 h-5 text-slate-400" />
                  {f.file_name}
                </td>
                <td className="px-6 py-4">
                  <span className="px-2 py-1 bg-slate-700 text-slate-300 rounded text-xs border border-slate-600">
                    {f.file_type}
                  </span>
                </td>
                <td className="px-6 py-4 font-mono text-xs">
                  {formatBytes(f.file_size_bytes)}
                </td>
                <td className="px-6 py-4 whitespace-nowrap">
                  {new Date(f.created_at).toLocaleString('id-ID')}
                </td>
                <td className="px-6 py-4 text-right">
                  <button
                    onClick={() => {
                      setFileToDelete(f)
                      setIsDeleteOpen(true)
                    }}
                    className="text-red-400 hover:text-red-300 transition-colors p-2 rounded hover:bg-slate-700"
                    title="Hapus"
                  >
                    <Trash2 className="w-4 h-4" />
                  </button>
                </td>
              </tr>
            ))}
            {(filesData?.data || []).length === 0 && (
              <tr>
                <td colSpan={5} className="px-6 py-8 text-center text-slate-500">
                  Belum ada file yang diunggah.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {isUploadOpen && (
        <Modal onClose={() => setIsUploadOpen(false)} title="Upload File">
          <form onSubmit={handleUpload} className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-slate-300 mb-1">Tipe File</label>
            <select
              value={fileType}
              onChange={(e) => setFileType(e.target.value)}
              className="w-full bg-slate-900 border border-slate-700 rounded-lg px-3 py-2 text-white focus:outline-none focus:border-blue-500"
            >
              <option value="1 Firmware Upgrade Image">1 Firmware Upgrade Image</option>
              <option value="2 Web Content">2 Web Content</option>
              <option value="3 Vendor Configuration File">3 Vendor Configuration File</option>
              <option value="4 Tone File">4 Tone File</option>
              <option value="5 Ringer Melody File">5 Ringer Melody File</option>
            </select>
          </div>
          <div>
            <label className="block text-sm font-medium text-slate-300 mb-1">Nama File (Opsional)</label>
            <input
              type="text"
              value={fileName}
              onChange={(e) => setFileName(e.target.value)}
              placeholder="Kosongkan untuk pakai nama asli"
              className="w-full bg-slate-900 border border-slate-700 rounded-lg px-3 py-2 text-white placeholder-slate-500 focus:outline-none focus:border-blue-500"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-slate-300 mb-1">Pilih File</label>
            <input
              type="file"
              onChange={(e) => setSelectedFile(e.target.files?.[0] || null)}
              required
              className="w-full bg-slate-900 border border-slate-700 rounded-lg px-3 py-2 text-slate-300 file:mr-4 file:py-1 file:px-3 file:rounded-md file:border-0 file:text-xs file:font-semibold file:bg-blue-600 file:text-white hover:file:bg-blue-500 cursor-pointer"
            />
          </div>
          <div className="pt-4 flex justify-end gap-3 border-t border-slate-700">
            <button
              type="button"
              onClick={() => setIsUploadOpen(false)}
              className="px-4 py-2 text-sm font-medium text-slate-300 hover:text-white transition-colors"
            >
              Batal
            </button>
            <button
              type="submit"
              disabled={uploadMut.isPending || !selectedFile}
              className="bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white px-4 py-2 rounded-lg text-sm font-medium transition-colors"
            >
              {uploadMut.isPending ? 'Mengunggah...' : 'Upload'}
            </button>
          </div>
        </form>
      </Modal>
      )}

      {isDeleteOpen && (
        <Modal onClose={() => setIsDeleteOpen(false)} title="Hapus File">
          <div className="space-y-4">
            <div className="flex gap-3 text-red-400 p-4 bg-red-400/10 rounded-lg border border-red-400/20">
            <AlertCircle className="w-5 h-5 shrink-0" />
            <p className="text-sm">
              Apakah Anda yakin ingin menghapus file <strong>{fileToDelete?.file_name}</strong>?
              Tindakan ini tidak dapat dibatalkan dan file akan dihapus dari sistem.
            </p>
          </div>
          <div className="flex justify-end gap-3 pt-2">
            <button
              onClick={() => setIsDeleteOpen(false)}
              className="px-4 py-2 text-sm font-medium text-slate-300 hover:text-white transition-colors"
            >
              Batal
            </button>
            <button
              onClick={handleDelete}
              disabled={deleteMut.isPending}
              className="bg-red-500 hover:bg-red-400 disabled:opacity-50 text-white px-4 py-2 rounded-lg text-sm font-medium transition-colors"
            >
              {deleteMut.isPending ? 'Menghapus...' : 'Ya, Hapus File'}
            </button>
          </div>
        </div>
      </Modal>
      )}
    </div>
  )
}
