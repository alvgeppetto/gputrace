import React from 'react';
import { TraceItem, NodeType } from '../types';
import { Box, Code, Cpu, Database, Info, Layers, LayoutGrid, XCircle, FileText, Activity } from 'lucide-react';

interface InspectorProps {
  item: TraceItem | null;
  onSelect?: (id: string | null) => void;
}

const InfoRow: React.FC<{ label: string; value: string | React.ReactNode }> = ({ label, value }) => (
  <div className="flex py-1 border-b border-gray-800 last:border-0">
    <span className="w-1/3 text-gray-500 text-xs">{label}</span>
    <span className="w-2/3 text-gray-300 text-xs font-mono break-all">{value}</span>
  </div>
);

const getAccessStyle = (access: string) => {
  switch (access) {
    case 'Read': return 'text-emerald-400 bg-emerald-400/10 border-emerald-400/20';
    case 'Write': return 'text-rose-400 bg-rose-400/10 border-rose-400/20';
    case 'Read/Write': return 'text-purple-400 bg-purple-400/10 border-purple-400/20';
    default: return 'text-gray-400 bg-gray-400/10 border-gray-400/20';
  }
};

const getStatusColor = (status: string) => {
    switch(status) {
        case 'Tracked': return 'bg-emerald-500 shadow-[0_0_6px_rgba(16,185,129,0.4)]';
        case 'Shared': return 'bg-amber-500 shadow-[0_0_6px_rgba(245,158,11,0.4)]';
        case 'Untracked': return 'bg-slate-500';
        default: return 'bg-gray-500';
    }
}

const ResourceSection: React.FC<{ title: string; resources?: any[]; onSelect?: (id: string) => void }> = ({ title, resources, onSelect }) => {
    if (!resources || resources.length === 0) return null;

    return (
        <div className="mb-6">
            <h4 className="text-[10px] uppercase font-bold text-gray-500 mb-2 tracking-wider flex items-center gap-2">
                {title}
                <span className="bg-gray-800 text-gray-400 px-1.5 rounded-full text-[9px]">{resources.length}</span>
            </h4>
            <div className="space-y-2">
                {resources.map((res, i) => (
                    <div
                        key={i}
                        className="bg-[#2d2d2d] rounded border border-gray-700/50 overflow-hidden group hover:border-blue-500/50 transition-colors cursor-pointer"
                        onClick={() => onSelect && onSelect(res.id)}
                    >
                         {/* Header */}
                         <div className="bg-[#252526] px-3 py-2 flex items-center gap-2 border-b border-gray-700/50 group-hover:bg-[#2a2a2b]">
                             <Database size={13} className="text-blue-400" />
                             <span className="text-xs font-medium text-gray-200 truncate flex-1 font-mono group-hover:text-blue-300 transition-colors" title={res.label}>{res.label}</span>

                             {/* Status Indicator */}
                             <div className="flex items-center gap-1.5 bg-[#1a1a1a] px-1.5 py-0.5 rounded border border-gray-800" title={`Status: ${res.status}`}>
                                 <div className={`w-1.5 h-1.5 rounded-full ${getStatusColor(res.status)}`} />
                                 <span className="text-[9px] text-gray-400 uppercase font-bold">{res.status}</span>
                             </div>
                         </div>

                         {/* Details */}
                         <div className="p-2 space-y-1.5">
                            <div className="flex items-center justify-between text-[10px]">
                                <span className="text-gray-500">Resource Type</span>
                                <span className="text-gray-300 capitalize">{res.type}</span>
                            </div>
                            <div className="flex items-center justify-between text-[10px]">
                                <span className="text-gray-500">Size</span>
                                <span className="text-gray-300 font-mono">{res.size}</span>
                            </div>

                            {/* Access Badge */}
                            <div className="flex items-center justify-between text-[10px] pt-1.5 mt-0.5 border-t border-gray-800/50">
                                <span className="text-gray-500">Access Mode</span>
                                <span className={`px-1.5 py-0.5 rounded border text-[9px] font-semibold uppercase tracking-wide ${getAccessStyle(res.access)}`}>
                                    {res.access}
                                </span>
                            </div>
                         </div>
                    </div>
                ))}
            </div>
        </div>
    )
}

export const Inspector: React.FC<InspectorProps> = ({ item, onSelect }) => {
  if (!item) {
    return (
      <div className="h-full bg-[#1e1e1e] border-l border-gray-800 w-80 flex-shrink-0 p-8 flex flex-col items-center justify-center text-gray-500 select-none">
        <div className="w-16 h-16 rounded-full bg-gray-800/50 flex items-center justify-center mb-4">
             <Layers size={32} strokeWidth={1} className="opacity-50" />
        </div>
        <p className="text-sm font-medium text-gray-400">No Selection</p>
        <p className="text-xs text-gray-600 mt-1 text-center">Select an item from the dependency graph or sidebar to view details.</p>
      </div>
    );
  }

  const renderIcon = () => {
       switch(item.type) {
           case NodeType.ENCODER: return <Cpu className="text-orange-500" size={20}/>
           case NodeType.DISPATCH: return <Code className="text-blue-500" size={20}/>
           case NodeType.BARRIER: return <XCircle className="text-red-500" size={20}/>
           case NodeType.BUFFER: return <Database className="text-green-500" size={20}/>
           case NodeType.ROOT: return <FileText className="text-purple-500" size={20}/>
           default: return <Info className="text-gray-400" size={20}/>
       }
  }

  return (
    <div className="h-full bg-[#1e1e1e] border-l border-gray-800 w-80 flex-shrink-0 flex flex-col shadow-xl z-20">
      {/* Header */}
      <div className="p-4 border-b border-gray-800 flex items-start gap-3 bg-[#252526]">
        <div className="mt-1 bg-[#1e1e1e] p-1.5 rounded border border-gray-700 shadow-sm">{renderIcon()}</div>
        <div className="flex-1 min-w-0">
            <h2 className="text-sm font-bold text-gray-100 truncate" title={item.label}>{item.label}</h2>
            <p className="text-xs text-gray-400 mt-0.5 truncate" title={item.description}>{item.description || 'No description available'}</p>
        </div>
      </div>

      <div className="flex-1 overflow-y-auto p-4 custom-scrollbar">
        {/* General Properties */}
        <div className="mb-6">
            <h3 className="text-[11px] uppercase font-bold text-gray-500 mb-3 tracking-wider flex items-center gap-2">
                <Activity size={12} /> General
            </h3>
            <div className="bg-[#252526] rounded-md border border-gray-700 p-3 space-y-0.5">
                <InfoRow label="Type" value={<span className="capitalize text-blue-300 bg-blue-900/20 px-1 rounded text-[10px]">{item.type}</span>} />
                <InfoRow label="ID" value={<span className="text-[10px] text-gray-500">{item.id}</span>} />
                {item.stats?.duration && <InfoRow label="Duration" value={item.stats.duration} />}
                {item.stats?.threads && <InfoRow label="Threads" value={item.stats.threads} />}
                {Object.entries(item.properties || {}).map(([key, val]) => (
                    <InfoRow key={key} label={key} value={String(val)} />
                ))}
            </div>
        </div>

        {/* Resources / Dependencies */}
        {(item.inputs || item.outputs) && (
            <div className="border-t border-gray-800 pt-4">
                <ResourceSection title="Inputs" resources={item.inputs} onSelect={onSelect} />
                <ResourceSection title="Outputs" resources={item.outputs} onSelect={onSelect} />
            </div>
        )}

      </div>
    </div>
  );
};
