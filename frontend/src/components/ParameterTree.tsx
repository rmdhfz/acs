import { useState, useMemo } from 'react'
import {
  ChevronRight,
  ChevronDown,
  Folder,
  FolderOpen,
  Hash,
  Type,
  ToggleLeft,
  Settings2,
  RefreshCw,
  Edit2,
  Plus,
  Trash2,
  Check,
  X
} from 'lucide-react'
import type { DeviceParameter } from '../lib/types'
import {
  useGetParameterNames,
  useGetParameterValues,
  useSetParameterValues,
  useAddObject,
  useDeleteObject
} from '../lib/hooks'
import { useAuth } from '../lib/auth'

interface ParameterTreeProps {
  deviceId: number
  parameters: DeviceParameter[]
  onRefresh: () => void
  isLoading: boolean
}

type TreeNode = {
  name: string
  fullPath: string
  children: Record<string, TreeNode>
  param?: DeviceParameter
  isLeaf: boolean
}

function buildTree(params: DeviceParameter[]): TreeNode {
  const root: TreeNode = { name: 'Root', fullPath: '', children: {}, isLeaf: false }

  for (const p of params) {
    const parts = p.parameter_name.split('.')
    let current = root
    
    for (let i = 0; i < parts.length; i++) {
      const part = parts[i]
      if (part === '') continue

      if (!current.children[part]) {
        current.children[part] = {
          name: part,
          // If it's the last part and not empty, it's a leaf path
          fullPath: parts.slice(0, i + 1).join('.') + (i === parts.length - 1 && !p.parameter_name.endsWith('.') ? '' : '.'),
          children: {},
          isLeaf: i === parts.length - 1 && !p.parameter_name.endsWith('.')
        }
      }
      current = current.children[part]
    }
    current.param = p
    current.isLeaf = !p.parameter_name.endsWith('.')
  }

  return root
}

interface TreeNodeViewProps {
  node: TreeNode
  depth?: number
  onRefreshNode: (path: string, isLeaf: boolean) => void
  onEditNode: (path: string, value: string) => void
  onAddObject: (path: string) => void
  onDeleteObject: (path: string) => void
  canEdit: boolean
}

function TreeNodeView({ node, depth = 0, onRefreshNode, onEditNode, onAddObject, onDeleteObject, canEdit }: TreeNodeViewProps) {
  const [isOpen, setIsOpen] = useState(depth < 1)
  const [isEditing, setIsEditing] = useState(false)
  const [editVal, setEditVal] = useState(node.param?.parameter_value ?? '')
  const hasChildren = Object.keys(node.children).length > 0
  const isInstance = !isNaN(Number(node.name))

  if (!hasChildren && node.isLeaf) {
    const isBool = node.param?.parameter_value === 'true' || node.param?.parameter_value === 'false' || node.param?.parameter_value === '1' || node.param?.parameter_value === '0'
    const isNum = !isNaN(Number(node.param?.parameter_value)) && node.param?.parameter_value !== ''

    return (
      <div className="flex items-center py-1.5 hover:bg-slate-800/50 rounded group transition-colors" style={{ paddingLeft: `${depth * 1.5 + 1.5}rem` }}>
        <div className="flex items-center gap-2 text-sm text-slate-300 min-w-[200px] flex-1">
          {isBool ? (
            <ToggleLeft className="w-4 h-4 text-emerald-400 shrink-0" />
          ) : isNum ? (
            <Hash className="w-4 h-4 text-blue-400 shrink-0" />
          ) : (
            <Type className="w-4 h-4 text-orange-400 shrink-0" />
          )}
          <span className="truncate" title={node.fullPath}>{node.name}</span>
        </div>
        <div className="flex-1 text-sm font-mono text-slate-400 truncate pl-4 border-l border-slate-700 flex items-center justify-between">
          {isEditing ? (
            <div className="flex items-center gap-2 w-full pr-2">
              <input
                type="text"
                value={editVal}
                onChange={(e) => setEditVal(e.target.value)}
                className="flex-1 bg-slate-900 border border-slate-600 rounded px-2 py-0.5 text-slate-200 focus:outline-none focus:border-blue-500 text-xs"
                autoFocus
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    onEditNode(node.fullPath, editVal)
                    setIsEditing(false)
                  }
                  if (e.key === 'Escape') {
                    setIsEditing(false)
                    setEditVal(node.param?.parameter_value ?? '')
                  }
                }}
              />
              <button onClick={() => { onEditNode(node.fullPath, editVal); setIsEditing(false) }} className="p-1 hover:text-emerald-400"><Check className="w-4 h-4" /></button>
              <button onClick={() => { setIsEditing(false); setEditVal(node.param?.parameter_value ?? '') }} className="p-1 hover:text-red-400"><X className="w-4 h-4" /></button>
            </div>
          ) : (
            <>
              <span className="truncate">{node.param?.parameter_value ?? <span className="text-slate-600 italic">null</span>}</span>
              {canEdit && (
                <div className="opacity-0 group-hover:opacity-100 flex items-center gap-1 pr-2 transition-opacity">
                  <button onClick={() => onRefreshNode(node.fullPath, true)} className="p-1 text-slate-400 hover:text-blue-400" title="Refresh Value">
                    <RefreshCw className="w-3.5 h-3.5" />
                  </button>
                  <button onClick={() => { setIsEditing(true); setEditVal(node.param?.parameter_value ?? '') }} className="p-1 text-slate-400 hover:text-orange-400" title="Edit Value">
                    <Edit2 className="w-3.5 h-3.5" />
                  </button>
                </div>
              )}
            </>
          )}
        </div>
      </div>
    )
  }

  return (
    <div>
      <div 
        className="flex items-center gap-1.5 py-1.5 hover:bg-slate-800/80 rounded group cursor-pointer transition-colors"
        style={{ paddingLeft: `${depth * 1.5}rem` }}
        onClick={() => setIsOpen(!isOpen)}
      >
        {isOpen ? (
          <ChevronDown className="w-4 h-4 text-slate-400 shrink-0" />
        ) : (
          <ChevronRight className="w-4 h-4 text-slate-400 shrink-0" />
        )}
        
        {isOpen ? (
          <FolderOpen className="w-4 h-4 text-blue-400 shrink-0" />
        ) : (
          <Folder className="w-4 h-4 text-blue-400 shrink-0" />
        )}
        
        <span className="text-sm font-medium text-slate-200 select-none truncate flex-1">
          {node.name}
        </span>

        {canEdit && node.name !== 'Root' && (
          <div className="opacity-0 group-hover:opacity-100 flex items-center gap-1 pr-2 transition-opacity" onClick={(e) => e.stopPropagation()}>
            <button onClick={() => onRefreshNode(node.fullPath, false)} className="p-1 text-slate-400 hover:text-blue-400" title="Refresh Children">
              <RefreshCw className="w-3.5 h-3.5" />
            </button>
            <button onClick={() => onAddObject(node.fullPath)} className="p-1 text-slate-400 hover:text-emerald-400" title="Add Object">
              <Plus className="w-3.5 h-3.5" />
            </button>
            {isInstance && (
              <button onClick={() => onDeleteObject(node.fullPath)} className="p-1 text-slate-400 hover:text-red-400" title="Delete Object">
                <Trash2 className="w-3.5 h-3.5" />
              </button>
            )}
          </div>
        )}
      </div>

      {isOpen && (
        <div className="flex flex-col">
          {Object.values(node.children).map((child) => (
            <TreeNodeView 
              key={child.name} 
              node={child} 
              depth={depth + 1} 
              onRefreshNode={onRefreshNode}
              onEditNode={onEditNode}
              onAddObject={onAddObject}
              onDeleteObject={onDeleteObject}
              canEdit={canEdit}
            />
          ))}
        </div>
      )}
    </div>
  )
}

export function ParameterTree({ deviceId, parameters, onRefresh, isLoading }: ParameterTreeProps) {
  const tree = useMemo(() => buildTree(parameters), [parameters])
  const [filter, setFilter] = useState('')
  const { hasRole } = useAuth()
  const canEdit = hasRole('ADMIN', 'SUPERADMIN')
  
  const getParamNamesMut = useGetParameterNames()
  const getParamValsMut = useGetParameterValues()
  const setParamValsMut = useSetParameterValues()
  const addObjectMut = useAddObject()
  const deleteObjectMut = useDeleteObject()

  const isMutating = getParamNamesMut.isPending || getParamValsMut.isPending || setParamValsMut.isPending || addObjectMut.isPending || deleteObjectMut.isPending

  const handleRefreshNode = (path: string, isLeaf: boolean) => {
    if (isLeaf) {
      getParamValsMut.mutate({ deviceId, names: [path] }, {
        onSuccess: () => alert('Task masuk antrean: Refresh ' + path)
      })
    } else {
      getParamNamesMut.mutate({ deviceId, path, nextLevel: true }, {
        onSuccess: () => alert('Task masuk antrean: Explore ' + path)
      })
    }
  }

  const handleEditNode = (path: string, value: string) => {
    setParamValsMut.mutate({ deviceId, values: { [path]: value } }, {
      onSuccess: () => alert('Task masuk antrean: Set ' + path)
    })
  }

  const handleAddObject = (path: string) => {
    if (confirm(`Tambahkan instance baru untuk ${path}?`)) {
      addObjectMut.mutate({ deviceId, objectName: path }, {
        onSuccess: () => alert('Task masuk antrean: AddObject ' + path)
      })
    }
  }

  const handleDeleteObject = (path: string) => {
    if (confirm(`Hapus instance ${path}?`)) {
      deleteObjectMut.mutate({ deviceId, objectName: path }, {
        onSuccess: () => alert('Task masuk antrean: DeleteObject ' + path)
      })
    }
  }

  const handleGlobalRefresh = () => {
    getParamNamesMut.mutate({ deviceId, path: 'InternetGatewayDevice.', nextLevel: false }, {
      onSuccess: () => {
        alert('Task GetParameterNames (Root) ditambahkan ke antrean.')
        onRefresh()
      }
    })
  }

  const filteredTree = useMemo(() => {
    if (!filter) return tree
    const filteredParams = parameters.filter(p => 
      p.parameter_name.toLowerCase().includes(filter.toLowerCase()) || 
      (p.parameter_value && String(p.parameter_value).toLowerCase().includes(filter.toLowerCase()))
    )
    return buildTree(filteredParams)
  }, [filter, parameters, tree])

  return (
    <div className="flex flex-col h-[600px] border border-slate-700 rounded-xl overflow-hidden bg-slate-900">
      <div className="flex items-center justify-between p-3 border-b border-slate-700 bg-slate-800/50">
        <div className="flex-1 max-w-md relative">
          <Settings2 className="w-4 h-4 text-slate-500 absolute left-3 top-1/2 -translate-y-1/2" />
          <input
            type="text"
            placeholder="Cari parameter atau nilai..."
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            className="w-full bg-slate-900 border border-slate-700 rounded-lg pl-9 pr-3 py-1.5 text-sm text-slate-200 placeholder-slate-500 focus:outline-none focus:border-blue-500"
          />
        </div>
        <button
          onClick={handleGlobalRefresh}
          disabled={isMutating || isLoading || !canEdit}
          className="ml-3 flex items-center gap-2 px-3 py-1.5 bg-slate-800 hover:bg-slate-700 border border-slate-600 rounded-lg text-sm text-slate-300 hover:text-white transition-colors disabled:opacity-50"
        >
          <RefreshCw className={`w-4 h-4 ${(isMutating || isLoading) ? 'animate-spin' : ''}`} />
          Refresh Root
        </button>
      </div>

      <div className="flex-1 overflow-auto p-2">
        {Object.keys(filteredTree.children).length === 0 ? (
          <div className="flex flex-col items-center justify-center h-full text-slate-500">
            <FolderOpen className="w-12 h-12 mb-3 text-slate-600" />
            <p>Tidak ada parameter yang ditemukan.</p>
          </div>
        ) : (
          Object.values(filteredTree.children).map((child) => (
            <TreeNodeView 
              key={child.name} 
              node={child} 
              depth={0} 
              onRefreshNode={handleRefreshNode}
              onEditNode={handleEditNode}
              onAddObject={handleAddObject}
              onDeleteObject={handleDeleteObject}
              canEdit={canEdit}
            />
          ))
        )}
      </div>
    </div>
  )
}
